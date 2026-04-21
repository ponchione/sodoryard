package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
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
	Watch            bool
	Verbosity        string
	ProjectRoot      string
}

const chainWatchFlushTimeout = 2 * time.Second

type chainTurnRunner interface {
	RunTurn(ctx context.Context, req agent.RunTurnRequest) (*agent.TurnResult, error)
	Close()
}

var buildChainRuntime = buildOrchestratorRuntime
var buildChainRegistry = buildOrchestratorRegistry
var newChainTurnRunner = func(deps agent.AgentLoopDeps) chainTurnRunner { return agent.NewAgentLoop(deps) }

type backgroundChainExecutionRequest struct {
	ChainID     string
	IsNew       bool
	Resumed     bool
	ProjectRoot string
	Brain       string
}

type backgroundChainChildHandle struct {
	wait <-chan error
}

var launchChainBackgroundChild = func(configPath string, req backgroundChainExecutionRequest) (*backgroundChainChildHandle, error) {
	exePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve executable: %w", err)
	}
	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", os.DevNull, err)
	}
	defer devNull.Close()
	args := make([]string, 0, 8)
	if strings.TrimSpace(configPath) != "" {
		args = append(args, "--config", configPath)
	}
	args = append(args, "_run-chain-background", "--chain-id", req.ChainID)
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
	return &backgroundChainChildHandle{wait: waitCh}, nil
}

func defaultChainFlags() chainFlags {
	return chainFlags{MaxSteps: 100, MaxResolverLoops: 3, MaxDuration: 4 * time.Hour, TokenBudget: 5_000_000, Watch: true, Verbosity: chainVerbosityNormal}
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
	cmd.Flags().BoolVar(&flags.Watch, "watch", true, "Stream live chain progress to stderr while the command runs")
	cmd.Flags().StringVar(&flags.Verbosity, "verbosity", chainVerbosityNormal, "Chain log verbosity: normal or debug")
	return cmd
}

func newRunChainBackgroundCmd(configPath *string) *cobra.Command {
	var req backgroundChainExecutionRequest
	cmd := &cobra.Command{
		Use:    "_run-chain-background",
		Short:  "Run a chain execution in hidden background worker mode",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(req.ChainID) == "" {
				return fmt.Errorf("--chain-id is required")
			}
			return runChainBackgroundWorker(cmd.Context(), *configPath, req, cmd)
		},
	}
	cmd.Flags().StringVar(&req.ChainID, "chain-id", "", "Chain execution identifier")
	cmd.Flags().StringVar(&req.ProjectRoot, "project", "", "Override project root")
	cmd.Flags().StringVar(&req.Brain, "brain", "", "Override brain vault path")
	cmd.Flags().BoolVar(&req.IsNew, "is-new", false, "Whether this background execution owns a newly created chain")
	cmd.Flags().BoolVar(&req.Resumed, "resumed", false, "Whether this background execution is resuming a paused chain")
	return cmd
}

func runChain(ctx context.Context, configPath string, flags chainFlags, cmd *cobra.Command) error {
	cfg, err := appconfig.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	applyChainOverrides(cfg, flags)
	if err := cfg.Validate(); err != nil {
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
	launchReq := backgroundChainExecutionRequest{ChainID: chainID, IsNew: existing == nil, ProjectRoot: strings.TrimSpace(flags.ProjectRoot), Brain: strings.TrimSpace(flags.Brain)}
	if existing != nil {
		flags, err = populateChainFlagsFromExisting(flags, existing)
		if err != nil {
			return err
		}
		launchReq.Resumed = existing.Status == "paused"
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
	child, err := launchChainBackgroundChild(configPath, launchReq)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", chainID)
	if err := waitForChainBackgroundHandshake(ctx, rt.ChainStore, chainID, os.Getpid(), child, 5*time.Second); err != nil {
		return err
	}
	if !flags.Watch {
		return nil
	}
	watchCtx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()
	watch := startChainWatch(watchCtx, cmd.ErrOrStderr(), rt.ChainStore, chainID, true, chainRenderOptions{Verbosity: normalizeChainVerbosity(flags.Verbosity)})
	if err := watch.wait(chainWatchFlushTimeout); err != nil {
		return err
	}
	if watchCtx.Err() != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "detached from live output; chain %s continues running\n", chainID)
	}
	return nil
}

