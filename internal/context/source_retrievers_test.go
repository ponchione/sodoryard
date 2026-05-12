package context

import (
	stdctx "context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ponchione/sodoryard/internal/codeintel"
	"github.com/ponchione/sodoryard/internal/config"
)

type sourceRetrieverBrainStub struct {
	requests []BrainSearchRequest
	results  []BrainSearchResult
}

func (s *sourceRetrieverBrainStub) Search(_ stdctx.Context, request BrainSearchRequest) ([]BrainSearchResult, error) {
	s.requests = append(s.requests, request)
	return append([]BrainSearchResult(nil), s.results...), nil
}

func TestCodeSearchSourceRetrieverReturnsNormalizedResults(t *testing.T) {
	searcher := &retrievalSearcherStub{results: []codeintel.SearchResult{{
		Chunk: codeintel.Chunk{
			ID:        "chunk-1",
			FilePath:  "internal/auth/service.go",
			Name:      "ValidateToken",
			Body:      "func ValidateToken() {}",
			Language:  "go",
			ChunkType: codeintel.ChunkTypeFunction,
			LineStart: 10,
			LineEnd:   20,
		},
		Score:     0.91,
		MatchedBy: "semantic",
	}}}
	retriever := NewCodeSearchSourceRetriever(searcher)

	results, err := retriever.Retrieve(stdctx.Background(), RetrievalRequest{
		Needs:         &ContextNeeds{},
		Queries:       []string{"auth middleware"},
		ContextConfig: config.ContextConfig{MaxChunks: 3},
	})
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}
	if retriever.Name() != "code" {
		t.Fatalf("Name() = %q, want code", retriever.Name())
	}
	if len(results) != 1 || results[0].Source != "code" || results[0].Kind != "code_chunk" || results[0].Path != "internal/auth/service.go" || results[0].Symbol != "ValidateToken" {
		t.Fatalf("results = %+v, want normalized code result", results)
	}
	if searcher.gotOpts.MaxResults != 3 || !searcher.gotOpts.EnableHopExpansion {
		t.Fatalf("search opts = %+v, want context config and hop expansion", searcher.gotOpts)
	}
}

func TestBrainSearchSourceRetrieverReturnsNormalizedResults(t *testing.T) {
	brain := &sourceRetrieverBrainStub{results: []BrainSearchResult{{
		DocumentPath:   "docs/decisions/auth.md",
		Title:          "Auth Decision",
		SectionHeading: "Decision",
		Snippet:        "Use middleware.",
		FinalScore:     0.75,
		MatchMode:      "keyword",
		MatchSources:   []string{"keyword"},
	}}}
	retriever := NewBrainSearchSourceRetriever(brain)

	results, err := retriever.Retrieve(stdctx.Background(), RetrievalRequest{
		Queries:       []string{"auth middleware"},
		ContextConfig: config.ContextConfig{MaxChunks: 4},
		BrainConfig:   config.BrainConfig{IncludeGraphHops: true, GraphHopDepth: 2, BrainRelevanceThreshold: 0.5},
	})
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}
	if retriever.Name() != "brain" {
		t.Fatalf("Name() = %q, want brain", retriever.Name())
	}
	if len(results) != 1 || results[0].Source != "brain" || results[0].Kind != "brain_doc" || results[0].Path != "docs/decisions/auth.md" || results[0].Symbol != "Decision" {
		t.Fatalf("results = %+v, want normalized brain result", results)
	}
	if len(brain.requests) == 0 || !brain.requests[0].IncludeGraphHops || brain.requests[0].GraphHopDepth != 2 {
		t.Fatalf("brain requests = %+v, want graph hop config passed through", brain.requests)
	}
}

