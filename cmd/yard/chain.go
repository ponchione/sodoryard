package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
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

type yardBackgroundChainExecutionRequest struct {
	ChainID     string
	IsNew       bool
	Resumed     bool
	ProjectRoot string
	Brain       string
}

type yardBackgroundChainChildHandle struct {
	wait <-chan error
}

var launchYardChainBackgroundChild = func(configPath string, req yardBackgroundChainExecutionRequest) (*yardBackgroundChainChildHandle, error) {
	exePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve executable: %w", err)
	}
	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", os.DevNull, err)
	}
	defer devNull.Close()
	args := make([]string, 0, 10)
	if strings.TrimSpace(configPath) != "" {
		args = append(args, "--config", configPath)
	}
	args = append(args, "chain", "_run-background", "--chain-id", req.ChainID)
	if strings.TrimSpace(req.ProjectRoot) != "" {
		args = append(args, "--project", req.ProjectRoot)
	}
	if strings.TrimSpace(req.Brain) != "" {
		args = append(args, "--brain", req.Brain)
	}
	if req.IsNew {
		args = append(args, "--is-new")
	}
	if req.Resumed {
		args = append(args, "--resumed")
	}
	cmd := exec.Command(exePath, args...)
	cmd.Stdin = devNull
	cmd.Stdout = devNull
	cmd.Stderr = devNull
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start background chain child: %w", err)
	}
	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
		close(waitCh)
	}()
	return &yardBackgroundChainChildHandle{wait: waitCh}, nil
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
		newYardChainRunBackgroundCmd(configPath),
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

func newYardChainRunBackgroundCmd(configPath *string) *cobra.Command {
	var req yardBackgroundChainExecutionRequest
	cmd := &cobra.Command{
		Use:    "_run-background",
		Short:  "Run a chain execution in hidden background worker mode",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(req.ChainID) == "" {
				return fmt.Errorf("--chain-id is required")
			}
			return yardRunChainBackgroundWorker(cmd.Context(), *configPath, req, cmd)
		},
	}
	cmd.Flags().StringVar(&req.ChainID, "chain-id", "", "Chain execution identifier")
	cmd.Flags().StringVar(&req.ProjectRoot, "project", "", "Override project root")
	cmd.Flags().StringVar(&req.Brain, "brain", "", "Override brain vault path")
	cmd.Flags().BoolVar(&req.IsNew, "is-new", false, "Whether this background execution owns a newly created chain")
	cmd.Flags().BoolVar(&req.Resumed, "resumed", false, "Whether this background execution is resuming a paused chain")
	return cmd
}

func yardRunChain(ctx context.Context, configPath string, flags yardChainFlags, cmd *cobra.Command) error {
	cfg, err := appconfig.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	applyYardChainOverrides(cfg, flags)
	if err := cfg.Validate(); err != nil {
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
	launchReq := yardBackgroundChainExecutionRequest{ChainID: chainID, IsNew: existing == nil, ProjectRoot: strings.TrimSpace(flags.ProjectRoot), Brain: strings.TrimSpace(flags.Brain)}
	if existing != nil {
		flags, err = populateYardChainFlagsFromExisting(flags, existing)
		if err != nil {
			return err
		}
		launchReq.Resumed = existing.Status == "paused"
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
	child, err := launchYardChainBackgroundChild(configPath, launchReq)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", chainID)
	if err := waitForYardChainBackgroundHandshake(ctx, rt.ChainStore, chainID, os.Getpid(), child, 5*time.Second); err != nil {
		return err
	}
	if !flags.Watch {
		return nil
	}
	watchCtx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()
	watch := startYardChainWatch(watchCtx, cmd.ErrOrStderr(), rt.ChainStore, chainID, true, chainRenderOptions{Verbosity: normalizeChainVerbosity(flags.Verbosity)})
	if err := watch.wait(yardChainWatchFlushTimeout); err != nil {
		return err
	}
	if watchCtx.Err() != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "detached from live output; chain %s continues running\n", chainID)
	}
	return nil
}

