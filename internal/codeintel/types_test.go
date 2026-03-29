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