func TestGraphSourceRetrieverReturnsNormalizedResults(t *testing.T) {
	graph := &retrievalGraphStoreStub{result: &codeintel.BlastRadiusResult{Upstream: []codeintel.GraphNode{{
		Symbol:    "AuthHandler",
		FilePath:  "internal/auth/handler.go",
		Kind:      "function",
		Depth:     1,
		LineStart: 12,
		LineEnd:   30,
	}}}}
	retriever := NewGraphSourceRetriever(graph)

	results, err := retriever.Retrieve(stdctx.Background(), RetrievalRequest{
		Needs:         &ContextNeeds{ExplicitSymbols: []string{"ValidateToken"}},
		ContextConfig: config.ContextConfig{StructuralHopDepth: 2, StructuralHopBudget: 7},
	})
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}
	if retriever.Name() != "graph" {
		t.Fatalf("Name() = %q, want graph", retriever.Name())
	}
	if len(results) != 1 || results[0].Source != "graph" || results[0].Kind != "symbol" || results[0].Path != "internal/auth/handler.go" || results[0].Symbol != "AuthHandler" {
		t.Fatalf("results = %+v, want normalized graph symbol", results)
	}
	if len(graph.queries) != 1 || graph.queries[0].Symbol != "ValidateToken" || graph.queries[0].MaxDepth != 2 || graph.queries[0].MaxNodes != 7 {
		t.Fatalf("graph queries = %+v, want explicit symbol and structural config", graph.queries)
	}
	if results[0].Metadata["relationship_type"] != "upstream" || results[0].Metadata["line_start"] != 12 {
		t.Fatalf("metadata = %+v, want graph relationship metadata", results[0].Metadata)
	}
}

func TestExplicitFileSourceRetrieverReturnsNormalizedResults(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "internal", "auth")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(path, "middleware.go"), []byte("package auth\nfunc Middleware() {}\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	retriever := NewExplicitFileSourceRetriever(root)

	results, err := retriever.Retrieve(stdctx.Background(), RetrievalRequest{
		Needs:         &ContextNeeds{ExplicitFiles: []string{"internal/auth/middleware.go"}},
		ContextConfig: config.ContextConfig{MaxExplicitFiles: 2},
	})
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}
	if retriever.Name() != "explicit_file" {
		t.Fatalf("Name() = %q, want explicit_file", retriever.Name())
	}
	if len(results) != 1 || results[0].Source != "explicit_file" || results[0].Kind != "explicit_file" || results[0].Path != "internal/auth/middleware.go" {
		t.Fatalf("results = %+v, want normalized explicit file", results)
	}
	if results[0].Content != "package auth\nfunc Middleware() {}\n" || results[0].TokenEstimate == 0 {
		t.Fatalf("result = %+v, want file content and token estimate", results[0])
	}
}

func TestConventionSourceRetrieverReturnsNormalizedResult(t *testing.T) {
	conventions := &retrievalConventionSourceStub{text: "\n- prefer table-driven tests\n"}
	retriever := NewConventionSourceRetriever(conventions)

	results, err := retriever.Retrieve(stdctx.Background(), RetrievalRequest{
		Needs: &ContextNeeds{IncludeConventions: true},
	})
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}
	if retriever.Name() != "conventions" {
		t.Fatalf("Name() = %q, want conventions", retriever.Name())
	}
	if conventions.calls != 1 {
		t.Fatalf("convention source calls = %d, want 1", conventions.calls)
	}
	if len(results) != 1 || results[0].Source != "conventions" || results[0].Kind != "convention" || results[0].Content != "- prefer table-driven tests" || results[0].TokenEstimate == 0 {
		t.Fatalf("results = %+v, want normalized convention text", results)
	}
}

func TestGitContextSourceRetrieverReturnsNormalizedResult(t *testing.T) {
	var gotWorkdir string
	var gotDepth int
	retriever := NewGitContextSourceRetriever("/repo")
	retriever.gitRunner = func(_ stdctx.Context, workdir string, depth int) (string, error) {
		gotWorkdir = workdir
		gotDepth = depth
		return "\nabc123 fix auth\n", nil
	}

	results, err := retriever.Retrieve(stdctx.Background(), RetrievalRequest{
		Needs: &ContextNeeds{IncludeGitContext: true, GitContextDepth: 3},
	})
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}
	if retriever.Name() != "git" {
		t.Fatalf("Name() = %q, want git", retriever.Name())
	}
	if gotWorkdir != "/repo" || gotDepth != 3 {
		t.Fatalf("git runner got workdir=%q depth=%d, want /repo depth 3", gotWorkdir, gotDepth)
	}
	if len(results) != 1 || results[0].Source != "git" || results[0].Kind != "git" || results[0].Content != "abc123 fix auth" || results[0].Metadata["depth"] != 3 {
		t.Fatalf("results = %+v, want normalized git context", results)
	}
}
