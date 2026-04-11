package goparser

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/ponchione/sodoryard/internal/codeintel"
	"github.com/ponchione/sodoryard/internal/codeintel/treesitter"
)

// repoRoot returns the root of the sirtopham repository.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file path")
	}
	// thisFile is internal/codeintel/goparser/goparser_test.go → root is ../../../
	root := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	return abs
}

func TestNew(t *testing.T) {
	root := repoRoot(t)
	p, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if p == nil {
		t.Fatal("parser is nil")
	}
}

func TestParse_GoFile(t *testing.T) {
	root := repoRoot(t)
	p, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Parse the codeintel types.go file — should find FuncRef, RawChunk, Chunk, etc.
	typesPath := filepath.Join(root, "internal", "codeintel", "types.go")
	content, err := os.ReadFile(typesPath)
	if err != nil {
		t.Fatalf("read types.go: %v", err)
	}

	chunks, err := p.Parse(typesPath, content)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if len(chunks) == 0 {
		t.Fatal("expected chunks from types.go, got 0")
	}

	// Should find the FuncRef type.
	found := false
	for _, c := range chunks {
		if c.Name == "FuncRef" && c.ChunkType == codeintel.ChunkTypeType {
			found = true
			break
		}
	}
	if !found {
		t.Error("FuncRef type not found in parsed chunks")
	}
}

func TestParse_FunctionWithCalls(t *testing.T) {
	root := repoRoot(t)
	p, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Parse hash.go — ChunkID calls sha256.Sum256 and fmt.Sprintf.
	hashPath := filepath.Join(root, "internal", "codeintel", "hash.go")
	content, err := os.ReadFile(hashPath)
	if err != nil {
		t.Fatalf("read hash.go: %v", err)
	}

	chunks, err := p.Parse(hashPath, content)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	var chunkID *codeintel.RawChunk
	for i := range chunks {
		if chunks[i].Name == "ChunkID" {
			chunkID = &chunks[i]
			break
		}
	}
	if chunkID == nil {
		t.Fatal("ChunkID function not found")
	}

	if chunkID.ChunkType != codeintel.ChunkTypeFunction {
		t.Errorf("ChunkID type = %q, want %q", chunkID.ChunkType, codeintel.ChunkTypeFunction)
	}

	// Should have calls.
	if len(chunkID.Calls) == 0 {
		t.Error("expected ChunkID to have non-empty Calls")
	}

	// Should have imports.
	if len(chunkID.Imports) == 0 {
		t.Error("expected ChunkID to have non-empty Imports")
	}
}

func TestParse_NonGoFile(t *testing.T) {
	root := repoRoot(t)
	p, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	chunks, err := p.Parse("readme.md", []byte("# Hello\nworld"))
	if err != nil {
		t.Fatalf("Parse non-Go: %v", err)
	}
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for non-Go file, got %d", len(chunks))
	}
}

func TestParse_MethodDetection(t *testing.T) {
	root := repoRoot(t)
	p, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Parse config.go — has methods like Validate(), normalize(), etc.
	cfgPath := filepath.Join(root, "internal", "config", "config.go")
	content, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config.go: %v", err)
	}

	chunks, err := p.Parse(cfgPath, content)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	var foundMethod bool
	for _, c := range chunks {
		if c.ChunkType == codeintel.ChunkTypeMethod {
			foundMethod = true
			break
		}
	}
	if !foundMethod {
		t.Error("expected at least one method chunk from config.go")
	}
}

func newTestParser(t *testing.T) *Parser {
	t.Helper()
	root := repoRoot(t)
	p, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return p
}

