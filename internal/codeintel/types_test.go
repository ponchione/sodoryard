package codeintel

import (
	"testing"
	"time"
)

func TestChunkTypeConstants(t *testing.T) {
	cases := map[string]ChunkType{
		"function":  ChunkTypeFunction,
		"method":    ChunkTypeMethod,
		"type":      ChunkTypeType,
		"interface": ChunkTypeInterface,
		"class":     ChunkTypeClass,
		"section":   ChunkTypeSection,
		"fallback":  ChunkTypeFallback,
	}

	for want, got := range cases {
		if got != ChunkType(want) {
			t.Fatalf("constant %q = %q, want %q", want, got, want)
		}
	}
}

func TestPipelineConstants(t *testing.T) {
	if MaxBodyLength != 2000 {
		t.Fatalf("MaxBodyLength = %d, want 2000", MaxBodyLength)
	}
	if DefaultEmbeddingDims != 3584 {
		t.Fatalf("DefaultEmbeddingDims = %d, want 3584", DefaultEmbeddingDims)
	}
	if DefaultEmbedBatchSize != 32 {
		t.Fatalf("DefaultEmbedBatchSize = %d, want 32", DefaultEmbedBatchSize)
	}
	if SchemaVersion != "1" {
		t.Fatalf("SchemaVersion = %q, want %q", SchemaVersion, "1")
	}
	if QueryPrefix != "Represent this query for searching relevant code: " {
		t.Fatalf("QueryPrefix = %q, want expected nomic query prefix", QueryPrefix)
	}
}

func TestRawChunkFields(t *testing.T) {
	raw := RawChunk{
		Name:      "ValidateToken",
		Signature: "func ValidateToken(token string) error",
		Body:      "{ return nil }",
		ChunkType: ChunkTypeFunction,
		LineStart: 10,
		LineEnd:   18,
	}

	if raw.Name != "ValidateToken" {
		t.Fatalf("Name = %q, want %q", raw.Name, "ValidateToken")
	}
	if raw.Signature != "func ValidateToken(token string) error" {
		t.Fatalf("Signature = %q, want function signature", raw.Signature)
	}
	if raw.Body != "{ return nil }" {
		t.Fatalf("Body = %q, want %q", raw.Body, "{ return nil }")
	}
	if raw.ChunkType != ChunkTypeFunction {
		t.Fatalf("ChunkType = %q, want %q", raw.ChunkType, ChunkTypeFunction)
	}
	if raw.LineStart != 10 || raw.LineEnd != 18 {
		t.Fatalf("line range = %d-%d, want 10-18", raw.LineStart, raw.LineEnd)
	}
}

