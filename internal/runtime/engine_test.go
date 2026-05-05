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
	"github.com/ponchione/sodoryard/internal/conversation"
	appdb "github.com/ponchione/sodoryard/internal/db"
	"github.com/ponchione/sodoryard/internal/projectmemory"
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

func TestBuildBrainBackendUsesShunterWithoutVault(t *testing.T) {
	ctx := context.Background()
	cfg := appconfig.BrainConfig{
		Enabled:        true,
		Backend:        "shunter",
		VaultPath:      filepath.Join(t.TempDir(), "missing-brain"),
		ShunterDataDir: filepath.Join(t.TempDir(), "memory"),
		DurableAck:     true,
	}

	backend, cleanup, err := BuildBrainBackend(ctx, cfg, slog.Default())
	if err != nil {
		t.Fatalf("BuildBrainBackend returned error: %v", err)
	}
	defer cleanup()
	if err := backend.WriteDocument(ctx, "notes/design.md", "# Design\n\nShunter backed."); err != nil {
		t.Fatalf("WriteDocument: %v", err)
	}
	got, err := backend.ReadDocument(ctx, "notes/design.md")
	if err != nil {
		t.Fatalf("ReadDocument: %v", err)
	}
	if got != "# Design\n\nShunter backed." {
		t.Fatalf("ReadDocument = %q, want Shunter content", got)
	}
}

func TestBuildBrainBackendUsesMemoryEndpointWithoutOpeningShunterDataDir(t *testing.T) {
	ctx := context.Background()
	projectRoot := t.TempDir()
	parent, err := projectmemory.OpenBrainBackend(ctx, projectmemory.Config{DataDir: filepath.Join(projectRoot, "parent-memory"), DurableAck: true})
	if err != nil {
		t.Fatalf("OpenBrainBackend: %v", err)
	}
	defer parent.Close()
	socketPath := filepath.Join(projectRoot, "run", "memory.sock")
	server, err := projectmemory.StartRPCServer(ctx, projectmemory.RPCConfig{Transport: "unix", Path: socketPath}, parent)
	if err != nil {
		t.Fatalf("StartRPCServer: %v", err)
	}
	defer server.Close()

	dataDir := filepath.Join(projectRoot, "child-should-not-open")
	t.Setenv(projectmemory.EnvMemoryEndpoint, "unix:"+socketPath)
	cfg := appconfig.BrainConfig{
		Enabled:        true,
		Backend:        "shunter",
		ShunterDataDir: dataDir,
		DurableAck:     true,
	}

	backend, cleanup, err := BuildBrainBackend(ctx, cfg, slog.Default())
	if err != nil {
		t.Fatalf("BuildBrainBackend returned error: %v", err)
	}
	defer cleanup()
	if err := backend.WriteDocument(ctx, "notes/remote.md", "# Remote\n\nChild wrote through RPC."); err != nil {
		t.Fatalf("WriteDocument: %v", err)
	}
	got, err := parent.ReadDocument(ctx, "notes/remote.md")
	if err != nil {
		t.Fatalf("parent ReadDocument: %v", err)
	}
	if got != "# Remote\n\nChild wrote through RPC." {
		t.Fatalf("parent content = %q, want RPC write", got)
	}
	if _, statErr := os.Stat(dataDir); !os.IsNotExist(statErr) {
		t.Fatalf("Shunter data dir stat err = %v, want not-exist", statErr)
	}
}

