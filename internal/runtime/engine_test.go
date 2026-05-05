package runtime

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ponchione/sodoryard/internal/chain"
	"github.com/ponchione/sodoryard/internal/codeintel/embedder"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	contextpkg "github.com/ponchione/sodoryard/internal/context"
	"github.com/ponchione/sodoryard/internal/conversation"
	appdb "github.com/ponchione/sodoryard/internal/db"
	"github.com/ponchione/sodoryard/internal/projectmemory"
	"github.com/ponchione/sodoryard/internal/provider/tracking"
	"github.com/ponchione/sodoryard/internal/tool"
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

func TestBuildEngineRuntimeStartsShunterModeWithoutYardDB(t *testing.T) {
	ctx := context.Background()
	t.Setenv(projectmemory.EnvMemoryEndpoint, "")
	cfg := newShunterOrchestratorTestConfig(t)

	rt, err := BuildEngineRuntime(ctx, cfg)
	if err != nil {
		t.Fatalf("BuildEngineRuntime returned error: %v", err)
	}
	cleaned := false
	t.Cleanup(func() {
		if !cleaned {
			rt.Cleanup()
		}
	})

	if rt.Database != nil || rt.Queries != nil {
		t.Fatalf("runtime SQLite = (%v, %v), want nil database and queries in Shunter mode", rt.Database, rt.Queries)
	}
	if rt.BrainBackend == nil || rt.MemoryBackend == nil || rt.ProviderRouter == nil || rt.ConversationManager == nil || rt.ContextAssembler == nil || rt.ToolRecorder == nil || rt.ChainStore == nil {
		t.Fatalf("runtime missing Shunter-mode components: %+v", rt)
	}
	if _, err := os.Stat(cfg.DatabasePath()); !os.IsNotExist(err) {
		t.Fatalf("database stat err = %v, want no yard.db created in Shunter mode", err)
	}
	if _, err := os.Stat(cfg.Brain.VaultPath); !os.IsNotExist(err) {
		t.Fatalf("brain vault stat err = %v, want no .brain dependency in Shunter mode", err)
	}
	if _, err := os.Stat(cfg.GraphDBPath()); err != nil {
		t.Fatalf("graph db stat err = %v, want derived graph store created", err)
	}

	conv, err := rt.ConversationManager.Create(ctx, cfg.ProjectRoot, conversation.WithTitle("Engine Shunter"))
	if err != nil {
		t.Fatalf("Create conversation: %v", err)
	}
	if err := rt.ConversationManager.PersistUserMessage(ctx, conv.ID, 1, "runtime should use Shunter project memory"); err != nil {
		t.Fatalf("PersistUserMessage: %v", err)
	}
	history, err := rt.ConversationManager.ReconstructHistory(ctx, conv.ID)
	if err != nil {
		t.Fatalf("ReconstructHistory: %v", err)
	}
	if len(history) != 1 || history[0].Content.String != "runtime should use Shunter project memory" {
		t.Fatalf("history = %+v, want Shunter-backed user message", history)
	}
	if err := rt.BrainBackend.WriteDocument(ctx, "notes/engine.md", "# Engine\n\nRuntime brain writes stay in Shunter."); err != nil {
		t.Fatalf("WriteDocument: %v", err)
	}
	doc, err := rt.BrainBackend.ReadDocument(ctx, "notes/engine.md")
	if err != nil {
		t.Fatalf("ReadDocument: %v", err)
	}
	if doc != "# Engine\n\nRuntime brain writes stay in Shunter." {
		t.Fatalf("ReadDocument = %q, want Shunter brain content", doc)
	}
	if err := rt.ToolRecorder.Record(ctx,
		tool.ToolCall{ID: "toolu-engine", Name: "brain_write"},
		tool.ToolResult{Success: true, Content: "wrote note", DurationMs: 17},
		tool.ExecutionMeta{ConversationID: conv.ID, TurnNumber: 1, Iteration: 1},
		time.Date(2026, 5, 5, 22, 0, 0, 0, time.UTC),
	); err != nil {
		t.Fatalf("Record tool execution: %v", err)
	}
	executionReader, ok := rt.MemoryBackend.(interface {
		ListToolExecutions(context.Context, string) ([]projectmemory.ToolExecution, error)
	})
	if !ok {
		t.Fatalf("MemoryBackend = %T, want tool execution reader", rt.MemoryBackend)
	}
	executions, err := executionReader.ListToolExecutions(ctx, conv.ID)
	if err != nil {
		t.Fatalf("ListToolExecutions: %v", err)
	}
	if len(executions) != 1 || executions[0].ToolUseID != "toolu-engine" || executions[0].DurationMs != 17 {
		t.Fatalf("executions = %+v, want Shunter-backed tool execution", executions)
	}
	chainID, err := rt.ChainStore.StartChain(ctx, chain.ChainSpec{ChainID: "engine-shunter-chain", SourceTask: "runtime Shunter chain"})
	if err != nil {
		t.Fatalf("StartChain: %v", err)
	}
	chainReader, ok := rt.MemoryBackend.(interface {
		ReadChain(context.Context, string) (projectmemory.Chain, bool, error)
	})
	if !ok {
		t.Fatalf("MemoryBackend = %T, want chain reader", rt.MemoryBackend)
	}
	chainRow, found, err := chainReader.ReadChain(ctx, chainID)
	if err != nil {
		t.Fatalf("ReadChain: %v", err)
	}
	if !found || chainRow.SourceTask != "runtime Shunter chain" {
		t.Fatalf("chain row = %+v found=%t, want Shunter-backed chain", chainRow, found)
	}

	rt.Cleanup()
	cleaned = true
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

func TestBuildSubCallStoreUsesProjectMemoryInShunterMode(t *testing.T) {
	ctx := context.Background()
	projectRoot := t.TempDir()
	cfg := appconfig.Default()
	cfg.ProjectRoot = projectRoot
	cfg.Memory.Backend = "shunter"
	backend, err := projectmemory.OpenBrainBackend(ctx, projectmemory.Config{DataDir: filepath.Join(projectRoot, "memory"), DurableAck: true})
	if err != nil {
		t.Fatalf("OpenBrainBackend: %v", err)
	}
	defer backend.Close()

	store, err := buildSubCallStore(cfg, nil, backend)
	if err != nil {
		t.Fatalf("buildSubCallStore: %v", err)
	}
	if _, ok := store.(*tracking.ProjectMemorySubCallStore); !ok {
		t.Fatalf("store = %T, want *tracking.ProjectMemorySubCallStore", store)
	}
	convID := "conv-runtime"
	turn := 1
	iter := 1
	if err := store.InsertSubCall(ctx, tracking.InsertSubCallParams{
		ConversationID: &convID,
		TurnNumber:     &turn,
		Iteration:      &iter,
		Provider:       "codex",
		Model:          "gpt-5.5",
		Purpose:        "chat",
		TokensIn:       12,
		TokensOut:      3,
		LatencyMs:      25,
		Success:        1,
		CreatedAt:      time.Date(2026, 5, 5, 19, 0, 0, 0, time.UTC).Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("InsertSubCall: %v", err)
	}
	subCalls, err := backend.ListSubCalls(ctx, convID)
	if err != nil {
		t.Fatalf("ListSubCalls: %v", err)
	}
	if len(subCalls) != 1 || subCalls[0].TokensIn != 12 || subCalls[0].LatencyMs != 25 {
		t.Fatalf("subcalls = %+v, want runtime Shunter record", subCalls)
	}
}

func TestBuildToolExecutionRecorderUsesProjectMemoryInShunterMode(t *testing.T) {
	ctx := context.Background()
	projectRoot := t.TempDir()
	cfg := appconfig.Default()
	cfg.ProjectRoot = projectRoot
	cfg.Memory.Backend = "shunter"
	backend, err := projectmemory.OpenBrainBackend(ctx, projectmemory.Config{DataDir: filepath.Join(projectRoot, "memory"), DurableAck: true})
	if err != nil {
		t.Fatalf("OpenBrainBackend: %v", err)
	}
	defer backend.Close()
	if err := backend.CreateConversation(ctx, projectmemory.CreateConversationArgs{
		ID:          "conv-runtime-tool",
		ProjectID:   "project-1",
		Title:       "Runtime Tool",
		CreatedAtUS: uint64(time.Now().UTC().UnixMicro()),
	}); err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}

	recorder, err := BuildToolExecutionRecorder(cfg, nil, backend)
	if err != nil {
		t.Fatalf("BuildToolExecutionRecorder: %v", err)
	}
	if recorder == nil {
		t.Fatal("BuildToolExecutionRecorder returned nil")
	}
	if err := recorder.Record(ctx,
		tool.ToolCall{ID: "toolu-runtime", Name: "file_read"},
		tool.ToolResult{Success: true, Content: "runtime tool output", DurationMs: 10},
		tool.ExecutionMeta{ConversationID: "conv-runtime-tool", TurnNumber: 1, Iteration: 1},
		time.Date(2026, 5, 5, 21, 0, 0, 0, time.UTC),
	); err != nil {
		t.Fatalf("Record: %v", err)
	}
	executions, err := backend.ListToolExecutions(ctx, "conv-runtime-tool")
	if err != nil {
		t.Fatalf("ListToolExecutions: %v", err)
	}
	if len(executions) != 1 || executions[0].ToolUseID != "toolu-runtime" || executions[0].DurationMs != 10 {
		t.Fatalf("executions = %+v, want runtime Shunter tool record", executions)
	}
}

func TestBuildContextReportStoreUsesProjectMemoryInShunterMode(t *testing.T) {
	ctx := context.Background()
	projectRoot := t.TempDir()
	cfg := appconfig.Default()
	cfg.ProjectRoot = projectRoot
	cfg.Memory.Backend = "shunter"
	backend, err := projectmemory.OpenBrainBackend(ctx, projectmemory.Config{DataDir: filepath.Join(projectRoot, "memory"), DurableAck: true})
	if err != nil {
		t.Fatalf("OpenBrainBackend: %v", err)
	}
	defer backend.Close()
	if err := backend.CreateConversation(ctx, projectmemory.CreateConversationArgs{
		ID:          "conv-runtime-context",
		ProjectID:   "project-1",
		Title:       "Runtime Context",
		CreatedAtUS: uint64(time.Now().UTC().UnixMicro()),
	}); err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}

	store, err := BuildContextReportStore(cfg, nil, backend)
	if err != nil {
		t.Fatalf("BuildContextReportStore: %v", err)
	}
	if _, ok := store.(*contextpkg.ProjectMemoryReportStore); !ok {
		t.Fatalf("store = %T, want *context.ProjectMemoryReportStore", store)
	}
	if err := store.Insert(ctx, "conv-runtime-context", &contextpkg.ContextAssemblyReport{
		TurnNumber:      1,
		RAGResults:      []contextpkg.RAGHit{{ChunkID: "chunk-runtime", FilePath: "internal/runtime/engine.go", Included: true}},
		BudgetBreakdown: map[string]int{},
	}); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	report, found, err := backend.ReadContextReport(ctx, "conv-runtime-context", 1)
	if err != nil {
		t.Fatalf("ReadContextReport: %v", err)
	}
	if !found || report.ID != projectmemory.ContextReportID("conv-runtime-context", 1) {
		t.Fatalf("report = %+v found=%t, want runtime Shunter context report", report, found)
	}
}

