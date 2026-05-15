package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindFilesBasic(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# readme\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "lib.go"), []byte("package lib\n"), 0o644)

	result, err := FindFiles{}.Execute(context.Background(), dir,
		json.RawMessage(`{"pattern":"*.go"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "main.go") {
		t.Fatalf("expected main.go in results, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "lib.go") {
		t.Fatalf("expected lib.go in results, got:\n%s", result.Content)
	}
	if strings.Contains(result.Content, "README.md") {
		t.Fatalf("did NOT expect README.md in results, got:\n%s", result.Content)
	}
}

func TestFindFilesRecursiveGlob(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "a", "b"), 0o755)
	os.WriteFile(filepath.Join(dir, "top_test.go"), []byte("package top\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "a", "a_test.go"), []byte("package a\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "a", "b", "b_test.go"), []byte("package b\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "a", "b", "impl.go"), []byte("package b\n"), 0o644)

	result, err := FindFiles{}.Execute(context.Background(), dir,
		json.RawMessage(`{"pattern":"**/*_test.go"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "top_test.go") {
		t.Fatalf("expected top_test.go in results, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "a_test.go") {
		t.Fatalf("expected a_test.go in results, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "b_test.go") {
		t.Fatalf("expected b_test.go in results, got:\n%s", result.Content)
	}
	if strings.Contains(result.Content, "impl.go") {
		t.Fatalf("did NOT expect impl.go in results, got:\n%s", result.Content)
	}
}

func TestFindFilesScopedPath(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "cmd", "foo"), 0o755)
	os.MkdirAll(filepath.Join(dir, "pkg"), 0o755)
	os.WriteFile(filepath.Join(dir, "cmd", "main.go"), []byte("package main\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "cmd", "foo", "foo.go"), []byte("package foo\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "pkg", "lib.go"), []byte("package lib\n"), 0o644)

	result, err := FindFiles{}.Execute(context.Background(), dir,
		json.RawMessage(`{"pattern":"*.go","path":"cmd"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "cmd/main.go") {
		t.Fatalf("expected cmd/main.go (with prefix) in results, got:\n%s", result.Content)
	}
	if strings.Contains(result.Content, "lib.go") {
		t.Fatalf("did NOT expect lib.go (outside scope) in results, got:\n%s", result.Content)
	}
}

func TestFindFilesNoResults(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644)

	result, err := FindFiles{}.Execute(context.Background(), dir,
		json.RawMessage(`{"pattern":"*.rs"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success=true for no matches, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "No files found matching pattern") {
		t.Fatalf("expected no-match message, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "*.rs") {
		t.Fatalf("expected pattern in no-match message, got: %s", result.Content)
	}
}

func TestFindFilesMaxResults(t *testing.T) {
	dir := t.TempDir()
	// Create 10 .go files
	for i := 0; i < 10; i++ {
		name := filepath.Join(dir, strings.Repeat("a", i+1)+".go")
		os.WriteFile(name, []byte("package x\n"), 0o644)
	}

	result, err := FindFiles{}.Execute(context.Background(), dir,
		json.RawMessage(`{"pattern":"*.go","max_results":3}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	count := strings.Count(result.Content, ".go")
	// Footer has "(3 files)" which doesn't contain ".go", so count == 3
	if count != 3 {
		t.Fatalf("expected exactly 3 .go entries (max_results=3), got %d\n%s", count, result.Content)
	}
	if !strings.Contains(result.Content, "(3 files)") {
		t.Fatalf("expected footer '(3 files)', got:\n%s", result.Content)
	}
}

func TestFindFilesCapsMaxResults(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 3; i++ {
		name := filepath.Join(dir, strings.Repeat("a", i+1)+".go")
		os.WriteFile(name, []byte("package x\n"), 0o644)
	}

	result, err := FindFiles{}.Execute(context.Background(), dir,
		json.RawMessage(`{"pattern":"*.go","max_results":5000}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	if count := strings.Count(result.Content, ".go"); count != 3 {
		t.Fatalf("expected all 3 files below cap, got %d\n%s", count, result.Content)
	}
}

func TestFindFilesExcludesNodeModules(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "node_modules", "lodash"), 0o755)
	os.WriteFile(filepath.Join(dir, "node_modules", "lodash", "index.js"), []byte("// lodash\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "app.js"), []byte("// app\n"), 0o644)

	result, err := FindFiles{}.Execute(context.Background(), dir,
		json.RawMessage(`{"pattern":"*.js"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	if strings.Contains(result.Content, "node_modules") {
		t.Fatalf("did NOT expect node_modules in results, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "app.js") {
		t.Fatalf("expected app.js in results, got:\n%s", result.Content)
	}
}

func TestFindFilesPathTraversal(t *testing.T) {
	dir := t.TempDir()

	result, err := FindFiles{}.Execute(context.Background(), dir,
		json.RawMessage(`{"pattern":"*.go","path":"../../etc"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure for path traversal attempt")
	}
	if !strings.Contains(result.Content, "escapes project root") {
		t.Fatalf("expected 'escapes project root' error, got: %s", result.Content)
	}
}

func TestFindFilesEmptyPattern(t *testing.T) {
	dir := t.TempDir()

	result, err := FindFiles{}.Execute(context.Background(), dir,
		json.RawMessage(`{"pattern":""}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure for empty pattern")
	}
}

func TestFindFilesSchema(t *testing.T) {
	schema := FindFiles{}.Schema()
	if !json.Valid(schema) {
		t.Fatal("Schema() is not valid JSON")
	}
	if !strings.Contains(string(schema), "find_files") {
		t.Fatal("Schema() does not contain tool name")
	}
	if !strings.Contains(string(schema), `"pattern"`) {
		t.Fatal("Schema() does not contain pattern property")
	}
}
