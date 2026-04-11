package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/ponchione/sodoryard/internal/agent"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/conversation"
	"github.com/ponchione/sodoryard/internal/provider"
	"github.com/ponchione/sodoryard/internal/provider/anthropic"
	"github.com/ponchione/sodoryard/internal/provider/codex"
	"github.com/ponchione/sodoryard/internal/provider/openai"
	"github.com/ponchione/sodoryard/internal/server"
	"github.com/ponchione/sodoryard/internal/tool"
	"github.com/ponchione/sodoryard/webfs"
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
	cfg, err := appconfig.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if portOverride > 0 {
		cfg.Server.Port = portOverride
	}
	if hostOverride != "" {
		cfg.Server.Host = hostOverride
	}
	if devMode {
		cfg.Server.DevMode = true
	}

	runtimeBundle, err := buildAppRuntime(cmd.Context(), cfg)
	if err != nil {
		return err
	}
	defer runtimeBundle.Cleanup()

	logger := runtimeBundle.Logger
	projectID := cfg.ProjectRoot
	logger.Info("sirtopham starting",
		"version", version,
		"project", cfg.ProjectRoot,
		"listen", fmt.Sprintf("http://%s:%d", cfg.Server.Host, cfg.Server.Port),
		"dev_mode", cfg.Server.DevMode,
	)

	registry := tool.NewRegistry()
	tool.RegisterFileTools(registry)
	tool.RegisterGitTools(registry)
	tool.RegisterShellTool(registry, tool.ShellConfig{
		TimeoutSeconds: cfg.Agent.ShellTimeoutSeconds,
		Denylist:       cfg.Agent.ShellDenylist,
	})
	tool.RegisterBrainToolsWithProviderRuntimeAndIndex(registry, runtimeBundle.BrainBackend, runtimeBundle.BrainSearcher, cfg.Brain, runtimeBundle.ProviderRouter, runtimeBundle.Queries, cfg.ProjectRoot)
	tool.RegisterSearchTools(registry, runtimeBundle.SemanticSearcher)

	executor := tool.NewExecutor(registry, tool.ExecutorConfig{
		MaxOutputTokens: cfg.Agent.ToolOutputMaxTokens,
		ProjectRoot:     cfg.ProjectRoot,
	}, logger)
	executor.SetRecorder(tool.NewToolExecutionRecorder(runtimeBundle.Queries))
	adapter := tool.NewAgentLoopAdapter(executor)
	titleGen := conversation.NewTitleGen(runtimeBundle.ConversationManager, runtimeBundle.ProviderRouter, cfg.Routing.Default.Model, logger)

	agentLoop := agent.NewAgentLoop(agent.AgentLoopDeps{
		ContextAssembler:    runtimeBundle.ContextAssembler,
		ConversationManager: runtimeBundle.ConversationManager,
		ProviderRouter:      runtimeBundle.ProviderRouter,
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
			StripHistoricalLineNumbers: cfg.Agent.StripHistoricalLineNumbers,
			ElideDuplicateReads:        cfg.Agent.ElideDuplicateReads,
			HistorySummarizeAfterTurns: cfg.Agent.HistorySummarizeAfterTurns,
		},
		Logger: logger,
	})
	defer agentLoop.Close()

	serverCfg := server.Config{Host: cfg.Server.Host, Port: cfg.Server.Port, DevMode: cfg.Server.DevMode}
	if !cfg.Server.DevMode {
		frontendFS, err := webfs.FS()
		if err != nil {
			logger.Warn("embedded frontend not available", "error", err)
		} else {
			serverCfg.FrontendFS = frontendFS
		}
	}

	srv := server.New(serverCfg, logger)
	runtimeDefaults := server.NewRuntimeDefaults(cfg)
	server.NewConversationHandler(srv, runtimeBundle.ConversationManager, projectID, logger)
	server.NewWebSocketHandler(srv, agentLoop, runtimeBundle.ConversationManager, cfg, runtimeDefaults, logger)
	server.NewProjectHandler(srv, cfg, logger)
	server.NewConfigHandler(srv, cfg, runtimeBundle.ProviderRouter, runtimeDefaults, logger)
	server.NewMetricsHandler(srv, runtimeBundle.Queries, logger)

	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if cfg.Server.OpenBrowser && !cfg.Server.DevMode {
		go launchBrowser(fmt.Sprintf("http://%s:%d", cfg.Server.Host, cfg.Server.Port), logger)
	}
	if err := srv.Start(ctx); err != nil {
		return fmt.Errorf("server: %w", err)
	}
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
		return withProviderAlias(name, anthropic.NewAnthropicProvider(creds)), nil

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
		p, err := codex.NewCodexProvider(opts...)
		if err != nil {
			return nil, err
		}
		return withProviderAlias(name, p), nil

	default:
		return nil, fmt.Errorf("unsupported provider type: %q", cfg.Type)
	}
}

func withProviderAlias(name string, inner provider.Provider) provider.Provider {
	if inner == nil || name == "" || inner.Name() == name {
		return inner
	}
	return aliasedProvider{name: name, inner: inner}
}

type aliasedProvider struct {
	name  string
	inner provider.Provider
}

func (p aliasedProvider) Name() string {
	return p.name
}

func (p aliasedProvider) Complete(ctx context.Context, req *provider.Request) (*provider.Response, error) {
	return p.inner.Complete(ctx, req)
}

func (p aliasedProvider) Stream(ctx context.Context, req *provider.Request) (<-chan provider.StreamEvent, error) {
	return p.inner.Stream(ctx, req)
}

func (p aliasedProvider) Models(ctx context.Context) ([]provider.Model, error) {
	return p.inner.Models(ctx)
}

func (p aliasedProvider) Ping(ctx context.Context) error {
	pinger, ok := p.inner.(provider.Pinger)
	if !ok {
		return nil
	}
	return pinger.Ping(ctx)
}

func (p aliasedProvider) AuthStatus(ctx context.Context) (*provider.AuthStatus, error) {
	reporter, ok := p.inner.(provider.AuthStatusReporter)
	if !ok {
		return nil, nil
	}
	status, err := reporter.AuthStatus(ctx)
	if err != nil || status == nil {
		return status, err
	}
	cloned := *status
	cloned.Provider = p.name
	return &cloned, nil
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
