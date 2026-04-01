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

	store := newMemoryReadStateStore()
	reader := NewFileRead(store)
	editor := NewFileEdit(store)
	_, err := reader.Execute(context.Background(), dir, json.RawMessage(`{"path":"main.go"}`))
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}

	input, _ := json.Marshal(fileEditInput{
		Path:   "main.go",
		OldStr: "\"hello\"",
		NewStr: "\"world\"",
	})

	result, err := editor.Execute(context.Background(), dir, input)
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
	store := newMemoryReadStateStore()
	reader := NewFileRead(store)
	editor := NewFileEdit(store)
	_, err := reader.Execute(context.Background(), dir, json.RawMessage(`{"path":"file.txt"}`))
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}

	input, _ := json.Marshal(fileEditInput{
		Path:   "file.txt",
		OldStr: "nonexistent string",
		NewStr: "replacement",
	})

	result, err := editor.Execute(context.Background(), dir, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure for zero matches")
	}
	if result.Error != "zero_match" {
		t.Fatalf("expected zero_match error code, got %q", result.Error)
	}
	if !strings.Contains(result.Content, "Check for typos") || !strings.Contains(result.Content, "full file_read") {
		t.Fatalf("expected recovery guidance, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Preview:") || !strings.Contains(result.Content, "hello world") {
		t.Fatalf("expected file preview in error, got: %s", result.Content)
	}
}

func TestFileEditMultipleMatches(t *testing.T) {
	dir := t.TempDir()
	content := "foo bar foo baz foo\n"
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte(content), 0o644)
	store := newMemoryReadStateStore()
	reader := NewFileRead(store)
	editor := NewFileEdit(store)
	_, err := reader.Execute(context.Background(), dir, json.RawMessage(`{"path":"file.txt"}`))
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}

	input, _ := json.Marshal(fileEditInput{
		Path:   "file.txt",
		OldStr: "foo",
		NewStr: "qux",
	})

	result, err := editor.Execute(context.Background(), dir, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure for multiple matches")
	}
	if result.Error != "multiple_matches" {
		t.Fatalf("expected multiple_matches error code, got %q", result.Error)
	}
	if !strings.Contains(result.Content, "3 times") || !strings.Contains(result.Content, "surrounding context") {
		t.Fatalf("expected disambiguation guidance, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Candidate lines:") || !strings.Contains(result.Content, "1") {
		t.Fatalf("expected candidate line info, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Candidate snippets:") || !strings.Contains(result.Content, "line 1: foo bar foo baz foo") {
		t.Fatalf("expected candidate snippet info, got: %s", result.Content)
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

func TestFileEditRequiresFullReadFirst(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello world\n"), 0o644)

	store := newMemoryReadStateStore()
	editor := NewFileEdit(store)
	input, _ := json.Marshal(fileEditInput{
		Path:   "file.txt",
		OldStr: "world",
		NewStr: "there",
	})

	result, err := editor.Execute(context.Background(), dir, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure without prior full read")
	}
	if result.Error != "not_read_first" {
		t.Fatalf("expected not_read_first, got %q", result.Error)
	}
	if !strings.Contains(result.Content, "full file_read") {
		t.Fatalf("expected recovery hint, got: %s", result.Content)
	}
}

func TestFileEditRejectsPartialReadAsPrecondition(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("line1\nline2\nline3\n"), 0o644)

	store := newMemoryReadStateStore()
	reader := NewFileRead(store)
	editor := NewFileEdit(store)
	_, err := reader.Execute(context.Background(), dir, json.RawMessage(`{"path":"file.txt","line_start":1,"line_end":2}`))
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}

	input, _ := json.Marshal(fileEditInput{
		Path:   "file.txt",
		OldStr: "line2",
		NewStr: "LINE2",
	})

	result, err := editor.Execute(context.Background(), dir, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure after partial read")
	}
	if result.Error != "not_read_first" {
		t.Fatalf("expected not_read_first, got %q", result.Error)
	}
}

func TestFileEditRejectsStaleReadSnapshot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	os.WriteFile(path, []byte("hello world\n"), 0o644)

	store := newMemoryReadStateStore()
	reader := NewFileRead(store)
	editor := NewFileEdit(store)
	_, err := reader.Execute(context.Background(), dir, json.RawMessage(`{"path":"file.txt"}`))
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}
	if err := os.WriteFile(path, []byte("hello changed world\n"), 0o644); err != nil {
		t.Fatalf("failed to mutate file: %v", err)
	}

	input, _ := json.Marshal(fileEditInput{
		Path:   "file.txt",
		OldStr: "world",
		NewStr: "there",
	})

	result, err := editor.Execute(context.Background(), dir, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected stale snapshot failure")
	}
	if result.Error != "stale_write" {
		t.Fatalf("expected stale_write, got %q", result.Error)
	}
	if !strings.Contains(result.Content, "Re-run file_read") {
		t.Fatalf("expected recovery hint, got: %s", result.Content)
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
	if result.Error != "invalid_create_via_edit" {
		t.Fatalf("expected invalid_create_via_edit, got %q", result.Error)
	}
	if !strings.Contains(result.Content, "file_write") {
		t.Fatalf("expected guidance to use file_write, got: %s", result.Content)
	}
}

func TestFileEditPreservesPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "script.sh")
	os.WriteFile(path, []byte("#!/bin/bash\necho old\n"), 0o755)
	store := newMemoryReadStateStore()
	reader := NewFileRead(store)
	editor := NewFileEdit(store)
	_, err := reader.Execute(context.Background(), dir, json.RawMessage(`{"path":"script.sh"}`))
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}

	input, _ := json.Marshal(fileEditInput{
		Path:   "script.sh",
		OldStr: "echo old",
		NewStr: "echo new",
	})

	result, err := editor.Execute(context.Background(), dir, input)
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

func TestFileEditRequiresFreshReadAfterSuccessfulEdit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	os.WriteFile(path, []byte("alpha beta\n"), 0o644)

	store := newMemoryReadStateStore()
	reader := NewFileRead(store)
	editor := NewFileEdit(store)
	_, err := reader.Execute(context.Background(), dir, json.RawMessage(`{"path":"file.txt"}`))
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}

	firstInput, _ := json.Marshal(fileEditInput{
		Path:   "file.txt",
		OldStr: "beta",
		NewStr: "gamma",
	})
	firstResult, err := editor.Execute(context.Background(), dir, firstInput)
	if err != nil {
		t.Fatalf("unexpected first edit error: %v", err)
	}
	if !firstResult.Success {
		t.Fatalf("expected first edit success, got: %s", firstResult.Content)
	}

	secondInput, _ := json.Marshal(fileEditInput{
		Path:   "file.txt",
		OldStr: "gamma",
		NewStr: "delta",
	})
	secondResult, err := editor.Execute(context.Background(), dir, secondInput)
	if err != nil {
		t.Fatalf("unexpected second edit error: %v", err)
	}
	if secondResult.Success {
		t.Fatal("expected second edit to require a fresh read")
	}
	if secondResult.Error != "not_read_first" {
		t.Fatalf("expected not_read_first after successful edit, got %q", secondResult.Error)
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
