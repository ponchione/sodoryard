package tool

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ponchione/sodoryard/internal/codeintel"
)

// mockSearcher is a configurable test double for SemanticSearcher.
type mockSearcher struct {
	results    []codeintel.SearchResult
	err        error
	lastOpts   codeintel.SearchOptions
	lastQueries []string
}

func (m *mockSearcher) Search(ctx context.Context, queries []string, opts codeintel.SearchOptions) ([]codeintel.SearchResult, error) {
	m.lastQueries = queries
	m.lastOpts = opts
	return m.results, m.err
}

func TestSearchSemanticSuccess(t *testing.T) {
	searcher := &mockSearcher{
		results: []codeintel.SearchResult{
			{
				Chunk: codeintel.Chunk{
					FilePath:    "internal/auth/middleware.go",
					Name:        "ValidateToken",
					ChunkType:   codeintel.ChunkType("function"),
					Description: "Validates JWT tokens and extracts claims",
					LineStart:   42,
					LineEnd:     68,
					Signature:   "func ValidateToken(token string) (*Claims, error)",
				},
				Score: 0.92,
			},
			{
				Chunk: codeintel.Chunk{
					FilePath:    "internal/auth/types.go",
					Name:        "Claims",
					ChunkType:   codeintel.ChunkType("type"),
					Description: "JWT claims struct with user ID and expiry",
					LineStart:   10,
					LineEnd:     18,
				},
				Score: 0.85,
			},
		},
	}

	tool := NewSearchSemantic(searcher)
	result, err := tool.Execute(context.Background(), "/tmp",
		json.RawMessage(`{"query":"authentication token validation"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "middleware.go") {
		t.Fatalf("expected middleware.go in results, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "ValidateToken") {
		t.Fatalf("expected ValidateToken in results, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "0.92") {
		t.Fatalf("expected score 0.92 in results, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, ":42-68") {
		t.Fatalf("expected line range in results, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "2 results") {
		t.Fatalf("expected result count, got:\n%s", result.Content)
	}
}

func TestSearchSemanticEmptyResults(t *testing.T) {
	searcher := &mockSearcher{results: nil}

	tool := NewSearchSemantic(searcher)
	result, err := tool.Execute(context.Background(), "/tmp",
		json.RawMessage(`{"query":"something obscure"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatal("expected success=true for empty results")
	}
	if !strings.Contains(result.Content, "No semantically relevant code found") {
		t.Fatalf("expected no-results message, got: %s", result.Content)
	}
}

func TestSearchSemanticIndexNotInitialized(t *testing.T) {
	searcher := &mockSearcher{err: errors.New("index not initialized")}

	tool := NewSearchSemantic(searcher)
	result, err := tool.Execute(context.Background(), "/tmp",
		json.RawMessage(`{"query":"test"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure for not-initialized index")
	}
	if !strings.Contains(result.Content, "sirtopham index") {
		t.Fatalf("expected guidance to run index, got: %s", result.Content)
	}
}

func TestSearchSemanticNilSearcher(t *testing.T) {
	tool := NewSearchSemantic(nil)
	result, err := tool.Execute(context.Background(), "/tmp",
		json.RawMessage(`{"query":"test"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure for nil searcher")
	}
	if !strings.Contains(result.Content, "not built") {
		t.Fatalf("expected index-not-built message, got: %s", result.Content)
	}
}

func TestSearchSemanticFiltersPassthrough(t *testing.T) {
	searcher := &mockSearcher{results: nil}

	tool := NewSearchSemantic(searcher)
	tool.Execute(context.Background(), "/tmp",
		json.RawMessage(`{"query":"auth","language":"go","chunk_type":"function","max_results":5}`))

	if searcher.lastOpts.Filter.Language != "go" {
		t.Fatalf("language filter = %q, want go", searcher.lastOpts.Filter.Language)
	}
	if string(searcher.lastOpts.Filter.ChunkType) != "function" {
		t.Fatalf("chunk_type filter = %q, want function", searcher.lastOpts.Filter.ChunkType)
	}
	if searcher.lastOpts.MaxResults != 5 {
		t.Fatalf("max_results = %d, want 5", searcher.lastOpts.MaxResults)
	}
	if len(searcher.lastQueries) != 1 || searcher.lastQueries[0] != "auth" {
		t.Fatalf("queries = %v, want [auth]", searcher.lastQueries)
	}
}

func TestSearchSemanticEmptyQuery(t *testing.T) {
	tool := NewSearchSemantic(&mockSearcher{})
	result, err := tool.Execute(context.Background(), "/tmp",
		json.RawMessage(`{"query":""}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure for empty query")
	}
}

func TestSearchSemanticSchema(t *testing.T) {
	tool := NewSearchSemantic(nil)
	schema := tool.Schema()
	if !json.Valid(schema) {
		t.Fatal("Schema() is not valid JSON")
	}
	if !strings.Contains(string(schema), "search_semantic") {
		t.Fatal("Schema() does not contain tool name")
	}
}

func TestRegisterSearchTools(t *testing.T) {
	reg := NewRegistry()
	RegisterSearchTools(reg, &mockSearcher{})

	if _, ok := reg.Get("search_text"); !ok {
		t.Fatal("search_text not registered")
	}
	if _, ok := reg.Get("search_semantic"); !ok {
		t.Fatal("search_semantic not registered")
	}
}

func TestRegisterSearchToolsNilSearcher(t *testing.T) {
	reg := NewRegistry()
	RegisterSearchTools(reg, nil)

	if _, ok := reg.Get("search_text"); !ok {
		t.Fatal("search_text should be registered even with nil searcher")
	}
	if _, ok := reg.Get("search_semantic"); ok {
		t.Fatal("search_semantic should NOT be registered with nil searcher")
	}
}
