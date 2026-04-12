package runtime

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/ponchione/sodoryard/internal/brain"
	"github.com/ponchione/sodoryard/internal/brain/mcpclient"
	"github.com/ponchione/sodoryard/internal/codeintel"
	"github.com/ponchione/sodoryard/internal/codeintel/embedder"
	codegraph "github.com/ponchione/sodoryard/internal/codeintel/graph"
	codesearcher "github.com/ponchione/sodoryard/internal/codeintel/searcher"
	"github.com/ponchione/sodoryard/internal/codestore"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	contextpkg "github.com/ponchione/sodoryard/internal/context"
	"github.com/ponchione/sodoryard/internal/conversation"
	appdb "github.com/ponchione/sodoryard/internal/db"
	"github.com/ponchione/sodoryard/internal/logging"
	"github.com/ponchione/sodoryard/internal/provider/router"
	"github.com/ponchione/sodoryard/internal/provider/tracking"
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
	SemanticSearcher    *codesearcher.Searcher
	BrainSearcher       *contextpkg.HybridBrainSearcher
	ConversationManager *conversation.Manager
	ContextAssembler    *contextpkg.ContextAssembler
	Cleanup             func()
}

// BuildEngineRuntime constructs a fully initialised EngineRuntime from cfg.
// It mirrors cmd/tidmouth's buildAppRuntime, delegating to the already-
// extracted helpers in this package (ChainCleanup, EnsureProjectRecord,
// BuildProvider, LogProviderAuthStatus).
func BuildEngineRuntime(ctx context.Context, cfg *appconfig.Config) (*EngineRuntime, error) {
	if cfg == nil {
		return nil, fmt.Errorf("runtime config is required")
	}

	logger, err := logging.Init(cfg.LogLevel, cfg.LogFormat)
	if err != nil {
		return nil, fmt.Errorf("init logging: %w", err)
	}

	database, err := appdb.OpenDB(ctx, cfg.DatabasePath())
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	cleanup := func() {
		_ = database.Close()
	}
	closeOnError := func(err error) (*EngineRuntime, error) {
		cleanup()
		return nil, err
	}

	// Bootstrap the base schema if this is a fresh .yard/yard.db. Without
	// this, engine commands against a project that has never been init-ed
	// fail inside the Ensure* schema-upgrade helpers because they try to
	// ALTER tables that do not exist yet.
	if _, err := appdb.InitIfNeeded(ctx, database); err != nil {
		return closeOnError(fmt.Errorf("init database schema: %w", err))
	}
	if err := appdb.EnsureMessageSearchIndexesIncludeTools(ctx, database); err != nil {
		return closeOnError(fmt.Errorf("upgrade message search indexes: %w", err))
	}
	if err := appdb.EnsureContextReportsIncludeTokenBudget(ctx, database); err != nil {
		return closeOnError(fmt.Errorf("upgrade context report token budget storage: %w", err))
	}
	if err := appdb.EnsureChainSchema(ctx, database); err != nil {
		return closeOnError(fmt.Errorf("ensure chain schema: %w", err))
	}
	queries := appdb.New(database)
	if err := EnsureProjectRecord(ctx, database, cfg); err != nil {
		return closeOnError(fmt.Errorf("ensure project record: %w", err))
	}

	routerCfg := router.RouterConfig{
		Default: router.RouteTarget{
			Provider: cfg.Routing.Default.Provider,
			Model:    cfg.Routing.Default.Model,
		},
		Fallback: router.RouteTarget{
			Provider: cfg.Routing.Fallback.Provider,
			Model:    cfg.Routing.Fallback.Model,
		},
	}
	provRouter, err := router.NewRouter(routerCfg, tracking.NewSQLiteSubCallStore(queries), logger)
	if err != nil {
		return closeOnError(fmt.Errorf("create router: %w", err))
	}
	for name, provCfg := range cfg.Providers {
		p, err := BuildProvider(name, provCfg)
		if err != nil {
			return closeOnError(fmt.Errorf("build provider %q: %w", name, err))
		}
		if err := provRouter.RegisterProvider(p); err != nil {
			return closeOnError(fmt.Errorf("register provider %q: %w", name, err))
		}
		LogProviderAuthStatus(ctx, logger, name, provCfg, p)
	}
	if err := provRouter.Validate(ctx); err != nil {
		return closeOnError(fmt.Errorf("validate providers: %w", err))
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

	graphStore, closeGraphStore, err := BuildGraphStore(cfg)
	if err != nil {
		return closeOnError(fmt.Errorf("build graph store: %w", err))
	}
	cleanup = ChainCleanup(cleanup, closeGraphStore)

	conventionSource := BuildConventionSource(cfg)
	retrievalOrchestrator := contextpkg.NewRetrievalOrchestrator(semanticSearcher, graphStore, conventionSource, brainSearcher, cfg.ProjectRoot)
	retrievalOrchestrator.SetLogBrainQueries(cfg.Brain.LogBrainQueries)
	retrievalOrchestrator.SetBrainConfig(cfg.Brain)
	budgetManager := contextpkg.PriorityBudgetManager{}
	budgetManager.SetBrainConfig(cfg.Brain)

	convManager := conversation.NewManager(database, nil, logger)
	contextAssembler := contextpkg.NewContextAssembler(
		contextpkg.RuleBasedAnalyzer{},
		contextpkg.HeuristicQueryExtractor{},
		contextpkg.HistoryMomentumTracker{},
		retrievalOrchestrator,
		budgetManager,
		contextpkg.MarkdownSerializer{},
		cfg.Context,
		database,
	)

	return &EngineRuntime{
		Config:              cfg,
		Logger:              logger,
		Database:            database,
		Queries:             queries,
		ProviderRouter:      provRouter,
		BrainBackend:        brainBackend,
		SemanticSearcher:    semanticSearcher,
		BrainSearcher:       brainSearcher,
		ConversationManager: convManager,
		ContextAssembler:    contextAssembler,
		Cleanup:             cleanup,
	}, nil
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
	client, err := mcpclient.Connect(ctx, cfg.VaultPath)
	if err != nil {
		return nil, func() {}, err
	}
	logger.Info("brain backend: MCP (in-process)", "vault", cfg.VaultPath)
	return client, func() { _ = client.Close() }, nil
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

// BuildConventionSource constructs a ConventionSource backed by the brain
// vault at the path derived from cfg when the brain is enabled. Disabled-brain
// mode returns a no-op source so context assembly does not read convention
// documents from the vault.
func BuildConventionSource(cfg *appconfig.Config) contextpkg.ConventionSource {
	if cfg == nil || !cfg.Brain.Enabled {
		return contextpkg.NoopConventionSource{}
	}
	return contextpkg.NewBrainConventionSource(cfg.BrainVaultPath())
}
