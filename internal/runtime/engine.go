package runtime

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/ponchione/sodoryard/internal/agent"
	"github.com/ponchione/sodoryard/internal/brain"
	"github.com/ponchione/sodoryard/internal/brain/mcpclient"
	"github.com/ponchione/sodoryard/internal/chain"
	"github.com/ponchione/sodoryard/internal/codeintel"
	"github.com/ponchione/sodoryard/internal/codeintel/embedder"
	codegraph "github.com/ponchione/sodoryard/internal/codeintel/graph"
	codesearcher "github.com/ponchione/sodoryard/internal/codeintel/searcher"
	"github.com/ponchione/sodoryard/internal/codestore"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	contextpkg "github.com/ponchione/sodoryard/internal/context"
	"github.com/ponchione/sodoryard/internal/conversation"
	appdb "github.com/ponchione/sodoryard/internal/db"
	"github.com/ponchione/sodoryard/internal/projectmemory"
	"github.com/ponchione/sodoryard/internal/provider/router"
	"github.com/ponchione/sodoryard/internal/tool"
)

// EngineRuntime holds all runtime dependencies required to serve engine
// requests. It is the exported equivalent of cmd/tidmouth's appRuntime.
type EngineRuntime struct {
	Config              *appconfig.Config
	Logger              *slog.Logger
	Database            *sql.DB
	Queries             *appdb.Queries
	ProviderRouter      *router.Router
	BrainBackend        brain.Backend
	MemoryBackend       any
	SemanticSearcher    *codesearcher.Searcher
	BrainSearcher       *contextpkg.HybridBrainSearcher
	ConversationManager *conversation.Manager
	ContextAssembler    *contextpkg.ContextAssembler
	CompressionEngine   agent.CompressionEngine
	ToolRecorder        *tool.ToolExecutionRecorder
	ChainStore          *chain.Store
	Cleanup             func()
}

