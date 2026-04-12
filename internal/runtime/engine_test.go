package runtime

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/ponchione/sodoryard/internal/codeintel/embedder"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	appdb "github.com/ponchione/sodoryard/internal/db"
)

func TestBuildBrainRuntimeReturnsNilComponentsWhenDisabled(t *testing.T) {
	projectRoot := t.TempDir()
	cfg := appconfig.Default()
	cfg.ProjectRoot = projectRoot
	cfg.Brain.Enabled = false

	queries := appdb.New(&sql.DB{})
	brainBackend, brainSearcher, cleanup, err := buildBrainRuntime(context.Background(), cfg, embedder.New(cfg.Embedding), queries, slog.Default())
	if err != nil {
		t.Fatalf("buildBrainRuntime returned error: %v", err)
	}
	t.Cleanup(cleanup)

	if brainBackend != nil {
		t.Fatalf("BrainBackend = %#v, want nil", brainBackend)
	}
	if brainSearcher != nil {
		t.Fatalf("BrainSearcher = %#v, want nil", brainSearcher)
	}
	if _, err := os.Stat(cfg.BrainLanceDBPath()); !os.IsNotExist(err) {
		t.Fatalf("BrainLanceDBPath stat err = %v, want not-exist for %q", err, cfg.BrainLanceDBPath())
	}
}

func TestBuildConventionSourceReturnsNoopWhenBrainDisabled(t *testing.T) {
	projectRoot := t.TempDir()
	cfg := appconfig.Default()
	cfg.ProjectRoot = projectRoot
	cfg.Brain.Enabled = false
	cfg.Brain.VaultPath = ".brain"

	brainDir := cfg.BrainVaultPath()
	if err := os.MkdirAll(filepath.Join(brainDir, "conventions"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(brainDir, "conventions", "coding.md"), []byte("- use context-aware errors\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	text, err := BuildConventionSource(cfg).Load(context.Background())
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if text != "" {
		t.Fatalf("Load returned %q, want empty string when brain disabled", text)
	}
}
