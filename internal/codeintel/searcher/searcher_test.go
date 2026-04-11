package searcher

import (
	"context"
	"fmt"
	"testing"

	"github.com/ponchione/sodoryard/internal/codeintel"
)

// fakeEmbedder returns a fixed vector for any query.
type fakeEmbedder struct {
	vec []float32
}

func (f *fakeEmbedder) EmbedTexts(_ context.Context, _ []string) ([][]float32, error) {
	return nil, nil
}

func (f *fakeEmbedder) EmbedQuery(_ context.Context, _ string) ([]float32, error) {
	return f.vec, nil
}

// fakeStore returns pre-configured results.
type fakeStore struct {
	searchResults []codeintel.SearchResult
	byName        map[string][]codeintel.Chunk
	lastFilter    codeintel.Filter
}

func (f *fakeStore) Upsert(_ context.Context, _ []codeintel.Chunk) error { return nil }
func (f *fakeStore) VectorSearch(_ context.Context, _ []float32, topK int, filter codeintel.Filter) ([]codeintel.SearchResult, error) {
	f.lastFilter = filter
	if topK < len(f.searchResults) {
		return f.searchResults[:topK], nil
	}
	return f.searchResults, nil
}
func (f *fakeStore) GetByFilePath(_ context.Context, _ string) ([]codeintel.Chunk, error) {
	return nil, nil
}
func (f *fakeStore) GetByName(_ context.Context, name string) ([]codeintel.Chunk, error) {
	return f.byName[name], nil
}
func (f *fakeStore) DeleteByFilePath(_ context.Context, _ string) error { return nil }
func (f *fakeStore) Close() error                                       { return nil }