// BuildEngineRuntime constructs a fully initialised EngineRuntime from cfg.
// It mirrors cmd/tidmouth's buildAppRuntime, delegating to the already-
// extracted helpers in this package (ChainCleanup, EnsureProjectRecord,
// BuildProvider, LogProviderAuthStatus).
func BuildEngineRuntime(ctx context.Context, cfg *appconfig.Config) (*EngineRuntime, error) {
	base, err := buildRuntimeBase(ctx, cfg)
	if err != nil {
		return nil, err
	}
	logger := base.logger
	database := base.database
	queries := base.queries
	cleanup := base.cleanup
	closeOnError := func(err error) (*EngineRuntime, error) {
		cleanup()
		return nil, err
	}

	if err := EnsureProjectRecord(ctx, database, cfg); err != nil {
		return closeOnError(fmt.Errorf("ensure project record: %w", err))
	}

	codeStore, err := codestore.Open(ctx, cfg.CodeLanceDBPath())
	if err != nil {
		return closeOnError(fmt.Errorf("open code vectorstore: %w", err))
	}
	cleanup = ChainCleanup(cleanup, func() { _ = codeStore.Close() })
	semanticEmbedder := embedder.New(cfg.Embedding)
	semanticSearcher := codesearcher.New(codeStore, semanticEmbedder)

	brainBackend, brainSearcher, closeBrainRuntime, err := buildBrainRuntime(ctx, cfg, semanticEmbedder, queries, logger)
	if err != nil {
		return closeOnError(err)
	}
	cleanup = ChainCleanup(cleanup, closeBrainRuntime)
	memoryBackend, closeMemoryBackend, err := BuildProjectMemoryStore(ctx, cfg, brainBackend, logger)
	if err != nil {
		return closeOnError(err)
	}
	cleanup = ChainCleanup(cleanup, closeMemoryBackend)

	provRouter, err := BuildProviderRouter(ctx, cfg, queries, logger, ProviderRouterOptions{
		ProviderNames: providerMapNames(cfg.Providers),
		LogAuthStatus: true,
		MemoryBackend: memoryBackend,
	})
	if err != nil {
		return closeOnError(err)
	}
	toolRecorder, err := BuildToolExecutionRecorder(cfg, queries, memoryBackend)
	if err != nil {
		return closeOnError(err)
	}
	contextReportStore, err := BuildContextReportStore(cfg, database, memoryBackend)
	if err != nil {
		return closeOnError(err)
	}
	chainStore, err := BuildChainStore(cfg, database, memoryBackend)
	if err != nil {
		return closeOnError(err)
	}

	graphStore, closeGraphStore, err := BuildGraphStore(cfg)
	if err != nil {
		return closeOnError(fmt.Errorf("build graph store: %w", err))
	}
	cleanup = ChainCleanup(cleanup, closeGraphStore)

	conventionSource := BuildConventionSource(cfg, brainBackend)
	retrievalOrchestrator := contextpkg.NewRetrievalOrchestrator(semanticSearcher, graphStore, conventionSource, brainSearcher, cfg.ProjectRoot)
	retrievalOrchestrator.SetLogBrainQueries(cfg.Brain.LogBrainQueries)
	retrievalOrchestrator.SetBrainConfig(cfg.Brain)
	budgetManager := contextpkg.PriorityBudgetManager{}
	budgetManager.SetBrainConfig(cfg.Brain)

	convManager, closeConversationManager, err := BuildConversationManager(ctx, cfg, database, memoryBackend, logger)
	if err != nil {
		return closeOnError(err)
	}
	cleanup = ChainCleanup(cleanup, closeConversationManager)
	compressionEngine := BuildCompressionEngine(cfg, database, memoryBackend, provRouter)
	contextAssembler := contextpkg.NewContextAssemblerWithReportStore(
		contextpkg.RuleBasedAnalyzer{},
		contextpkg.HeuristicQueryExtractor{},
		contextpkg.HistoryMomentumTracker{},
		retrievalOrchestrator,
		budgetManager,
		contextpkg.MarkdownSerializer{},
		cfg.Context,
		contextReportStore,
	)

	return &EngineRuntime{
		Config:              cfg,
		Logger:              logger,
		Database:            database,
		Queries:             queries,
		ProviderRouter:      provRouter,
		BrainBackend:        brainBackend,
		MemoryBackend:       memoryBackend,
		SemanticSearcher:    semanticSearcher,
		BrainSearcher:       brainSearcher,
		ConversationManager: convManager,
		ContextAssembler:    contextAssembler,
		CompressionEngine:   compressionEngine,
		ToolRecorder:        toolRecorder,
		ChainStore:          chainStore,
		Cleanup:             cleanup,
	}, nil
}

func BuildCompressionEngine(cfg *appconfig.Config, database *sql.DB, memoryBackend any, providerRouter *router.Router) agent.CompressionEngine {
	if cfg == nil || !cfg.Agent.CompressHistoricalResults {
		return nil
	}
	if cfg.Memory.Backend == "shunter" {
		store, ok := memoryBackend.(contextpkg.ProjectMemoryCompressionStore)
		if !ok || store == nil {
			return nil
		}
		return contextpkg.NewProjectMemoryCompressionEngine(store, providerRouter)
	}
	if database == nil {
		return nil
	}
	return contextpkg.NewCompressionEngine(database, providerRouter)
}

func buildBrainRuntime(ctx context.Context, cfg *appconfig.Config, semanticEmbedder codeintel.Embedder, queries *appdb.Queries, logger *slog.Logger) (brain.Backend, *contextpkg.HybridBrainSearcher, func(), error) {
	if cfg == nil {
		return nil, nil, func() {}, fmt.Errorf("runtime config is required")
	}
	if !cfg.Brain.Enabled {
		return nil, nil, func() {}, nil
	}
	brainStore, err := codestore.Open(ctx, cfg.BrainLanceDBPath())
	if err != nil {
		return nil, nil, func() {}, fmt.Errorf("open brain vectorstore: %w", err)
	}
	brainBackend, closeBrainBackend, err := BuildBrainBackend(ctx, cfg.Brain, logger)
	if err != nil {
		_ = brainStore.Close()
		return nil, nil, func() {}, fmt.Errorf("build brain backend: %w", err)
	}
	cleanup := ChainCleanup(closeBrainBackend, func() { _ = brainStore.Close() })
	brainSearcher := contextpkg.NewHybridBrainSearcher(brainBackend, brainStore, semanticEmbedder, queries, cfg.ProjectRoot)
	return brainBackend, brainSearcher, cleanup, nil
}

