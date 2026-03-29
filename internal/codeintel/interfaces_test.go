package codeintel

import (
	"context"
	"testing"
)

type parserStub struct{}

type storeStub struct{}

type embedderStub struct{}

type describerStub struct{}

type searcherStub struct{}

type graphStoreStub struct{}

func (parserStub) Parse(filePath string, content []byte) ([]RawChunk, error) {
	return []RawChunk{}, nil
}

func (storeStub) Upsert(ctx context.Context, chunks []Chunk) error {
	return nil
}

func (storeStub) VectorSearch(ctx context.Context, queryEmbedding []float32, topK int, filter Filter) ([]SearchResult, error) {
	return []SearchResult{}, nil
}

func (storeStub) GetByFilePath(ctx context.Context, filePath string) ([]Chunk, error) {
	return []Chunk{}, nil
}

func (storeStub) GetByName(ctx context.Context, name string) ([]Chunk, error) {
	return []Chunk{}, nil
}

func (storeStub) DeleteByFilePath(ctx context.Context, filePath string) error {
	return nil
}

func (storeStub) Close() error {
	return nil
}

func (embedderStub) EmbedTexts(ctx context.Context, texts []string) ([][]float32, error) {
	return [][]float32{{0.1, 0.2}}, nil
}

func (embedderStub) EmbedQuery(ctx context.Context, query string) ([]float32, error) {
	return []float32{0.3, 0.4}, nil
}

func (describerStub) DescribeFile(ctx context.Context, fileContent string, relationshipContext string) ([]Description, error) {
	return []Description{{Name: "ValidateToken", Description: "Validates a token."}}, nil
}

func (searcherStub) Search(ctx context.Context, queries []string, opts SearchOptions) ([]SearchResult, error) {
	return []SearchResult{{
		Chunk: Chunk{ID: "chunk-id", Name: "ValidateToken", ChunkType: ChunkTypeFunction},
		Score: 0.9,
	}}, nil
}

func (graphStoreStub) BlastRadius(ctx context.Context, query GraphQuery) (*BlastRadiusResult, error) {
	return &BlastRadiusResult{
		Upstream:   []GraphNode{{Symbol: "api.Handler", Kind: "function", Depth: 1}},
		Downstream: []GraphNode{{Symbol: "auth.ParseToken", Kind: "function", Depth: 1}},
		Interfaces: []GraphNode{{Symbol: "auth.Validator", Kind: "interface", Depth: 1}},
	}, nil
}

func (graphStoreStub) Close() error {
	return nil
}

func TestParserInterfaceIsSatisfied(t *testing.T) {
	var parser Parser = parserStub{}

	chunks, err := parser.Parse("main.go", []byte("package main"))
	if err != nil {
		t.Fatalf("Parse returned unexpected error: %v", err)
	}
	if chunks == nil {
		t.Fatal("Parse returned nil slice, want empty slice")
	}
}

func TestStoreInterfaceIsSatisfied(t *testing.T) {
	var store Store = storeStub{}
	ctx := context.Background()

	if err := store.Upsert(ctx, []Chunk{{ID: "chunk-id"}}); err != nil {
		t.Fatalf("Upsert returned unexpected error: %v", err)
	}

	results, err := store.VectorSearch(ctx, []float32{0.1, 0.2}, 5, Filter{Language: "go"})
	if err != nil {
		t.Fatalf("VectorSearch returned unexpected error: %v", err)
	}
	if results == nil {
		t.Fatal("VectorSearch returned nil slice, want empty slice")
	}

	chunksByPath, err := store.GetByFilePath(ctx, "main.go")
	if err != nil {
		t.Fatalf("GetByFilePath returned unexpected error: %v", err)
	}
	if chunksByPath == nil {
		t.Fatal("GetByFilePath returned nil slice, want empty slice")
	}

	chunksByName, err := store.GetByName(ctx, "main")
	if err != nil {
		t.Fatalf("GetByName returned unexpected error: %v", err)
	}
	if chunksByName == nil {
		t.Fatal("GetByName returned nil slice, want empty slice")
	}

	if err := store.DeleteByFilePath(ctx, "main.go"); err != nil {
		t.Fatalf("DeleteByFilePath returned unexpected error: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close returned unexpected error: %v", err)
	}
}

