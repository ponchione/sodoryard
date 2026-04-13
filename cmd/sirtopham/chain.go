package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
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
	Specs            string
	Task             string
	ChainID          string
	Brain            string
	MaxSteps         int
	MaxResolverLoops int
	MaxDuration      time.Duration
	TokenBudget      int
	DryRun           bool
	ProjectRoot      string
}

var buildChainRuntime = buildOrchestratorRuntime
var newChainAgentLoop = func(deps agent.AgentLoopDeps) *agent.AgentLoop { return agent.NewAgentLoop(deps) }

func defaultChainFlags() chainFlags {
	return chainFlags{MaxSteps: 100, MaxResolverLoops: 3, MaxDuration: 4 * time.Hour, TokenBudget: 5_000_000}
}

func newChainCmd(configPath *string) *cobra.Command {
	flags := defaultChainFlags()
	cmd := &cobra.Command{Use: "chain", Short: "Start a new chain execution", RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateChainFlags(flags); err != nil {
			return err
		}
		return runChain(cmd.Context(), *configPath, flags, cmd)
	}}
	cmd.Flags().StringVar(&flags.Specs, "specs", "", "Comma-separated brain-relative paths to spec docs")
	cmd.Flags().StringVar(&flags.Task, "task", "", "Free-form task description")
	cmd.Flags().StringVar(&flags.ProjectRoot, "project", "", "Override project root")
	cmd.Flags().StringVar(&flags.Brain, "brain", "", "Override brain vault path")
	cmd.Flags().StringVar(&flags.ChainID, "chain-id", "", "Chain execution identifier")
	cmd.Flags().IntVar(&flags.MaxSteps, "max-steps", 100, "Maximum total agent invocations")
	cmd.Flags().IntVar(&flags.MaxResolverLoops, "max-resolver-loops", 3, "Maximum fix-audit cycles per task")
	cmd.Flags().DurationVar(&flags.MaxDuration, "max-duration", 4*time.Hour, "Wall-clock timeout for entire chain")
	cmd.Flags().IntVar(&flags.TokenBudget, "token-budget", 5_000_000, "Total token ceiling across all agents")
	cmd.Flags().BoolVar(&flags.DryRun, "dry-run", false, "Create the chain row but do not run the orchestrator")
	return cmd
}

func runChain(ctx context.Context, configPath string, flags chainFlags, cmd *cobra.Command) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()

	cfg, err := appconfig.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	applyChainOverrides(cfg, flags)
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

	existing, err := resolveExistingChain(ctx, rt.ChainStore, chainID)
	if err != nil {
		return err
	}
	if existing != nil {
		flags, err = populateChainFlagsFromExisting(flags, existing)
		if err != nil {
			return err
		}
		if err := prepareExistingChainForExecution(ctx, rt.ChainStore, existing, cmd); err != nil {
			return err
		}
	} else {
		if _, err := rt.ChainStore.StartChain(ctx, chainSpecFromFlags(chainID, flags)); err != nil {
			return err
		}
		_ = rt.ChainStore.LogEvent(ctx, chainID, "", chain.EventChainStarted, map[string]any{"specs": parseSpecs(flags.Specs), "task": flags.Task})
	}

	if flags.DryRun {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", chainID)
		return nil
	}

	if err := registerActiveChainExecution(ctx, rt.ChainStore, chainID, existing == nil, existing != nil && existing.Status == "paused"); err != nil {
		return err
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
		if handled, handleErr := handleChainRunInterruption(ctx, rt.ChainStore, chainID, err, cmd); handled || handleErr != nil {
			return handleErr
		}
		return err
	}
	if err := finalizeRequestedChainStatus(ctx, rt.ChainStore, chainID); err != nil {
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
		return fmt.Sprintf("You are managing a chain execution. Source specs: %s. Chain ID: %s. Read the specs from the brain, inspect existing receipts/state for this chain, and continue orchestrating from the current point.", strings.Join(parseSpecs(flags.Specs), ", "), chainID)
	}
	return fmt.Sprintf("You are managing a chain execution. Task: %s. Chain ID: %s. Inspect existing receipts/state for this chain and continue orchestrating from the current point.", strings.TrimSpace(flags.Task), chainID)
}

func resolveExistingChain(ctx context.Context, store *chain.Store, chainID string) (*chain.Chain, error) {
	if strings.TrimSpace(chainID) == "" {
		return nil, nil
	}
	existing, err := store.GetChain(ctx, chainID)
	if err != nil {
		if strings.Contains(err.Error(), "sql: no rows") {
			return nil, nil
		}
		return nil, err
	}
	return existing, nil
}