// BuildBrainBackend constructs a brain.Backend from a BrainConfig. It returns
// a no-op backend and cleanup when the brain is disabled.
func BuildBrainBackend(ctx context.Context, cfg appconfig.BrainConfig, logger *slog.Logger) (brain.Backend, func(), error) {
	if !cfg.Enabled {
		return nil, func() {}, nil
	}
	if cfg.Backend == "" {
		if strings.EqualFold(strings.TrimSpace(cfg.MemoryBackend), "shunter") || strings.TrimSpace(cfg.ShunterDataDir) != "" {
			cfg.Backend = "shunter"
		} else {
			cfg.Backend = "vault"
		}
	}
	if cfg.Backend == "shunter" {
		if endpoint := os.Getenv(projectmemory.EnvMemoryEndpoint); endpoint != "" {
			client, err := projectmemory.DialBrainBackend(endpoint)
			if err != nil {
				return nil, func() {}, err
			}
			if logger != nil {
				logger.Info("brain backend: Shunter RPC", "endpoint", endpoint)
			}
			return client, func() { _ = client.Close() }, nil
		}
		backend, err := projectmemory.OpenBrainBackend(ctx, projectmemory.Config{
			DataDir:    cfg.ShunterDataDir,
			DurableAck: cfg.DurableAck,
		})
		if err != nil {
			return nil, func() {}, err
		}
		if logger != nil {
			logger.Info("brain backend: Shunter", "data_dir", cfg.ShunterDataDir)
		}
		return backend, func() { _ = backend.Close() }, nil
	}
	if cfg.Backend != "vault" {
		return nil, func() {}, fmt.Errorf("unsupported brain backend %q", cfg.Backend)
	}
	if strings.TrimSpace(cfg.MemoryBackend) != "" && cfg.MemoryBackend != "legacy" {
		return nil, func() {}, fmt.Errorf("vault brain backend requires memory.backend: legacy")
	}
	client, err := mcpclient.Connect(ctx, cfg.VaultPath)
	if err != nil {
		return nil, func() {}, err
	}
	if logger != nil {
		logger.Info("brain backend: MCP (in-process)", "vault", cfg.VaultPath)
	}
	return client, func() { _ = client.Close() }, nil
}

func BuildConversationManager(ctx context.Context, cfg *appconfig.Config, database *sql.DB, memoryBackend any, logger *slog.Logger) (*conversation.Manager, func(), error) {
	if cfg == nil || cfg.Memory.Backend != "shunter" {
		return conversation.NewManager(database, nil, logger), func() {}, nil
	}
	if store, ok := memoryBackend.(conversation.ProjectMemoryStore); ok && store != nil {
		return conversation.NewProjectMemoryManager(store, nil, logger), func() {}, nil
	}
	if endpoint := os.Getenv(projectmemory.EnvMemoryEndpoint); endpoint != "" {
		client, err := projectmemory.DialBrainBackend(endpoint)
		if err != nil {
			return nil, func() {}, err
		}
		return conversation.NewProjectMemoryManager(client, nil, logger), func() { _ = client.Close() }, nil
	}
	backend, err := projectmemory.OpenBrainBackend(ctx, projectmemory.Config{
		DataDir:    cfg.Memory.ShunterDataDir,
		DurableAck: cfg.Memory.DurableAck,
	})
	if err != nil {
		return nil, func() {}, err
	}
	return conversation.NewProjectMemoryManager(backend, nil, logger), func() { _ = backend.Close() }, nil
}