func TestChunkFields(t *testing.T) {
	indexedAt := time.Date(2026, time.March, 29, 12, 0, 0, 0, time.UTC)
	chunk := Chunk{
		ID:               "chunk-id",
		ProjectName:      "sirtopham",
		FilePath:         "internal/auth/middleware.go",
		Language:         "go",
		ChunkType:        ChunkTypeFunction,
		Name:             "ValidateToken",
		Signature:        "func ValidateToken(token string) error",
		Body:             "{ return nil }",
		Description:      "Validates a token and returns an error on failure.",
		LineStart:        10,
		LineEnd:          18,
		ContentHash:      "hash",
		IndexedAt:        indexedAt,
		Calls:            []string{"auth.ParseToken"},
		CalledBy:         []string{"main.main"},
		TypesUsed:        []string{"auth.Claims"},
		ImplementsIfaces: []string{"auth.Validator"},
		Imports:          []string{"context", "fmt"},
		Embedding:        []float32{0.1, 0.2, 0.3},
	}

	if chunk.ID != "chunk-id" || chunk.ProjectName != "sirtopham" {
		t.Fatalf("identity fields = %+v, want ID and ProjectName preserved", chunk)
	}
	if chunk.FilePath != "internal/auth/middleware.go" || chunk.Language != "go" {
		t.Fatalf("location fields = %+v, want FilePath and Language preserved", chunk)
	}
	if chunk.ChunkType != ChunkTypeFunction || chunk.Name != "ValidateToken" {
		t.Fatalf("content identity fields = %+v, want ChunkType and Name preserved", chunk)
	}
	if chunk.Description == "" || chunk.ContentHash != "hash" {
		t.Fatalf("metadata fields = %+v, want Description and ContentHash preserved", chunk)
	}
	if !chunk.IndexedAt.Equal(indexedAt) {
		t.Fatalf("IndexedAt = %v, want %v", chunk.IndexedAt, indexedAt)
	}
	if len(chunk.Calls) != 1 || chunk.Calls[0] != "auth.ParseToken" {
		t.Fatalf("Calls = %#v, want auth.ParseToken", chunk.Calls)
	}
	if len(chunk.CalledBy) != 1 || chunk.CalledBy[0] != "main.main" {
		t.Fatalf("CalledBy = %#v, want main.main", chunk.CalledBy)
	}
	if len(chunk.TypesUsed) != 1 || chunk.TypesUsed[0] != "auth.Claims" {
		t.Fatalf("TypesUsed = %#v, want auth.Claims", chunk.TypesUsed)
	}
	if len(chunk.ImplementsIfaces) != 1 || chunk.ImplementsIfaces[0] != "auth.Validator" {
		t.Fatalf("ImplementsIfaces = %#v, want auth.Validator", chunk.ImplementsIfaces)
	}
	if len(chunk.Imports) != 2 || chunk.Imports[0] != "context" || chunk.Imports[1] != "fmt" {
		t.Fatalf("Imports = %#v, want context and fmt", chunk.Imports)
	}
	if len(chunk.Embedding) != 3 || chunk.Embedding[2] != float32(0.3) {
		t.Fatalf("Embedding = %#v, want 3 float32 values", chunk.Embedding)
	}
}

func TestSearchResultFields(t *testing.T) {
	result := SearchResult{
		Chunk: Chunk{
			ID:        "chunk-id",
			Name:      "ValidateToken",
			ChunkType: ChunkTypeFunction,
		},
		Score:     0.91,
		MatchedBy: "auth middleware",
		HitCount:  2,
		FromHop:   true,
	}

	if result.Chunk.ID != "chunk-id" || result.Chunk.Name != "ValidateToken" {
		t.Fatalf("Chunk = %+v, want embedded chunk preserved", result.Chunk)
	}
	if result.Score != 0.91 {
		t.Fatalf("Score = %v, want 0.91", result.Score)
	}
	if result.MatchedBy != "auth middleware" {
		t.Fatalf("MatchedBy = %q, want %q", result.MatchedBy, "auth middleware")
	}
	if result.HitCount != 2 {
		t.Fatalf("HitCount = %d, want 2", result.HitCount)
	}
	if !result.FromHop {
		t.Fatal("FromHop = false, want true")
	}
}

func TestFilterFields(t *testing.T) {
	filter := Filter{
		Language:       "go",
		ChunkType:      ChunkTypeFunction,
		FilePathPrefix: "internal/auth",
	}

	if filter.Language != "go" {
		t.Fatalf("Language = %q, want %q", filter.Language, "go")
	}
	if filter.ChunkType != ChunkTypeFunction {
		t.Fatalf("ChunkType = %q, want %q", filter.ChunkType, ChunkTypeFunction)
	}
	if filter.FilePathPrefix != "internal/auth" {
		t.Fatalf("FilePathPrefix = %q, want %q", filter.FilePathPrefix, "internal/auth")
	}
}

func TestDescriptionFields(t *testing.T) {
	description := Description{
		Name:        "ValidateToken",
		Description: "Validates a token and returns an error when it is invalid.",
	}

	if description.Name != "ValidateToken" {
		t.Fatalf("Name = %q, want %q", description.Name, "ValidateToken")
	}
	if description.Description == "" {
		t.Fatal("Description is empty, want semantic summary")
	}
}

