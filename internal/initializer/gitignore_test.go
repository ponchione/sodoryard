package initializer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureGitignoreEntriesCreatesFileWhenMissing(t *testing.T) {
	projectRoot := t.TempDir()
	added, err := EnsureGitignoreEntries(projectRoot)
	if err != nil {
		t.Fatalf("EnsureGitignoreEntries: %v", err)
	}
	if len(added) != 2 {
		t.Errorf("expected 2 entries added, got %d: %v", len(added), added)
	}

	data, err := os.ReadFile(filepath.Join(projectRoot, ".gitignore"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	for _, want := range []string{".yard/", ".brain/"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("expected .gitignore to contain %q, got:\n%s", want, data)
		}
	}
}

func TestEnsureGitignoreEntriesAppendsToExistingFile(t *testing.T) {
	projectRoot := t.TempDir()
	existing := "node_modules/\ndist/\n"
	if err := os.WriteFile(filepath.Join(projectRoot, ".gitignore"), []byte(existing), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	added, err := EnsureGitignoreEntries(projectRoot)
	if err != nil {
		t.Fatalf("EnsureGitignoreEntries: %v", err)
	}
	if len(added) != 2 {
		t.Errorf("expected 2 entries added, got %d", len(added))
	}

	data, err := os.ReadFile(filepath.Join(projectRoot, ".gitignore"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "node_modules/") || !strings.Contains(got, "dist/") {
		t.Errorf("expected existing entries preserved, got:\n%s", got)
	}
	if !strings.Contains(got, ".yard/") || !strings.Contains(got, ".brain/") {
		t.Errorf("expected new entries appended, got:\n%s", got)
	}
}

func TestEnsureGitignoreEntriesIsIdempotent(t *testing.T) {
	projectRoot := t.TempDir()
	if _, err := EnsureGitignoreEntries(projectRoot); err != nil {
		t.Fatalf("first call: %v", err)
	}
	added, err := EnsureGitignoreEntries(projectRoot)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if len(added) != 0 {
		t.Errorf("second call should add nothing, got %d: %v", len(added), added)
	}
}