func yardRunChainBackgroundWorker(ctx context.Context, configPath string, req yardBackgroundChainExecutionRequest, cmd *cobra.Command) (err error) {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()
	cfg, err := appconfig.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	applyYardChainOverrides(cfg, yardChainFlags{ProjectRoot: req.ProjectRoot, Brain: req.Brain})
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
	existing, err := resolveYardExistingChain(ctx, rt.ChainStore, req.ChainID)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("chain %s not found", req.ChainID)
	}
	flags, err := populateYardChainFlagsFromExisting(yardChainFlags{ChainID: req.ChainID}, existing)
	if err != nil {
		return err
	}
	executionRegistered := false
	defer func() {
		if err == nil || !executionRegistered {
			return
		}
		if closeErr := closeErroredYardChainExecution(context.WithoutCancel(ctx), rt.ChainStore, req.ChainID, err.Error()); closeErr != nil {
			err = fmt.Errorf("%w (while closing active execution: %v)", err, closeErr)
		}
	}()
	if err := registerYardActiveChainExecution(ctx, rt.ChainStore, req.ChainID, req.IsNew, req.Resumed); err != nil {
		return err
	}
	executionRegistered = true
	registry, err := buildYardChainRegistry(rt, roleCfg, req.ChainID)
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
	steps, err := rt.ChainStore.ListSteps(ctx, req.ChainID)
	if err != nil {
		return err
	}
	turnTask := yardBuildChainTask(flags, req.ChainID, existingReceiptPaths(steps))
	if _, err := loop.RunTurn(ctx, agent.RunTurnRequest{ConversationID: conv.ID, TurnNumber: 1, Message: turnTask, ModelContextLimit: limit}); err != nil {
		if handled, handleErr := handleYardChainRunInterruption(ctx, rt.ChainStore, req.ChainID, err, cmd); handled || handleErr != nil {
			return handleErr
		}
		return err
	}
	if err := finalizeYardRequestedChainStatus(ctx, rt.ChainStore, req.ChainID); err != nil {
		return err
	}
	stored, err := rt.ChainStore.GetChain(ctx, req.ChainID)
	if err != nil {
		return err
	}
	if stored.Status == "failed" {
		return fmt.Errorf("chain %s failed", req.ChainID)
	}
	return nil
}

