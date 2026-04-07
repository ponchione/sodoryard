package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/ponchione/sirtopham/internal/agent"
	"github.com/ponchione/sirtopham/internal/brain"
	"github.com/ponchione/sirtopham/internal/brain/mcpclient"
	"github.com/ponchione/sirtopham/internal/codeintel/embedder"
	codesearcher "github.com/ponchione/sirtopham/internal/codeintel/searcher"
	"github.com/ponchione/sirtopham/internal/codestore"
	appconfig "github.com/ponchione/sirtopham/internal/config"
	contextpkg "github.com/ponchione/sirtopham/internal/context"
	"github.com/ponchione/sirtopham/internal/conversation"
	appdb "github.com/ponchione/sirtopham/internal/db"
	"github.com/ponchione/sirtopham/internal/logging"
	"github.com/ponchione/sirtopham/internal/provider"
	"github.com/ponchione/sirtopham/internal/provider/anthropic"
	"github.com/ponchione/sirtopham/internal/provider/codex"
	"github.com/ponchione/sirtopham/internal/provider/openai"
	"github.com/ponchione/sirtopham/internal/provider/router"
	"github.com/ponchione/sirtopham/internal/provider/tracking"
	"github.com/ponchione/sirtopham/internal/server"
	"github.com/ponchione/sirtopham/internal/tool"
	"github.com/ponchione/sirtopham/webfs"
)

func newServeCmd(configPath *string) *cobra.Command {
	var (
		portOverride int
		hostOverride string
		devMode      bool
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the sirtopham server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(cmd, *configPath, portOverride, hostOverride, devMode)
		},
	}

	cmd.Flags().IntVar(&portOverride, "port", 0, "Override server port")
	cmd.Flags().StringVar(&hostOverride, "host", "", "Override server host")
	cmd.Flags().BoolVar(&devMode, "dev", false, "Enable development mode")

	return cmd
}

