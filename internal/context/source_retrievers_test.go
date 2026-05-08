package context

import (
	stdctx "context"
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