func waitForYardChainBackgroundHandshake(ctx context.Context, store *chain.Store, chainID string, parentPID int, child *yardBackgroundChainChildHandle, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for {
		events, err := store.ListEvents(ctx, chainID)
		if err == nil {
			if exec, ok := chain.LatestActiveExecution(events); ok && exec.OrchestratorPID > 0 && exec.OrchestratorPID != parentPID {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case waitErr, ok := <-child.wait:
			if ok {
				events, _ := store.ListEvents(context.WithoutCancel(ctx), chainID)
				if exec, active := chain.LatestActiveExecution(events); active && exec.OrchestratorPID > 0 && exec.OrchestratorPID != parentPID {
					return nil
				}
				if waitErr != nil {
					return fmt.Errorf("background chain child exited before registering execution: %w", waitErr)
				}
				return fmt.Errorf("background chain child exited before registering execution")
			}
		default:
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("background chain child did not register active execution for chain %s", chainID)
		}
		time.Sleep(25 * time.Millisecond)
	}
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

type yardChainWatchHandle struct {
	cancel func()
	done   <-chan error
}

func startYardChainWatch(ctx context.Context, out io.Writer, store *chain.Store, chainID string, enabled bool, opts chainRenderOptions) *yardChainWatchHandle {
	if !enabled {
		return &yardChainWatchHandle{}
	}
	watchCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- yardFollowChainEvents(watchCtx, out, store, chainID, 0, opts)
	}()
	return &yardChainWatchHandle{cancel: cancel, done: done}
}

func (h *yardChainWatchHandle) wait(timeout time.Duration) error {
	if h == nil || h.done == nil {
		return nil
	}
	if timeout <= 0 {
		timeout = yardChainWatchFlushTimeout
	}
	select {
	case err := <-h.done:
		if h.cancel != nil {
			h.cancel()
		}
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	case <-time.After(timeout):
		if h.cancel != nil {
			h.cancel()
		}
		select {
		case err := <-h.done:
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		case <-time.After(250 * time.Millisecond):
			return nil
		}
	}
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
	var follow bool
	var verbosity string
	cmd := &cobra.Command{Use: "logs <chain-id>", Short: "Show chain event log", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := appconfig.Load(*configPath)
		if err != nil {
			return err
		}
		rt, err := rtpkg.BuildOrchestratorRuntime(cmd.Context(), cfg)
		if err != nil {
			return err
		}
		defer rt.Cleanup()
		if !follow {
			events, err := rt.ChainStore.ListEvents(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			for _, event := range events {
				_, _ = fmt.Fprint(cmd.OutOrStdout(), formatChainEvent(event, chainRenderOptions{Verbosity: normalizeChainVerbosity(verbosity)}))
			}
			return nil
		}
		return yardFollowChainEvents(cmd.Context(), cmd.OutOrStdout(), rt.ChainStore, args[0], 0, chainRenderOptions{Verbosity: normalizeChainVerbosity(verbosity)})
	}}
	cmd.Flags().BoolVar(&follow, "follow", false, "Poll and print new events until the chain stops")
	cmd.Flags().StringVar(&verbosity, "verbosity", chainVerbosityNormal, "Chain log verbosity: normal or debug")
	return cmd
}

func yardStreamChainEvents(ctx context.Context, out io.Writer, store *chain.Store, chainID string, afterID int64, opts chainRenderOptions) (int64, error) {
	events, err := store.ListEventsSince(ctx, chainID, afterID)
	if err != nil {
		return afterID, err
	}
	for _, event := range events {
		_, _ = fmt.Fprint(out, formatChainEvent(event, opts))
		afterID = event.ID
	}
	return afterID, nil
}

func yardFollowChainEvents(ctx context.Context, out io.Writer, store *chain.Store, chainID string, afterID int64, opts chainRenderOptions) error {
	for {
		nextAfterID, err := yardStreamChainEvents(ctx, out, store, chainID, afterID, opts)
		if err != nil {
			return err
		}
		afterID = nextAfterID
		ch, err := store.GetChain(ctx, chainID)
		if err != nil {
			return err
		}
		if ch.Status != "running" && ch.Status != "pause_requested" && ch.Status != "cancel_requested" {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}
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
	flags := yardChainFlags{MaxSteps: 100, MaxResolverLoops: 3, MaxDuration: 4 * time.Hour, TokenBudget: 5_000_000, Watch: true, Verbosity: chainVerbosityNormal}
	cmd := &cobra.Command{Use: "resume <chain-id>", Short: "Resume a paused chain", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		flags.ChainID = args[0]
		return yardRunChain(cmd.Context(), *configPath, flags, cmd)
	}}
	cmd.Flags().BoolVar(&flags.Watch, "watch", true, "Stream live chain progress to stderr while the command runs")
	cmd.Flags().StringVar(&flags.Verbosity, "verbosity", chainVerbosityNormal, "Chain log verbosity: normal or debug")
	return cmd
}

func signalYardActiveChainProcess(ctx context.Context, store *chain.Store, chainID string) error {
	events, err := store.ListEvents(ctx, chainID)
	if err != nil {
		return err
	}
	exec, ok := chain.LatestActiveExecution(events)
	if !ok || exec.OrchestratorPID <= 0 {
		return nil
	}
	if err := interruptYardChainPID(exec.OrchestratorPID); err != nil {
		if errors.Is(err, errYardChainPIDNotRunning) {
			return nil
		}
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

func formatChainEvent(event chain.Event, opts chainRenderOptions) string {
	formatted := formatKnownChainEvent(event, opts)
	if formatted == "" {
		if event.EventType == chain.EventStepOutput {
			return ""
		}
		formatted = event.EventData
		if formatted == "" {
			return ""
		}
	}
	return fmt.Sprintf("%d\t%s\t%s\t%s\n", event.ID, event.CreatedAt.Format(time.RFC3339), event.EventType, formatted)
}

func shouldSuppressStepOutput(line string, opts chainRenderOptions) bool {
	if normalizeChainVerbosity(opts.Verbosity) == chainVerbosityDebug {
		return false
	}
	trimmed := strings.TrimSpace(strings.ToLower(line))
	for _, noisy := range []string{
		"provider registered",
		"registered provider",
		"provider failed ping() startup validation",
		"brain backend: mcp (in-process)",
		"status: waiting_for_llm",
		"status: executing_tools",
		"status: assembling_context",
	} {
		if strings.Contains(trimmed, noisy) {
			return true
		}
	}
	return false
}

func normalizeChainVerbosity(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), chainVerbosityDebug) {
		return chainVerbosityDebug
	}
	return chainVerbosityNormal
}

func formatKnownChainEvent(event chain.Event, opts chainRenderOptions) string {
	var payload map[string]any
	if err := json.Unmarshal([]byte(event.EventData), &payload); err != nil {
		return ""
	}
	quoted := func(key string) string {
		value, ok := payload[key]
		if !ok {
			return ""
		}
		text := strings.TrimSpace(fmt.Sprint(value))
		if text == "" || text == "<nil>" {
			return ""
		}
		return fmt.Sprintf(`%s=%q`, key, text)
	}
	plain := func(key string) string {
		value, ok := payload[key]
		if !ok {
			return ""
		}
		text := strings.TrimSpace(fmt.Sprint(value))
		if text == "" || text == "<nil>" {
			return ""
		}
		return fmt.Sprintf("%s=%s", key, text)
	}
	join := func(parts ...string) string {
		filtered := make([]string, 0, len(parts))
		for _, part := range parts {
			if strings.TrimSpace(part) != "" {
				filtered = append(filtered, part)
			}
		}
		return strings.Join(filtered, " ")
	}

	switch event.EventType {
	case chain.EventChainStarted:
		return join(quoted("task"), plain("orchestrator_pid"), plain("execution_id"), plain("continued_by"), plain("resumed_by"))
	case chain.EventChainResumed:
		return join(plain("resumed_by"), plain("continued_by"), plain("orchestrator_pid"), plain("execution_id"))
	case chain.EventStepStarted:
		return join(plain("role"), quoted("task"), plain("receipt_path"))
	case chain.EventStepOutput:
		line := strings.TrimSpace(fmt.Sprint(payload["line"]))
		if line == "" || line == "<nil>" || shouldSuppressStepOutput(line, opts) {
			return ""
		}
		stream := strings.TrimSpace(fmt.Sprint(payload["stream"]))
		if stream == "" || stream == "<nil>" {
			stream = "stdout"
		}
		return fmt.Sprintf("[%s] %s", stream, line)
	case chain.EventStepCompleted, chain.EventStepFailed:
		return join(plain("role"), plain("verdict"), plain("tokens_used"), plain("duration_secs"), plain("exit_code"), quoted("error"))
	case chain.EventResolverLoop:
		return join(plain("count"), quoted("task_context"))
	case chain.EventReindexStarted, chain.EventReindexCompleted, chain.EventSafetyLimitHit, chain.EventChainPaused, chain.EventChainCancelled, chain.EventChainCompleted:
		parts := make([]string, 0, len(payload))
		for _, key := range []string{"status", "summary", "duration_secs", "limit", "role", "exit_code", "execution_id", "finalized_from"} {
			if key == "summary" {
				parts = append(parts, quoted(key))
				continue
			}
			parts = append(parts, plain(key))
		}
		return join(parts...)
	default:
		return ""
	}
}
