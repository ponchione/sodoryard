package runtime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ponchione/sodoryard/internal/chain"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/projectmemory"
	spawnpkg "github.com/ponchione/sodoryard/internal/spawn"
)

func TestBuildOrchestratorRuntimeStartsMemoryRPCForShunterBrain(t *testing.T) {
	ctx := context.Background()
	t.Setenv(projectmemory.EnvMemoryEndpoint, "")
	cfg := newShunterOrchestratorTestConfig(t)

	rt, err := BuildOrchestratorRuntime(ctx, cfg)
	if err != nil {
		t.Fatalf("BuildOrchestratorRuntime returned error: %v", err)
	}
	cleaned := false
	t.Cleanup(func() {
		if !cleaned {
			rt.Cleanup()
		}
	})

	expectedEnv := []string{projectmemory.EnvMemoryEndpoint + "=unix:" + cfg.Memory.RPC.Path}
	if strings.Join(rt.MemoryEndpointEnv, "\n") != strings.Join(expectedEnv, "\n") {
		t.Fatalf("MemoryEndpointEnv = %v, want %v", rt.MemoryEndpointEnv, expectedEnv)
	}
	if rt.Database != nil || rt.Queries != nil {
		t.Fatalf("runtime SQLite = (%v, %v), want nil database and queries in Shunter mode", rt.Database, rt.Queries)
	}
	if _, err := os.Stat(cfg.DatabasePath()); !os.IsNotExist(err) {
		t.Fatalf("database stat err = %v, want no yard.db created in Shunter mode", err)
	}

	client, err := projectmemory.DialBrainBackend("unix:" + cfg.Memory.RPC.Path)
	if err != nil {
		t.Fatalf("DialBrainBackend: %v", err)
	}
	if err := client.WriteDocument(ctx, "notes/rpc-runtime.md", "# Runtime\n\nRPC is live."); err != nil {
		t.Fatalf("client WriteDocument: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("client Close: %v", err)
	}
	got, err := rt.BrainBackend.ReadDocument(ctx, "notes/rpc-runtime.md")
	if err != nil {
		t.Fatalf("runtime ReadDocument: %v", err)
	}
	if got != "# Runtime\n\nRPC is live." {
		t.Fatalf("runtime content = %q, want RPC content", got)
	}
	chainID, err := rt.ChainStore.StartChain(ctx, chain.ChainSpec{ChainID: "orchestrator-chain", SourceTask: "rpc chain state"})
	if err != nil {
		t.Fatalf("runtime ChainStore StartChain: %v", err)
	}
	client, err = projectmemory.DialBrainBackend("unix:" + cfg.Memory.RPC.Path)
	if err != nil {
		t.Fatalf("DialBrainBackend for chain read: %v", err)
	}
	chainRow, found, err := client.ReadChain(ctx, chainID)
	if err != nil {
		t.Fatalf("client ReadChain: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("client Close after chain read: %v", err)
	}
	if !found || chainRow.SourceTask != "rpc chain state" {
		t.Fatalf("RPC chain row = %+v found=%t, want rpc chain state", chainRow, found)
	}

	rt.Cleanup()
	cleaned = true
	if _, err := os.Stat(cfg.Memory.RPC.Path); !os.IsNotExist(err) {
		t.Fatalf("RPC socket stat err = %v, want not-exist after cleanup", err)
	}
}

func TestBuildOrchestratorRegistryPassesMemoryEndpointEnvToSpawnAgent(t *testing.T) {
	cfg := appconfig.Default()
	cfg.ProjectRoot = t.TempDir()
	expectedEnv := []string{projectmemory.EnvMemoryEndpoint + "=unix:/tmp/memory.sock"}

	registry, err := BuildOrchestratorRegistry(&OrchestratorRuntime{
		Config:            cfg,
		MemoryEndpointEnv: expectedEnv,
	}, appconfig.AgentRoleConfig{CustomTools: []string{"spawn_agent"}}, "chain-env")
	if err != nil {
		t.Fatalf("BuildOrchestratorRegistry returned error: %v", err)
	}
	registered, ok := registry.Get("spawn_agent")
	if !ok {
		t.Fatal("spawn_agent tool was not registered")
	}
	spawnTool, ok := registered.(*spawnpkg.SpawnAgentTool)
	if !ok {
		t.Fatalf("spawn_agent tool = %T, want *spawn.SpawnAgentTool", registered)
	}
	if strings.Join(spawnTool.SubprocessEnv, "\n") != strings.Join(expectedEnv, "\n") {
		t.Fatalf("SubprocessEnv = %v, want %v", spawnTool.SubprocessEnv, expectedEnv)
	}
}

func newShunterOrchestratorTestConfig(t *testing.T) *appconfig.Config {
	t.Helper()
	projectRoot := t.TempDir()
	providerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(providerServer.Close)
	cfg := appconfig.Default()
	cfg.ProjectRoot = projectRoot
	cfg.LogLevel = "error"
	cfg.Routing.Default.Provider = "test"
	cfg.Routing.Default.Model = "test-model"
	cfg.Providers = map[string]appconfig.ProviderConfig{
		"test": {
			Type:          "openai-compatible",
			BaseURL:       providerServer.URL,
			Model:         "test-model",
			ContextLength: 4096,
		},
	}
	cfg.ConfiguredProviders = []string{"test"}
	cfg.Memory.Backend = "shunter"
	cfg.Memory.ShunterDataDir = filepath.Join(projectRoot, ".yard", "shunter", "project-memory")
	cfg.Memory.DurableAck = true
	cfg.Memory.RPC.Transport = "unix"
	cfg.Memory.RPC.Path = filepath.Join(projectRoot, ".yard", "run", "memory.sock")
	cfg.Brain.Enabled = true
	cfg.Brain.Backend = "shunter"
	cfg.Brain.VaultPath = filepath.Join(projectRoot, ".brain-missing")
	cfg.Brain.MemoryBackend = cfg.Memory.Backend
	cfg.Brain.ShunterDataDir = cfg.Memory.ShunterDataDir
	cfg.Brain.DurableAck = cfg.Memory.DurableAck
	cfg.Brain.RPCTransport = cfg.Memory.RPC.Transport
	cfg.Brain.RPCPath = cfg.Memory.RPC.Path
	return cfg
}
