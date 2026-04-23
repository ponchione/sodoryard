package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/ponchione/sodoryard/internal/agent"
	"github.com/ponchione/sodoryard/internal/chain"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/conversation"
	"github.com/ponchione/sodoryard/internal/id"
	rtpkg "github.com/ponchione/sodoryard/internal/runtime"
)

var errYardChainPIDNotRunning = errors.New("chain orchestrator pid not running")

var interruptYardChainPID = func(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := proc.Signal(os.Interrupt); err != nil {
		if errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.ESRCH) {
			return errYardChainPIDNotRunning
		}
		return err
	}
	return nil
}

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
	Watch            bool
	Verbosity        string
	ProjectRoot      string
}

const (
	yardChainWatchFlushTimeout = 2 * time.Second
	chainVerbosityNormal       = "normal"
	chainVerbosityDebug        = "debug"
)

type chainRenderOptions struct {
	Verbosity string
}

type chainTurnRunner interface {
	RunTurn(ctx context.Context, req agent.RunTurnRequest) (*agent.TurnResult, error)
	Close()
}

var buildYardChainRuntime = rtpkg.BuildOrchestratorRuntime
var buildYardChainRegistry = rtpkg.BuildOrchestratorRegistry
var newYardChainTurnRunner = func(deps agent.AgentLoopDeps) chainTurnRunner { return agent.NewAgentLoop(deps) }

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
	flags := yardChainFlags{MaxSteps: 100, MaxResolverLoops: 3, MaxDuration: 4 * time.Hour, TokenBudget: 5_000_000, Watch: true, Verbosity: chainVerbosityNormal}
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
	cmd.Flags().BoolVar(&flags.Watch, "watch", true, "Stream live chain progress to stderr while the command runs")
	cmd.Flags().StringVar(&flags.Verbosity, "verbosity", chainVerbosityNormal, "Chain log verbosity: normal or debug")
	return cmd
}

