package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileEditSuccess(t *testing.T) {
	dir := t.TempDir()
	content := "func main() {\n\tfmt.Println(\"hello\")\n}\n"
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(content), 0o644)

	input, _ := json.Marshal(fileEditInput{
		Path:   "main.go",
		OldStr: "\"hello\"",
		NewStr: "\"world\"",
	})

	result, err := FileEdit{}.Execute(context.Background(), dir, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Content)
	}

	// Should contain a diff.
	if !strings.Contains(result.Content, "-") && !strings.Contains(result.Content, "+") {
		t.Fatalf("expected diff output, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "\"hello\"") || !strings.Contains(result.Content, "\"world\"") {
		t.Fatalf("expected old and new strings in diff, got:\n%s", result.Content)
	}

	// Verify file content changed.
	data, _ := os.ReadFile(filepath.Join(dir, "main.go"))
	if !strings.Contains(string(data), "\"world\"") {
		t.Fatalf("file not updated, got: %s", data)
	}
	if strings.Contains(string(data), "\"hello\"") {
		t.Fatal("old string still present in file")
	}
}

func TestFileEditZeroMatches(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello world\n"), 0o644)

	input, _ := json.Marshal(fileEditInput{
		Path:   "file.txt",
		OldStr: "nonexistent string",
		NewStr: "replacement",
	})

	result, err := FileEdit{}.Execute(context.Background(), dir, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure for zero matches")
	}
	if !strings.Contains(result.Content, "not found") {
		t.Fatalf("expected 'not found' error, got: %s", result.Content)
	}
}

func TestFileEditMultipleMatches(t *testing.T) {
	dir := t.TempDir()
	content := "foo bar foo baz foo\n"
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte(content), 0o644)

	input, _ := json.Marshal(fileEditInput{
		Path:   "file.txt",
		OldStr: "foo",
		NewStr: "qux",
	})

	result, err := FileEdit{}.Execute(context.Background(), dir, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure for multiple matches")
	}
	if !strings.Contains(result.Content, "3 times") {
		t.Fatalf("expected match count in error, got: %s", result.Content)
	}
}

func TestFileEditFileNotFound(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "other.go"), []byte("x"), 0o644)

	input, _ := json.Marshal(fileEditInput{
		Path:   "missing.go",
		OldStr: "old",
		NewStr: "new",
	})

	result, err := FileEdit{}.Execute(context.Background(), dir, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure for missing file")
	}
	if !strings.Contains(result.Content, "File not found") {
		t.Fatalf("expected 'File not found', got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "other.go") {
		t.Fatalf("expected directory listing, got: %s", result.Content)
	}
}

func TestFileEditPathTraversal(t *testing.T) {
	dir := t.TempDir()

	input, _ := json.Marshal(fileEditInput{
		Path:   "../../../etc/passwd",
		OldStr: "root",
		NewStr: "hacked",
	})

	result, err := FileEdit{}.Execute(context.Background(), dir, input)
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

func TestFileEditEmptyOldStr(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello\n"), 0o644)

	input, _ := json.Marshal(fileEditInput{
		Path:   "file.txt",
		OldStr: "",
		NewStr: "new",
	})

	result, err := FileEdit{}.Execute(context.Background(), dir, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure for empty old_str")
	}
}

func TestFileEditPreservesPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "script.sh")
	os.WriteFile(path, []byte("#!/bin/bash\necho old\n"), 0o755)

	input, _ := json.Marshal(fileEditInput{
		Path:   "script.sh",
		OldStr: "echo old",
		NewStr: "echo new",
	})

	result, err := FileEdit{}.Execute(context.Background(), dir, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Content)
	}

	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("permissions changed to %o, want 755", info.Mode().Perm())
	}
}

func TestFileEditSchema(t *testing.T) {
	schema := FileEdit{}.Schema()
	if !json.Valid(schema) {
		t.Fatal("Schema() is not valid JSON")
	}
	if !strings.Contains(string(schema), "file_edit") {
		t.Fatal("Schema() does not contain tool name")
	}
}
