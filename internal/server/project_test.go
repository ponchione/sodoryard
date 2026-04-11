package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	brainindexstate "github.com/ponchione/sodoryard/internal/brain/indexstate"
	"github.com/ponchione/sodoryard/internal/config"
	appdb "github.com/ponchione/sodoryard/internal/db"
	"github.com/ponchione/sodoryard/internal/server"
)

func TestProjectEndpoint(t *testing.T) {
	// Create a temp project directory with some Go files so detection returns "go".
	dir := t.TempDir()
	for _, name := range []string{"main.go", "util.go", "helper.go"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("package main\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	cfg := &config.Config{ProjectRoot: dir}
	srv := server.New(server.Config{Host: "127.0.0.1", Port: 0}, newTestLogger())
	server.NewProjectHandler(srv, cfg, newTestLogger())

	_, base := startServer(t, srv)

	// First request — triggers language detection.
	resp, err := http.Get(base + "/api/project")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body struct {
		ID       string `json:"id"`
		RootPath string `json:"root_path"`
		Language string `json:"language"`
		Name     string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if body.ID != dir {
		t.Fatalf("expected id=%q, got %q", dir, body.ID)
	}
	if body.Language != "go" {
		t.Fatalf("expected language=go, got %q", body.Language)
	}
	if body.RootPath != dir {
		t.Fatalf("expected root_path=%q, got %q", dir, body.RootPath)
	}
	if body.Name == "" {
		t.Fatal("expected non-empty name")
	}
}

func TestProjectEndpointCachesLanguage(t *testing.T) {
	// Verify the second request returns the same result (cached), even after
	// we remove the files that made it detect "go".
	dir := t.TempDir()
	goFile := filepath.Join(dir, "main.go")
	if err := os.WriteFile(goFile, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{ProjectRoot: dir}
	srv := server.New(server.Config{Host: "127.0.0.1", Port: 0}, newTestLogger())
	server.NewProjectHandler(srv, cfg, newTestLogger())

	_, base := startServer(t, srv)

	// First request — caches "go".
	resp1, err := http.Get(base + "/api/project")
	if err != nil {
		t.Fatalf("request 1 failed: %v", err)
	}
	var body1 struct{ Language string }
	json.NewDecoder(resp1.Body).Decode(&body1)
	resp1.Body.Close()

	if body1.Language != "go" {
		t.Fatalf("request 1: expected language=go, got %q", body1.Language)
	}

	// Remove all Go files so a fresh walk would return "".
	os.Remove(goFile)

	// Second request — should still return "go" from cache.
	resp2, err := http.Get(base + "/api/project")
	if err != nil {
		t.Fatalf("request 2 failed: %v", err)
	}
	var body2 struct{ Language string }
	json.NewDecoder(resp2.Body).Decode(&body2)
	resp2.Body.Close()

	if body2.Language != "go" {
		t.Fatalf("request 2: expected cached language=go, got %q (cache not working)", body2.Language)
	}
}

func TestProjectEndpointIncludesIndexMetadata(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.ProjectRoot = dir
	cfg.Brain.Enabled = false

	database, err := appdb.OpenDB(context.Background(), cfg.DatabasePath())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()
	if err := appdb.Init(context.Background(), database); err != nil {
		t.Fatalf("init db: %v", err)
	}
	_, err = database.ExecContext(context.Background(), `
INSERT INTO projects(id, name, root_path, language, last_indexed_commit, last_indexed_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`, cfg.ProjectRoot, filepath.Base(dir), dir, "go", "abc123def", "2026-04-06T12:34:56Z", "2026-04-06T12:00:00Z", "2026-04-06T12:34:56Z")
	if err != nil {
		t.Fatalf("insert project row: %v", err)
	}

	srv := server.New(server.Config{Host: "127.0.0.1", Port: 0}, newTestLogger())
	server.NewProjectHandler(srv, cfg, newTestLogger())
	_, base := startServer(t, srv)

	resp, err := http.Get(base + "/api/project")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var body struct {
		LastIndexedAt     string `json:"last_indexed_at"`
		LastIndexedCommit string `json:"last_indexed_commit"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.LastIndexedAt != "2026-04-06T12:34:56Z" {
		t.Fatalf("last_indexed_at = %q, want 2026-04-06T12:34:56Z", body.LastIndexedAt)
	}
	if body.LastIndexedCommit != "abc123def" {
		t.Fatalf("last_indexed_commit = %q, want abc123def", body.LastIndexedCommit)
	}
}

func TestProjectEndpointIncludesBrainIndexState(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.ProjectRoot = dir
	cfg.Brain.Enabled = true
	staleAt := time.Date(2026, 4, 9, 16, 5, 0, 0, time.UTC)
	if err := brainindexstate.MarkStale(dir, "brain_update", staleAt); err != nil {
		t.Fatalf("MarkStale: %v", err)
	}

	srv := server.New(server.Config{Host: "127.0.0.1", Port: 0}, newTestLogger())
	server.NewProjectHandler(srv, cfg, newTestLogger())
	_, base := startServer(t, srv)

	resp, err := http.Get(base + "/api/project")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var body struct {
		BrainIndex *struct {
			Status        string `json:"status"`
			LastIndexedAt string `json:"last_indexed_at"`
			StaleSince    string `json:"stale_since"`
			StaleReason   string `json:"stale_reason"`
		} `json:"brain_index"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.BrainIndex == nil {
		t.Fatal("expected brain_index payload")
	}
	if body.BrainIndex.Status != brainindexstate.StatusStale {
		t.Fatalf("brain_index.status = %q, want %q", body.BrainIndex.Status, brainindexstate.StatusStale)
	}
	if body.BrainIndex.StaleSince != staleAt.Format(time.RFC3339) {
		t.Fatalf("brain_index.stale_since = %q, want %q", body.BrainIndex.StaleSince, staleAt.Format(time.RFC3339))
	}
	if body.BrainIndex.StaleReason != "brain_update" {
		t.Fatalf("brain_index.stale_reason = %q, want brain_update", body.BrainIndex.StaleReason)
	}
}

func TestProjectTreeHonorsYamlExcludePatterns(t *testing.T) {
	dir := t.TempDir()
	mustWriteTreeFile(t, dir, "src/main.go", "package main\n")
	mustWriteTreeFile(t, dir, "web/node_modules/react/index.js", "export const x = 1\n")
	mustWriteTreeFile(t, dir, ".brain/notes/hello.md", "hello\n")
	mustWriteTreeFile(t, dir, ".sirtopham/lancedb/code/test.txt", "ignored\n")

	cfg := config.Default()
	cfg.ProjectRoot = dir
	cfg.Index.Exclude = []string{"**/.git/**", "**/.sirtopham/**", "**/.brain/**", "**/node_modules/**"}
	cfg.Brain.Enabled = false

	srv := server.New(server.Config{Host: "127.0.0.1", Port: 0}, newTestLogger())
	server.NewProjectHandler(srv, cfg, newTestLogger())
	_, base := startServer(t, srv)

	resp, err := http.Get(base + "/api/project/tree")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body struct {
		Name     string `json:"name"`
		Type     string `json:"type"`
		Children []struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"children"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	raw, _ := json.Marshal(body)
	got := string(raw)
	if strings.Contains(got, "node_modules") || strings.Contains(got, ".brain") || strings.Contains(got, ".sirtopham") {
		t.Fatalf("excluded directories leaked into tree: %s", got)
	}
	foundSrc := false
	for _, child := range body.Children {
		if child.Name == "src" {
			foundSrc = true
			break
		}
	}
	if !foundSrc {
		t.Fatalf("expected normal project content in tree, got: %s", got)
	}
}

func mustWriteTreeFile(t *testing.T, root, relPath, content string) {
	t.Helper()
	path := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
