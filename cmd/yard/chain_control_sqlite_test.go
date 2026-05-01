//go:build sqlite_fts5
// +build sqlite_fts5

package main

import (
	"bytes"
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ponchione/sodoryard/internal/chain"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	appdb "github.com/ponchione/sodoryard/internal/db"
	rtpkg "github.com/ponchione/sodoryard/internal/runtime"
)

func TestYardChainPauseCommandPrintsRequestedMessage(t *testing.T) {
	ctx := context.Background()
	pingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer pingServer.Close()

	configPath, cfg := writeYardChainControlConfig(t, pingServer.URL)
	rt, err := rtpkg.BuildOrchestratorRuntime(ctx, cfg)
	if err != nil {
		t.Fatalf("BuildOrchestratorRuntime returned error: %v", err)
	}
	defer rt.Cleanup()

	chainID, err := rt.ChainStore.StartChain(ctx, chain.ChainSpec{ChainID: "pause-requested", MaxSteps: 5, MaxResolverLoops: 1, MaxDuration: time.Hour, TokenBudget: 100})
	if err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	if err := rt.ChainStore.SetChainStatus(ctx, chainID, "running"); err != nil {
		t.Fatalf("SetChainStatus returned error: %v", err)
	}

	var stdout bytes.Buffer
	cmd := newYardChainPauseCmd(&configPath)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{chainID})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if got := stdout.String(); got != "chain pause-requested pause requested\n" {
		t.Fatalf("stdout = %q, want pause-requested message", got)
	}

	reloaded, err := rt.ChainStore.GetChain(ctx, chainID)
	if err != nil {
		t.Fatalf("GetChain returned error: %v", err)
	}
	if reloaded.Status != "pause_requested" {
		t.Fatalf("status = %q, want pause_requested", reloaded.Status)
	}
	events, err := rt.ChainStore.ListEvents(ctx, chainID)
	if err != nil {
		t.Fatalf("ListEvents returned error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}
	if events[0].EventType != chain.EventChainPaused {
		t.Fatalf("event type = %s, want %s", events[0].EventType, chain.EventChainPaused)
	}
	if !strings.Contains(events[0].EventData, `"status":"pause_requested"`) {
		t.Fatalf("EventData = %s, want pause_requested status", events[0].EventData)
	}
}

func TestYardChainCancelCommandPrintsRequestedMessage(t *testing.T) {
	ctx := context.Background()
	pingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer pingServer.Close()

	configPath, cfg := writeYardChainControlConfig(t, pingServer.URL)
	rt, err := rtpkg.BuildOrchestratorRuntime(ctx, cfg)
	if err != nil {
		t.Fatalf("BuildOrchestratorRuntime returned error: %v", err)
	}
	defer rt.Cleanup()

	chainID, err := rt.ChainStore.StartChain(ctx, chain.ChainSpec{ChainID: "cancel-requested", MaxSteps: 5, MaxResolverLoops: 1, MaxDuration: time.Hour, TokenBudget: 100})
	if err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	if err := rt.ChainStore.SetChainStatus(ctx, chainID, "running"); err != nil {
		t.Fatalf("SetChainStatus returned error: %v", err)
	}

	var stdout bytes.Buffer
	cmd := newYardChainCancelCmd(&configPath)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{chainID})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if got := stdout.String(); got != "chain cancel-requested cancel requested\n" {
		t.Fatalf("stdout = %q, want cancel-requested message", got)
	}

	reloaded, err := rt.ChainStore.GetChain(ctx, chainID)
	if err != nil {
		t.Fatalf("GetChain returned error: %v", err)
	}
	if reloaded.Status != "cancel_requested" {
		t.Fatalf("status = %q, want cancel_requested", reloaded.Status)
	}
}

func newYardChainControlTestDB(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "yard-chain-control.db")
	db, err := appdb.OpenDB(ctx, path)
	if err != nil {
		t.Fatalf("OpenDB returned error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := appdb.InitIfNeeded(ctx, db); err != nil {
		t.Fatalf("InitIfNeeded returned error: %v", err)
	}
	if err := appdb.EnsureChainSchema(ctx, db); err != nil {
		t.Fatalf("EnsureChainSchema returned error: %v", err)
	}
	return db
}

func writeYardChainControlConfig(t *testing.T, providerBaseURL string) (string, *appconfig.Config) {
	t.Helper()
	projectRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectRoot, ".brain"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.brain) returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "orchestrator-prompt.md"), []byte("You are the orchestrator."), 0o644); err != nil {
		t.Fatalf("WriteFile(prompt) returned error: %v", err)
	}
	configPath := filepath.Join(projectRoot, "yard.yaml")
	config := strings.Join([]string{
		"project_root: " + projectRoot,
		"brain:",
		"  enabled: false",
		"local_services:",
		"  enabled: false",
		"routing:",
		"  default:",
		"    provider: localtest",
		"    model: test-model",
		"providers:",
		"  localtest:",
		"    type: openai-compatible",
		"    base_url: " + providerBaseURL,
		"    model: test-model",
		"    context_length: 4096",
		"agent_roles:",
		"  orchestrator:",
		"    system_prompt: orchestrator-prompt.md",
	}, "\n") + "\n"
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatalf("WriteFile(config) returned error: %v", err)
	}
	cfg, err := appconfig.Load(configPath)
	if err != nil {
		t.Fatalf("Load(config) returned error: %v", err)
	}
	return configPath, cfg
}
