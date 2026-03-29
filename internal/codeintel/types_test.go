package codeintel

import "testing"

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
