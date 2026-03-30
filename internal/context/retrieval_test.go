package context

import (
	stdctx "context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/ponchione/sirtopham/internal/codeintel"
	"github.com/ponchione/sirtopham/internal/config"
)

type retrievalSearcherStub struct {
	results    []codeintel.SearchResult
	err        error
	delay      time.Duration
	calls      int
	gotQueries []string
	gotOpts    codeintel.SearchOptions
}

func (s *retrievalSearcherStub) Search(ctx stdctx.Context, queries []string, opts codeintel.SearchOptions) ([]codeintel.SearchResult, error) {
	s.calls++
	s.gotQueries = append([]string{}, queries...)
	s.gotOpts = opts
	if s.delay > 0 {
		select {
		case <-time.After(s.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if s.err != nil {
		return nil, s.err
	}
	return append([]codeintel.SearchResult(nil), s.results...), nil
}

type retrievalGraphStoreStub struct {
	result  *codeintel.BlastRadiusResult
	err     error
	delay   time.Duration
	calls   int
	queries []codeintel.GraphQuery
}

func (s *retrievalGraphStoreStub) BlastRadius(ctx stdctx.Context, query codeintel.GraphQuery) (*codeintel.BlastRadiusResult, error) {
	s.calls++
	s.queries = append(s.queries, query)
	if s.delay > 0 {
		select {
		case <-time.After(s.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if s.err != nil {
		return nil, s.err
	}
	if s.result == nil {
		return &codeintel.BlastRadiusResult{}, nil
	}
	return s.result, nil
}

func (s *retrievalGraphStoreStub) Close() error { return nil }

type retrievalConventionSourceStub struct {
	text  string
	err   error
	delay time.Duration
	calls int
}

func (s *retrievalConventionSourceStub) Load(ctx stdctx.Context) (string, error) {
	s.calls++
	if s.delay > 0 {
		select {
		case <-time.After(s.delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	if s.err != nil {
		return "", s.err
	}
	return s.text, nil
}

func TestNoopConventionSourceReturnsEmptyString(t *testing.T) {
	source := NoopConventionSource{}

	text, err := source.Load(stdctx.Background())
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if text != "" {
		t.Fatalf("text = %q, want empty", text)
	}
}

func TestRetrievalOrchestratorRunsAllEnabledPathsAndMapsResults(t *testing.T) {
	projectRoot := t.TempDir()
	mustWriteFile(t, projectRoot, "internal/auth/middleware.go", "package auth\n\nfunc ValidateToken() {}\n")

	searcher := &retrievalSearcherStub{results: []codeintel.SearchResult{{
		Chunk: codeintel.Chunk{
			ID:          "chunk-1",
			FilePath:    "internal/auth/service.go",
			Name:        "ValidateToken",
			Signature:   "func ValidateToken() error",
			Description: "Validates tokens.",
			Body:        "func ValidateToken() error { return nil }",
			Language:    "go",
			ChunkType:   codeintel.ChunkTypeFunction,
			LineStart:   10,
			LineEnd:     20,
		},
		Score:     0.91,
		MatchedBy: "auth middleware",
		HitCount:  2,
	}}}
	graph := &retrievalGraphStoreStub{result: &codeintel.BlastRadiusResult{Upstream: []codeintel.GraphNode{{
		Symbol:    "AuthHandler",
		FilePath:  "internal/auth/handler.go",
		Kind:      "function",
		Depth:     1,
		LineStart: 5,
		LineEnd:   18,
	}}}}
	conventions := &retrievalConventionSourceStub{text: "- use table-driven tests"}

	orchestrator := NewRetrievalOrchestrator(searcher, graph, conventions, projectRoot)
	var gotGitDepth int
	var gotGitDir string
	orchestrator.gitRunner = func(ctx stdctx.Context, workdir string, depth int) (string, error) {
		gotGitDepth = depth
		gotGitDir = workdir
		return "abc123 fix auth", nil
	}

	results, err := orchestrator.Retrieve(stdctx.Background(), &ContextNeeds{
		ExplicitFiles:      []string{"internal/auth/middleware.go"},
		ExplicitSymbols:    []string{"ValidateToken"},
		IncludeConventions: true,
		IncludeGitContext:  true,
		GitContextDepth:    3,
	}, []string{"auth middleware"}, config.ContextConfig{
		MaxChunks:           25,
		MaxExplicitFiles:    5,
		RelevanceThreshold:  0.35,
		StructuralHopDepth:  1,
		StructuralHopBudget: 10,
	})
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}

	if searcher.calls != 1 {
		t.Fatalf("searcher calls = %d, want 1", searcher.calls)
	}
	if !slices.Equal(searcher.gotQueries, []string{"auth middleware"}) {
		t.Fatalf("queries = %v, want [auth middleware]", searcher.gotQueries)
	}
	if searcher.gotOpts.TopK != 10 {
		t.Fatalf("TopK = %d, want 10", searcher.gotOpts.TopK)
	}
	if searcher.gotOpts.HopBudgetFraction != 0.5 {
		t.Fatalf("HopBudgetFraction = %v, want 0.5", searcher.gotOpts.HopBudgetFraction)
	}
	if searcher.gotOpts.MaxResults != 25 {
		t.Fatalf("MaxResults = %d, want 25", searcher.gotOpts.MaxResults)
	}

	if len(results.RAGHits) != 1 {
		t.Fatalf("len(RAGHits) = %d, want 1", len(results.RAGHits))
	}
	if results.RAGHits[0].ChunkID != "chunk-1" {
		t.Fatalf("ChunkID = %q, want chunk-1", results.RAGHits[0].ChunkID)
	}
	if !slices.Equal(results.RAGHits[0].Sources, []string{"rag"}) {
		t.Fatalf("Sources = %v, want [rag]", results.RAGHits[0].Sources)
	}

	if len(results.FileResults) != 1 {
		t.Fatalf("len(FileResults) = %d, want 1", len(results.FileResults))
	}
	if results.FileResults[0].FilePath != "internal/auth/middleware.go" {
		t.Fatalf("FilePath = %q, want internal/auth/middleware.go", results.FileResults[0].FilePath)
	}
	if results.FileResults[0].Content == "" {
		t.Fatal("expected file content")
	}

	if len(results.GraphHits) != 1 {
		t.Fatalf("len(GraphHits) = %d, want 1", len(results.GraphHits))
	}
	if results.GraphHits[0].RelationshipType != "upstream" {
		t.Fatalf("RelationshipType = %q, want upstream", results.GraphHits[0].RelationshipType)
	}

	if results.ConventionText != "- use table-driven tests" {
		t.Fatalf("ConventionText = %q, want convention text", results.ConventionText)
	}
	if results.GitContext != "abc123 fix auth" {
		t.Fatalf("GitContext = %q, want git output", results.GitContext)
	}
	if gotGitDepth != 3 {
		t.Fatalf("git depth = %d, want 3", gotGitDepth)
	}
	if gotGitDir != projectRoot {
		t.Fatalf("git dir = %q, want %q", gotGitDir, projectRoot)
	}
}

func TestRetrievalOrchestratorFiltersAndMergesOverlappingGraphHits(t *testing.T) {
	searcher := &retrievalSearcherStub{results: []codeintel.SearchResult{
		{
			Chunk: codeintel.Chunk{ID: "chunk-good", FilePath: "internal/auth/service.go", Name: "ValidateToken", ChunkType: codeintel.ChunkTypeFunction},
			Score: 0.81,
		},
		{
			Chunk: codeintel.Chunk{ID: "chunk-low", FilePath: "internal/auth/noise.go", Name: "Noise", ChunkType: codeintel.ChunkTypeFunction},
			Score: 0.20,
		},
	}}
	graph := &retrievalGraphStoreStub{result: &codeintel.BlastRadiusResult{Downstream: []codeintel.GraphNode{{
		Symbol:    "ValidateToken",
		FilePath:  "internal/auth/service.go",
		Depth:     1,
		LineStart: 10,
		LineEnd:   20,
	}}}}
	orchestrator := NewRetrievalOrchestrator(searcher, graph, NoopConventionSource{}, t.TempDir())

	results, err := orchestrator.Retrieve(stdctx.Background(), &ContextNeeds{ExplicitSymbols: []string{"ValidateToken"}}, []string{"auth token"}, config.ContextConfig{RelevanceThreshold: 0.35})
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}

	if len(results.RAGHits) != 1 {
		t.Fatalf("len(RAGHits) = %d, want 1", len(results.RAGHits))
	}
	if results.RAGHits[0].ChunkID != "chunk-good" {
		t.Fatalf("ChunkID = %q, want chunk-good", results.RAGHits[0].ChunkID)
	}
	if !slices.Contains(results.RAGHits[0].Sources, "rag") || !slices.Contains(results.RAGHits[0].Sources, "graph") {
		t.Fatalf("Sources = %v, want rag and graph", results.RAGHits[0].Sources)
	}
	if len(results.GraphHits) != 0 {
		t.Fatalf("GraphHits = %v, want overlap to be merged into RAG hit", results.GraphHits)
	}
}

func TestRetrievalOrchestratorSkipsTraversalAndMissingFilesGracefully(t *testing.T) {
	projectRoot := t.TempDir()
	mustWriteFile(t, projectRoot, "internal/auth/middleware.go", "0123456789abcdef")

	orchestrator := NewRetrievalOrchestrator(nil, nil, NoopConventionSource{}, projectRoot)
	orchestrator.maxExplicitFileBytes = 8

	results, err := orchestrator.Retrieve(stdctx.Background(), &ContextNeeds{ExplicitFiles: []string{
		"../secret.txt",
		"missing.go",
		"internal/auth/middleware.go",
	}}, nil, config.ContextConfig{MaxExplicitFiles: 5})
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}

	if len(results.FileResults) != 1 {
		t.Fatalf("FileResults = %v, want only the valid in-root file", results.FileResults)
	}
	if !results.FileResults[0].Truncated {
		t.Fatal("expected file result to be truncated")
	}
	if results.FileResults[0].Content != "01234567" {
		t.Fatalf("Content = %q, want truncated prefix", results.FileResults[0].Content)
	}
}

func TestRetrievalOrchestratorTimeoutDoesNotBlockOtherPaths(t *testing.T) {
	projectRoot := t.TempDir()
	mustWriteFile(t, projectRoot, "internal/auth/middleware.go", "package auth")

	searcher := &retrievalSearcherStub{
		results: []codeintel.SearchResult{{Chunk: codeintel.Chunk{ID: "chunk-slow", FilePath: "internal/auth/middleware.go", Name: "ValidateToken"}, Score: 0.9}},
		delay:   200 * time.Millisecond,
	}
	orchestrator := NewRetrievalOrchestrator(searcher, nil, NoopConventionSource{}, projectRoot)
	orchestrator.timeout = 20 * time.Millisecond

	start := time.Now()
	results, err := orchestrator.Retrieve(stdctx.Background(), &ContextNeeds{ExplicitFiles: []string{"internal/auth/middleware.go"}}, []string{"auth middleware"}, config.ContextConfig{MaxExplicitFiles: 5})
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 150*time.Millisecond {
		t.Fatalf("Retrieve took %v, expected timeout to return sooner", elapsed)
	}
	if len(results.RAGHits) != 0 {
		t.Fatalf("RAGHits = %v, want empty after timeout", results.RAGHits)
	}
	if len(results.FileResults) != 1 {
		t.Fatalf("len(FileResults) = %d, want 1", len(results.FileResults))
	}
}

func TestRetrievalOrchestratorContinuesAfterPathErrors(t *testing.T) {
	projectRoot := t.TempDir()
	mustWriteFile(t, projectRoot, "internal/auth/middleware.go", "package auth")

	searcher := &retrievalSearcherStub{err: errors.New("search failed")}
	conventions := &retrievalConventionSourceStub{err: errors.New("cache unavailable")}
	orchestrator := NewRetrievalOrchestrator(searcher, nil, conventions, projectRoot)
	orchestrator.gitRunner = func(ctx stdctx.Context, workdir string, depth int) (string, error) {
		return "", errors.New("git failed")
	}

	results, err := orchestrator.Retrieve(stdctx.Background(), &ContextNeeds{
		ExplicitFiles:      []string{"internal/auth/middleware.go"},
		IncludeConventions: true,
		IncludeGitContext:  true,
		GitContextDepth:    2,
	}, []string{"auth middleware"}, config.ContextConfig{MaxExplicitFiles: 5})
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}
	if len(results.FileResults) != 1 {
		t.Fatalf("len(FileResults) = %d, want 1", len(results.FileResults))
	}
	if len(results.RAGHits) != 0 {
		t.Fatalf("RAGHits = %v, want empty after search error", results.RAGHits)
	}
	if results.ConventionText != "" {
		t.Fatalf("ConventionText = %q, want empty", results.ConventionText)
	}
	if results.GitContext != "" {
		t.Fatalf("GitContext = %q, want empty", results.GitContext)
	}
}

func mustWriteFile(t *testing.T, root string, relativePath string, content string) {
	t.Helper()
	fullPath := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
}
