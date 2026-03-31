package tool

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePathValid(t *testing.T) {
	dir := t.TempDir()
	got, err := resolvePath(dir, "src/main.go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(dir, "src/main.go")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestResolvePathAbsolute(t *testing.T) {
	_, err := resolvePath("/tmp", "/etc/passwd")
	if err == nil {
		t.Fatal("expected error for absolute path")
	}
}

func TestResolvePathTraversal(t *testing.T) {
	_, err := resolvePath("/tmp/project", "../../../etc/passwd")
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
}

func TestResolvePathEmpty(t *testing.T) {
	_, err := resolvePath("/tmp", "")
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestListDirFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "c.go"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("x"), 0o644)
	os.MkdirAll(filepath.Join(dir, "subdir"), 0o755)

	files := listDirFiles(dir)
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d: %v", len(files), files)
	}
	// Should be sorted.
	if files[0] != "a.go" || files[1] != "c.go" {
		t.Fatalf("expected [a.go, c.go], got %v", files)
	}
}

func TestListDirFilesMissing(t *testing.T) {
	files := listDirFiles("/nonexistent/path")
	if files != nil {
		t.Fatalf("expected nil for missing dir, got %v", files)
	}
}

func TestIsBinaryContent(t *testing.T) {
	if isBinaryContent([]byte("hello world")) {
		t.Fatal("text detected as binary")
	}
	if !isBinaryContent([]byte("hello\x00world")) {
		t.Fatal("binary not detected")
	}
	if isBinaryContent([]byte("")) {
		t.Fatal("empty detected as binary")
	}
}

func TestFileNotFoundError(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "foo.go"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "bar.go"), []byte("x"), 0o644)

	msg := fileNotFoundError(dir, "missing.go")
	if msg == "" {
		t.Fatal("expected non-empty error message")
	}
	if !contains(msg, "File not found") {
		t.Fatalf("expected 'File not found' in: %s", msg)
	}
}

func TestRegisterFileTools(t *testing.T) {
	reg := NewRegistry()
	RegisterFileTools(reg)

	if _, ok := reg.Get("file_read"); !ok {
		t.Fatal("file_read not registered")
	}
	if _, ok := reg.Get("file_write"); !ok {
		t.Fatal("file_write not registered")
	}
	if _, ok := reg.Get("file_edit"); !ok {
		t.Fatal("file_edit not registered")
	}

	all := reg.All()
	if len(all) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(all))
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
