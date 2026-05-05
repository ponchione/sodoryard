package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ponchione/sodoryard/internal/projectmemory"
)

func TestMemoryMigrateAndVerifyDocuments(t *testing.T) {
	ctx := context.Background()
	configPath, dataDir := writeMemoryTestProject(t)

	count, err := runMemoryMigrate(ctx, configPath, "", "")
	if err != nil {
		t.Fatalf("runMemoryMigrate returned error: %v", err)
	}
	if count != 2 {
		t.Fatalf("migrated count = %d, want 2", count)
	}
	result, err := runMemoryVerify(ctx, configPath, "", "")
	if err != nil {
		t.Fatalf("runMemoryVerify returned error: %v", err)
	}
	if result.Verified != 2 {
		t.Fatalf("verified count = %d, want 2", result.Verified)
	}
	if _, err := os.Stat(dataDir); err != nil {
		t.Fatalf("Shunter data dir stat returned error: %v", err)
	}
}

func TestMemoryVerifyDetectsDocumentMismatch(t *testing.T) {
	ctx := context.Background()
	configPath, dataDir := writeMemoryTestProject(t)
	if _, err := runMemoryMigrate(ctx, configPath, "", ""); err != nil {
		t.Fatalf("runMemoryMigrate returned error: %v", err)
	}
	backend, err := projectmemory.OpenBrainBackend(ctx, projectmemory.Config{DataDir: dataDir, DurableAck: true})
	if err != nil {
		t.Fatalf("OpenBrainBackend returned error: %v", err)
	}
	if err := backend.WriteDocument(ctx, "notes/a.md", "# A\n\nChanged."); err != nil {
		t.Fatalf("WriteDocument returned error: %v", err)
	}
	if err := backend.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	_, err = runMemoryVerify(ctx, configPath, "", "")
	if err == nil || !strings.Contains(err.Error(), "document content mismatch: notes/a.md") {
		t.Fatalf("runMemoryVerify error = %v, want notes/a.md content mismatch", err)
	}
}

func writeMemoryTestProject(t *testing.T) (string, string) {
	t.Helper()
	projectRoot := t.TempDir()
	brainDir := filepath.Join(projectRoot, ".brain")
	if err := os.MkdirAll(filepath.Join(brainDir, "notes"), 0o755); err != nil {
		t.Fatalf("mkdir notes: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(brainDir, "conventions"), 0o755); err != nil {
		t.Fatalf("mkdir conventions: %v", err)
	}
	if err := os.WriteFile(filepath.Join(brainDir, "notes", "a.md"), []byte("# A\n\nAlpha."), 0o644); err != nil {
		t.Fatalf("write notes/a.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(brainDir, "conventions", "coding.md"), []byte("# Coding\n\n- Test."), 0o644); err != nil {
		t.Fatalf("write conventions/coding.md: %v", err)
	}
	dataDir := filepath.Join(projectRoot, ".yard", "shunter", "project-memory")
	configPath := filepath.Join(projectRoot, "yard.yaml")
	configYAML := strings.Join([]string{
		"project_root: " + projectRoot,
		"memory:",
		"  backend: shunter",
		"  shunter_data_dir: .yard/shunter/project-memory",
		"  durable_ack: true",
		"brain:",
		"  enabled: true",
		"  vault_path: .brain",
		"local_services:",
		"  enabled: false",
	}, "\n") + "\n"
	if err := os.WriteFile(configPath, []byte(configYAML), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return configPath, dataDir
}
