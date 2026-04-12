package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListDirectoryBasic(t *testing.T) {
	dir := t.TempDir()

	// Create files and subdirs.
	os.MkdirAll(filepath.Join(dir, "subdir"), 0o755)
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello"), 0o644)
	os.WriteFile(filepath.Join(dir, "subdir", "nested.go"), []byte("package p"), 0o644)

	result, err := ListDirectory{}.Execute(context.Background(), dir, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "subdir/") {
		t.Fatalf("expected 'subdir/' in output, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "file.txt") {
		t.Fatalf("expected 'file.txt' in output, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "nested.go") {
		t.Fatalf("expected 'nested.go' in output, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "directories") && !strings.Contains(result.Content, "files") {
		t.Fatalf("expected footer with directory/file counts, got:\n%s", result.Content)
	}
}

func TestListDirectoryDepthLimit(t *testing.T) {
	dir := t.TempDir()

	// Create 4 levels deep.
	os.MkdirAll(filepath.Join(dir, "level1", "level2", "level3", "level4"), 0o755)
	os.WriteFile(filepath.Join(dir, "level1", "a.txt"), []byte("a"), 0o644)
	os.WriteFile(filepath.Join(dir, "level1", "level2", "b.txt"), []byte("b"), 0o644)
	os.WriteFile(filepath.Join(dir, "level1", "level2", "level3", "c.txt"), []byte("c"), 0o644)
	os.WriteFile(filepath.Join(dir, "level1", "level2", "level3", "level4", "d.txt"), []byte("d"), 0o644)

	result, err := ListDirectory{}.Execute(context.Background(), dir, json.RawMessage(`{"depth":2}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	// depth=2 means root (depth 0) + 2 levels inside → level1 and level2 contents visible
	if !strings.Contains(result.Content, "level1/") {
		t.Fatalf("expected level1/ in output, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "a.txt") {
		t.Fatalf("expected a.txt (depth 2) in output, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "b.txt") {
		t.Fatalf("expected b.txt (depth 2) in output, got:\n%s", result.Content)
	}
	// c.txt is at depth 3, should NOT appear with depth=2
	if strings.Contains(result.Content, "c.txt") {
		t.Fatalf("did NOT expect c.txt (depth 3) with depth=2, got:\n%s", result.Content)
	}
	if strings.Contains(result.Content, "d.txt") {
		t.Fatalf("did NOT expect d.txt (depth 4) with depth=2, got:\n%s", result.Content)
	}
}

func TestListDirectoryExcludesNodeModules(t *testing.T) {
	dir := t.TempDir()

	os.MkdirAll(filepath.Join(dir, "node_modules", "lodash"), 0o755)
	os.WriteFile(filepath.Join(dir, "node_modules", "lodash", "index.js"), []byte("// lodash"), 0o644)
	os.WriteFile(filepath.Join(dir, "app.js"), []byte("// app"), 0o644)

	result, err := ListDirectory{}.Execute(context.Background(), dir, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	if strings.Contains(result.Content, "node_modules") {
		t.Fatalf("did NOT expect node_modules in output, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "app.js") {
		t.Fatalf("expected app.js in output, got:\n%s", result.Content)
	}
}

func TestListDirectoryHiddenFiles(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, ".env"), []byte("SECRET=1"), 0o644)
	os.WriteFile(filepath.Join(dir, "visible.txt"), []byte("hello"), 0o644)

	// By default, .env should be hidden.
	result, err := ListDirectory{}.Execute(context.Background(), dir, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	if strings.Contains(result.Content, ".env") {
		t.Fatalf("did NOT expect .env in default output, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "visible.txt") {
		t.Fatalf("expected visible.txt in output, got:\n%s", result.Content)
	}

	// With include_hidden=true, .env should appear.
	result2, err := ListDirectory{}.Execute(context.Background(), dir, json.RawMessage(`{"include_hidden":true}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result2.Success {
		t.Fatalf("expected success with include_hidden, got: %s", result2.Content)
	}
	if !strings.Contains(result2.Content, ".env") {
		t.Fatalf("expected .env in include_hidden output, got:\n%s", result2.Content)
	}
}

func TestListDirectoryHiddenFilesAlwaysExcludesDefaultDirExcludes(t *testing.T) {
	dir := t.TempDir()

	// .git is always excluded even with include_hidden=true.
	os.MkdirAll(filepath.Join(dir, ".git", "objects"), 0o755)
	os.WriteFile(filepath.Join(dir, ".env"), []byte("SECRET=1"), 0o644)

	result, err := ListDirectory{}.Execute(context.Background(), dir, json.RawMessage(`{"include_hidden":true}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	if strings.Contains(result.Content, ".git") {
		t.Fatalf("did NOT expect .git even with include_hidden=true, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, ".env") {
		t.Fatalf("expected .env with include_hidden=true, got:\n%s", result.Content)
	}
}

func TestListDirectoryPathTraversal(t *testing.T) {
	dir := t.TempDir()

	result, err := ListDirectory{}.Execute(context.Background(), dir, json.RawMessage(`{"path":"../../etc"}`))
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

func TestListDirectoryNotFound(t *testing.T) {
	dir := t.TempDir()

	result, err := ListDirectory{}.Execute(context.Background(), dir, json.RawMessage(`{"path":"nonexistent_dir"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure for nonexistent path")
	}
	if !strings.Contains(strings.ToLower(result.Content), "not found") && !strings.Contains(strings.ToLower(result.Content), "no such file") {
		t.Fatalf("expected not-found error, got: %s", result.Content)
	}
}

func TestListDirectoryNotADirectory(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "afile.txt"), []byte("content"), 0o644)

	result, err := ListDirectory{}.Execute(context.Background(), dir, json.RawMessage(`{"path":"afile.txt"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure when path is a file, not a directory")
	}
	if !strings.Contains(result.Content, "Not a directory") {
		t.Fatalf("expected 'Not a directory' error, got: %s", result.Content)
	}
}

func TestListDirectorySchema(t *testing.T) {
	schema := ListDirectory{}.Schema()
	if !json.Valid(schema) {
		t.Fatal("Schema() is not valid JSON")
	}
	s := string(schema)
	if !strings.Contains(s, "list_directory") {
		t.Fatal("Schema() does not contain tool name")
	}
	if !strings.Contains(s, `"path"`) {
		t.Fatal("Schema() does not contain path property")
	}
	if !strings.Contains(s, `"depth"`) {
		t.Fatal("Schema() does not contain depth property")
	}
	if !strings.Contains(s, `"include_hidden"`) {
		t.Fatal("Schema() does not contain include_hidden property")
	}
}

func TestListDirectoryDirectoriesSortedFirst(t *testing.T) {
	dir := t.TempDir()

	// Create items whose names alphabetically interleave dirs and files.
	os.MkdirAll(filepath.Join(dir, "beta"), 0o755)
	os.WriteFile(filepath.Join(dir, "alpha.txt"), []byte("a"), 0o644)
	os.WriteFile(filepath.Join(dir, "gamma.txt"), []byte("g"), 0o644)
	os.MkdirAll(filepath.Join(dir, "delta"), 0o755)

	result, err := ListDirectory{}.Execute(context.Background(), dir, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Content)
	}

	betaIdx := strings.Index(result.Content, "beta/")
	deltaIdx := strings.Index(result.Content, "delta/")
	alphaIdx := strings.Index(result.Content, "alpha.txt")
	gammaIdx := strings.Index(result.Content, "gamma.txt")

	if betaIdx == -1 || deltaIdx == -1 || alphaIdx == -1 || gammaIdx == -1 {
		t.Fatalf("missing expected entries in output:\n%s", result.Content)
	}
	// Dirs must appear before files.
	if betaIdx > alphaIdx || deltaIdx > alphaIdx {
		t.Fatalf("expected directories before files in output:\n%s", result.Content)
	}
	// beta < delta alphabetically.
	if betaIdx > deltaIdx {
		t.Fatalf("expected beta/ before delta/ alphabetically:\n%s", result.Content)
	}
}

func TestListDirectorySubpath(t *testing.T) {
	dir := t.TempDir()

	os.MkdirAll(filepath.Join(dir, "src", "pkg"), 0o755)
	os.WriteFile(filepath.Join(dir, "src", "main.go"), []byte("package main"), 0o644)
	os.WriteFile(filepath.Join(dir, "src", "pkg", "util.go"), []byte("package pkg"), 0o644)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# readme"), 0o644)

	result, err := ListDirectory{}.Execute(context.Background(), dir, json.RawMessage(`{"path":"src"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "main.go") {
		t.Fatalf("expected main.go in output, got:\n%s", result.Content)
	}
	// README.md is outside src — should not appear.
	if strings.Contains(result.Content, "README.md") {
		t.Fatalf("did NOT expect README.md in src-scoped output, got:\n%s", result.Content)
	}
}