func TestBuildConversationManagerUsesShunterMemoryBackend(t *testing.T) {
	ctx := context.Background()
	projectRoot := t.TempDir()
	cfg := appconfig.Default()
	cfg.ProjectRoot = projectRoot
	cfg.Memory.Backend = "shunter"
	cfg.Memory.ShunterDataDir = filepath.Join(projectRoot, ".yard", "shunter", "project-memory")
	cfg.Memory.DurableAck = true

	manager, cleanup, err := BuildConversationManager(ctx, cfg, nil, nil, slog.Default())
	if err != nil {
		t.Fatalf("BuildConversationManager returned error: %v", err)
	}
	defer cleanup()
	conv, err := manager.Create(ctx, projectRoot, conversation.WithTitle("Shunter Runtime"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := manager.PersistUserMessage(ctx, conv.ID, 1, "runtime conversation write"); err != nil {
		t.Fatalf("PersistUserMessage: %v", err)
	}
	history, err := manager.ReconstructHistory(ctx, conv.ID)
	if err != nil {
		t.Fatalf("ReconstructHistory: %v", err)
	}
	if len(history) != 1 || history[0].Content.String != "runtime conversation write" {
		t.Fatalf("history = %+v, want runtime conversation write", history)
	}
}

func TestBuildConversationManagerUsesMemoryEndpointWithoutOpeningShunterDataDir(t *testing.T) {
	ctx := context.Background()
	projectRoot := t.TempDir()
	parent, err := projectmemory.OpenBrainBackend(ctx, projectmemory.Config{DataDir: filepath.Join(projectRoot, "parent-memory"), DurableAck: true})
	if err != nil {
		t.Fatalf("OpenBrainBackend: %v", err)
	}
	defer parent.Close()
	socketPath := filepath.Join(projectRoot, "run", "memory.sock")
	server, err := projectmemory.StartRPCServer(ctx, projectmemory.RPCConfig{Transport: "unix", Path: socketPath}, parent)
	if err != nil {
		t.Fatalf("StartRPCServer: %v", err)
	}
	defer server.Close()

	dataDir := filepath.Join(projectRoot, "child-should-not-open")
	t.Setenv(projectmemory.EnvMemoryEndpoint, "unix:"+socketPath)
	cfg := appconfig.Default()
	cfg.ProjectRoot = projectRoot
	cfg.Memory.Backend = "shunter"
	cfg.Memory.ShunterDataDir = dataDir
	cfg.Memory.DurableAck = true

	manager, cleanup, err := BuildConversationManager(ctx, cfg, nil, nil, slog.Default())
	if err != nil {
		t.Fatalf("BuildConversationManager returned error: %v", err)
	}
	defer cleanup()
	conv, err := manager.Create(ctx, projectRoot, conversation.WithTitle("Remote Conversation"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	parentConversation, found, err := parent.ReadConversation(ctx, conv.ID)
	if err != nil {
		t.Fatalf("parent ReadConversation: %v", err)
	}
	if !found || parentConversation.Title != "Remote Conversation" {
		t.Fatalf("parent conversation = %+v found=%t, want Remote Conversation", parentConversation, found)
	}
	if _, statErr := os.Stat(dataDir); !os.IsNotExist(statErr) {
		t.Fatalf("Shunter data dir stat err = %v, want not-exist", statErr)
	}
}

func TestBuildConventionSourceUsesShunterBackendWithBrainAbsent(t *testing.T) {
	ctx := context.Background()
	projectRoot := t.TempDir()
	cfg := appconfig.Default()
	cfg.ProjectRoot = projectRoot
	cfg.Brain.Enabled = true
	cfg.Brain.Backend = "shunter"
	cfg.Brain.VaultPath = filepath.Join(projectRoot, ".brain")
	cfg.Brain.ShunterDataDir = filepath.Join(projectRoot, ".yard", "shunter", "project-memory")
	cfg.Brain.DurableAck = true

	backend, cleanup, err := BuildBrainBackend(ctx, cfg.Brain, slog.Default())
	if err != nil {
		t.Fatalf("BuildBrainBackend returned error: %v", err)
	}
	defer cleanup()
	if err := backend.WriteDocument(ctx, "conventions/coding.md", "# Coding\n\n- Use focused tests\n"); err != nil {
		t.Fatalf("WriteDocument: %v", err)
	}
	if _, err := os.Stat(cfg.Brain.VaultPath); !os.IsNotExist(err) {
		t.Fatalf("brain vault stat err = %v, want not-exist", err)
	}
	text, err := BuildConventionSource(cfg, backend).Load(ctx)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if text != "Use focused tests" {
		t.Fatalf("Load returned %q, want Shunter convention bullet", text)
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
