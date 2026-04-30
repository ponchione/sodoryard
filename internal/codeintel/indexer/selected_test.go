package indexer

import (
	"context"
	"path/filepath"
	"testing"
)

func TestIndexFilesUsesKnownFileHash(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "main.go"), "package main\n\nfunc main() {}\n")

	result, err := IndexFiles(
		context.Background(),
		IndexConfig{
			ProjectName:     "test",
			ProjectRoot:     dir,
			KnownFileHashes: map[string]string{"main.go": "known-hash"},
		},
		&mockParser{},
		&mockStore{},
		&mockEmbedder{},
		&mockDescriber{},
		[]string{"main.go"},
	)
	if err != nil {
		t.Fatalf("IndexFiles returned error: %v", err)
	}
	if len(result.Files) != 1 {
		t.Fatalf("indexed files = %d, want 1", len(result.Files))
	}
	if result.Files[0].FileHash != "known-hash" {
		t.Fatalf("FileHash = %q, want known-hash", result.Files[0].FileHash)
	}
}