func TestEmbedderInterfaceIsSatisfied(t *testing.T) {
	var embedder Embedder = embedderStub{}
	ctx := context.Background()

	vectors, err := embedder.EmbedTexts(ctx, []string{"func ValidateToken() error\nValidates a token."})
	if err != nil {
		t.Fatalf("EmbedTexts returned unexpected error: %v", err)
	}
	if len(vectors) != 1 || len(vectors[0]) != 2 {
		t.Fatalf("EmbedTexts returned %#v, want one embedding vector", vectors)
	}

	queryVector, err := embedder.EmbedQuery(ctx, "auth middleware")
	if err != nil {
		t.Fatalf("EmbedQuery returned unexpected error: %v", err)
	}
	if len(queryVector) != 2 {
		t.Fatalf("EmbedQuery returned %#v, want one embedding vector", queryVector)
	}
}

func TestDescriberInterfaceIsSatisfied(t *testing.T) {
	var describer Describer = describerStub{}
	ctx := context.Background()

	descriptions, err := describer.DescribeFile(ctx, "func ValidateToken() error { return nil }", "calls: auth.ParseToken")
	if err != nil {
		t.Fatalf("DescribeFile returned unexpected error: %v", err)
	}
	if len(descriptions) != 1 {
		t.Fatalf("DescribeFile returned %#v, want one description", descriptions)
	}
	if descriptions[0].Name != "ValidateToken" {
		t.Fatalf("Description name = %q, want %q", descriptions[0].Name, "ValidateToken")
	}
	if descriptions[0].Description == "" {
		t.Fatal("Description text is empty, want semantic summary")
	}
}

func TestSearcherInterfaceIsSatisfied(t *testing.T) {
	var searcher Searcher = searcherStub{}
	ctx := context.Background()

	results, err := searcher.Search(ctx, []string{"auth middleware", "jwt validator"}, SearchOptions{
		TopK:               10,
		Filter:             Filter{Language: "go"},
		MaxResults:         30,
		EnableHopExpansion: true,
		HopBudgetFraction:  0.4,
		HopDepth:           1,
	})
	if err != nil {
		t.Fatalf("Search returned unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Search returned %#v, want one result", results)
	}
	if results[0].Chunk.ID != "chunk-id" {
		t.Fatalf("Search result chunk ID = %q, want %q", results[0].Chunk.ID, "chunk-id")
	}
	if results[0].Score != 0.9 {
		t.Fatalf("Search result score = %v, want 0.9", results[0].Score)
	}
}

func TestGraphStoreInterfaceIsSatisfied(t *testing.T) {
	var graphStore GraphStore = graphStoreStub{}
	ctx := context.Background()

	result, err := graphStore.BlastRadius(ctx, GraphQuery{
		Symbol:       "auth.ValidateToken",
		MaxDepth:     2,
		MaxNodes:     25,
		IncludeKinds: []string{"function", "method"},
		ExcludeKinds: []string{"interface"},
	})
	if err != nil {
		t.Fatalf("BlastRadius returned unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("BlastRadius returned nil result, want non-nil pointer")
	}
	if len(result.Upstream) != 1 || result.Upstream[0].Symbol != "api.Handler" {
		t.Fatalf("Upstream = %#v, want api.Handler", result.Upstream)
	}
	if len(result.Downstream) != 1 || result.Downstream[0].Symbol != "auth.ParseToken" {
		t.Fatalf("Downstream = %#v, want auth.ParseToken", result.Downstream)
	}
	if len(result.Interfaces) != 1 || result.Interfaces[0].Symbol != "auth.Validator" {
		t.Fatalf("Interfaces = %#v, want auth.Validator", result.Interfaces)
	}
	if err := graphStore.Close(); err != nil {
		t.Fatalf("Close returned unexpected error: %v", err)
	}
}