func populateChainFlagsFromExisting(flags chainFlags, existing *chain.Chain) (chainFlags, error) {
	if existing == nil {
		return flags, nil
	}
	if strings.TrimSpace(flags.Specs) == "" && len(existing.SourceSpecs) > 0 {
		flags.Specs = strings.Join(existing.SourceSpecs, ",")
	}
	if strings.TrimSpace(flags.Task) == "" {
		flags.Task = existing.SourceTask
	}
	if strings.TrimSpace(flags.Task) == "" && strings.TrimSpace(flags.Specs) == "" {
		return flags, fmt.Errorf("chain %s has no stored task/specs to resume from", existing.ID)
	}
	return flags, nil
}

func prepareExistingChainForExecution(ctx context.Context, store *chain.Store, existing *chain.Chain, cmd *cobra.Command) error {
	if existing == nil {
		return nil
	}
	switch existing.Status {
	case "paused", "pause_requested":
		if err := store.SetChainStatus(ctx, existing.ID, "running"); err != nil {
			return err
		}
		_ = store.LogEvent(ctx, existing.ID, "", chain.EventChainResumed, map[string]any{"resumed_by": "cli"})
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "chain %s resumed\n", existing.ID)
		return nil
	case "running":
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "chain %s continuing from existing running state\n", existing.ID)
		return nil
	case "cancelled", "completed", "failed", "partial", "cancel_requested":
		return fmt.Errorf("chain %s is %s and cannot be resumed", existing.ID, existing.Status)
	default:
		return fmt.Errorf("chain %s is in unsupported state %q", existing.ID, existing.Status)
	}
}

func registerActiveChainExecution(ctx context.Context, store *chain.Store, chainID string, isNew bool, resumed bool) error {
	eventType := chain.EventChainStarted
	payload := map[string]any{"orchestrator_pid": os.Getpid()}
	if resumed {
		eventType = chain.EventChainResumed
		payload["resumed_by"] = "cli"
	} else if !isNew {
		eventType = chain.EventChainResumed
		payload["continued_by"] = "cli"
	}
	if err := store.LogEvent(ctx, chainID, "", eventType, payload); err != nil {
		return err
	}
	return nil
}

func handleChainRunInterruption(ctx context.Context, store *chain.Store, chainID string, err error, cmd *cobra.Command) (bool, error) {
	if !errors.Is(err, agent.ErrTurnCancelled) {
		return false, nil
	}
	if err := finalizeRequestedChainStatus(ctx, store, chainID); err != nil {
		return true, err
	}
	ch, loadErr := store.GetChain(ctx, chainID)
	if loadErr != nil {
		return true, loadErr
	}
	switch ch.Status {
	case "cancelled":
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "chain %s cancelled\n", chainID)
		return true, nil
	case "paused":
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "chain %s paused\n", chainID)
		return true, nil
	default:
		return false, nil
	}
}

func finalizeRequestedChainStatus(ctx context.Context, store *chain.Store, chainID string) error {
	ch, err := store.GetChain(ctx, chainID)
	if err != nil {
		return err
	}
	if finalStatus, ok := chain.FinalizeControlStatus(ch.Status); ok {
		if err := store.SetChainStatus(ctx, chainID, finalStatus); err != nil {
			return err
		}
	}
	return nil
}

func validateChainFlags(flags chainFlags) error {
	if strings.TrimSpace(flags.Task) == "" && strings.TrimSpace(flags.Specs) == "" && strings.TrimSpace(flags.ChainID) == "" {
		return fmt.Errorf("one of --task or --specs is required")
	}
	if flags.MaxSteps <= 0 {
		return fmt.Errorf("--max-steps must be > 0")
	}
	if flags.MaxResolverLoops < 0 {
		return fmt.Errorf("--max-resolver-loops must be >= 0")
	}
	if flags.MaxDuration <= 0 {
		return fmt.Errorf("--max-duration must be > 0")
	}
	if flags.TokenBudget <= 0 {
		return fmt.Errorf("--token-budget must be > 0")
	}
	return nil
}

func chainSpecFromFlags(chainID string, flags chainFlags) chain.ChainSpec {
	return chain.ChainSpec{ChainID: chainID, SourceSpecs: parseSpecs(flags.Specs), SourceTask: strings.TrimSpace(flags.Task), MaxSteps: flags.MaxSteps, MaxResolverLoops: flags.MaxResolverLoops, MaxDuration: flags.MaxDuration, TokenBudget: flags.TokenBudget}
}

func applyChainOverrides(cfg *appconfig.Config, flags chainFlags) {
	if strings.TrimSpace(flags.ProjectRoot) != "" {
		cfg.ProjectRoot = strings.TrimSpace(flags.ProjectRoot)
	}
	if strings.TrimSpace(flags.Brain) != "" {
		cfg.Brain.VaultPath = strings.TrimSpace(flags.Brain)
	}
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
