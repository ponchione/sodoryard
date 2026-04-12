package main

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	appdb "github.com/ponchione/sodoryard/internal/db"
	rtpkg "github.com/ponchione/sodoryard/internal/runtime"
)

func TestEnsureProjectRecordCreatesProjectRow(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "serve-test.db")
	database, err := sql.Open(appdb.DriverName, "file:"+dbPath+"?_foreign_keys=on")
	if err != nil {
		t.Fatalf("OpenDB error: %v", err)
	}
	defer database.Close()
	if _, err := database.ExecContext(ctx, `CREATE TABLE projects (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		root_path TEXT NOT NULL UNIQUE,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("create projects table: %v", err)
	}

	cfg := &appconfig.Config{ProjectRoot: "/tmp/sirtopham-project"}
	if err := rtpkg.EnsureProjectRecord(ctx, database, cfg); err != nil {
		t.Fatalf("ensureProjectRecord error: %v", err)
	}

	var id, name, root string
	var createdAt, updatedAt string
	if err := database.QueryRowContext(ctx, `SELECT id, name, root_path, created_at, updated_at FROM projects WHERE id = ?`, cfg.ProjectRoot).Scan(&id, &name, &root, &createdAt, &updatedAt); err != nil {
		t.Fatalf("QueryRow error: %v", err)
	}
	if id != cfg.ProjectRoot {
		t.Fatalf("id = %q, want %q", id, cfg.ProjectRoot)
	}
	if name != "sirtopham-project" {
		t.Fatalf("name = %q, want sirtopham-project", name)
	}
	if root != cfg.ProjectRoot {
		t.Fatalf("root_path = %q, want %q", root, cfg.ProjectRoot)
	}
	if createdAt == "" || updatedAt == "" {
		t.Fatalf("expected timestamps to be populated, got created_at=%q updated_at=%q", createdAt, updatedAt)
	}
}

func TestEnsureProjectRecordUpdatesExistingProjectRow(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "serve-test.db")
	database, err := sql.Open(appdb.DriverName, "file:"+dbPath+"?_foreign_keys=on")
	if err != nil {
		t.Fatalf("OpenDB error: %v", err)
	}
	defer database.Close()
	if _, err := database.ExecContext(ctx, `CREATE TABLE projects (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		root_path TEXT NOT NULL UNIQUE,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("create projects table: %v", err)
	}

	createdAt := time.Unix(1700000000, 0).UTC().Format(time.RFC3339)
	if _, err := database.ExecContext(ctx, `INSERT INTO projects(id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, "/tmp/sirtopham-project", "old-name", "/tmp/sirtopham-project", createdAt, createdAt); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	cfg := &appconfig.Config{ProjectRoot: "/tmp/sirtopham-project"}
	if err := rtpkg.EnsureProjectRecord(ctx, database, cfg); err != nil {
		t.Fatalf("ensureProjectRecord error: %v", err)
	}

	var name, root, updatedAt string
	if err := database.QueryRowContext(ctx, `SELECT name, root_path, updated_at FROM projects WHERE id = ?`, cfg.ProjectRoot).Scan(&name, &root, &updatedAt); err != nil {
		t.Fatalf("QueryRow error: %v", err)
	}
	if name != "sirtopham-project" {
		t.Fatalf("name = %q, want sirtopham-project", name)
	}
	if root != cfg.ProjectRoot {
		t.Fatalf("root_path = %q, want %q", root, cfg.ProjectRoot)
	}
	if updatedAt == createdAt {
		t.Fatalf("expected updated_at to change, still %q", updatedAt)
	}
}

func TestBuildBrainBackendUsesMCPClient(t *testing.T) {
	cfg := appconfig.BrainConfig{Enabled: true, VaultPath: t.TempDir()}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	backend, cleanup, err := rtpkg.BuildBrainBackend(context.Background(), cfg, logger)
	if err != nil {
		t.Fatalf("BuildBrainBackend error: %v", err)
	}
	defer cleanup()
	if backend == nil {
		t.Fatal("expected non-nil backend")
	}
}

func TestBuildGraphStoreUsesProjectStatePath(t *testing.T) {
	projectRoot := t.TempDir()
	cfg := &appconfig.Config{ProjectRoot: projectRoot}
	store, cleanup, err := rtpkg.BuildGraphStore(cfg)
	if err != nil {
		t.Fatalf("BuildGraphStore error: %v", err)
	}
	defer cleanup()
	if store == nil {
		t.Fatal("expected non-nil graph store")
	}
	if _, err := os.Stat(cfg.GraphDBPath()); err != nil {
		t.Fatalf("expected graph db at %s: %v", cfg.GraphDBPath(), err)
	}
}

func TestBuildConventionSourceUsesBrainVaultPath(t *testing.T) {
	projectRoot := t.TempDir()
	vaultPath := filepath.Join(projectRoot, ".brain")
	if err := os.MkdirAll(filepath.Join(vaultPath, "conventions"), 0o755); err != nil {
		t.Fatalf("MkdirAll(conventions): %v", err)
	}
	if err := os.WriteFile(filepath.Join(vaultPath, "conventions", "testing.md"), []byte("- Prefer table-driven tests\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(convention): %v", err)
	}
	cfg := &appconfig.Config{ProjectRoot: projectRoot, Brain: appconfig.BrainConfig{VaultPath: ".brain"}}
	source := rtpkg.BuildConventionSource(cfg)
	text, err := source.Load(context.Background())
	if err != nil {
		t.Fatalf("Load conventions: %v", err)
	}
	if text != "Prefer table-driven tests" {
		t.Fatalf("convention text = %q, want extracted bullet", text)
	}
}

func TestBuildProviderSupportsCodex(t *testing.T) {
	binDir := t.TempDir()
	codexPath := filepath.Join(binDir, "codex")
	script := "#!/bin/sh\nexit 0\n"
	if runtime.GOOS == "windows" {
		codexPath += ".bat"
		script = "@echo off\r\nexit /b 0\r\n"
	}
	if err := os.WriteFile(codexPath, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile(codex stub): %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	provider, err := rtpkg.BuildProvider("codex", appconfig.ProviderConfig{Type: "codex"})
	if err != nil {
		t.Fatalf("BuildProvider(codex) error = %v, want nil", err)
	}
	if got := provider.Name(); got != "codex" {
		t.Fatalf("provider.Name() = %q, want codex", got)
	}
}

func TestBuildProviderPreservesConfiguredAliasForAnthropic(t *testing.T) {
	provider, err := rtpkg.BuildProvider("primary_claude", appconfig.ProviderConfig{Type: "anthropic", APIKey: "test-key"})
	if err != nil {
		t.Fatalf("BuildProvider(primary_claude) error = %v, want nil", err)
	}
	if got := provider.Name(); got != "primary_claude" {
		t.Fatalf("provider.Name() = %q, want primary_claude", got)
	}
}

func TestBuildProviderPreservesConfiguredAliasForCodex(t *testing.T) {
	binDir := t.TempDir()
	codexPath := filepath.Join(binDir, "codex")
	script := "#!/bin/sh\nexit 0\n"
	if runtime.GOOS == "windows" {
		codexPath += ".bat"
		script = "@echo off\r\nexit /b 0\r\n"
	}
	if err := os.WriteFile(codexPath, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile(codex stub): %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	provider, err := rtpkg.BuildProvider("corp_codex", appconfig.ProviderConfig{Type: "codex"})
	if err != nil {
		t.Fatalf("BuildProvider(corp_codex) error = %v, want nil", err)
	}
	if got := provider.Name(); got != "corp_codex" {
		t.Fatalf("provider.Name() = %q, want corp_codex", got)
	}
}