func buildBrainBackend(ctx context.Context, cfg appconfig.BrainConfig, logger *slog.Logger) (brain.Backend, func(), error) {
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

func runServe(cmd *cobra.Command, configPath string, portOverride int, hostOverride string, devMode bool) error {
	// ── 1. Load configuration ──────────────────────────────────────────
	cfg, err := appconfig.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Apply flag overrides.
	if portOverride > 0 {
		cfg.Server.Port = portOverride
	}
	if hostOverride != "" {
		cfg.Server.Host = hostOverride
	}
	if devMode {
		cfg.Server.DevMode = true
	}

	// ── 2. Set up structured logger ────────────────────────────────────
	logger, err := logging.Init(cfg.LogLevel, cfg.LogFormat)
	if err != nil {
		return fmt.Errorf("init logging: %w", err)
	}

	// ── 3. Open database ───────────────────────────────────────────────
	database, err := appdb.OpenDB(cmd.Context(), cfg.DatabasePath())
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close()
	if err := appdb.EnsureMessageSearchIndexesIncludeTools(cmd.Context(), database); err != nil {
		return fmt.Errorf("upgrade message search indexes: %w", err)
	}
	queries := appdb.New(database)

	// Project ID is the project root path.
	projectID := cfg.ProjectRoot
	if err := ensureProjectRecord(cmd.Context(), database, cfg); err != nil {
		return fmt.Errorf("ensure project record: %w", err)
	}

	logger.Info("sirtopham starting",
		"version", version,
		"project", cfg.ProjectRoot,
		"listen", fmt.Sprintf("http://%s:%d", cfg.Server.Host, cfg.Server.Port),
		"dev_mode", cfg.Server.DevMode,
	)

	// ── 4. Build provider router ───────────────────────────────────────
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

	subCallStore := tracking.NewSQLiteSubCallStore(queries)
	provRouter, err := router.NewRouter(routerCfg, subCallStore, logger)
	if err != nil {
		return fmt.Errorf("create router: %w", err)
	}

	// Register configured providers.
	for name, provCfg := range cfg.Providers {
		p, err := buildProvider(name, provCfg)
		if err != nil {
			return fmt.Errorf("build provider %q: %w", name, err)
		}
		if err := provRouter.RegisterProvider(p); err != nil {
			return fmt.Errorf("register provider %q: %w", name, err)
		}
		logProviderAuthStatus(cmd.Context(), logger, name, provCfg, p)
	}
	if err := provRouter.Validate(cmd.Context()); err != nil {
		return fmt.Errorf("validate providers: %w", err)
	}

	// ── 5. Build semantic retrieval runtime ────────────────────────────
	codeStore, err := codestore.Open(cmd.Context(), cfg.CodeLanceDBPath())
	if err != nil {
		return fmt.Errorf("open code vectorstore: %w", err)
	}
	defer codeStore.Close()
	semanticEmbedder := embedder.New(cfg.Embedding)
	semanticSearcher := codesearcher.New(codeStore, semanticEmbedder)

	brainBackend, closeBrainBackend, err := buildBrainBackend(cmd.Context(), cfg.Brain, logger)
	if err != nil {
		return fmt.Errorf("build brain backend: %w", err)
	}
	defer closeBrainBackend()

	retrievalOrchestrator := contextpkg.NewRetrievalOrchestrator(semanticSearcher, nil, nil, brainBackend, cfg.ProjectRoot)
	retrievalOrchestrator.SetLogBrainQueries(cfg.Brain.LogBrainQueries)

	// ── 6. Build tool registry + executor ──────────────────────────────
	registry := tool.NewRegistry()
	tool.RegisterFileTools(registry)
	tool.RegisterGitTools(registry)
	tool.RegisterShellTool(registry, tool.ShellConfig{
		TimeoutSeconds: cfg.Agent.ShellTimeoutSeconds,
		Denylist:       cfg.Agent.ShellDenylist,
	})
	tool.RegisterBrainToolsWithProvider(registry, brainBackend, cfg.Brain, provRouter)
	tool.RegisterSearchTools(registry, semanticSearcher)

	executor := tool.NewExecutor(registry, tool.ExecutorConfig{
		MaxOutputTokens: cfg.Agent.ToolOutputMaxTokens,
		ProjectRoot:     cfg.ProjectRoot,
	}, logger)

	toolRecorder := tool.NewToolExecutionRecorder(queries)
	executor.SetRecorder(toolRecorder)

	adapter := tool.NewAgentLoopAdapter(executor)

	// ── 7. Build conversation manager ──────────────────────────────────
	convManager := conversation.NewManager(database, nil, logger)

	// ── 8. Build context assembler ─────────────────────────────────────
	contextAssembler := contextpkg.NewContextAssembler(
		contextpkg.RuleBasedAnalyzer{},
		contextpkg.HeuristicQueryExtractor{},
		contextpkg.HistoryMomentumTracker{},
		retrievalOrchestrator,
		contextpkg.PriorityBudgetManager{},
		contextpkg.MarkdownSerializer{},
		cfg.Context,
		database,
	)

	// ── 9. Build title generator ───────────────────────────────────────
	titleGen := conversation.NewTitleGen(
		convManager,
		provRouter,
		cfg.Routing.Default.Model,
		logger,
	)

	// ── 10. Build agent loop ───────────────────────────────────────────
	agentLoop := agent.NewAgentLoop(agent.AgentLoopDeps{
		ContextAssembler:    contextAssembler,
		ConversationManager: convManager,
		ProviderRouter:      provRouter,
		ToolExecutor:        adapter,
		ToolDefinitions:     registry.ToolDefinitions(),
		PromptBuilder:       agent.NewPromptBuilder(logger),
		TitleGenerator:      titleGen,
		Config: agent.AgentLoopConfig{
			MaxIterations:              cfg.Agent.MaxIterationsPerTurn,
			LoopDetectionThreshold:     cfg.Agent.LoopDetectionThreshold,
			ExtendedThinking:           cfg.Agent.ExtendedThinking,
			ProviderName:               cfg.Routing.Default.Provider,
			ModelName:                  cfg.Routing.Default.Model,
			EmitContextDebug:           cfg.Context.EmitContextDebug,
			ContextConfig:              cfg.Context,
			ToolResultStoreRoot:        cfg.Agent.ToolResultStoreRoot,
			CacheSystemPrompt:          cfg.Agent.CacheSystemPrompt,
			CacheAssembledContext:      cfg.Agent.CacheAssembledContext,
			CacheConversationHistory:   cfg.Agent.CacheConversationHistory,
			CompressHistoricalResults:  cfg.Agent.CompressHistoricalResults,
			HistorySummarizeAfterTurns: cfg.Agent.HistorySummarizeAfterTurns,
		},
		Logger: logger,
	})
	defer agentLoop.Close()

	// ── 11. Build HTTP server ──────────────────────────────────────────
	serverCfg := server.Config{
		Host:    cfg.Server.Host,
		Port:    cfg.Server.Port,
		DevMode: cfg.Server.DevMode,
	}

	// In production mode, embed the frontend built by `make build`.
	if !cfg.Server.DevMode {
		frontendFS, err := webfs.FS()
		if err != nil {
			logger.Warn("embedded frontend not available", "error", err)
		} else {
			serverCfg.FrontendFS = frontendFS
		}
	}

	srv := server.New(serverCfg, logger)

	// Register handlers.
	runtimeDefaults := server.NewRuntimeDefaults(cfg)
	server.NewConversationHandler(srv, convManager, projectID, logger)
	server.NewWebSocketHandler(srv, agentLoop, convManager, cfg, runtimeDefaults, logger)
	server.NewProjectHandler(srv, cfg, logger)
	server.NewConfigHandler(srv, cfg, provRouter, runtimeDefaults, logger)
	server.NewMetricsHandler(srv, queries, logger)

	// ── 12. Signal handling + graceful shutdown ────────────────────────
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Browser launch (non-blocking, best-effort).
	if cfg.Server.OpenBrowser && !cfg.Server.DevMode {
		go launchBrowser(fmt.Sprintf("http://%s:%d", cfg.Server.Host, cfg.Server.Port), logger)
	}

	// Start server — blocks until context is cancelled.
	if err := srv.Start(ctx); err != nil {
		return fmt.Errorf("server: %w", err)
	}

	// Ordered teardown (database.Close is handled by defer above).
	logger.Info("shutting down")
	agentLoop.Cancel()
	logger.Info("shutdown complete")

	return nil
}

func resolveProviderAPIKey(cfg appconfig.ProviderConfig) string {
	if cfg.APIKey != "" {
		return cfg.APIKey
	}
	if cfg.APIKeyEnv != "" {
		return os.Getenv(cfg.APIKeyEnv)
	}
	return ""
}

// buildProvider constructs a provider.Provider from config.
func buildProvider(name string, cfg appconfig.ProviderConfig) (provider.Provider, error) {
	apiKey := resolveProviderAPIKey(cfg)

	switch cfg.Type {
	case "anthropic":
		var credOpts []anthropic.CredentialOption
		if apiKey != "" {
			credOpts = append(credOpts, anthropic.WithAPIKey(apiKey))
		}
		creds, err := anthropic.NewCredentialManager(credOpts...)
		if err != nil {
			return nil, fmt.Errorf("anthropic credentials: %w", err)
		}
		return anthropic.NewAnthropicProvider(creds), nil

	case "openai-compatible":
		return openai.NewOpenAIProvider(openai.OpenAIConfig{
			Name:          name,
			BaseURL:       cfg.BaseURL,
			APIKey:        apiKey,
			Model:         cfg.Model,
			ContextLength: cfg.ContextLength,
		})

	case "codex":
		var opts []codex.ProviderOption
		if cfg.BaseURL != "" {
			opts = append(opts, codex.WithBaseURL(cfg.BaseURL))
		}
		return codex.NewCodexProvider(opts...)

	default:
		return nil, fmt.Errorf("unsupported provider type: %q", cfg.Type)
	}
}

func ensureProjectRecord(ctx context.Context, database *sql.DB, cfg *appconfig.Config) error {
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now().UTC().Format(time.RFC3339)
	name := filepath.Base(cfg.ProjectRoot)
	_, err := database.ExecContext(ctx, `
INSERT INTO projects(id, name, root_path, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	name = excluded.name,
	root_path = excluded.root_path,
	updated_at = excluded.updated_at
`, cfg.ProjectRoot, name, cfg.ProjectRoot, now, now)
	return err
}

func launchBrowser(url string, logger *slog.Logger) {
	time.Sleep(500 * time.Millisecond) // Let server start.
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		logger.Debug("failed to open browser", "error", err)
	}
}
