package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ponchione/sodoryard/internal/agent"
	"github.com/ponchione/sodoryard/internal/chain"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/conversation"
	"github.com/ponchione/sodoryard/internal/id"
	rtpkg "github.com/ponchione/sodoryard/internal/runtime"
	"github.com/spf13/cobra"
)

type chainFlags struct {
	Specs       string
	Task        string
	ChainID     string
	MaxSteps    int
	MaxDuration time.Duration
	TokenBudget int
	DryRun      bool
}

var buildChainRuntime = buildOrchestratorRuntime
var newChainAgentLoop = func(deps agent.AgentLoopDeps) *agent.AgentLoop { return agent.NewAgentLoop(deps) }

func newChainCmd(configPath *string) *cobra.Command {
	flags := chainFlags{MaxSteps: 100, MaxDuration: 4 * time.Hour, TokenBudget: 5_000_000}
	cmd := &cobra.Command{Use: "chain", Short: "Start a new chain execution", RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(flags.Task) == "" && strings.TrimSpace(flags.Specs) == "" {
			return fmt.Errorf("one of --task or --specs is required")
		}
		return runChain(cmd.Context(), *configPath, flags, cmd)
	}}
	cmd.Flags().StringVar(&flags.Specs, "specs", "", "Comma-separated brain-relative paths to spec docs")
	cmd.Flags().StringVar(&flags.Task, "task", "", "Free-form task description")
	cmd.Flags().StringVar(&flags.ChainID, "chain-id", "", "Chain execution identifier")
	cmd.Flags().IntVar(&flags.MaxSteps, "max-steps", 100, "Maximum total agent invocations")
	cmd.Flags().DurationVar(&flags.MaxDuration, "max-duration", 4*time.Hour, "Wall-clock timeout for entire chain")
	cmd.Flags().IntVar(&flags.TokenBudget, "token-budget", 5_000_000, "Total token ceiling across all agents")
	cmd.Flags().BoolVar(&flags.DryRun, "dry-run", false, "Create the chain row but do not run the orchestrator")
	return cmd
}

func runChain(ctx context.Context, configPath string, flags chainFlags, cmd *cobra.Command) error {
	cfg, err := appconfig.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	roleCfg, ok := cfg.AgentRoles["orchestrator"]
	if !ok {
		return fmt.Errorf("agent role %q not found in config", "orchestrator")
	}
	systemPrompt, err := rtpkg.LoadRoleSystemPrompt(cfg.ProjectRoot, roleCfg.SystemPrompt)
	if err != nil {
		return err
	}
	rt, err := buildChainRuntime(ctx, cfg)
	if err != nil {
		return err
	}
	defer rt.Cleanup()
	chainID := strings.TrimSpace(flags.ChainID)
	if chainID == "" {
		chainID = id.New()
	}
	if _, err := rt.ChainStore.StartChain(ctx, chainSpecFromFlags(chainID, flags)); err != nil {
		return err
	}
	_ = rt.ChainStore.LogEvent(ctx, chainID, "", "chain_started", map[string]any{"specs": parseSpecs(flags.Specs), "task": flags.Task})
	if flags.DryRun {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", chainID)
		return nil
	}
	registry, err := buildOrchestratorRegistry(rt, roleCfg, chainID)
	if err != nil {
		return err
	}
	conv, err := rt.ConversationManager.Create(ctx, cfg.ProjectRoot, conversation.WithProvider(cfg.Routing.Default.Provider), conversation.WithModel(cfg.Routing.Default.Model))
	if err != nil {
		return fmt.Errorf("create conversation: %w", err)
	}
	limit, err := rtpkg.ResolveModelContextLimit(cfg, cfg.Routing.Default.Provider)
	if err != nil {
		return err
	}
	loop := newChainAgentLoop(agent.AgentLoopDeps{ContextAssembler: rt.ContextAssembler, ConversationManager: rt.ConversationManager, ProviderRouter: rt.ProviderRouter, ToolExecutor: &rtpkg.RegistryToolExecutor{Registry: registry, ProjectRoot: cfg.ProjectRoot}, ToolDefinitions: registry.ToolDefinitions(), PromptBuilder: agent.NewPromptBuilder(rt.Logger), TitleGenerator: conversation.NewTitleGen(rt.ConversationManager, rt.ProviderRouter, cfg.Routing.Default.Model, rt.Logger), Config: agent.AgentLoopConfig{MaxIterations: roleCfg.MaxTurns, BasePrompt: systemPrompt, ProviderName: cfg.Routing.Default.Provider, ModelName: cfg.Routing.Default.Model, ContextConfig: cfg.Context}, Logger: rt.Logger})
	defer loop.Close()
	turnTask := buildChainTask(flags, chainID)
	if _, err := loop.RunTurn(ctx, agent.RunTurnRequest{ConversationID: conv.ID, TurnNumber: 1, Message: turnTask, ModelContextLimit: limit}); err != nil {
		return err
	}
	stored, err := rt.ChainStore.GetChain(ctx, chainID)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", chainID)
	if stored.Status == "failed" {
		return fmt.Errorf("chain %s failed", chainID)
	}
	return nil
}

func buildChainTask(flags chainFlags, chainID string) string {
	if strings.TrimSpace(flags.Specs) != "" {
		return fmt.Sprintf("You are managing a new chain execution. Source specs: %s. Chain ID: %s. Read the specs from the brain and begin orchestrating.", strings.Join(parseSpecs(flags.Specs), ", "), chainID)
	}
	return fmt.Sprintf("You are managing a new chain execution. Task: %s. Chain ID: %s. Begin orchestrating.", strings.TrimSpace(flags.Task), chainID)
}

func chainSpecFromFlags(chainID string, flags chainFlags) chain.ChainSpec {
	return chain.ChainSpec{ChainID: chainID, SourceSpecs: parseSpecs(flags.Specs), SourceTask: strings.TrimSpace(flags.Task), MaxSteps: flags.MaxSteps, MaxResolverLoops: 3, MaxDuration: flags.MaxDuration, TokenBudget: flags.TokenBudget}
}

func parseSpecs(specs string) []string {
	if strings.TrimSpace(specs) == "" {
		return nil
	}
	parts := strings.Split(specs, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

