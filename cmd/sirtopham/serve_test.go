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

	appconfig "github.com/ponchione/sirtopham/internal/config"
	appdb "github.com/ponchione/sirtopham/internal/db"
	_ "github.com/mattn/go-sqlite3"
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
	if err := ensureProjectRecord(ctx, database, cfg); err != nil {
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
	if err := ensureProjectRecord(ctx, database, cfg); err != nil {
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
	backend, cleanup, err := buildBrainBackend(context.Background(), cfg, logger)
	if err != nil {
		t.Fatalf("buildBrainBackend error: %v", err)
	}
	defer cleanup()
	if backend == nil {
		t.Fatal("expected non-nil backend")
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

	provider, err := buildProvider("codex", appconfig.ProviderConfig{Type: "codex"})
	if err != nil {
		t.Fatalf("buildProvider(codex) error = %v, want nil", err)
	}
	if got := provider.Name(); got != "codex" {
		t.Fatalf("provider.Name() = %q, want codex", got)
	}
}

func TestBuildProviderPreservesConfiguredAliasForAnthropic(t *testing.T) {
	provider, err := buildProvider("primary_claude", appconfig.ProviderConfig{Type: "anthropic", APIKey: "test-key"})
	if err != nil {
		t.Fatalf("buildProvider(primary_claude) error = %v, want nil", err)
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

	provider, err := buildProvider("corp_codex", appconfig.ProviderConfig{Type: "codex"})
	if err != nil {
		t.Fatalf("buildProvider(corp_codex) error = %v, want nil", err)
	}
	if got := provider.Name(); got != "corp_codex" {
		t.Fatalf("provider.Name() = %q, want corp_codex", got)
	}
}