func TestParse_NonGoFile_ReturnsEmptySlice(t *testing.T) {
	p := newTestParser(t)

	chunks, err := p.Parse("readme.md", []byte("# Hello"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if chunks == nil {
		t.Fatal("expected non-nil empty slice, got nil")
	}
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks, got %d", len(chunks))
	}
}

func TestParserImplementsInterface(t *testing.T) {
	var _ codeintel.Parser = (*Parser)(nil)
}

func TestParse_NonGoFile_FallsBackToTreeSitter(t *testing.T) {
	root := repoRoot(t)
	p, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	p.WithFallback(treesitter.New())

	pyContent := []byte("def greet(name):\n    return f'Hello, {name}'\n\nclass Greeter:\n    pass\n")
	chunks, err := p.Parse("example.py", pyContent)
	if err != nil {
		t.Fatalf("Parse python: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("expected tree-sitter to return chunks for .py file, got 0")
	}
}

func TestParse_Generics(t *testing.T) {
	tmpDir := t.TempDir()

	// Write go.mod
	goMod := "module example\n\ngo 1.21\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	// Write generic Go file
	goSrc := `package example

type Set[T comparable] struct { items map[T]bool }

func NewSet[T comparable]() *Set[T] { return &Set[T]{items: make(map[T]bool)} }

func (s *Set[T]) Add(item T) { s.items[item] = true }
`
	srcPath := filepath.Join(tmpDir, "set.go")
	if err := os.WriteFile(srcPath, []byte(goSrc), 0644); err != nil {
		t.Fatalf("write set.go: %v", err)
	}

	p, err := New(tmpDir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	content, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("read set.go: %v", err)
	}

	chunks, err := p.Parse(srcPath, content)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if len(chunks) < 3 {
		t.Fatalf("expected at least 3 chunks (Set type, NewSet func, Add method), got %d", len(chunks))
	}

	names := make(map[string]bool)
	for _, c := range chunks {
		names[c.Name] = true
	}
	for _, want := range []string{"Set", "NewSet", "Add"} {
		if !names[want] {
			t.Errorf("chunk %q not found in parsed chunks", want)
		}
	}
}

func TestParse_InterfaceExtraction(t *testing.T) {
	root := repoRoot(t)
	p, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ifacePath := filepath.Join(root, "internal", "codeintel", "interfaces.go")
	content, err := os.ReadFile(ifacePath)
	if err != nil {
		t.Fatalf("read interfaces.go: %v", err)
	}

	chunks, err := p.Parse(ifacePath, content)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	var foundInterface bool
	var foundParser bool
	for _, c := range chunks {
		if c.ChunkType == codeintel.ChunkTypeInterface {
			foundInterface = true
		}
		if c.Name == "Parser" && c.ChunkType == codeintel.ChunkTypeInterface {
			foundParser = true
		}
	}
	if !foundInterface {
		t.Error("expected at least one chunk with ChunkType == ChunkTypeInterface")
	}
	if !foundParser {
		t.Error("Parser interface not found by name in parsed chunks")
	}
}

func TestParse_EmbeddedTypes(t *testing.T) {
	tmpDir := t.TempDir()

	// Write go.mod
	goMod := "module example\n\ngo 1.21\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	// Write Go file with embedded types
	goSrc := `package example

type Base struct { ID int }

type Child struct {
	Base
	Name string
}
`
	srcPath := filepath.Join(tmpDir, "types.go")
	if err := os.WriteFile(srcPath, []byte(goSrc), 0644); err != nil {
		t.Fatalf("write types.go: %v", err)
	}

	p, err := New(tmpDir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	content, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("read types.go: %v", err)
	}

	chunks, err := p.Parse(srcPath, content)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	names := make(map[string]bool)
	for _, c := range chunks {
		names[c.Name] = true
	}

	if !names["Base"] {
		t.Error("Base type not found in parsed chunks")
	}
	if !names["Child"] {
		t.Error("Child type not found in parsed chunks")
	}
}

func TestParse_TruncatesFunctionBodyOnUTF8Boundary(t *testing.T) {
	tmpDir := t.TempDir()
	goMod := "module example\n\ngo 1.21\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	bodyLine := strings.Repeat("é", codeintel.MaxBodyLength/2) + "🙂"
	goSrc := "package example\n\nfunc Demo() string {\n    return \"" + bodyLine + "\"\n}\n"
	srcPath := filepath.Join(tmpDir, "demo.go")
	if err := os.WriteFile(srcPath, []byte(goSrc), 0644); err != nil {
		t.Fatalf("write demo.go: %v", err)
	}
	p, err := New(tmpDir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	content, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("read demo.go: %v", err)
	}
	chunks, err := p.Parse(srcPath, content)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	var demo *codeintel.RawChunk
	for i := range chunks {
		if chunks[i].Name == "Demo" {
			demo = &chunks[i]
			break
		}
	}
	if demo == nil {
		t.Fatal("Demo function not found")
	}
	if !utf8.ValidString(demo.Body) {
		t.Fatal("function body is invalid UTF-8")
	}
	if len(demo.Body) > codeintel.MaxBodyLength {
		t.Fatalf("len(body) = %d, want <= %d", len(demo.Body), codeintel.MaxBodyLength)
	}
}
