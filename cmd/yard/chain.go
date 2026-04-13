package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ponchione/sodoryard/internal/agent"
	"github.com/ponchione/sodoryard/internal/chain"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/conversation"
	"github.com/ponchione/sodoryard/internal/id"
	rtpkg "github.com/ponchione/sodoryard/internal/runtime"
)

type yardChainFlags struct {
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

func newYardChainCmd(configPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "chain",
		Short: "Chain orchestration commands",
	}
	cmd.AddCommand(
		newYardChainStartCmd(configPath),
		newYardChainStatusCmd(configPath),
		newYardChainLogsCmd(configPath),
		newYardChainReceiptCmd(configPath),
		newYardChainCancelCmd(configPath),
		newYardChainPauseCmd(configPath),
		newYardChainResumeCmd(configPath),
	)
	return cmd
}

func newYardChainStartCmd(configPath *string) *cobra.Command {
	flags := yardChainFlags{MaxSteps: 100, MaxResolverLoops: 3, MaxDuration: 4 * time.Hour, TokenBudget: 5_000_000}
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start a new chain execution",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateYardChainFlags(flags); err != nil {
				return err
			}
			return yardRunChain(cmd.Context(), *configPath, flags, cmd)
		},
	}
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

func yardRunChain(ctx context.Context, configPath string, flags yardChainFlags, cmd *cobra.Command) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()

	cfg, err := appconfig.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	applyYardChainOverrides(cfg, flags)
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
	rt, err := rtpkg.BuildOrchestratorRuntime(ctx, cfg)
	if err != nil {
		return err
	}
	defer rt.Cleanup()

	chainID := strings.TrimSpace(flags.ChainID)
	if chainID == "" {
		chainID = id.New()
	}

	existing, err := resolveYardExistingChain(ctx, rt.ChainStore, chainID)
	if err != nil {
		return err
	}
	if existing != nil {
		flags, err = populateYardChainFlagsFromExisting(flags, existing)
		if err != nil {
			return err
		}
		if err := prepareYardExistingChainForExecution(ctx, rt.ChainStore, existing, cmd); err != nil {
			return err
		}
	} else {
		if _, err := rt.ChainStore.StartChain(ctx, yardChainSpecFromFlags(chainID, flags)); err != nil {
			return err
		}
		_ = rt.ChainStore.LogEvent(ctx, chainID, "", chain.EventChainStarted, map[string]any{"specs": yardParseSpecs(flags.Specs), "task": flags.Task})
	}

	if flags.DryRun {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", chainID)
		return nil
	}
	if err := registerYardActiveChainExecution(ctx, rt.ChainStore, chainID, existing == nil, existing != nil && existing.Status == "paused"); err != nil {
		return err
	}
	registry, err := rtpkg.BuildOrchestratorRegistry(rt, roleCfg, chainID)
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
	loop := agent.NewAgentLoop(agent.AgentLoopDeps{ContextAssembler: rt.ContextAssembler, ConversationManager: rt.ConversationManager, ProviderRouter: rt.ProviderRouter, ToolExecutor: &rtpkg.RegistryToolExecutor{Registry: registry, ProjectRoot: cfg.ProjectRoot}, ToolDefinitions: registry.ToolDefinitions(), PromptBuilder: agent.NewPromptBuilder(rt.Logger), TitleGenerator: conversation.NewTitleGen(rt.ConversationManager, rt.ProviderRouter, cfg.Routing.Default.Model, rt.Logger), Config: agent.AgentLoopConfig{MaxIterations: roleCfg.MaxTurns, BasePrompt: systemPrompt, ProviderName: cfg.Routing.Default.Provider, ModelName: cfg.Routing.Default.Model, ContextConfig: cfg.Context}, Logger: rt.Logger})
	defer loop.Close()
	turnTask := yardBuildChainTask(flags, chainID)
	if _, err := loop.RunTurn(ctx, agent.RunTurnRequest{ConversationID: conv.ID, TurnNumber: 1, Message: turnTask, ModelContextLimit: limit}); err != nil {
		if handled, handleErr := handleYardChainRunInterruption(ctx, rt.ChainStore, chainID, err, cmd); handled || handleErr != nil {
			return handleErr
		}
		return err
	}
	if err := finalizeYardRequestedChainStatus(ctx, rt.ChainStore, chainID); err != nil {
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

func yardBuildChainTask(flags yardChainFlags, chainID string) string {
	if strings.TrimSpace(flags.Specs) != "" {
		return fmt.Sprintf("You are managing a chain execution. Source specs: %s. Chain ID: %s. Read the specs from the brain, inspect existing receipts/state for this chain, and continue orchestrating from the current point.", strings.Join(yardParseSpecs(flags.Specs), ", "), chainID)
	}
	return fmt.Sprintf("You are managing a chain execution. Task: %s. Chain ID: %s. Inspect existing receipts/state for this chain and continue orchestrating from the current point.", strings.TrimSpace(flags.Task), chainID)
}

func resolveYardExistingChain(ctx context.Context, store *chain.Store, chainID string) (*chain.Chain, error) {
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

func populateYardChainFlagsFromExisting(flags yardChainFlags, existing *chain.Chain) (yardChainFlags, error) {
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

func prepareYardExistingChainForExecution(ctx context.Context, store *chain.Store, existing *chain.Chain, cmd *cobra.Command) error {
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

func registerYardActiveChainExecution(ctx context.Context, store *chain.Store, chainID string, isNew bool, resumed bool) error {
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

func handleYardChainRunInterruption(ctx context.Context, store *chain.Store, chainID string, err error, cmd *cobra.Command) (bool, error) {
	if !errors.Is(err, agent.ErrTurnCancelled) {
		return false, nil
	}
	if err := finalizeYardRequestedChainStatus(ctx, store, chainID); err != nil {
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

func finalizeYardRequestedChainStatus(ctx context.Context, store *chain.Store, chainID string) error {
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

func validateYardChainFlags(flags yardChainFlags) error {
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

func yardChainSpecFromFlags(chainID string, flags yardChainFlags) chain.ChainSpec {
	return chain.ChainSpec{ChainID: chainID, SourceSpecs: yardParseSpecs(flags.Specs), SourceTask: strings.TrimSpace(flags.Task), MaxSteps: flags.MaxSteps, MaxResolverLoops: flags.MaxResolverLoops, MaxDuration: flags.MaxDuration, TokenBudget: flags.TokenBudget}
}

func applyYardChainOverrides(cfg *appconfig.Config, flags yardChainFlags) {
	if strings.TrimSpace(flags.ProjectRoot) != "" {
		cfg.ProjectRoot = strings.TrimSpace(flags.ProjectRoot)
	}
	if strings.TrimSpace(flags.Brain) != "" {
		cfg.Brain.VaultPath = strings.TrimSpace(flags.Brain)
	}
}

func yardParseSpecs(specs string) []string {
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

func newYardChainStatusCmd(configPath *string) *cobra.Command {
	return &cobra.Command{Use: "status [chain-id]", Short: "Show chain status", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := appconfig.Load(*configPath)
		if err != nil {
			return err
		}
		rt, err := rtpkg.BuildOrchestratorRuntime(cmd.Context(), cfg)
		if err != nil {
			return err
		}
		defer rt.Cleanup()
		if len(args) == 0 {
			chains, err := rt.ChainStore.ListChains(cmd.Context(), 20)
			if err != nil {
				return err
			}
			for _, ch := range chains {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\tsteps=%d\ttokens=%d\n", ch.ID, ch.Status, ch.TotalSteps, ch.TotalTokens)
			}
			return nil
		}
		ch, err := rt.ChainStore.GetChain(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		steps, err := rt.ChainStore.ListSteps(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "chain=%s status=%s steps=%d tokens=%d duration=%d summary=%s\n", ch.ID, ch.Status, ch.TotalSteps, ch.TotalTokens, ch.TotalDurationSecs, ch.Summary)
		for _, step := range steps {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "step=%d role=%s status=%s verdict=%s receipt=%s\n", step.SequenceNum, step.Role, step.Status, step.Verdict, step.ReceiptPath)
		}
		return nil
	}}
}

func newYardChainLogsCmd(configPath *string) *cobra.Command {
	return &cobra.Command{Use: "logs <chain-id>", Short: "Show chain event log", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := appconfig.Load(*configPath)
		if err != nil {
			return err
		}
		rt, err := rtpkg.BuildOrchestratorRuntime(cmd.Context(), cfg)
		if err != nil {
			return err
		}
		defer rt.Cleanup()
		events, err := rt.ChainStore.ListEvents(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		for _, event := range events {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%d\t%s\t%s\t%s\n", event.ID, event.CreatedAt.Format(time.RFC3339), event.EventType, event.EventData)
		}
		return nil
	}}
}

func newYardChainReceiptCmd(configPath *string) *cobra.Command {
	return &cobra.Command{Use: "receipt <chain-id> [step]", Short: "Show orchestrator or step receipt", Args: cobra.RangeArgs(1, 2), RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := appconfig.Load(*configPath)
		if err != nil {
			return err
		}
		rt, err := rtpkg.BuildOrchestratorRuntime(cmd.Context(), cfg)
		if err != nil {
			return err
		}
		defer rt.Cleanup()
		path := fmt.Sprintf("receipts/orchestrator/%s.md", args[0])
		if len(args) == 2 {
			steps, err := rt.ChainStore.ListSteps(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			for _, step := range steps {
				if fmt.Sprintf("%d", step.SequenceNum) == args[1] {
					path = step.ReceiptPath
					break
				}
			}
		}
		content, err := rt.BrainBackend.ReadDocument(cmd.Context(), path)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprint(cmd.OutOrStdout(), content)
		return nil
	}}
}

func newYardChainCancelCmd(configPath *string) *cobra.Command {
	return &cobra.Command{Use: "cancel <chain-id>", Short: "Cancel a chain", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return yardSetChainStatus(cmd, *configPath, args[0], "cancelled", chain.EventChainCancelled, "cancelled")
	}}
}

func newYardChainPauseCmd(configPath *string) *cobra.Command {
	return &cobra.Command{Use: "pause <chain-id>", Short: "Pause a chain", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return yardSetChainStatus(cmd, *configPath, args[0], "paused", chain.EventChainPaused, "paused")
	}}
}

func newYardChainResumeCmd(configPath *string) *cobra.Command {
	return &cobra.Command{Use: "resume <chain-id>", Short: "Resume a paused chain", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		flags := yardChainFlags{MaxSteps: 100, MaxResolverLoops: 3, MaxDuration: 4 * time.Hour, TokenBudget: 5_000_000, ChainID: args[0]}
		return yardRunChain(cmd.Context(), *configPath, flags, cmd)
	}}
}

func signalYardActiveChainProcess(ctx context.Context, store *chain.Store, chainID string) error {
	events, err := store.ListEvents(ctx, chainID)
	if err != nil {
		return err
	}
	pid := 0
	for i := len(events) - 1; i >= 0; i-- {
		var payload struct {
			OrchestratorPID int `json:"orchestrator_pid"`
		}
		if err := json.Unmarshal([]byte(events[i].EventData), &payload); err != nil {
			continue
		}
		if payload.OrchestratorPID > 0 {
			pid = payload.OrchestratorPID
			break
		}
	}
	if pid <= 0 {
		return nil
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := proc.Signal(os.Interrupt); err != nil {
		return err
	}
	return nil
}

func validateYardChainStatusTransition(currentStatus string, targetStatus string, chainID string) error {
	_, err := chain.NextControlStatus(currentStatus, targetStatus)
	if err != nil {
		return fmt.Errorf("chain %s %w", chainID, err)
	}
	return nil
}

func yardSetChainStatus(cmd *cobra.Command, configPath string, chainID string, status string, eventType chain.EventType, message string) error {
	cfg, err := appconfig.Load(configPath)
	if err != nil {
		return err
	}
	rt, err := rtpkg.BuildOrchestratorRuntime(cmd.Context(), cfg)
	if err != nil {
		return err
	}
	defer rt.Cleanup()

	existing, err := rt.ChainStore.GetChain(cmd.Context(), chainID)
	if err != nil {
		return err
	}
	if err := validateYardChainStatusTransition(existing.Status, status, chainID); err != nil {
		return err
	}
	nextStatus, err := chain.NextControlStatus(existing.Status, status)
	if err != nil {
		return fmt.Errorf("chain %s %w", chainID, err)
	}
	if existing.Status == nextStatus {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "chain %s already %s\n", chainID, message)
		return nil
	}
	if nextStatus == "cancel_requested" {
		_ = signalYardActiveChainProcess(cmd.Context(), rt.ChainStore, chainID)
	}
	if err := rt.ChainStore.SetChainStatus(cmd.Context(), chainID, nextStatus); err != nil {
		return err
	}
	_ = rt.ChainStore.LogEvent(cmd.Context(), chainID, "", eventType, map[string]any{"status": nextStatus})
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "chain %s %s\n", chainID, yardControlStatusMessage(status, nextStatus, message))
	return nil
}

func yardControlStatusMessage(targetStatus string, persistedStatus string, fallback string) string {
	switch {
	case targetStatus == "paused" && persistedStatus == "pause_requested":
		return "pause requested"
	case targetStatus == "cancelled" && persistedStatus == "cancel_requested":
		return "cancel requested"
	default:
		return fallback
	}
}
