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

	details := decodeToolResultDetails(t, result.Details)
	if details["kind"] != "file_mutation" || details["operation"] != "write" {
		t.Fatalf("details kind/operation = %#v/%#v", details["kind"], details["operation"])
	}
	if details["path"] != "new.txt" {
		t.Fatalf("path = %#v, want new.txt", details["path"])
	}
	if details["created"] != true || details["changed"] != true {
		t.Fatalf("created/changed = %#v/%#v, want true/true", details["created"], details["changed"])
	}
	if got := detailInt(t, details, "bytes_before"); got != 0 {
		t.Fatalf("bytes_before = %d, want 0", got)
	}
	if got := detailInt(t, details, "bytes_after"); got != len("hello world\n") {
		t.Fatalf("bytes_after = %d, want %d", got, len("hello world\n"))
	}
	if got := detailInt(t, details, "diff_line_count"); got != 0 {
		t.Fatalf("diff_line_count = %d, want 0", got)
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

	store := newMemoryReadStateStore()
	reader := NewFileRead(store)
	writer := NewFileWrite(store)
	_, err := reader.Execute(context.Background(), dir, json.RawMessage(`{"path":"existing.txt"}`))
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}

	newContent := "line one\nline TWO modified\nline three\n"
	input, _ := json.Marshal(fileWriteInput{Path: "existing.txt", Content: newContent})

	result, err := writer.Execute(context.Background(), dir, input)
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

	details := decodeToolResultDetails(t, result.Details)
	if details["kind"] != "file_mutation" || details["operation"] != "write" {
		t.Fatalf("details kind/operation = %#v/%#v", details["kind"], details["operation"])
	}
	if details["path"] != "existing.txt" {
		t.Fatalf("path = %#v, want existing.txt", details["path"])
	}
	if details["created"] != false || details["changed"] != true {
		t.Fatalf("created/changed = %#v/%#v, want false/true", details["created"], details["changed"])
	}
	if got := detailInt(t, details, "bytes_before"); got != len(oldContent) {
		t.Fatalf("bytes_before = %d, want %d", got, len(oldContent))
	}
	if got := detailInt(t, details, "bytes_after"); got != len(newContent) {
		t.Fatalf("bytes_after = %d, want %d", got, len(newContent))
	}
	if got := detailInt(t, details, "diff_line_count"); got <= 0 {
		t.Fatalf("diff_line_count = %d, want > 0", got)
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

	store := newMemoryReadStateStore()
	reader := NewFileRead(store)
	writer := NewFileWrite(store)
	_, err := reader.Execute(context.Background(), dir, json.RawMessage(`{"path":"big.txt"}`))
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}

	input, _ := json.Marshal(fileWriteInput{Path: "big.txt", Content: new_.String()})
	result, err := writer.Execute(context.Background(), dir, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "diff truncated") {
		t.Fatalf("expected truncation notice, got:\n%s", result.Content)
	}
	details := decodeToolResultDetails(t, result.Details)
	if details["diff_truncated"] != true {
		t.Fatalf("diff_truncated = %#v, want true", details["diff_truncated"])
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

func TestFileWriteOverwriteRequiresFullReadFirst(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.txt")
	if err := os.WriteFile(path, []byte("hello world\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	store := newMemoryReadStateStore()
	writer := NewFileWrite(store)
	input, _ := json.Marshal(fileWriteInput{Path: "existing.txt", Content: "updated\n"})

	result, err := writer.Execute(context.Background(), dir, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure without prior full read")
	}
	if result.Error != "not_read_first" {
		t.Fatalf("expected not_read_first, got %q", result.Error)
	}
	if !strings.Contains(result.Content, "prior full file_read") {
		t.Fatalf("expected full read guidance, got: %s", result.Content)
	}
}

func TestFileWriteRejectsPartialReadAsPrecondition(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.txt")
	if err := os.WriteFile(path, []byte("line1\nline2\nline3\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	store := newMemoryReadStateStore()
	reader := NewFileRead(store)
	writer := NewFileWrite(store)
	_, err := reader.Execute(context.Background(), dir, json.RawMessage(`{"path":"existing.txt","line_start":1,"line_end":2}`))
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}

	input, _ := json.Marshal(fileWriteInput{Path: "existing.txt", Content: "updated\n"})
	result, err := writer.Execute(context.Background(), dir, input)
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

func TestFileWriteRejectsStaleReadSnapshot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.txt")
	if err := os.WriteFile(path, []byte("hello world\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	store := newMemoryReadStateStore()
	reader := NewFileRead(store)
	writer := NewFileWrite(store)
	_, err := reader.Execute(context.Background(), dir, json.RawMessage(`{"path":"existing.txt"}`))
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}
	if err := os.WriteFile(path, []byte("hello changed world\n"), 0o644); err != nil {
		t.Fatalf("mutate file: %v", err)
	}

	input, _ := json.Marshal(fileWriteInput{Path: "existing.txt", Content: "updated\n"})
	result, err := writer.Execute(context.Background(), dir, input)
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
		t.Fatalf("expected recovery guidance, got: %s", result.Content)
	}
}

func TestFileWriteAllowsOverwriteOfExistingEmptyFileWithoutRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("write empty file: %v", err)
	}

	input, _ := json.Marshal(fileWriteInput{Path: "empty.txt", Content: "now populated\n"})
	result, err := FileWrite{}.Execute(context.Background(), dir, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(data) != "now populated\n" {
		t.Fatalf("file content = %q", string(data))
	}
}

func TestFileWriteRequiresFreshReadAfterSuccessfulOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.txt")
	if err := os.WriteFile(path, []byte("alpha\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	store := newMemoryReadStateStore()
	reader := NewFileRead(store)
	writer := NewFileWrite(store)
	_, err := reader.Execute(context.Background(), dir, json.RawMessage(`{"path":"existing.txt"}`))
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}

	firstInput, _ := json.Marshal(fileWriteInput{Path: "existing.txt", Content: "beta\n"})
	firstResult, err := writer.Execute(context.Background(), dir, firstInput)
	if err != nil {
		t.Fatalf("unexpected first write error: %v", err)
	}
	if !firstResult.Success {
		t.Fatalf("expected first write success, got: %s", firstResult.Content)
	}

	secondInput, _ := json.Marshal(fileWriteInput{Path: "existing.txt", Content: "gamma\n"})
	secondResult, err := writer.Execute(context.Background(), dir, secondInput)
	if err != nil {
		t.Fatalf("unexpected second write error: %v", err)
	}
	if secondResult.Success {
		t.Fatal("expected second write to require a fresh read")
	}
	if secondResult.Error != "not_read_first" {
		t.Fatalf("expected not_read_first after successful overwrite, got %q", secondResult.Error)
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
