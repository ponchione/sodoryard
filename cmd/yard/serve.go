package main

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"os/signal"
	goruntime "runtime"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/ponchione/sodoryard/internal/agent"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/conversation"
	"github.com/ponchione/sodoryard/internal/operator"
	rtpkg "github.com/ponchione/sodoryard/internal/runtime"
	"github.com/ponchione/sodoryard/internal/server"
	"github.com/ponchione/sodoryard/internal/tool"
	"github.com/ponchione/sodoryard/webfs"
)

func newYardServeCmd(configPath *string) *cobra.Command {
	var (
		portOverride  int
		hostOverride  string
		devMode       bool
		allowExternal bool
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the web UI and API server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runYardServe(cmd, *configPath, portOverride, hostOverride, devMode, allowExternal)
		},
	}

	cmd.Flags().IntVar(&portOverride, "port", 0, "Override server port")
	cmd.Flags().StringVar(&hostOverride, "host", "", "Override server host")
	cmd.Flags().BoolVar(&devMode, "dev", false, "Enable development mode")
	cmd.Flags().BoolVar(&allowExternal, "allow-external", false, "Allow listening on a non-loopback host")

	return cmd
}

func runYardServe(cmd *cobra.Command, configPath string, portOverride int, hostOverride string, devMode bool, allowExternal bool) error {
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
	if allowExternal {
		cfg.Server.AllowExternal = true
	}
	if err := validateYardServeBind(cfg.Server.Host, cfg.Server.AllowExternal); err != nil {
		return err
	}

	rt, err := rtpkg.BuildEngineRuntime(cmd.Context(), cfg)
	if err != nil {
		return err
	}
	defer rt.Cleanup()

	logger := rt.Logger
	projectID := cfg.ProjectRoot
	logger.Info("yard serve starting",
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
	tool.RegisterBrainToolsWithProviderRuntimeAndIndex(registry, rt.BrainBackend, rt.BrainSearcher, cfg.Brain, rt.ProviderRouter, rt.Queries, cfg.ProjectRoot)
	tool.RegisterSearchTools(registry, rt.SemanticSearcher)

	executor := tool.NewExecutor(registry, tool.ExecutorConfig{
		MaxOutputTokens: cfg.Agent.ToolOutputMaxTokens,
		ProjectRoot:     cfg.ProjectRoot,
	}, logger)
	executor.SetRecorder(rt.ToolRecorder)
	adapter := tool.NewAgentLoopAdapter(executor)
	titleGen := conversation.NewTitleGen(rt.ConversationManager, rt.ProviderRouter, cfg.Routing.Default.Model, logger)

	agentLoop := agent.NewAgentLoop(agent.AgentLoopDeps{
		ContextAssembler:    rt.ContextAssembler,
		ConversationManager: rt.ConversationManager,
		ProviderRouter:      rt.ProviderRouter,
		ToolExecutor:        adapter,
		ToolDefinitions:     registry.ToolDefinitions(),
		PromptBuilder:       agent.NewPromptBuilder(logger),
		TitleGenerator:      titleGen,
		Config:              rtpkg.BuildAgentLoopConfig(cfg, cfg.Agent.MaxIterationsPerTurn, ""),
		Logger:              logger,
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
	server.NewConversationHandler(srv, rt.ConversationManager, projectID, logger)
	server.NewWebSocketHandler(srv, agentLoop, rt.ConversationManager, cfg, runtimeDefaults, logger)
	server.NewProjectHandler(srv, cfg, logger, rt.MemoryBackend)
	server.NewConfigHandler(srv, cfg, rt.ProviderRouter, runtimeDefaults, logger)
	server.NewMetricsHandler(srv, rt.Queries, logger)
	operatorRuntime := &rtpkg.OrchestratorRuntime{
		Config:              cfg,
		Logger:              logger,
		Database:            rt.Database,
		Queries:             rt.Queries,
		ProviderRouter:      rt.ProviderRouter,
		BrainBackend:        rt.BrainBackend,
		MemoryBackend:       rt.MemoryBackend,
		ConversationManager: rt.ConversationManager,
		ChainStore:          rt.ChainStore,
		Cleanup:             func() {},
	}
	operatorSvc, err := operator.NewForRuntime(operatorRuntime, operator.Options{ProcessID: os.Getpid})
	if err != nil {
		return fmt.Errorf("operator service: %w", err)
	}
	defer operatorSvc.Close()
	server.NewChainInspectorHandler(srv, operatorSvc, logger)

	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if cfg.Server.OpenBrowser && !cfg.Server.DevMode {
		go yardLaunchBrowser(fmt.Sprintf("http://%s:%d", cfg.Server.Host, cfg.Server.Port), logger)
	}
	if err := srv.Start(ctx); err != nil {
		return fmt.Errorf("server: %w", err)
	}
	logger.Info("shutting down")
	agentLoop.Cancel()
	logger.Info("shutdown complete")
	return nil
}

func validateYardServeBind(host string, allowExternal bool) error {
	if allowExternal || yardServeHostIsLoopback(host) {
		return nil
	}
	displayHost := host
	if strings.TrimSpace(displayHost) == "" {
		displayHost = "<empty>"
	}
	return fmt.Errorf("refusing to listen on non-loopback host %q without explicit external access; use --allow-external or set server.allow_external: true only on a trusted network", displayHost)
}

func yardServeHostIsLoopback(host string) bool {
	host = strings.TrimSpace(host)
	if host == "" {
		return false
	}
	host = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func yardLaunchBrowser(url string, logger *slog.Logger) {
	time.Sleep(500 * time.Millisecond)
	var cmd *exec.Cmd
	switch goruntime.GOOS {
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
