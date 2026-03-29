package vectorstore

import (
	"context"
	"testing"
	"time"

	"github.com/ponchione/sirtopham/internal/codeintel"
)

func TestEscapeLanceFilter(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"clean string", "hello world", "hello world"},
		{"single quote", "it's a test", "it''s a test"},
		{"double quote unchanged", `say "hello"`, `say "hello"`},
		{"multiple single quotes", "a'b'c", "a''b''c"},
		{"empty string", "", ""},
		{"injection attempt", "'; DROP TABLE chunks; --", "''; DROP TABLE chunks; --"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeLanceFilter(tt.in)
			if got != tt.want {
				t.Errorf("escapeLanceFilter(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestBuildFilterString(t *testing.T) {
	tests := []struct {
		name   string
		filter codeintel.Filter
		want   string
	}{
		{"empty filter", codeintel.Filter{}, ""},
		{"language only", codeintel.Filter{Language: "go"}, "language = 'go'"},
		{"chunk type only", codeintel.Filter{ChunkType: codeintel.ChunkTypeFunction}, "chunk_type = 'function'"},
		{"file path prefix", codeintel.Filter{FilePathPrefix: "internal/"}, "file_path LIKE 'internal/%'"},
		{
			"all fields",
			codeintel.Filter{Language: "go", ChunkType: codeintel.ChunkTypeMethod, FilePathPrefix: "cmd/"},
			"language = 'go' AND chunk_type = 'method' AND file_path LIKE 'cmd/%'",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildFilterString(tt.filter)
			if got != tt.want {
				t.Errorf("buildFilterString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMarshalJSON(t *testing.T) {
	refs := []codeintel.FuncRef{{Name: "Foo", Package: "bar"}}
	got := marshalJSON(refs)
	if got != `[{"name":"Foo","package":"bar"}]` {
		t.Errorf("marshalJSON(refs) = %q", got)
	}

	gotNil := marshalJSON(nil)
	if gotNil != "[]" {
		t.Errorf("marshalJSON(nil) = %q, want []", gotNil)
	}
}

func TestUnmarshalFuncRefs(t *testing.T) {
	refs := unmarshalFuncRefs(`[{"name":"Foo","package":"bar"}]`)
	if len(refs) != 1 || refs[0].Name != "Foo" || refs[0].Package != "bar" {
		t.Errorf("unmarshalFuncRefs = %#v", refs)
	}

	empty := unmarshalFuncRefs("[]")
	if empty != nil {
		t.Errorf("unmarshalFuncRefs([]) = %#v, want nil", empty)
	}
}

func TestUnmarshalStrings(t *testing.T) {
	strs := unmarshalStrings(`["a","b"]`)
	if len(strs) != 2 || strs[0] != "a" || strs[1] != "b" {
		t.Errorf("unmarshalStrings = %#v", strs)
	}
}

func TestNewStore_And_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	store, err := NewStore(ctx, dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	dims := codeintel.DefaultEmbeddingDims
	embedding := make([]float32, dims)
	for i := range embedding {
		embedding[i] = 0.01
	}

	chunk := codeintel.Chunk{
		ID:               "test-id-1",
		ProjectName:      "test-project",
		FilePath:         "internal/auth/handler.go",
		Language:         "go",
		ChunkType:        codeintel.ChunkTypeFunction,
		Name:             "HandleLogin",
		Signature:        "func HandleLogin(w http.ResponseWriter, r *http.Request)",
		Body:             "{ /* body */ }",
		Description:      "Handles user login requests.",
		LineStart:        10,
		LineEnd:          25,
		ContentHash:      "abc123",
		IndexedAt:        time.Now(),
		Calls:            []codeintel.FuncRef{{Name: "ParseToken", Package: "auth"}},
		CalledBy:         []codeintel.FuncRef{{Name: "main", Package: "main"}},
		TypesUsed:        []string{"http.Request"},
		ImplementsIfaces: []string{"http.Handler"},
		Imports:          []string{"net/http", "fmt"},
		Embedding:        embedding,
	}

	// Upsert
	if err := store.Upsert(ctx, []codeintel.Chunk{chunk}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	// GetByFilePath
	found, err := store.GetByFilePath(ctx, "internal/auth/handler.go")
	if err != nil {
		t.Fatalf("GetByFilePath: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("GetByFilePath: got %d, want 1", len(found))
	}
	if found[0].Name != "HandleLogin" {
		t.Errorf("Name = %q, want HandleLogin", found[0].Name)
	}

	// GetByName
	byName, err := store.GetByName(ctx, "HandleLogin")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if len(byName) != 1 {
		t.Fatalf("GetByName: got %d, want 1", len(byName))
	}

	// VectorSearch
	results, err := store.VectorSearch(ctx, embedding, 5, codeintel.Filter{})
	if err != nil {
		t.Fatalf("VectorSearch: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("VectorSearch: got %d, want 1", len(results))
	}
	if results[0].Chunk.Name != "HandleLogin" {
		t.Errorf("VectorSearch result Name = %q", results[0].Chunk.Name)
	}

	// DeleteByFilePath
	if err := store.DeleteByFilePath(ctx, "internal/auth/handler.go"); err != nil {
		t.Fatalf("DeleteByFilePath: %v", err)
	}
	afterDelete, err := store.GetByFilePath(ctx, "internal/auth/handler.go")
	if err != nil {
		t.Fatalf("GetByFilePath after delete: %v", err)
	}
	if len(afterDelete) != 0 {
		t.Errorf("expected 0 chunks after delete, got %d", len(afterDelete))
	}
}

func TestStoreImplementsInterface(t *testing.T) {
	var _ codeintel.Store = (*Store)(nil)
}

func TestUpsert_WrongEmbeddingDims(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	store, err := NewStore(ctx, dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	chunk := codeintel.Chunk{
		ID:        "bad-dims",
		Name:      "BadFunc",
		FilePath:  "bad.go",
		Language:  "go",
		ChunkType: codeintel.ChunkTypeFunction,
		Embedding: make([]float32, 10), // Wrong: should be 3584
	}

	err = store.Upsert(ctx, []codeintel.Chunk{chunk})
	if err == nil {
		t.Fatal("expected error for wrong embedding dimensions")
	}
}

func TestVectorSearch_WithFilters(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	store, err := NewStore(ctx, dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	dims := codeintel.DefaultEmbeddingDims
	embedding := make([]float32, dims)
	for i := range embedding {
		embedding[i] = 0.01
	}

	chunks := []codeintel.Chunk{
		{
			ID: "go-func", Name: "GoFunc", FilePath: "internal/auth/auth.go",
			Language: "go", ChunkType: codeintel.ChunkTypeFunction,
			Embedding: embedding, ContentHash: "h1", IndexedAt: time.Now(),
		},
		{
			ID: "py-func", Name: "PyFunc", FilePath: "scripts/run.py",
			Language: "python", ChunkType: codeintel.ChunkTypeFunction,
			Embedding: embedding, ContentHash: "h2", IndexedAt: time.Now(),
		},
		{
			ID: "go-type", Name: "GoType", FilePath: "internal/auth/types.go",
			Language: "go", ChunkType: codeintel.ChunkTypeType,
			Embedding: embedding, ContentHash: "h3", IndexedAt: time.Now(),
		},
	}

	if err := store.Upsert(ctx, chunks); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	// Filter by language
	results, err := store.VectorSearch(ctx, embedding, 10, codeintel.Filter{Language: "go"})
	if err != nil {
		t.Fatalf("VectorSearch language filter: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("language filter: got %d results, want 2", len(results))
	}

	// Filter by chunk type
	results, err = store.VectorSearch(ctx, embedding, 10, codeintel.Filter{ChunkType: codeintel.ChunkTypeFunction})
	if err != nil {
		t.Fatalf("VectorSearch chunk type filter: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("chunk type filter: got %d results, want 2", len(results))
	}

	// Filter by file path prefix
	results, err = store.VectorSearch(ctx, embedding, 10, codeintel.Filter{FilePathPrefix: "internal/"})
	if err != nil {
		t.Fatalf("VectorSearch file path filter: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("file path prefix filter: got %d results, want 2", len(results))
	}

	// Combined filter: go + function
	results, err = store.VectorSearch(ctx, embedding, 10, codeintel.Filter{
		Language:  "go",
		ChunkType: codeintel.ChunkTypeFunction,
	})
	if err != nil {
		t.Fatalf("VectorSearch combined filter: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("combined filter: got %d results, want 1", len(results))
	}
	if len(results) > 0 && results[0].Chunk.Name != "GoFunc" {
		t.Errorf("combined filter: got %q, want GoFunc", results[0].Chunk.Name)
	}
}
