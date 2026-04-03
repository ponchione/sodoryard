package treesitter

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/ponchione/sirtopham/internal/codeintel"
)

func TestParserImplementsInterface(t *testing.T) {
	var _ codeintel.Parser = (*Parser)(nil)
}

// --- Go ---

func TestParseGo_FunctionAndType(t *testing.T) {
	source := `package main

func Hello(name string) string {
	return "hello " + name
}

type Config struct {
	Port int
}
`
	p := New()
	chunks, err := p.Parse("main.go", []byte(source))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("got %d chunks, want 2; names: %v", len(chunks), chunkNames(chunks))
	}
	if chunks[0].Name != "Hello" || chunks[0].ChunkType != codeintel.ChunkTypeFunction {
		t.Errorf("chunk[0] = %s(%s), want Hello(function)", chunks[0].Name, chunks[0].ChunkType)
	}
	if chunks[1].Name != "Config" || chunks[1].ChunkType != codeintel.ChunkTypeType {
		t.Errorf("chunk[1] = %s(%s), want Config(type)", chunks[1].Name, chunks[1].ChunkType)
	}
}

func TestParseGo_Method(t *testing.T) {
	source := `package main

func (c *Config) Validate() error {
	return nil
}
`
	p := New()
	chunks, err := p.Parse("config.go", []byte(source))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want 1", len(chunks))
	}
	if chunks[0].ChunkType != codeintel.ChunkTypeMethod {
		t.Errorf("ChunkType = %q, want method", chunks[0].ChunkType)
	}
}

func TestParseGo_Interface(t *testing.T) {
	source := `package main

type Reader interface {
	Read(p []byte) (n int, err error)
}
`
	p := New()
	chunks, err := p.Parse("reader.go", []byte(source))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want 1; names: %v", len(chunks), chunkNames(chunks))
	}
	if chunks[0].Name != "Reader" {
		t.Errorf("Name = %q, want Reader", chunks[0].Name)
	}
	if chunks[0].ChunkType != codeintel.ChunkTypeInterface {
		t.Errorf("ChunkType = %q, want interface", chunks[0].ChunkType)
	}
}

// --- Python ---

func TestParsePython_TopLevelFunction(t *testing.T) {
	source := `def greet(name: str) -> str:
    return "hello " + name
`
	p := New()
	chunks, err := p.Parse("greet.py", []byte(source))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want 1", len(chunks))
	}
	c := chunks[0]
	if c.Name != "greet" {
		t.Errorf("Name = %q, want greet", c.Name)
	}
	if c.ChunkType != codeintel.ChunkTypeFunction {
		t.Errorf("ChunkType = %q, want function", c.ChunkType)
	}
	if !strings.Contains(c.Signature, "def greet") {
		t.Errorf("Signature %q missing 'def greet'", c.Signature)
	}
}

func TestParsePython_DecoratedFunction(t *testing.T) {
	source := `@app.route("/api/data")
@login_required
def get_data():
    return jsonify(data)
`
	p := New()
	chunks, err := p.Parse("api.py", []byte(source))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want 1", len(chunks))
	}
	c := chunks[0]
	if c.Name != "get_data" {
		t.Errorf("Name = %q, want get_data", c.Name)
	}
	if !strings.Contains(c.Signature, "@app.route") {
		t.Errorf("Signature %q missing decorator", c.Signature)
	}
}

func TestParsePython_ClassWithMethods(t *testing.T) {
	source := `class UserService:
    def __init__(self, db):
        self.db = db

    def get_user(self, user_id: int) -> dict:
        return self.db.find(user_id)
`
	p := New()
	chunks, err := p.Parse("service.py", []byte(source))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	// class + __init__ + get_user = 3
	if len(chunks) != 3 {
		t.Fatalf("got %d chunks, want 3; names: %v", len(chunks), chunkNames(chunks))
	}
	if chunks[0].ChunkType != codeintel.ChunkTypeClass {
		t.Errorf("chunk[0].ChunkType = %q, want class", chunks[0].ChunkType)
	}
	if chunks[1].ChunkType != codeintel.ChunkTypeMethod {
		t.Errorf("chunk[1].ChunkType = %q, want method", chunks[1].ChunkType)
	}
}

// --- TypeScript ---

func TestParseTypeScript_FunctionDecl(t *testing.T) {
	source := `function greet(name: string): string {
	return "hello " + name;
}`
	p := New()
	chunks, err := p.Parse("greet.ts", []byte(source))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("no chunks")
	}
	if chunks[0].Name != "greet" || chunks[0].ChunkType != codeintel.ChunkTypeFunction {
		t.Errorf("got %s(%s), want greet(function)", chunks[0].Name, chunks[0].ChunkType)
	}
}