func runChainBackgroundWorker(ctx context.Context, configPath string, req backgroundChainExecutionRequest, cmd *cobra.Command) (err error) {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()
	cfg, err := appconfig.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	applyChainOverrides(cfg, chainFlags{ProjectRoot: req.ProjectRoot, Brain: req.Brain})
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
	rt, err := buildChainRuntime(ctx, cfg)
	if err != nil {
		return err
	}
	defer rt.Cleanup()
	existing, err := resolveExistingChain(ctx, rt.ChainStore, req.ChainID)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("chain %s not found", req.ChainID)
	}
	flags, err := populateChainFlagsFromExisting(chainFlags{ChainID: req.ChainID}, existing)
	if err != nil {
		return err
	}
	executionRegistered := false
	defer func() {
		if err == nil || !executionRegistered {
			return
		}
		if closeErr := closeErroredChainExecution(context.WithoutCancel(ctx), rt.ChainStore, req.ChainID, err.Error()); closeErr != nil {
			err = fmt.Errorf("%w (while closing active execution: %v)", err, closeErr)
		}
	}()
	if err := registerActiveChainExecution(ctx, rt.ChainStore, req.ChainID, req.IsNew, req.Resumed); err != nil {
		return err
	}
	executionRegistered = true
	registry, err := buildChainRegistry(rt, roleCfg, req.ChainID)
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
	loop := newChainTurnRunner(agent.AgentLoopDeps{ContextAssembler: rt.ContextAssembler, ConversationManager: rt.ConversationManager, ProviderRouter: rt.ProviderRouter, ToolExecutor: &rtpkg.RegistryToolExecutor{Registry: registry, ProjectRoot: cfg.ProjectRoot}, ToolDefinitions: registry.ToolDefinitions(), PromptBuilder: agent.NewPromptBuilder(rt.Logger), TitleGenerator: conversation.NewTitleGen(rt.ConversationManager, rt.ProviderRouter, cfg.Routing.Default.Model, rt.Logger), Config: agent.AgentLoopConfig{MaxIterations: roleCfg.MaxTurns, BasePrompt: systemPrompt, ProviderName: cfg.Routing.Default.Provider, ModelName: cfg.Routing.Default.Model, ContextConfig: cfg.Context}, Logger: rt.Logger})
	defer loop.Close()
	steps, err := rt.ChainStore.ListSteps(ctx, req.ChainID)
	if err != nil {
		return err
	}
	turnTask := buildChainTask(flags, req.ChainID, existingReceiptPaths(steps))
	if _, err := loop.RunTurn(ctx, agent.RunTurnRequest{ConversationID: conv.ID, TurnNumber: 1, Message: turnTask, ModelContextLimit: limit}); err != nil {
		if handled, handleErr := handleChainRunInterruption(ctx, rt.ChainStore, req.ChainID, err, cmd); handled || handleErr != nil {
			return handleErr
		}
		return err
	}
	if err := finalizeRequestedChainStatus(ctx, rt.ChainStore, req.ChainID); err != nil {
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

func waitForChainBackgroundHandshake(ctx context.Context, store *chain.Store, chainID string, parentPID int, child *backgroundChainChildHandle, timeout time.Duration) error {
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

func buildChainTask(flags chainFlags, chainID string, receiptPaths []string) string {
	history := "No existing receipt paths were found for this chain yet."
	if len(receiptPaths) > 0 {
		history = fmt.Sprintf("Relevant existing receipt paths to read first: %s.", strings.Join(receiptPaths, ", "))
	}
	if strings.TrimSpace(flags.Specs) != "" {
		return fmt.Sprintf("You are managing a chain execution. Source specs: %s. Chain ID: %s. Read the specs from the brain. %s Continue orchestrating from the current point.", strings.Join(parseSpecs(flags.Specs), ", "), chainID, history)
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

type chainWatchHandle struct {
	cancel func()
	done   <-chan error
}

func startChainWatch(ctx context.Context, out io.Writer, store *chain.Store, chainID string, enabled bool, opts chainRenderOptions) *chainWatchHandle {
	if !enabled {
		return &chainWatchHandle{}
	}
	watchCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- followChainEvents(watchCtx, out, store, chainID, 0, opts)
	}()
	return &chainWatchHandle{cancel: cancel, done: done}
}

func (h *chainWatchHandle) wait(timeout time.Duration) error {
	if h == nil || h.done == nil {
		return nil
	}
	if timeout <= 0 {
		timeout = chainWatchFlushTimeout
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

func registerActiveChainExecution(ctx context.Context, store *chain.Store, chainID string, isNew bool, resumed bool) error {
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

func handleChainRunInterruption(ctx context.Context, store *chain.Store, chainID string, err error, cmd *cobra.Command) (bool, error) {
	if !errors.Is(err, agent.ErrTurnCancelled) {
		return false, nil
	}
	cleanupCtx := context.WithoutCancel(ctx)
	if err := finalizeRequestedChainStatus(cleanupCtx, store, chainID); err != nil {
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

func finalizeRequestedChainStatus(ctx context.Context, store *chain.Store, chainID string) error {
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

func closeErroredChainExecution(ctx context.Context, store *chain.Store, chainID string, summary string) error {
	events := mustListChainEvents(ctx, store, chainID)
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

func mustListChainEvents(ctx context.Context, store *chain.Store, chainID string) []chain.Event {
	events, err := store.ListEvents(ctx, chainID)
	if err != nil {
		return nil
	}
	return events
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
