package indexer

import (
	"strings"
	"testing"

	"github.com/ponchione/sodoryard/internal/codeintel"
)

func TestLangFromExt(t *testing.T) {
	tests := []struct {
		ext, want string
	}{
		{".go", "go"},
		{".py", "python"},
		{".ts", "typescript"},
		{".tsx", "tsx"},
		{".md", "markdown"},
		{".rs", ""},
	}
	for _, tt := range tests {
		got := langFromExt(tt.ext)
		if got != tt.want {
			t.Errorf("langFromExt(%q) = %q, want %q", tt.ext, got, tt.want)
		}
	}
}

func TestMatchesAnyGlob(t *testing.T) {
	patterns := []string{"**/*.go", "*.md"}

	if !matchesAnyGlob(patterns, "internal/codeintel/graph/store.go") {
		t.Error("should match **/*.go")
	}
	if !matchesAnyGlob(patterns, "README.md") {
		t.Error("should match *.md")
	}
	if matchesAnyGlob(patterns, "main.py") {
		t.Error("should not match main.py")
	}
	if matchesAnyGlob(nil, "anything.go") {
		t.Error("nil patterns should not match")
	}
}

func TestMatchesGlob_YAMLExcludesNodeModulesAndHiddenState(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		{pattern: "**/node_modules/**", path: "web/node_modules/react/index.js", want: true},
		{pattern: "**/.brain/**", path: ".brain/notes/hello.md", want: true},
		{pattern: "**/.sirtopham/**", path: ".sirtopham/lancedb/code/0001.lance", want: true},
		{pattern: "**/node_modules/**", path: "web/src/main.tsx", want: false},
	}
	for _, tt := range tests {
		if got := matchesGlob(tt.pattern, tt.path); got != tt.want {
			t.Fatalf("matchesGlob(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
		}
	}
}

func TestNewChunk(t *testing.T) {
	raw := codeintel.RawChunk{
		Name:      "MyFunc",
		Signature: "func MyFunc(x int) error",
		Body:      "func MyFunc(x int) error { return nil }",
		ChunkType: codeintel.ChunkTypeFunction,
		LineStart: 10,
		LineEnd:   12,
		Calls:     []codeintel.FuncRef{{Name: "Println", Package: "fmt"}},
		Imports:   []string{"fmt"},
	}

	chunk := newChunk(raw, "myproject", "internal/foo.go", "go", "Does something")
	if chunk.ID == "" {
		t.Error("chunk ID should not be empty")
	}
	if chunk.ProjectName != "myproject" {
		t.Errorf("ProjectName = %q", chunk.ProjectName)
	}
	if chunk.FilePath != "internal/foo.go" {
		t.Errorf("FilePath = %q", chunk.FilePath)
	}
	if chunk.Language != "go" {
		t.Errorf("Language = %q", chunk.Language)
	}
	if chunk.Description != "Does something" {
		t.Errorf("Description = %q", chunk.Description)
	}
	if chunk.ContentHash == "" {
		t.Error("ContentHash should not be empty")
	}
	if chunk.IndexedAt.IsZero() {
		t.Error("IndexedAt should be set")
	}
	if len(chunk.Calls) != 1 {
		t.Errorf("Calls count = %d, want 1", len(chunk.Calls))
	}
}

func TestNewChunk_TruncatesBody(t *testing.T) {
	longBody := strings.Repeat("x", codeintel.MaxBodyLength+500)

	raw := codeintel.RawChunk{
		Name:      "Big",
		Signature: "func Big()",
		Body:      longBody,
		ChunkType: codeintel.ChunkTypeFunction,
		LineStart: 1,
		LineEnd:   100,
	}

	chunk := newChunk(raw, "proj", "big.go", "go", "")
	if len(chunk.Body) != codeintel.MaxBodyLength {
		t.Errorf("Body length = %d, want %d", len(chunk.Body), codeintel.MaxBodyLength)
	}
}



func TestFormatRelationshipContext(t *testing.T) {
	chunks := []codeintel.Chunk{
		{
			Name:      "HandleRequest",
			ChunkType: codeintel.ChunkTypeFunction,
			Calls:     []codeintel.FuncRef{{Name: "Validate", Package: "pkg"}},
			CalledBy:  []codeintel.FuncRef{{Name: "main", Package: "main"}},
		},
	}

	ctx := formatRelationshipContext(chunks)
	if ctx == "" {
		t.Error("expected non-empty context")
	}
	if !strings.Contains(ctx, "HandleRequest") {
		t.Error("should mention HandleRequest")
	}
	if !strings.Contains(ctx, "Validate") {
		t.Error("should mention called function")
	}
	if !strings.Contains(ctx, "main") {
		t.Error("should mention caller")
	}
}

func TestFormatRelationshipContext_Empty(t *testing.T) {
	if ctx := formatRelationshipContext(nil); ctx != "" {
		t.Errorf("expected empty for nil, got %q", ctx)
	}

	// Chunks with no relationships should also return empty.
	chunks := []codeintel.Chunk{{Name: "Bare", ChunkType: codeintel.ChunkTypeFunction}}
	if ctx := formatRelationshipContext(chunks); ctx != "" {
		t.Errorf("expected empty for no-relationship chunks, got %q", ctx)
	}
}