func BuildProjectMemoryStore(ctx context.Context, cfg *appconfig.Config, existing any, logger *slog.Logger) (any, func(), error) {
	if cfg == nil || cfg.Memory.Backend != "shunter" {
		return nil, func() {}, nil
	}
	if existing != nil {
		if _, ok := existing.(projectmemory.SubCallRecorder); ok {
			return existing, func() {}, nil
		}
		if _, ok := existing.(conversation.ProjectMemoryStore); ok {
			return existing, func() {}, nil
		}
		if _, ok := existing.(projectmemory.LaunchStore); ok {
			return existing, func() {}, nil
		}
	}
	if endpoint := os.Getenv(projectmemory.EnvMemoryEndpoint); endpoint != "" {
		client, err := projectmemory.DialBrainBackend(endpoint)
		if err != nil {
			return nil, func() {}, err
		}
		if logger != nil {
			logger.Info("project memory store: Shunter RPC", "endpoint", endpoint)
		}
		return client, func() { _ = client.Close() }, nil
	}
	backend, err := projectmemory.OpenBrainBackend(ctx, projectmemory.Config{
		DataDir:    cfg.Memory.ShunterDataDir,
		DurableAck: cfg.Memory.DurableAck,
	})
	if err != nil {
		return nil, func() {}, err
	}
	if logger != nil {
		logger.Info("project memory store: Shunter", "data_dir", cfg.Memory.ShunterDataDir)
	}
	return backend, func() { _ = backend.Close() }, nil
}

func BuildToolExecutionRecorder(cfg *appconfig.Config, queries *appdb.Queries, memoryBackend any) (*tool.ToolExecutionRecorder, error) {
	if cfg != nil && cfg.Memory.Backend == "shunter" {
		recorder, ok := memoryBackend.(projectmemory.ToolExecutionRecorder)
		if !ok || recorder == nil {
			return nil, fmt.Errorf("shunter memory backend requires a project memory tool execution recorder")
		}
		return tool.NewProjectMemoryToolExecutionRecorder(recorder), nil
	}
	return tool.NewToolExecutionRecorder(queries), nil
}

func BuildContextReportStore(cfg *appconfig.Config, database *sql.DB, memoryBackend any) (contextpkg.ReportStore, error) {
	if cfg != nil && cfg.Memory.Backend == "shunter" {
		store, ok := memoryBackend.(projectmemory.ContextReportStore)
		if !ok || store == nil {
			return nil, fmt.Errorf("shunter memory backend requires a project memory context report store")
		}
		return contextpkg.NewProjectMemoryReportStore(store), nil
	}
	return contextpkg.NewSQLiteReportStore(database), nil
}

func BuildChainStore(cfg *appconfig.Config, database *sql.DB, memoryBackend any) (*chain.Store, error) {
	if cfg != nil && cfg.Memory.Backend == "shunter" {
		store, ok := memoryBackend.(projectmemory.ChainStore)
		if !ok || store == nil {
			return nil, fmt.Errorf("shunter memory backend requires a project memory chain store")
		}
		return chain.NewProjectMemoryStore(store), nil
	}
	return chain.NewStore(database), nil
}

// BuildGraphStore opens (or creates) the code-graph SQLite store at the path
// derived from cfg.
func BuildGraphStore(cfg *appconfig.Config) (*codegraph.Store, func(), error) {
	if err := os.MkdirAll(filepath.Dir(cfg.GraphDBPath()), 0o755); err != nil {
		return nil, func() {}, err
	}
	store, err := codegraph.NewStore(cfg.GraphDBPath())
	if err != nil {
		return nil, func() {}, err
	}
	return store, func() { _ = store.Close() }, nil
}

// BuildConventionSource constructs a ConventionSource for the configured brain
// backend. Shunter mode reads conventions through project memory; legacy vault
// mode reads .brain/conventions directly. Disabled-brain mode returns a no-op
// source.
func BuildConventionSource(cfg *appconfig.Config, backend ...brain.Backend) contextpkg.ConventionSource {
	if cfg == nil || !cfg.Brain.Enabled {
		return contextpkg.NoopConventionSource{}
	}
	if cfg.Brain.Backend == "shunter" {
		if len(backend) > 0 && backend[0] != nil {
			return contextpkg.NewBrainBackendConventionSource(backend[0])
		}
		return contextpkg.NoopConventionSource{}
	}
	return contextpkg.NewBrainConventionSource(cfg.BrainVaultPath())
}
