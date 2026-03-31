package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileReadNormal(t *testing.T) {
	dir := t.TempDir()
	content := "line one\nline two\nline three\n"
	os.WriteFile(filepath.Join(dir, "test.go"), []byte(content), 0o644)

	result, err := FileRead{}.Execute(context.Background(), dir, json.RawMessage(`{"path":"test.go"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "1\tline one") {
		t.Fatalf("expected line numbers, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "3\tline three") {
		t.Fatalf("expected line 3, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "(3 lines)") {
		t.Fatalf("expected line count header, got:\n%s", result.Content)
	}
}

func TestFileReadLineRange(t *testing.T) {
	dir := t.TempDir()
	var lines []string
	for i := 1; i <= 20; i++ {
		lines = append(lines, "line "+string(rune('A'-1+i)))
	}
	os.WriteFile(filepath.Join(dir, "big.txt"), []byte(strings.Join(lines, "\n")+"\n"), 0o644)

	result, err := FileRead{}.Execute(context.Background(), dir,
		json.RawMessage(`{"path":"big.txt","line_start":5,"line_end":10}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "lines 5-10 of 20") {
		t.Fatalf("expected range header, got:\n%s", result.Content)
	}
	// Should contain lines 5-10 but not line 4 or 11.
	if !strings.Contains(result.Content, "5\t") {
		t.Fatalf("expected line 5, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "10\t") {
		t.Fatalf("expected line 10, got:\n%s", result.Content)
	}
	// Count the numbered lines.
	numbered := 0
	for _, l := range strings.Split(result.Content, "\n") {
		if strings.Contains(l, "\t") && !strings.HasPrefix(l, "File:") {
			numbered++
		}
	}
	if numbered != 6 {
		t.Fatalf("expected 6 numbered lines (5-10), got %d", numbered)
	}
}

func TestFileReadFileNotFound(t *testing.T) {
	dir := t.TempDir()
	// Create some files so the listing is populated.
	os.WriteFile(filepath.Join(dir, "foo.go"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "bar.go"), []byte("x"), 0o644)

	result, err := FileRead{}.Execute(context.Background(), dir,
		json.RawMessage(`{"path":"missing.go"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure for missing file")
	}
	if !strings.Contains(result.Content, "File not found") {
		t.Fatalf("expected 'File not found', got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "bar.go") || !strings.Contains(result.Content, "foo.go") {
		t.Fatalf("expected directory listing in error, got: %s", result.Content)
	}
}

func TestFileReadPathTraversal(t *testing.T) {
	dir := t.TempDir()

	result, err := FileRead{}.Execute(context.Background(), dir,
		json.RawMessage(`{"path":"../../../etc/passwd"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure for path traversal")
	}
	if !strings.Contains(result.Content, "escapes project root") {
		t.Fatalf("expected path traversal error, got: %s", result.Content)
	}
}

func TestFileReadAbsolutePath(t *testing.T) {
	dir := t.TempDir()

	result, err := FileRead{}.Execute(context.Background(), dir,
		json.RawMessage(`{"path":"/etc/passwd"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure for absolute path")
	}
	if !strings.Contains(result.Content, "absolute paths") {
		t.Fatalf("expected absolute path error, got: %s", result.Content)
	}
}

func TestFileReadBinaryFile(t *testing.T) {
	dir := t.TempDir()
	// Write a file with null bytes.
	data := []byte("hello\x00world\x00binary")
	os.WriteFile(filepath.Join(dir, "binary.bin"), data, 0o644)

	result, err := FileRead{}.Execute(context.Background(), dir,
		json.RawMessage(`{"path":"binary.bin"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure for binary file")
	}
	if !strings.Contains(result.Content, "Binary file") {
		t.Fatalf("expected binary file message, got: %s", result.Content)
	}
}

func TestFileReadEmptyFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "empty.txt"), []byte(""), 0o644)

	result, err := FileRead{}.Execute(context.Background(), dir,
		json.RawMessage(`{"path":"empty.txt"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success for empty file, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "empty file") {
		t.Fatalf("expected empty file note, got: %s", result.Content)
	}
}

func TestFileReadBeyondEndOfFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "short.txt"), []byte("one\ntwo\n"), 0o644)

	result, err := FileRead{}.Execute(context.Background(), dir,
		json.RawMessage(`{"path":"short.txt","line_start":500}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success (clamped), got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "beyond end of file") {
		t.Fatalf("expected beyond-end note, got: %s", result.Content)
	}
}

func TestFileReadSubdirectory(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "pkg", "auth")
	os.MkdirAll(subDir, 0o755)
	os.WriteFile(filepath.Join(subDir, "handler.go"), []byte("package auth\n"), 0o644)

	result, err := FileRead{}.Execute(context.Background(), dir,
		json.RawMessage(`{"path":"pkg/auth/handler.go"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "package auth") {
		t.Fatalf("expected file content, got: %s", result.Content)
	}
}

func TestFileReadSchema(t *testing.T) {
	schema := FileRead{}.Schema()
	if !json.Valid(schema) {
		t.Fatal("Schema() is not valid JSON")
	}
	if !strings.Contains(string(schema), "file_read") {
		t.Fatal("Schema() does not contain tool name")
	}
}