func yardRunChain(ctx context.Context, configPath string, flags yardChainFlags, cmd *cobra.Command) (err error) {
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
	systemPrompt, _, err := rtpkg.LoadRoleSystemPrompt("orchestrator", cfg.ProjectRoot, roleCfg.SystemPrompt)
	if err != nil {
		return err
	}
	rt, err := buildYardChainRuntime(ctx, cfg)
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
	isNew := existing == nil
	resumed := false
	if existing != nil {
		flags, err = populateYardChainFlagsFromExisting(flags, existing)
		if err != nil {
			return err
		}
		resumed = existing.Status == "paused"
		if err := prepareYardExistingChainForExecution(ctx, rt.ChainStore, existing, cmd); err != nil {
			return err
		}
	} else {
		if _, err := rt.ChainStore.StartChain(ctx, yardChainSpecFromFlags(chainID, flags)); err != nil {
			return err
		}
		_ = rt.ChainStore.LogEvent(ctx, chainID, "", chain.EventChainStarted, map[string]any{"specs": yardParseSpecs(flags.Specs), "task": flags.Task})
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", chainID)
	if flags.DryRun {
		return nil
	}

	executionRegistered := false
	defer func() {
		if err == nil || !executionRegistered {
			return
		}
		if closeErr := closeErroredYardChainExecution(context.WithoutCancel(ctx), rt.ChainStore, chainID, err.Error()); closeErr != nil {
			err = fmt.Errorf("%w (while closing active execution: %v)", err, closeErr)
		}
	}()
	if err := registerYardActiveChainExecution(ctx, rt.ChainStore, chainID, isNew, resumed); err != nil {
		return err
	}
	executionRegistered = true

	registry, err := buildYardChainRegistry(rt, roleCfg, chainID)
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
	loop := newYardChainTurnRunner(agent.AgentLoopDeps{ContextAssembler: rt.ContextAssembler, ConversationManager: rt.ConversationManager, ProviderRouter: rt.ProviderRouter, ToolExecutor: &rtpkg.RegistryToolExecutor{Registry: registry, ProjectRoot: cfg.ProjectRoot}, ToolDefinitions: registry.ToolDefinitions(), PromptBuilder: agent.NewPromptBuilder(rt.Logger), TitleGenerator: conversation.NewTitleGen(rt.ConversationManager, rt.ProviderRouter, cfg.Routing.Default.Model, rt.Logger), Config: agent.AgentLoopConfig{MaxIterations: roleCfg.MaxTurns, BasePrompt: systemPrompt, ProviderName: cfg.Routing.Default.Provider, ModelName: cfg.Routing.Default.Model, ContextConfig: cfg.Context}, Logger: rt.Logger})
	defer loop.Close()
	watch := startYardChainWatch(ctx, cmd.ErrOrStderr(), rt.ChainStore, chainID, flags.Watch, chainRenderOptions{Verbosity: normalizeChainVerbosity(flags.Verbosity)})
	steps, err := rt.ChainStore.ListSteps(ctx, chainID)
	if err != nil {
		return err
	}
	turnTask := yardBuildChainTask(flags, chainID, existingReceiptPaths(steps))
	if _, err := loop.RunTurn(ctx, agent.RunTurnRequest{ConversationID: conv.ID, TurnNumber: 1, Message: turnTask, ModelContextLimit: limit}); err != nil {
		if handled, handleErr := handleYardChainRunInterruption(ctx, rt.ChainStore, chainID, err, cmd); handled || handleErr != nil {
			if handleErr != nil {
				return handleErr
			}
			return watch.wait(yardChainWatchFlushTimeout)
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
	if stored.Status == "failed" {
		return fmt.Errorf("chain %s failed", chainID)
	}
	if err := watch.wait(yardChainWatchFlushTimeout); err != nil {
		return err
	}
	return nil
}

func yardBuildChainTask(flags yardChainFlags, chainID string, receiptPaths []string) string {
	history := "No existing receipt paths were found for this chain yet."
	if len(receiptPaths) > 0 {
		history = fmt.Sprintf("Relevant existing receipt paths to read first: %s.", strings.Join(receiptPaths, ", "))
	}
	if strings.TrimSpace(flags.Specs) != "" {
		return fmt.Sprintf("You are managing a chain execution. Source specs: %s. Chain ID: %s. Read the specs from the brain. %s Continue orchestrating from the current point.", strings.Join(yardParseSpecs(flags.Specs), ", "), chainID, history)
	}
	return fmt.Sprintf("You are managing a chain execution. Task: %s. Chain ID: %s. %s Continue orchestrating from the current point.", strings.TrimSpace(flags.Task), chainID, history)
}

func existingReceiptPaths(steps []chain.Step) []string {
	paths := make([]string, 0, len(steps))
	seen := make(map[string]struct{}, len(steps))
	for _, step := range steps {
		path := strings.TrimSpace(step.ReceiptPath)
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	return paths
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
	resumeReady, err := chain.ResumeExecutionReady(existing.Status)
	if err != nil {
		if errors.Is(err, chain.ErrChainAlreadyRunning) {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "chain %s already running\n", existing.ID)
		}
		return fmt.Errorf("chain %s %w", existing.ID, err)
	}
	if !resumeReady {
		return nil
	}
	if err := store.SetChainStatus(ctx, existing.ID, "running"); err != nil {
		return err
	}
	_ = store.LogEvent(ctx, existing.ID, "", chain.EventChainResumed, map[string]any{"resumed_by": "cli"})
	return nil
}

func registerYardActiveChainExecution(ctx context.Context, store *chain.Store, chainID string, isNew bool, resumed bool) error {
	eventType := chain.EventChainStarted
	payload := map[string]any{"orchestrator_pid": os.Getpid(), "active_execution": true, "execution_id": id.New()}
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
	cleanupCtx := context.WithoutCancel(ctx)
	if err := finalizeYardRequestedChainStatus(cleanupCtx, store, chainID); err != nil {
		return true, err
	}
	ch, loadErr := store.GetChain(cleanupCtx, chainID)
	if loadErr != nil {
		return true, loadErr
	}
	switch ch.Status {
	case "cancelled":
		if err := chain.CloseTerminalizedActiveExecution(cleanupCtx, store, chainID, ch.Status, nil); err != nil {
			return true, err
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "chain %s cancelled\n", chainID)
		return true, nil
	case "paused":
		if err := chain.CloseTerminalizedActiveExecution(cleanupCtx, store, chainID, ch.Status, nil); err != nil {
			return true, err
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "chain %s paused\n", chainID)
		return true, nil
	case "running":
		if ctx.Err() != nil {
			if err := chain.ApplyTerminalChainClosure(cleanupCtx, store, chainID, chain.TerminalChainClosure{Status: "cancelled", EventType: chain.EventChainCancelled, Extra: map[string]any{"finalized_from": "interrupted"}}); err != nil {
				return true, err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "chain %s cancelled\n", chainID)
			return true, nil
		}
		return false, nil
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
		eventType, eventOK := chain.FinalizeControlEventType(ch.Status)
		if eventOK {
			return chain.ApplyTerminalChainClosure(ctx, store, chainID, chain.TerminalChainClosure{
				Status:    finalStatus,
				EventType: eventType,
				Extra:     map[string]any{"finalized_from": ch.Status},
			})
		}
		if err := store.SetChainStatus(ctx, chainID, finalStatus); err != nil {
			return err
		}
	}
	return nil
}

func closeErroredYardChainExecution(ctx context.Context, store *chain.Store, chainID string, summary string) error {
	events := mustListYardChainEvents(ctx, store, chainID)
	if _, ok := chain.LatestActiveExecution(events); !ok {
		return nil
	}
	return chain.ApplyTerminalChainClosure(ctx, store, chainID, chain.TerminalChainClosure{
		Status:    "failed",
		EventType: chain.EventChainCompleted,
		Summary:   &summary,
		Extra:     map[string]any{"summary": summary},
	})
}

func mustListYardChainEvents(ctx context.Context, store *chain.Store, chainID string) []chain.Event {
	events, err := store.ListEvents(ctx, chainID)
	if err != nil {
		return nil
	}
	return events
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
	flags := yardChainFlags{MaxSteps: 100, MaxResolverLoops: 3, MaxDuration: 4 * time.Hour, TokenBudget: 5_000_000, Watch: true, Verbosity: chainVerbosityNormal}
	cmd := &cobra.Command{Use: "resume <chain-id>", Short: "Resume a paused chain", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		flags.ChainID = args[0]
		return yardRunChain(cmd.Context(), *configPath, flags, cmd)
	}}
	cmd.Flags().BoolVar(&flags.Watch, "watch", true, "Stream live chain progress to stderr while the command runs")
	cmd.Flags().StringVar(&flags.Verbosity, "verbosity", chainVerbosityNormal, "Chain log verbosity: normal or debug")
	return cmd
}