func TestBuildChainStoreUsesProjectMemoryInShunterMode(t *testing.T) {
	ctx := context.Background()
	projectRoot := t.TempDir()
	cfg := appconfig.Default()
	cfg.ProjectRoot = projectRoot
	cfg.Memory.Backend = "shunter"
	backend, err := projectmemory.OpenBrainBackend(ctx, projectmemory.Config{DataDir: filepath.Join(projectRoot, "memory"), DurableAck: true})
	if err != nil {
		t.Fatalf("OpenBrainBackend: %v", err)
	}
	defer backend.Close()

	store, err := BuildChainStore(cfg, nil, backend)
	if err != nil {
		t.Fatalf("BuildChainStore: %v", err)
	}
	chainID, err := store.StartChain(ctx, chain.ChainSpec{ChainID: "runtime-chain", SourceTask: "runtime shunter chain", MaxSteps: 2, MaxResolverLoops: 1, MaxDuration: time.Minute, TokenBudget: 10})
	if err != nil {
		t.Fatalf("StartChain: %v", err)
	}
	row, found, err := backend.ReadChain(ctx, chainID)
	if err != nil {
		t.Fatalf("ReadChain: %v", err)
	}
	if !found || row.ID != "runtime-chain" || row.SourceTask != "runtime shunter chain" {
		t.Fatalf("chain row = %+v found=%t, want runtime Shunter chain", row, found)
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
