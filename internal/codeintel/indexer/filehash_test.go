package indexer

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTestFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0644)
}

func TestFileHashes_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hashes.json")

	hashes := map[string]string{
		"main.go":          "abc123",
		"pkg/types.go":     "def456",
		"__schema_version": "1",
	}

	if err := saveFileHashes(path, hashes); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := loadFileHashes(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	for k, v := range hashes {
		if loaded[k] != v {
			t.Errorf("loaded[%q] = %q, want %q", k, loaded[k], v)
		}
	}
}

func TestFileHashes_MissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.json")
	hashes, err := loadFileHashes(path)
	if err != nil {
		t.Fatalf("load missing: %v", err)
	}
	if len(hashes) != 0 {
		t.Errorf("expected empty map, got %d entries", len(hashes))
	}
}

func TestFileHashes_CorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corrupt.json")
	writeTestFile(path, "not json {{{")

	hashes, err := loadFileHashes(path)
	if err != nil {
		t.Fatalf("load corrupt: %v", err)
	}
	if len(hashes) != 0 {
		t.Errorf("expected empty map for corrupt file, got %d entries", len(hashes))
	}
}