func TestSearch_SingleQuery(t *testing.T) {
	store := &fakeStore{
		searchResults: []codeintel.SearchResult{
			{Chunk: codeintel.Chunk{ID: "a", Name: "FuncA"}, Score: 0.9},
			{Chunk: codeintel.Chunk{ID: "b", Name: "FuncB"}, Score: 0.8},
		},
	}
	embedder := &fakeEmbedder{vec: make([]float32, 10)}

	s := New(store, embedder)

	results, err := s.Search(context.Background(), []string{"find auth"}, codeintel.SearchOptions{
		TopK:       10,
		MaxResults: 5,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[0].Chunk.Name != "FuncA" {
		t.Errorf("results[0].Name = %q, want FuncA", results[0].Chunk.Name)
	}
}

func TestSearch_MultiQueryDedup(t *testing.T) {
	// Same chunk returned for both queries — should be deduped with hitCount=2.
	store := &fakeStore{
		searchResults: []codeintel.SearchResult{
			{Chunk: codeintel.Chunk{ID: "a", Name: "FuncA"}, Score: 0.9},
		},
	}
	embedder := &fakeEmbedder{vec: make([]float32, 10)}

	s := New(store, embedder)

	results, err := s.Search(context.Background(), []string{"query1", "query2"}, codeintel.SearchOptions{
		TopK:       10,
		MaxResults: 5,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1 (deduped)", len(results))
	}
	if results[0].HitCount != 2 {
		t.Errorf("HitCount = %d, want 2", results[0].HitCount)
	}
}

func TestSearch_HopExpansion(t *testing.T) {
	store := &fakeStore{
		searchResults: []codeintel.SearchResult{
			{Chunk: codeintel.Chunk{
				ID:   "a",
				Name: "FuncA",
				Calls: []codeintel.FuncRef{{Name: "HelperB", Package: "pkg"}},
			}, Score: 0.9},
		},
		byName: map[string][]codeintel.Chunk{
			"HelperB": {{ID: "b", Name: "HelperB"}},
		},
	}
	embedder := &fakeEmbedder{vec: make([]float32, 10)}

	s := New(store, embedder)

	results, err := s.Search(context.Background(), []string{"find auth"}, codeintel.SearchOptions{
		TopK:              10,
		MaxResults:        10,
		EnableHopExpansion: true,
		HopBudgetFraction: 0.5,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("got %d results, want >= 2 (with hop)", len(results))
	}

	var foundHop bool
	for _, r := range results {
		if r.FromHop && r.Chunk.Name == "HelperB" {
			foundHop = true
		}
	}
	if !foundHop {
		t.Error("expected HelperB as a hop result")
	}
}

func TestSearch_EmptyQueries(t *testing.T) {
	s := New(&fakeStore{}, &fakeEmbedder{})

	_, err := s.Search(context.Background(), nil, codeintel.SearchOptions{})
	if err == nil {
		t.Fatal("expected error for nil queries")
	}

	_, err = s.Search(context.Background(), []string{}, codeintel.SearchOptions{})
	if err == nil {
		t.Fatal("expected error for empty queries")
	}
}

func TestSearch_MaxResultsDefault(t *testing.T) {
	results := make([]codeintel.SearchResult, 40)
	for i := range results {
		results[i] = codeintel.SearchResult{
			Chunk: codeintel.Chunk{ID: fmt.Sprintf("chunk-%d", i), Name: fmt.Sprintf("Func%d", i)},
			Score: float64(40-i) / 40.0,
		}
	}
	store := &fakeStore{searchResults: results}
	embedder := &fakeEmbedder{vec: make([]float32, 10)}

	s := New(store, embedder)

	got, err := s.Search(context.Background(), []string{"query"}, codeintel.SearchOptions{
		TopK: 50,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 30 {
		t.Errorf("got %d results, want 30 (default MaxResults)", len(got))
	}
}

func TestSearch_HopBudgetFractionDefault(t *testing.T) {
	store := &fakeStore{
		searchResults: []codeintel.SearchResult{
			{Chunk: codeintel.Chunk{
				ID:   "a",
				Name: "FuncA",
				Calls: []codeintel.FuncRef{{Name: "HelperB", Package: "pkg"}},
			}, Score: 0.9},
		},
		byName: map[string][]codeintel.Chunk{
			"HelperB": {{ID: "b", Name: "HelperB"}},
		},
	}
	embedder := &fakeEmbedder{vec: make([]float32, 10)}

	s := New(store, embedder)

	results, err := s.Search(context.Background(), []string{"query"}, codeintel.SearchOptions{
		TopK:               10,
		MaxResults:         10,
		EnableHopExpansion: true,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	var foundHop bool
	for _, r := range results {
		if r.FromHop {
			foundHop = true
		}
	}
	if !foundHop {
		t.Error("expected hop expansion with default HopBudgetFraction=0.4")
	}
}

type errorEmbedder struct{}

func (e *errorEmbedder) EmbedTexts(_ context.Context, _ []string) ([][]float32, error) {
	return nil, fmt.Errorf("embed failed")
}

func (e *errorEmbedder) EmbedQuery(_ context.Context, _ string) ([]float32, error) {
	return nil, fmt.Errorf("embed failed")
}

func TestSearch_AllQueriesFail(t *testing.T) {
	s := New(&fakeStore{}, &errorEmbedder{})

	_, err := s.Search(context.Background(), []string{"q1", "q2"}, codeintel.SearchOptions{
		TopK:       10,
		MaxResults: 5,
	})
	if err == nil {
		t.Fatal("expected error when all queries fail to embed")
	}
}

func TestSearch_HopDepthRespected(t *testing.T) {
	// Chain: A calls B, B calls C.
	// Direct search finds only A.
	// HopDepth=1 should find A + B.
	// HopDepth=2 should find A + B + C.
	store := &fakeStore{
		searchResults: []codeintel.SearchResult{
			{Chunk: codeintel.Chunk{
				ID:   "a",
				Name: "FuncA",
				Calls: []codeintel.FuncRef{{Name: "FuncB", Package: "pkg"}},
			}, Score: 0.9},
		},
		byName: map[string][]codeintel.Chunk{
			"FuncB": {{
				ID:   "b",
				Name: "FuncB",
				Calls: []codeintel.FuncRef{{Name: "FuncC", Package: "pkg"}},
			}},
			"FuncC": {{
				ID:   "c",
				Name: "FuncC",
			}},
		},
	}
	embedder := &fakeEmbedder{vec: make([]float32, 10)}

	s := New(store, embedder)

	// HopDepth=1: should find A and B, but not C.
	results, err := s.Search(context.Background(), []string{"find func"}, codeintel.SearchOptions{
		TopK:               10,
		MaxResults:         10,
		EnableHopExpansion: true,
		HopBudgetFraction:  0.5,
		HopDepth:           1,
	})
	if err != nil {
		t.Fatalf("Search HopDepth=1: %v", err)
	}

	names := map[string]bool{}
	for _, r := range results {
		names[r.Chunk.Name] = true
	}
	if !names["FuncA"] {
		t.Error("HopDepth=1: expected FuncA in results")
	}
	if !names["FuncB"] {
		t.Error("HopDepth=1: expected FuncB in results")
	}
	if names["FuncC"] {
		t.Error("HopDepth=1: did not expect FuncC in results")
	}

	// HopDepth=2: should find A, B, and C.
	results, err = s.Search(context.Background(), []string{"find func"}, codeintel.SearchOptions{
		TopK:               10,
		MaxResults:         10,
		EnableHopExpansion: true,
		HopBudgetFraction:  0.5,
		HopDepth:           2,
	})
	if err != nil {
		t.Fatalf("Search HopDepth=2: %v", err)
	}

	names = map[string]bool{}
	for _, r := range results {
		names[r.Chunk.Name] = true
	}
	if !names["FuncA"] {
		t.Error("HopDepth=2: expected FuncA in results")
	}
	if !names["FuncB"] {
		t.Error("HopDepth=2: expected FuncB in results")
	}
	if !names["FuncC"] {
		t.Error("HopDepth=2: expected FuncC in results")
	}
}

func TestSearch_FilterPassthrough(t *testing.T) {
	store := &fakeStore{
		searchResults: []codeintel.SearchResult{
			{Chunk: codeintel.Chunk{ID: "a", Name: "FuncA"}, Score: 0.9},
		},
	}
	embedder := &fakeEmbedder{vec: make([]float32, 10)}

	s := New(store, embedder)

	wantFilter := codeintel.Filter{Language: "go"}
	_, err := s.Search(context.Background(), []string{"find auth"}, codeintel.SearchOptions{
		TopK:       10,
		MaxResults: 5,
		Filter:     wantFilter,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if store.lastFilter != wantFilter {
		t.Errorf("filter not passed through: got %+v, want %+v", store.lastFilter, wantFilter)
	}
}

func TestSearcherImplementsInterface(t *testing.T) {
	var _ codeintel.Searcher = (*Searcher)(nil)
}
