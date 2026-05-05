package runtime

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/ponchione/sodoryard/internal/agent"
	"github.com/ponchione/sodoryard/internal/brain"
	"github.com/ponchione/sodoryard/internal/chain"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	contextpkg "github.com/ponchione/sodoryard/internal/context"
	"github.com/ponchione/sodoryard/internal/conversation"
	appdb "github.com/ponchione/sodoryard/internal/db"
	"github.com/ponchione/sodoryard/internal/projectmemory"
	"github.com/ponchione/sodoryard/internal/provider"
	"github.com/ponchione/sodoryard/internal/provider/router"
	"github.com/ponchione/sodoryard/internal/role"
	spawnpkg "github.com/ponchione/sodoryard/internal/spawn"
	"github.com/ponchione/sodoryard/internal/tool"
)

// OrchestratorRuntime holds all shared dependencies needed to run a chain
// orchestrator. It is constructed by BuildOrchestratorRuntime and should be
// torn down by calling Cleanup when the process exits.
type OrchestratorRuntime struct {
	Config              *appconfig.Config
	Logger              *slog.Logger
	Database            *sql.DB
	Queries             *appdb.Queries
	ProviderRouter      *router.Router
	BrainBackend        brain.Backend
	ConversationManager *conversation.Manager
	ContextAssembler    agent.ContextAssembler
	ChainStore          *chain.Store
	MemoryEndpointEnv   []string
	Cleanup             func()
}

// NoopContextAssembler is a context assembler that always returns an empty,
// frozen context package. Orchestrators don't need retrieval-augmented context
// assembly; they use this to satisfy the agent.ContextAssembler interface.
type NoopContextAssembler struct{}

func (NoopContextAssembler) Assemble(ctx context.Context, message string, history []appdb.Message, scope contextpkg.AssemblyScope, modelContextLimit int, historyTokenCount int) (*contextpkg.FullContextPackage, bool, error) {
	return &contextpkg.FullContextPackage{Content: "", Frozen: true, Report: &contextpkg.ContextAssemblyReport{TurnNumber: scope.TurnNumber}}, false, nil
}

func (NoopContextAssembler) UpdateQuality(context.Context, string, int, bool, []string) error {
	return nil
}

// RegistryToolExecutor adapts a tool.Registry into an agent.ToolExecutor.
// It is used by the orchestrator agent loop to dispatch tool calls into the
// registered spawn_agent and chain_complete tools.
type RegistryToolExecutor struct {
	Registry    *tool.Registry
	ProjectRoot string
}

func (e *RegistryToolExecutor) Execute(ctx context.Context, call provider.ToolCall) (*provider.ToolResult, error) {
	t, ok := e.Registry.Get(call.Name)
	if !ok {
		return &provider.ToolResult{ToolUseID: call.ID, Content: fmt.Sprintf("Unknown tool: %s", call.Name), IsError: true}, nil
	}
	result, err := t.Execute(ctx, e.ProjectRoot, call.Input)
	if err != nil {
		return nil, err
	}
	result.CallID = call.ID
	return &provider.ToolResult{ToolUseID: call.ID, Content: result.Content, IsError: !result.Success, Details: result.Details}, nil
}

// BuildOrchestratorRuntime constructs and returns a fully initialised
// OrchestratorRuntime. The caller must invoke rt.Cleanup() when done.
func BuildOrchestratorRuntime(ctx context.Context, cfg *appconfig.Config) (*OrchestratorRuntime, error) {
	base, err := buildRuntimeBase(ctx, cfg)
	if err != nil {
		return nil, err
	}
	logger := base.logger
	database := base.database
	queries := base.queries
	cleanup := base.cleanup

	if err := EnsureProjectRecord(ctx, database, cfg); err != nil {
		cleanup()
		return nil, fmt.Errorf("ensure project record: %w", err)
	}

	// Only register providers the YAML explicitly listed. This avoids
	// registering Default() providers that the operator's config never asked
	// for (TECH-DEBT R6).
	provRouter, err := BuildProviderRouter(ctx, cfg, queries, logger, ProviderRouterOptions{
		ProviderNames: cfg.ProviderNamesForSurfaces(),
	})
	if err != nil {
		cleanup()
		return nil, err
	}

	brainBackend, closeBrainBackend, err := buildOrchestratorBrainBackend(ctx, cfg.Brain, logger)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("build brain backend: %w", err)
	}
	memoryEndpointEnv, closeMemoryRPC, err := buildOrchestratorMemoryRPC(ctx, cfg, brainBackend, logger)
	if err != nil {
		closeBrainBackend()
		cleanup()
		return nil, fmt.Errorf("start project memory RPC: %w", err)
	}

	convManager, closeConversationManager, err := BuildConversationManager(ctx, cfg, database, brainBackend, logger)
	if err != nil {
		closeMemoryRPC()
		closeBrainBackend()
		cleanup()
		return nil, err
	}

	rt := &OrchestratorRuntime{
		Config:              cfg,
		Logger:              logger,
		Database:            database,
		Queries:             queries,
		ProviderRouter:      provRouter,
		BrainBackend:        brainBackend,
		ConversationManager: convManager,
		ContextAssembler:    NoopContextAssembler{},
		ChainStore:          chain.NewStore(database),
		MemoryEndpointEnv:   memoryEndpointEnv,
		Cleanup: func() {
			// Drain in-flight sub-call writes before closing the DB so stream
			// goroutines don't race against database.Close() (TECH-DEBT R5).
			provRouter.DrainTracking()
			closeConversationManager()
			closeMemoryRPC()
			closeBrainBackend()
			cleanup()
		},
	}
	return rt, nil
}

