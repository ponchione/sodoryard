package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileWriteNewFile(t *testing.T) {
	dir := t.TempDir()

	result, err := FileWrite{}.Execute(context.Background(), dir,
		json.RawMessage(`{"path":"new.txt","content":"hello world\n"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "[new file created]") {
		t.Fatalf("expected new file message, got: %s", result.Content)
	}

	// Verify file was written.
	data, err := os.ReadFile(filepath.Join(dir, "new.txt"))
	if err != nil {
		t.Fatalf("file not written: %v", err)
	}
	if string(data) != "hello world\n" {
		t.Fatalf("file content = %q, want 'hello world\\n'", data)
	}
}

func TestFileWriteNestedDirectories(t *testing.T) {
	dir := t.TempDir()

	result, err := FileWrite{}.Execute(context.Background(), dir,
		json.RawMessage(`{"path":"deep/nested/dir/file.go","content":"package main\n"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Content)
	}

	// Verify directories and file created.
	data, err := os.ReadFile(filepath.Join(dir, "deep", "nested", "dir", "file.go"))
	if err != nil {
		t.Fatalf("file not written: %v", err)
	}
	if string(data) != "package main\n" {
		t.Fatalf("file content = %q", data)
	}
}

func TestFileWriteOverwriteWithDiff(t *testing.T) {
	dir := t.TempDir()
	oldContent := "line one\nline two\nline three\n"
	os.WriteFile(filepath.Join(dir, "existing.txt"), []byte(oldContent), 0o644)

	newContent := "line one\nline TWO modified\nline three\n"
	input, _ := json.Marshal(fileWriteInput{Path: "existing.txt", Content: newContent})

	result, err := FileWrite{}.Execute(context.Background(), dir, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Content)
	}

	// Should contain unified diff markers.
	if !strings.Contains(result.Content, "---") || !strings.Contains(result.Content, "+++") {
		t.Fatalf("expected unified diff, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "-line two") {
		t.Fatalf("expected removed line in diff, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "+line TWO modified") {
		t.Fatalf("expected added line in diff, got:\n%s", result.Content)
	}

	// Verify file was actually updated.
	data, _ := os.ReadFile(filepath.Join(dir, "existing.txt"))
	if string(data) != newContent {
		t.Fatalf("file not updated, got: %q", data)
	}
}

func TestFileWriteDiffTruncation(t *testing.T) {
	dir := t.TempDir()

	// Create a file with many lines.
	var old, new_ strings.Builder
	for i := 0; i < 100; i++ {
		old.WriteString("old line\n")
		new_.WriteString("new line\n")
	}
	os.WriteFile(filepath.Join(dir, "big.txt"), []byte(old.String()), 0o644)

	input, _ := json.Marshal(fileWriteInput{Path: "big.txt", Content: new_.String()})
	result, err := FileWrite{}.Execute(context.Background(), dir, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "diff truncated") {
		t.Fatalf("expected truncation notice, got:\n%s", result.Content)
	}
}

func TestFileWritePathTraversal(t *testing.T) {
	dir := t.TempDir()

	result, err := FileWrite{}.Execute(context.Background(), dir,
		json.RawMessage(`{"path":"../../evil.txt","content":"pwned"}`))
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

func TestFileWriteAbsolutePath(t *testing.T) {
	dir := t.TempDir()

	result, err := FileWrite{}.Execute(context.Background(), dir,
		json.RawMessage(`{"path":"/tmp/evil.txt","content":"pwned"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure for absolute path")
	}
}

func TestFileWriteSchema(t *testing.T) {
	schema := FileWrite{}.Schema()
	if !json.Valid(schema) {
		t.Fatal("Schema() is not valid JSON")
	}
	if !strings.Contains(string(schema), "file_write") {
		t.Fatal("Schema() does not contain tool name")
	}
}