func TestSearchOptionsFields(t *testing.T) {
	opts := SearchOptions{
		TopK:               10,
		Filter:             Filter{Language: "go", ChunkType: ChunkTypeFunction, FilePathPrefix: "internal/auth"},
		MaxResults:         30,
		EnableHopExpansion: true,
		HopBudgetFraction:  0.4,
		HopDepth:           1,
	}

	if opts.TopK != 10 {
		t.Fatalf("TopK = %d, want 10", opts.TopK)
	}
	if opts.Filter.Language != "go" || opts.Filter.ChunkType != ChunkTypeFunction || opts.Filter.FilePathPrefix != "internal/auth" {
		t.Fatalf("Filter = %+v, want go/function/internal/auth", opts.Filter)
	}
	if opts.MaxResults != 30 {
		t.Fatalf("MaxResults = %d, want 30", opts.MaxResults)
	}
	if !opts.EnableHopExpansion {
		t.Fatal("EnableHopExpansion = false, want true")
	}
	if opts.HopBudgetFraction != 0.4 {
		t.Fatalf("HopBudgetFraction = %v, want 0.4", opts.HopBudgetFraction)
	}
	if opts.HopDepth != 1 {
		t.Fatalf("HopDepth = %d, want 1", opts.HopDepth)
	}
}

func TestGraphNodeFields(t *testing.T) {
	node := GraphNode{
		Symbol:    "auth.ValidateToken",
		FilePath:  "internal/auth/service.go",
		Kind:      "function",
		Depth:     2,
		LineStart: 15,
		LineEnd:   27,
	}

	if node.Symbol != "auth.ValidateToken" {
		t.Fatalf("Symbol = %q, want %q", node.Symbol, "auth.ValidateToken")
	}
	if node.FilePath != "internal/auth/service.go" {
		t.Fatalf("FilePath = %q, want %q", node.FilePath, "internal/auth/service.go")
	}
	if node.Kind != "function" {
		t.Fatalf("Kind = %q, want %q", node.Kind, "function")
	}
	if node.Depth != 2 || node.LineStart != 15 || node.LineEnd != 27 {
		t.Fatalf("node location fields = %+v, want depth 2 and lines 15-27", node)
	}
}

func TestBlastRadiusResultFields(t *testing.T) {
	result := BlastRadiusResult{
		Upstream:   []GraphNode{{Symbol: "api.Handler", Kind: "function", Depth: 1}},
		Downstream: []GraphNode{{Symbol: "auth.ParseToken", Kind: "function", Depth: 1}},
		Interfaces: []GraphNode{{Symbol: "auth.Validator", Kind: "interface", Depth: 1}},
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
}

func TestGraphQueryFields(t *testing.T) {
	query := GraphQuery{
		Symbol:       "auth.ValidateToken",
		MaxDepth:     2,
		MaxNodes:     25,
		IncludeKinds: []string{"function", "method"},
		ExcludeKinds: []string{"interface"},
	}

	if query.Symbol != "auth.ValidateToken" {
		t.Fatalf("Symbol = %q, want %q", query.Symbol, "auth.ValidateToken")
	}
	if query.MaxDepth != 2 || query.MaxNodes != 25 {
		t.Fatalf("query numeric fields = %+v, want MaxDepth 2 and MaxNodes 25", query)
	}
	if len(query.IncludeKinds) != 2 || query.IncludeKinds[0] != "function" || query.IncludeKinds[1] != "method" {
		t.Fatalf("IncludeKinds = %#v, want function/method", query.IncludeKinds)
	}
	if len(query.ExcludeKinds) != 1 || query.ExcludeKinds[0] != "interface" {
		t.Fatalf("ExcludeKinds = %#v, want interface", query.ExcludeKinds)
	}
}