// buildOrchestratorBrainBackend constructs the brain backend for the
// orchestrator and returns cleanup for OrchestratorRuntime.Cleanup.
func buildOrchestratorBrainBackend(ctx context.Context, cfg appconfig.BrainConfig, logger *slog.Logger) (brain.Backend, func(), error) {
	if !cfg.Enabled {
		return nil, func() {}, nil
	}
	return BuildBrainBackend(ctx, cfg, logger)
}

func buildOrchestratorMemoryRPC(ctx context.Context, cfg *appconfig.Config, backend brain.Backend, logger *slog.Logger) ([]string, func(), error) {
	if cfg == nil || !cfg.Brain.Enabled || cfg.Brain.Backend != "shunter" {
		return projectMemoryEndpointEnv(cfg), func() {}, nil
	}
	if strings.TrimSpace(os.Getenv(projectmemory.EnvMemoryEndpoint)) != "" {
		return projectMemoryEndpointEnv(cfg), func() {}, nil
	}
	localBackend, ok := backend.(*projectmemory.BrainBackend)
	if !ok || localBackend == nil {
		return nil, func() {}, fmt.Errorf("local Shunter brain backend is required to own project memory RPC")
	}
	transport, path := configuredMemoryRPC(cfg)
	if transport == "" {
		transport = "unix"
	}
	if path == "" {
		return nil, func() {}, fmt.Errorf("project memory RPC path is required")
	}
	server, err := projectmemory.StartRPCServer(ctx, projectmemory.RPCConfig{Transport: transport, Path: path}, localBackend)
	if err != nil {
		return nil, func() {}, err
	}
	endpoint := transport + ":" + path
	if logger != nil {
		logger.Info("project memory RPC listening", "endpoint", endpoint)
	}
	return []string{projectmemory.EnvMemoryEndpoint + "=" + endpoint}, func() { _ = server.Close() }, nil
}

func projectMemoryEndpointEnv(cfg *appconfig.Config) []string {
	endpoint := strings.TrimSpace(os.Getenv(projectmemory.EnvMemoryEndpoint))
	if endpoint == "" {
		endpoint = configuredMemoryEndpoint(cfg)
	}
	if endpoint == "" {
		return nil
	}
	return []string{projectmemory.EnvMemoryEndpoint + "=" + endpoint}
}

func configuredMemoryEndpoint(cfg *appconfig.Config) string {
	if cfg == nil || !cfg.Brain.Enabled || cfg.Brain.Backend != "shunter" {
		return ""
	}
	transport, path := configuredMemoryRPC(cfg)
	if transport == "" {
		transport = "unix"
	}
	if path == "" {
		return ""
	}
	return transport + ":" + path
}

func configuredMemoryRPC(cfg *appconfig.Config) (string, string) {
	if cfg == nil {
		return "", ""
	}
	transport := strings.TrimSpace(cfg.Memory.RPC.Transport)
	if transport == "" {
		transport = strings.TrimSpace(cfg.Brain.RPCTransport)
	}
	path := strings.TrimSpace(cfg.Memory.RPC.Path)
	if path == "" {
		path = strings.TrimSpace(cfg.Brain.RPCPath)
	}
	return transport, path
}

// BuildOrchestratorRegistry constructs a tool.Registry for the orchestrator
// agent loop. It registers spawn_agent and chain_complete as custom tools
// alongside any standard tools declared in roleCfg.
func BuildOrchestratorRegistry(rt *OrchestratorRuntime, roleCfg appconfig.AgentRoleConfig, chainID string) (*tool.Registry, error) {
	factory := map[string]func() tool.Tool{
		"spawn_agent": func() tool.Tool {
			return spawnpkg.NewSpawnAgentTool(spawnpkg.SpawnAgentDeps{
				Store:         rt.ChainStore,
				Backend:       rt.BrainBackend,
				Config:        rt.Config,
				ChainID:       chainID,
				EngineBinary:  "tidmouth",
				ProjectRoot:   rt.Config.ProjectRoot,
				SubprocessEnv: rt.MemoryEndpointEnv,
			})
		},
		"chain_complete": func() tool.Tool {
			return spawnpkg.NewChainCompleteTool(rt.ChainStore, rt.BrainBackend, chainID)
		},
	}
	registry, _, err := role.BuildRegistry(rt.Config, roleCfg, role.BuilderDeps{
		BrainBackend:      rt.BrainBackend,
		ProviderRuntime:   rt.ProviderRouter,
		Queries:           rt.Queries,
		ProjectID:         rt.Config.ProjectRoot,
		CustomToolFactory: factory,
	})
	if err != nil {
		return nil, err
	}
	return registry, nil
}
