package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/ponchione/sirtopham/internal/agent"
	appconfig "github.com/ponchione/sirtopham/internal/config"
	contextpkg "github.com/ponchione/sirtopham/internal/context"
	"github.com/ponchione/sirtopham/internal/conversation"
	appdb "github.com/ponchione/sirtopham/internal/db"
	"github.com/ponchione/sirtopham/internal/provider"
	"github.com/ponchione/sirtopham/internal/provider/anthropic"
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
	var logHandler slog.Handler
	logLevel := parseLogLevel(cfg.LogLevel)
	if cfg.LogFormat == "json" {
		logHandler = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})
	} else {
		logHandler = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})
	}
	logger := slog.New(logHandler)
	slog.SetDefault(logger)

	// ── 3. Open database ───────────────────────────────────────────────
	database, err := appdb.OpenDB(cmd.Context(), cfg.DatabasePath())
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close()
	queries := appdb.New(database)

	// Project ID is the project root path.
	projectID := cfg.ProjectRoot

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
	}
	if cfg.Routing.Fallback.Provider != "" {
		routerCfg.Fallback = &router.RouteTarget{
			Provider: cfg.Routing.Fallback.Provider,
			Model:    cfg.Routing.Fallback.Model,
		}
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
		tracked := tracking.NewTrackedProvider(p, subCallStore, logger)
		if err := provRouter.RegisterProvider(tracked); err != nil {
			return fmt.Errorf("register provider %q: %w", name, err)
		}
		logger.Info("registered provider", "name", name, "type", provCfg.Type)
	}

	// ── 5. Build tool registry + executor ──────────────────────────────
	registry := tool.NewRegistry()
	tool.RegisterFileTools(registry)
	tool.RegisterGitTools(registry)
	tool.RegisterShellTool(registry, tool.ShellConfig{
		TimeoutSeconds: cfg.Agent.ShellTimeoutSeconds,
		Denylist:       cfg.Agent.ShellDenylist,
	})
	tool.RegisterSearchTools(registry, nil) // No semantic searcher in v0.1 serve

	executor := tool.NewExecutor(registry, tool.ExecutorConfig{
		MaxOutputTokens: cfg.Agent.ToolOutputMaxTokens,
		ProjectRoot:     cfg.ProjectRoot,
	}, logger)

	toolRecorder := tool.NewToolExecutionRecorder(queries)
	executor.SetRecorder(toolRecorder)

	adapter := tool.NewAgentLoopAdapter(executor)

	// ── 6. Build conversation manager ──────────────────────────────────
	convManager := conversation.NewManager(database, nil, logger)

	// ── 7. Build context assembler ─────────────────────────────────────
	contextAssembler := contextpkg.NewContextAssembler(
		contextpkg.RuleBasedAnalyzer{},
		contextpkg.HeuristicQueryExtractor{},
		contextpkg.HistoryMomentumTracker{},
		contextpkg.NewRetrievalOrchestrator(nil, nil, nil, cfg.ProjectRoot),
		contextpkg.PriorityBudgetManager{},
		contextpkg.MarkdownSerializer{},
		cfg.Context,
		database,
	)

	// ── 8. Build title generator ───────────────────────────────────────
	titleGen := conversation.NewTitleGen(
		convManager,
		provRouter,
		cfg.Routing.Default.Model,
		logger,
	)

	// ── 9. Build agent loop ────────────────────────────────────────────
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
			CompressHistoricalResults:  cfg.Agent.CompressHistoricalResults,
			HistorySummarizeAfterTurns: cfg.Agent.HistorySummarizeAfterTurns,
		},
		Logger: logger,
	})
	defer agentLoop.Close()

	// ── 10. Build HTTP server ──────────────────────────────────────────
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
	server.NewConversationHandler(srv, convManager, projectID, logger)
	server.NewWebSocketHandler(srv, agentLoop, convManager, projectID, logger)
	server.NewProjectHandler(srv, cfg, logger)
	server.NewConfigHandler(srv, cfg, provRouter, logger)
	server.NewMetricsHandler(srv, queries, logger)

	// ── 11. Signal handling + graceful shutdown ────────────────────────
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

	// Ordered teardown.
	logger.Info("shutting down")
	agentLoop.Cancel()
	database.Close()
	logger.Info("shutdown complete")

	return nil
}

// buildProvider constructs a provider.Provider from config.
func buildProvider(name string, cfg appconfig.ProviderConfig) (provider.Provider, error) {
	apiKey := cfg.APIKey
	if apiKey == "" && cfg.APIKeyEnv != "" {
		apiKey = os.Getenv(cfg.APIKeyEnv)
	}

	switch cfg.Type {
	case "anthropic":
		creds, err := anthropic.NewCredentialManager()
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

	default:
		return nil, fmt.Errorf("unsupported provider type: %q", cfg.Type)
	}
}

func parseLogLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
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