func TestParseTypeScript_Interface(t *testing.T) {
	source := `interface User {
	id: number;
	name: string;
}`
	p := New()
	chunks, err := p.Parse("user.ts", []byte(source))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("no chunks")
	}
	if chunks[0].Name != "User" || chunks[0].ChunkType != codeintel.ChunkTypeInterface {
		t.Errorf("got %s(%s), want User(interface)", chunks[0].Name, chunks[0].ChunkType)
	}
}

func TestParseTypeScript_ArrowFunction(t *testing.T) {
	source := `const add = (a: number, b: number): number => {
	return a + b;
};`
	p := New()
	chunks, err := p.Parse("math.ts", []byte(source))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("no chunks")
	}
	if chunks[0].Name != "add" || chunks[0].ChunkType != codeintel.ChunkTypeFunction {
		t.Errorf("got %s(%s), want add(function)", chunks[0].Name, chunks[0].ChunkType)
	}
}

func TestParseTSX_Component(t *testing.T) {
	source := `const Button = (props: { label: string }) => {
	return <button>{props.label}</button>;
};`
	p := New()
	chunks, err := p.Parse("button.tsx", []byte(source))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("no chunks")
	}
	if chunks[0].Name != "Button" {
		t.Errorf("Name = %q, want Button", chunks[0].Name)
	}
}

func TestParseTypeScript_ExportedFunction(t *testing.T) {
	source := `export function fetchUser(id: number): Promise<User> {
	return api.get("/users/" + id);
}`
	p := New()
	chunks, err := p.Parse("api.ts", []byte(source))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("no chunks")
	}
	if chunks[0].Name != "fetchUser" {
		t.Errorf("Name = %q, want fetchUser", chunks[0].Name)
	}
}

// --- Markdown ---

func TestParseMarkdown(t *testing.T) {
	source := `## Section One
Some content

## Section Two
More content
`
	p := New()
	chunks, err := p.Parse("readme.md", []byte(source))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("got %d chunks, want 2", len(chunks))
	}
	if chunks[0].Name != "Section One" || chunks[0].ChunkType != codeintel.ChunkTypeSection {
		t.Errorf("chunk[0] = %s(%s)", chunks[0].Name, chunks[0].ChunkType)
	}
	if chunks[1].Name != "Section Two" || chunks[1].ChunkType != codeintel.ChunkTypeSection {
		t.Errorf("chunk[1] = %s(%s)", chunks[1].Name, chunks[1].ChunkType)
	}
}

func TestParseMarkdown_TruncatesOnUTF8Boundary(t *testing.T) {
	longLine := strings.Repeat("é", codeintel.MaxBodyLength/2) + "🙂"
	source := "## Section One\n" + longLine + "\n"
	p := New()
	chunks, err := p.Parse("readme.md", []byte(source))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want 1", len(chunks))
	}
	if !utf8.ValidString(chunks[0].Body) {
		t.Fatal("markdown chunk body is invalid UTF-8")
	}
	if len(chunks[0].Body) > codeintel.MaxBodyLength {
		t.Fatalf("len(body) = %d, want <= %d", len(chunks[0].Body), codeintel.MaxBodyLength)
	}
}

// --- Fallback ---

func TestParseFallback(t *testing.T) {
	// 50 lines of content should produce sliding-window chunks.
	var lines []string
	for i := range 50 {
		lines = append(lines, strings.Repeat("x", 10)+" "+string(rune('a'+i%26)))
	}
	source := strings.Join(lines, "\n")

	p := New()
	chunks, err := p.Parse("data.txt", []byte(source))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("expected fallback chunks, got 0")
	}
	for _, c := range chunks {
		if c.ChunkType != codeintel.ChunkTypeFallback {
			t.Errorf("ChunkType = %q, want fallback", c.ChunkType)
		}
	}
}

// --- Language detection ---

func TestLanguageDetection(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"main.go", "go"},
		{"app.py", "python"},
		{"index.ts", "typescript"},
		{"App.tsx", "tsx"},
		{"README.md", "markdown"},
		{"schema.sql", "sql"},
		{"data.txt", ""},
	}
	for _, tt := range tests {
		got := detectLanguage(tt.path)
		if got != tt.want {
			t.Errorf("detectLanguage(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestParseGo_InvalidSyntax(t *testing.T) {
	// Binary garbage — tree-sitter should not error, just return empty/nil.
	garbage := []byte{0x00, 0xFF, 0xFE, 0x80, 0x81, 0x82, 0x83, 0x84, 0x85}
	p := New()
	chunks, err := p.Parse("garbage.go", garbage)
	if err != nil {
		t.Fatalf("expected no error for invalid syntax, got: %v", err)
	}
	if len(chunks) != 0 {
		t.Errorf("expected empty/nil slice, got %d chunks", len(chunks))
	}
}

func chunkNames(chunks []codeintel.RawChunk) []string {
	names := make([]string, len(chunks))
	for i, c := range chunks {
		names[i] = c.Name + "(" + string(c.ChunkType) + ")"
	}
	return names
}
