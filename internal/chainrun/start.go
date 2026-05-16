package chainrun

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ponchione/sodoryard/internal/agent"
	"github.com/ponchione/sodoryard/internal/chain"
	"github.com/ponchione/sodoryard/internal/chaininput"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/conversation"
	"github.com/ponchione/sodoryard/internal/id"
	"github.com/ponchione/sodoryard/internal/receipt"
	rtpkg "github.com/ponchione/sodoryard/internal/runtime"
	spawnpkg "github.com/ponchione/sodoryard/internal/spawn"
	"github.com/ponchione/sodoryard/internal/tool"
)

type TurnRunner interface {
	RunTurn(ctx context.Context, req agent.RunTurnRequest) (*agent.TurnResult, error)
	Close()
}

type WatchHandle interface {
	Wait(timeout time.Duration) error
}

type Mode string

const (
	ModeOrchestrator Mode = "sir_topham_decides"
	ModeConstrained  Mode = "constrained_orchestration"
	ModeOneStep      Mode = "one_step_chain"
	ModeManualRoster Mode = "manual_roster"
)

const (
	headlessExitSafetyLimit = 2
	headlessExitEscalation  = 3
	dryRunStatus            = "dry_run"
	dryRunSummary           = "dry run: orchestrator not started"
)

type StepRunner interface {
	RunStep(ctx context.Context, in spawnpkg.AgentStepInput) (spawnpkg.AgentStepResult, string, error)
}

type StepRequest struct {
	Role          string
	TaskContext   string
	Note          string
	Sources       []string
	ReindexBefore bool
}

type Options struct {
	ChainID           string
	Mode              Mode
	Role              string
	AllowedRoles      []string
	Step              StepRequest
	Roster            []StepRequest
	SourceSpecs       []string
	SourceTask        string
	MaxSteps          int
	MaxResolverLoops  int
	MaxDuration       time.Duration
	TokenBudget       int
	StepMaxTurns      int
	StepMaxTokens     int
	AllowApprovalWait bool
	DryRun            bool

	OnChainID         func(string)
	OnMessage         func(string)
	StartWatch        func(context.Context, *chain.Store, string) WatchHandle
	WatchFlushTimeout time.Duration
}

type Result struct {
	ChainID string
	Status  string
}

type Deps struct {
	BuildRuntime  func(context.Context, *appconfig.Config) (*rtpkg.OrchestratorRuntime, error)
	BuildRegistry func(*rtpkg.OrchestratorRuntime, appconfig.AgentRoleConfig, string) (*tool.Registry, error)
	NewTurnRunner func(agent.AgentLoopDeps) TurnRunner
	NewStepRunner func(*rtpkg.OrchestratorRuntime, string) StepRunner
	NewChainID    func() string
	ProcessID     func() int
}

type ExitError struct {
	Code int
	Err  error
}

func (e ExitError) Error() string {
	if e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e ExitError) Unwrap() error { return e.Err }
func (e ExitError) ExitCode() int { return e.Code }

func Start(ctx context.Context, cfg *appconfig.Config, opts Options, deps Deps) (result *Result, err error) {
	deps = withDefaultDeps(deps)
	if cfg == nil {
		return nil, fmt.Errorf("chain start: config is required")
	}
	rt, err := deps.BuildRuntime(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer rt.Cleanup()

	chainID := strings.TrimSpace(opts.ChainID)
	if chainID == "" {
		chainID = deps.NewChainID()
	}

	existing, err := resolveExistingChain(ctx, rt.ChainStore, chainID)
	if err != nil {
		return nil, err
	}
	isNew := existing == nil
	resumed := false
	if existing != nil {
		opts, err = populateOptionsFromExisting(ctx, rt.ChainStore, opts, existing)
		if err != nil {
			return nil, err
		}
		resumed = existing.Status == "paused"
		if existing.Status == chain.StatusWaitingApproval {
			opts.AllowApprovalWait = true
		}
	}
	mode, err := resolveMode(opts)
	if err != nil {
		return nil, err
	}
	opts.Mode = mode
	var roleCfg appconfig.AgentRoleConfig
	var systemPrompt string
	if mode == ModeOrchestrator || mode == ModeConstrained {
		var ok bool
		roleCfg, ok = cfg.AgentRoles["orchestrator"]
		if !ok {
			return nil, fmt.Errorf("agent role %q not found in config", "orchestrator")
		}
		if mode == ModeConstrained {
			opts, err = resolveAllowedRoles(cfg, opts)
			if err != nil {
				return nil, err
			}
		}
		opts.Role = "orchestrator"
		systemPrompt, _, err = rtpkg.LoadRoleSystemPrompt("orchestrator", cfg.ProjectRoot, roleCfg.SystemPrompt)
		if err != nil {
			return nil, err
		}
	} else {
		opts, err = resolveStepRoles(cfg, opts, mode)
		if err != nil {
			return nil, err
		}
	}
	if isNew {
		if _, err := rt.ChainStore.StartChain(ctx, chainSpecFromOptions(chainID, opts)); err != nil {
			return nil, err
		}
		payload := chainLaunchEventPayload(opts)
		if opts.DryRun {
			payload["dry_run"] = true
		}
		if len(opts.AllowedRoles) > 0 {
			payload["allowed_roles"] = opts.AllowedRoles
		}
		_ = rt.ChainStore.LogEvent(ctx, chainID, "", chain.EventChainStarted, payload)
	} else if !opts.DryRun {
		if err := prepareExistingChainForExecution(ctx, rt.ChainStore, existing); err != nil {
			return nil, err
		}
	}

	if opts.OnChainID != nil {
		opts.OnChainID(chainID)
	}
	if opts.DryRun {
		if isNew {
			if err := markDryRunChain(ctx, rt.ChainStore, chainID); err != nil {
				return nil, err
			}
		}
		return &Result{ChainID: chainID, Status: dryRunStatus}, nil
	}

	executionRegistered := false
	defer func() {
		if err == nil || !executionRegistered {
			return
		}
		if closeErr := closeErroredExecution(context.WithoutCancel(ctx), rt.ChainStore, chainID, err.Error()); closeErr != nil {
			err = fmt.Errorf("%w (while closing active execution: %v)", err, closeErr)
		}
	}()
	if err := registerActiveExecution(ctx, rt.ChainStore, chainID, isNew, resumed, deps.ProcessID()); err != nil {
		return nil, err
	}
	executionRegistered = true

	var watch WatchHandle
	if opts.StartWatch != nil {
		watch = opts.StartWatch(ctx, rt.ChainStore, chainID)
	}

	if mode == ModeOneStep {
		return runOneStepMode(ctx, rt, opts, deps, chainID, watch)
	}
	if mode == ModeManualRoster {
		return runManualRosterMode(ctx, rt, opts, deps, chainID, watch)
	}
	return runOrchestratorMode(ctx, cfg, rt, opts, deps, chainID, roleCfg, systemPrompt, watch)
}

func runOrchestratorMode(ctx context.Context, cfg *appconfig.Config, rt *rtpkg.OrchestratorRuntime, opts Options, deps Deps, chainID string, roleCfg appconfig.AgentRoleConfig, systemPrompt string, watch WatchHandle) (*Result, error) {
	registry, err := deps.BuildRegistry(rt, roleCfg, chainID)
	if err != nil {
		return nil, err
	}
	configureToolRegistryApprovalWait(registry, opts.AllowApprovalWait)
	conv, err := rt.ConversationManager.Create(ctx, cfg.ProjectRoot, conversation.WithProvider(cfg.Routing.Default.Provider), conversation.WithModel(cfg.Routing.Default.Model))
	if err != nil {
		return nil, fmt.Errorf("create conversation: %w", err)
	}
	limit, err := rtpkg.ResolveModelContextLimit(cfg, cfg.Routing.Default.Provider)
	if err != nil {
		return nil, err
	}
	runCtx := ctx
	cancelRun := func() {}
	if timeout := roleCfg.Timeout.Duration(); timeout > 0 {
		runCtx, cancelRun = context.WithTimeout(ctx, timeout)
	} else if opts.AllowApprovalWait {
		runCtx, cancelRun = context.WithCancel(ctx)
	}
	defer cancelRun()
	loop := deps.NewTurnRunner(agent.AgentLoopDeps{
		ContextAssembler:    rt.ContextAssembler,
		ConversationManager: rt.ConversationManager,
		ProviderRouter:      rt.ProviderRouter,
		ToolExecutor:        &rtpkg.RegistryToolExecutor{Registry: registry, ProjectRoot: cfg.ProjectRoot, TraceRecorder: rt.TraceRecorder},
		ToolDefinitions:     registry.ToolDefinitions(),
		PromptBuilder:       agent.NewPromptBuilder(rt.Logger),
		TitleGenerator:      conversation.NewTitleGen(rt.ConversationManager, rt.ProviderRouter, cfg.Routing.Default.Model, rt.Logger),
		EventSink:           newApprovalEventSink(ctx, rt.ChainStore, chainID, approvalEventSinkOptions{Wait: opts.AllowApprovalWait, Cancel: cancelRun}),
		CompressionEngine:   rt.CompressionEngine,
		TraceRecorder:       rt.TraceRecorder,
		Config:              rtpkg.BuildAgentLoopConfig(cfg, roleCfg.MaxTurns, systemPrompt),
		Logger:              rt.Logger,
	})
	defer loop.Close()

	steps, err := rt.ChainStore.ListSteps(ctx, chainID)
	if err != nil {
		return nil, err
	}
	turnTask := buildTask(opts, chainID, existingReceiptPaths(steps))
	if _, err := loop.RunTurn(runCtx, agent.RunTurnRequest{ConversationID: conv.ID, TurnNumber: 1, Message: turnTask, ModelContextLimit: limit, ChainID: chainID}); err != nil {
		if handled, handleErr := handleInterruption(runCtx, rt.ChainStore, chainID, err, opts.OnMessage); handled || handleErr != nil {
			if handleErr != nil {
				return nil, handleErr
			}
			status := terminalStatus(ctx, rt.ChainStore, chainID)
			if err := waitWatch(watch, opts.WatchFlushTimeout); err != nil {
				return nil, err
			}
			if code := exitCode(status, mustListEvents(ctx, rt.ChainStore, chainID)); code != 0 {
				return nil, ExitError{Code: code, Err: fmt.Errorf("chain %s ended with status %s", chainID, status)}
			}
			return &Result{ChainID: chainID, Status: status}, nil
		}
		return nil, err
	}
	if err := finalizeRequestedChainStatus(ctx, rt.ChainStore, chainID); err != nil {
		return nil, err
	}
	stored, err := rt.ChainStore.GetChain(ctx, chainID)
	if err != nil {
		return nil, err
	}
	if code := exitCode(stored.Status, mustListEvents(ctx, rt.ChainStore, chainID)); code != 0 {
		return nil, ExitError{Code: code, Err: fmt.Errorf("chain %s ended with status %s", chainID, stored.Status)}
	}
	if err := waitWatch(watch, opts.WatchFlushTimeout); err != nil {
		return nil, err
	}
	return &Result{ChainID: chainID, Status: stored.Status}, nil
}

func runOneStepMode(ctx context.Context, rt *rtpkg.OrchestratorRuntime, opts Options, deps Deps, chainID string, watch WatchHandle) (*Result, error) {
	runner := deps.NewStepRunner(rt, chainID)
	configureStepRunnerApprovalWait(runner, opts.AllowApprovalWait)
	stepResult, _, err := runner.RunStep(ctx, spawnpkg.AgentStepInput{Role: opts.Role, Task: buildOneStepTask(opts), ReindexBefore: opts.Step.ReindexBefore, MaxTurns: opts.StepMaxTurns, MaxTokens: opts.StepMaxTokens})
	if err != nil {
		if errors.Is(err, tool.ErrChainComplete) {
			cleanupCtx := context.WithoutCancel(ctx)
			if finalizeErr := finalizeRequestedChainStatus(cleanupCtx, rt.ChainStore, chainID); finalizeErr != nil {
				return nil, finalizeErr
			}
			return finishControlledChain(cleanupCtx, rt.ChainStore, chainID, watch, opts.WatchFlushTimeout)
		}
		cleanupCtx := context.WithoutCancel(ctx)
		if finalizeErr := finalizeRequestedChainStatus(cleanupCtx, rt.ChainStore, chainID); finalizeErr != nil {
			return nil, finalizeErr
		}
		status := terminalStatus(cleanupCtx, rt.ChainStore, chainID)
		if status == "cancelled" || status == "paused" {
			return finishControlledChain(cleanupCtx, rt.ChainStore, chainID, watch, opts.WatchFlushTimeout)
		}
		return nil, err
	}
	if err := finalizeRequestedChainStatus(ctx, rt.ChainStore, chainID); err != nil {
		return nil, err
	}
	stored, err := rt.ChainStore.GetChain(ctx, chainID)
	if err != nil {
		return nil, err
	}
	if stored.Status == "running" {
		status := oneStepTerminalStatus(stepResult)
		summary := fmt.Sprintf("one-step chain %s finished with verdict %s", chainID, stepResult.Verdict)
		if status == "failed" && stepResultSafetyLimited(stepResult) {
			limit := "receipt verdict safety_limit"
			if stepResult.Verdict != receipt.VerdictSafetyLimit {
				limit = "headless exit safety_limit"
			}
			_ = rt.ChainStore.LogEvent(ctx, chainID, stepResult.StepID, chain.EventSafetyLimitHit, map[string]any{"role": opts.Role, "limit": limit, "exit_code": stepResult.ExitCode})
		}
		if err := chain.ApplyTerminalChainClosure(ctx, rt.ChainStore, chainID, chain.TerminalChainClosure{
			Status:    status,
			EventType: chain.EventChainCompleted,
			Summary:   &summary,
			Extra:     chainCompletionEventPayload(opts, map[string]any{"summary": summary, "mode": string(ModeOneStep), "role": opts.Role, "verdict": stepResult.Verdict}),
		}); err != nil {
			return nil, err
		}
	}
	return finishControlledChain(ctx, rt.ChainStore, chainID, watch, opts.WatchFlushTimeout)
}

func runManualRosterMode(ctx context.Context, rt *rtpkg.OrchestratorRuntime, opts Options, deps Deps, chainID string, watch WatchHandle) (*Result, error) {
	runner := deps.NewStepRunner(rt, chainID)
	configureStepRunnerApprovalWait(runner, opts.AllowApprovalWait)
	existingSteps := mustListSteps(ctx, rt.ChainStore, chainID)
	receiptPaths := existingReceiptPaths(existingSteps)
	startIndex := len(existingSteps)
	results := make([]spawnpkg.AgentStepResult, 0, len(opts.Roster))
	for i, step := range opts.Roster {
		if i < startIndex {
			continue
		}
		if controlled, result, err := stopIfRequested(ctx, rt.ChainStore, chainID, watch, opts.WatchFlushTimeout); controlled || err != nil {
			return result, err
		}
		task := buildManualRosterTask(opts, chainID, i+1, step, receiptPaths)
		taskContext := strings.TrimSpace(step.TaskContext)
		if taskContext == "" {
			taskContext = manualRosterTaskContext(chainID, i+1, step.Role)
		}
		stepResult, _, err := runner.RunStep(ctx, spawnpkg.AgentStepInput{Role: step.Role, Task: task, TaskContext: taskContext, ReindexBefore: step.ReindexBefore, MaxTurns: opts.StepMaxTurns, MaxTokens: opts.StepMaxTokens})
		if err != nil {
			if errors.Is(err, tool.ErrChainComplete) {
				cleanupCtx := context.WithoutCancel(ctx)
				if finalizeErr := finalizeRequestedChainStatus(cleanupCtx, rt.ChainStore, chainID); finalizeErr != nil {
					return nil, finalizeErr
				}
				return finishControlledChain(cleanupCtx, rt.ChainStore, chainID, watch, opts.WatchFlushTimeout)
			}
			cleanupCtx := context.WithoutCancel(ctx)
			if finalizeErr := finalizeRequestedChainStatus(cleanupCtx, rt.ChainStore, chainID); finalizeErr != nil {
				return nil, finalizeErr
			}
			status := terminalStatus(cleanupCtx, rt.ChainStore, chainID)
			if status == "cancelled" || status == "paused" {
				return finishControlledChain(cleanupCtx, rt.ChainStore, chainID, watch, opts.WatchFlushTimeout)
			}
			return nil, err
		}
		results = append(results, stepResult)
		if strings.TrimSpace(stepResult.ReceiptPath) != "" {
			receiptPaths = append(receiptPaths, stepResult.ReceiptPath)
		}
		if controlled, result, err := stopIfRequested(ctx, rt.ChainStore, chainID, watch, opts.WatchFlushTimeout); controlled || err != nil {
			return result, err
		}
		if shouldStopManualRoster(stepResult) {
			return closeManualRoster(ctx, rt.ChainStore, chainID, opts, results, watch, opts.WatchFlushTimeout)
		}
	}
	return closeManualRoster(ctx, rt.ChainStore, chainID, opts, results, watch, opts.WatchFlushTimeout)
}

type approvalWaitConfigurable interface {
	SetApprovalWait(bool)
}

func configureStepRunnerApprovalWait(runner StepRunner, allow bool) {
	if configurable, ok := runner.(approvalWaitConfigurable); ok {
		configurable.SetApprovalWait(allow)
	}
}

func configureToolRegistryApprovalWait(registry *tool.Registry, allow bool) {
	if registry == nil {
		return
	}
	for _, registered := range registry.All() {
		if configurable, ok := registered.(approvalWaitConfigurable); ok {
			configurable.SetApprovalWait(allow)
		}
	}
}

func withDefaultDeps(deps Deps) Deps {
	if deps.BuildRuntime == nil {
		deps.BuildRuntime = rtpkg.BuildOrchestratorRuntime
	}
	if deps.BuildRegistry == nil {
		deps.BuildRegistry = rtpkg.BuildOrchestratorRegistry
	}
	if deps.NewTurnRunner == nil {
		deps.NewTurnRunner = func(deps agent.AgentLoopDeps) TurnRunner { return agent.NewAgentLoop(deps) }
	}
	if deps.NewStepRunner == nil {
		deps.NewStepRunner = func(rt *rtpkg.OrchestratorRuntime, chainID string) StepRunner {
			return spawnpkg.NewSpawnAgentTool(spawnpkg.SpawnAgentDeps{
				Store:         rt.ChainStore,
				Backend:       rt.BrainBackend,
				Config:        rt.Config,
				ChainID:       chainID,
				EngineBinary:  "tidmouth",
				ProjectRoot:   rt.Config.ProjectRoot,
				SubprocessEnv: rt.MemoryEndpointEnv,
				TraceRecorder: rt.TraceRecorder,
			})
		}
	}
	if deps.NewChainID == nil {
		deps.NewChainID = id.New
	}
	if deps.ProcessID == nil {
		deps.ProcessID = os.Getpid
	}
	return deps
}

func resolveMode(opts Options) (Mode, error) {
	if opts.Mode != "" {
		switch opts.Mode {
		case ModeOrchestrator, ModeConstrained, ModeOneStep, ModeManualRoster:
			return opts.Mode, nil
		default:
			return "", fmt.Errorf("unsupported chain mode %q", opts.Mode)
		}
	}
	if len(opts.Roster) > 0 {
		return ModeManualRoster, nil
	}
	if len(opts.AllowedRoles) > 0 {
		return ModeConstrained, nil
	}
	if strings.TrimSpace(opts.Role) != "" {
		return ModeOneStep, nil
	}
	return ModeOrchestrator, nil
}

func resolveStepRoles(cfg *appconfig.Config, opts Options, mode Mode) (Options, error) {
	switch mode {
	case ModeOneStep:
		roleName, _, err := cfg.ResolveAgentRole(opts.Role)
		if err != nil {
			return opts, fmt.Errorf("chain start: %w", err)
		}
		opts.Role = roleName
		if strings.TrimSpace(opts.Step.Role) == "" {
			opts.Step.Role = roleName
		} else {
			stepRole, _, err := cfg.ResolveAgentRole(opts.Step.Role)
			if err != nil {
				return opts, fmt.Errorf("chain start: step role: %w", err)
			}
			opts.Step.Role = stepRole
			if opts.Step.Role != roleName {
				return opts, fmt.Errorf("chain start: one-step role does not match structured step")
			}
		}
		opts.Step.Sources = chaininput.NormalizeSpecs(opts.Step.Sources)
		opts.Step.Note = strings.TrimSpace(opts.Step.Note)
		return opts, nil
	case ModeManualRoster:
		if len(opts.Roster) == 0 {
			return opts, fmt.Errorf("chain start: manual roster requires at least one role")
		}
		for i := range opts.Roster {
			roleName, _, err := cfg.ResolveAgentRole(opts.Roster[i].Role)
			if err != nil {
				return opts, fmt.Errorf("chain start: roster role %d: %w", i+1, err)
			}
			opts.Roster[i].Role = roleName
			opts.Roster[i].Note = strings.TrimSpace(opts.Roster[i].Note)
			opts.Roster[i].Sources = chaininput.NormalizeSpecs(opts.Roster[i].Sources)
		}
		return opts, nil
	default:
		return opts, nil
	}
}

func resolveAllowedRoles(cfg *appconfig.Config, opts Options) (Options, error) {
	roles := chaininput.NormalizeRoleSet(opts.AllowedRoles)
	if len(roles) == 0 && strings.TrimSpace(opts.Role) != "" && opts.Role != "orchestrator" {
		roles = chaininput.ParseRoleSet(opts.Role)
	}
	if len(roles) == 0 {
		return opts, fmt.Errorf("chain start: constrained orchestration requires at least one allowed role")
	}
	for i := range roles {
		roleName, _, err := cfg.ResolveAgentRole(roles[i])
		if err != nil {
			return opts, fmt.Errorf("chain start: constrained role %d: %w", i+1, err)
		}
		roles[i] = roleName
	}
	opts.AllowedRoles = roles
	opts.Role = "orchestrator"
	return opts, nil
}

func markDryRunChain(ctx context.Context, store *chain.Store, chainID string) error {
	summary := dryRunSummary
	return chain.ApplyTerminalChainClosure(ctx, store, chainID, chain.TerminalChainClosure{
		Status:    dryRunStatus,
		EventType: chain.EventChainCompleted,
		Summary:   &summary,
		Extra: map[string]any{
			"dry_run": true,
			"summary": summary,
		},
	})
}

func resolveExistingChain(ctx context.Context, store *chain.Store, chainID string) (*chain.Chain, error) {
	if strings.TrimSpace(chainID) == "" {
		return nil, nil
	}
	existing, err := store.GetChain(ctx, chainID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "sql: no rows") {
			return nil, nil
		}
		return nil, err
	}
	return existing, nil
}

func populateOptionsFromExisting(ctx context.Context, store *chain.Store, opts Options, existing *chain.Chain) (Options, error) {
	if existing == nil {
		return opts, nil
	}
	if store != nil {
		if events, err := store.ListEvents(ctx, existing.ID); err == nil {
			opts = hydrateLaunchOptionsFromEvents(opts, events)
		} else {
			return opts, err
		}
	}
	if len(opts.SourceSpecs) == 0 && len(existing.SourceSpecs) > 0 {
		opts.SourceSpecs = append([]string(nil), existing.SourceSpecs...)
	}
	if strings.TrimSpace(opts.SourceTask) == "" {
		opts.SourceTask = existing.SourceTask
	}
	if strings.TrimSpace(opts.SourceTask) == "" && len(opts.SourceSpecs) == 0 {
		return opts, fmt.Errorf("chain %s has no stored task/specs to resume from", existing.ID)
	}
	return opts, nil
}

func hydrateLaunchOptionsFromEvents(opts Options, events []chain.Event) Options {
	for _, event := range events {
		if event.EventType != chain.EventChainStarted {
			continue
		}
		var payload struct {
			Mode              string        `json:"mode"`
			Role              string        `json:"role"`
			Roster            []string      `json:"roster"`
			AllowedRoles      []string      `json:"allowed_roles"`
			Steps             []StepRequest `json:"steps"`
			Task              string        `json:"task"`
			Specs             []string      `json:"specs"`
			StepMaxTurns      int           `json:"step_max_turns"`
			StepMaxTokens     int           `json:"step_max_tokens"`
			AllowApprovalWait bool          `json:"allow_approval_wait"`
		}
		if err := json.Unmarshal([]byte(event.EventData), &payload); err != nil || strings.TrimSpace(payload.Mode) == "" {
			continue
		}
		if opts.Mode == "" {
			opts.Mode = Mode(payload.Mode)
		}
		if strings.TrimSpace(opts.Role) == "" {
			opts.Role = strings.TrimSpace(payload.Role)
		}
		if len(opts.Roster) == 0 {
			if len(payload.Steps) > 0 {
				opts.Roster = cloneStepRequests(payload.Steps)
			} else if len(payload.Roster) > 0 {
				opts.Roster = stepRequestsFromRoles(payload.Roster)
			}
		}
		if len(opts.AllowedRoles) == 0 && len(payload.AllowedRoles) > 0 {
			opts.AllowedRoles = append([]string(nil), payload.AllowedRoles...)
		}
		if len(opts.SourceSpecs) == 0 && len(payload.Specs) > 0 {
			opts.SourceSpecs = append([]string(nil), payload.Specs...)
		}
		if strings.TrimSpace(opts.SourceTask) == "" {
			opts.SourceTask = strings.TrimSpace(payload.Task)
		}
		if opts.StepMaxTurns == 0 {
			opts.StepMaxTurns = payload.StepMaxTurns
		}
		if opts.StepMaxTokens == 0 {
			opts.StepMaxTokens = payload.StepMaxTokens
		}
		if payload.AllowApprovalWait {
			opts.AllowApprovalWait = true
		}
		if opts.Mode == ModeOneStep && strings.TrimSpace(opts.Step.Role) == "" && len(payload.Steps) == 1 {
			opts.Step = cloneStepRequests(payload.Steps)[0]
		}
	}
	return opts
}

func prepareExistingChainForExecution(ctx context.Context, store *chain.Store, existing *chain.Chain) error {
	if existing == nil {
		return nil
	}
	resumeReady, err := chain.ResumeExecutionReady(existing.Status)
	if err != nil {
		return fmt.Errorf("chain %s %w", existing.ID, err)
	}
	if !resumeReady {
		return nil
	}
	if existing.Status == chain.StatusWaitingApproval {
		pending, err := store.PendingApprovals(ctx, existing.ID)
		if err != nil {
			return err
		}
		if len(pending) > 0 {
			return fmt.Errorf("chain %s has %d pending approval(s); approve or deny them before resuming", existing.ID, len(pending))
		}
	}
	if err := store.SetChainStatus(ctx, existing.ID, "running"); err != nil {
		return err
	}
	_ = store.LogEvent(ctx, existing.ID, "", chain.EventChainResumed, map[string]any{"resumed_by": "cli"})
	return nil
}

func registerActiveExecution(ctx context.Context, store *chain.Store, chainID string, isNew bool, resumed bool, pid int) error {
	eventType := chain.EventChainStarted
	payload := map[string]any{"orchestrator_pid": pid, "active_execution": true, "execution_id": id.New()}
	if resumed {
		eventType = chain.EventChainResumed
		payload["resumed_by"] = "cli"
	} else if !isNew {
		eventType = chain.EventChainResumed
		payload["continued_by"] = "cli"
	}
	return store.LogEvent(ctx, chainID, "", eventType, payload)
}

func handleInterruption(ctx context.Context, store *chain.Store, chainID string, err error, onMessage func(string)) (bool, error) {
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
		emit(onMessage, "chain %s cancelled\n", chainID)
		return true, nil
	case "paused":
		if err := chain.CloseTerminalizedActiveExecution(cleanupCtx, store, chainID, ch.Status, nil); err != nil {
			return true, err
		}
		emit(onMessage, "chain %s paused\n", chainID)
		return true, nil
	case chain.StatusWaitingApproval:
		if err := chain.CloseTerminalizedActiveExecution(cleanupCtx, store, chainID, ch.Status, nil); err != nil {
			return true, err
		}
		emit(onMessage, "chain %s waiting for approval\n", chainID)
		return true, nil
	case "running":
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			summary := fmt.Sprintf("chain %s hit orchestrator timeout", chainID)
			_ = store.LogEvent(cleanupCtx, chainID, "", chain.EventSafetyLimitHit, map[string]any{"role": "orchestrator", "limit": "timeout"})
			if err := chain.ApplyTerminalChainClosure(cleanupCtx, store, chainID, chain.TerminalChainClosure{Status: "failed", EventType: chain.EventChainCompleted, Summary: &summary, Extra: map[string]any{"summary": summary}}); err != nil {
				return true, err
			}
			emit(onMessage, "%s\n", summary)
			return true, nil
		}
		if ctx.Err() != nil {
			if err := chain.ApplyTerminalChainClosure(cleanupCtx, store, chainID, chain.TerminalChainClosure{Status: "cancelled", EventType: chain.EventChainCancelled, Extra: map[string]any{"finalized_from": "interrupted"}}); err != nil {
				return true, err
			}
			emit(onMessage, "chain %s cancelled\n", chainID)
			return true, nil
		}
		return false, nil
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

func closeErroredExecution(ctx context.Context, store *chain.Store, chainID string, summary string) error {
	events := mustListEvents(ctx, store, chainID)
	if _, ok := chain.LatestActiveExecution(events); !ok {
		return nil
	}
	ch, err := store.GetChain(ctx, chainID)
	if err != nil {
		return err
	}
	if finalStatus, ok := chain.FinalizeControlStatus(ch.Status); ok {
		eventType, eventOK := chain.FinalizeControlEventType(ch.Status)
		if !eventOK {
			return store.SetChainStatus(ctx, chainID, finalStatus)
		}
		return chain.ApplyTerminalChainClosure(ctx, store, chainID, chain.TerminalChainClosure{
			Status:    finalStatus,
			EventType: eventType,
			Extra:     map[string]any{"finalized_from": ch.Status},
		})
	}
	return chain.ApplyTerminalChainClosure(ctx, store, chainID, chain.TerminalChainClosure{
		Status:    "failed",
		EventType: chain.EventChainCompleted,
		Summary:   &summary,
		Extra:     map[string]any{"summary": summary},
	})
}

func chainSpecFromOptions(chainID string, opts Options) chain.ChainSpec {
	return chain.ChainSpec{ChainID: chainID, SourceSpecs: append([]string(nil), opts.SourceSpecs...), SourceTask: strings.TrimSpace(opts.SourceTask), MaxSteps: opts.MaxSteps, MaxResolverLoops: opts.MaxResolverLoops, MaxDuration: opts.MaxDuration, TokenBudget: opts.TokenBudget}
}

func chainLaunchEventPayload(opts Options) map[string]any {
	payload := map[string]any{
		"specs":           opts.SourceSpecs,
		"task":            opts.SourceTask,
		"mode":            string(opts.Mode),
		"role":            opts.Role,
		"step_max_turns":  opts.StepMaxTurns,
		"step_max_tokens": opts.StepMaxTokens,
	}
	if opts.AllowApprovalWait {
		payload["allow_approval_wait"] = true
	}
	switch opts.Mode {
	case ModeOneStep:
		step := opts.Step
		if strings.TrimSpace(step.Role) == "" {
			step.Role = opts.Role
		}
		payload["steps"] = []map[string]any{stepPayload(step)}
	case ModeManualRoster:
		payload["roster"] = stepRequestRoles(opts.Roster)
		payload["steps"] = stepPayloads(opts.Roster)
	}
	return payload
}

func chainCompletionEventPayload(opts Options, extra map[string]any) map[string]any {
	if extra == nil {
		extra = map[string]any{}
	}
	extra["step_max_turns"] = opts.StepMaxTurns
	extra["step_max_tokens"] = opts.StepMaxTokens
	return extra
}

func buildTask(opts Options, chainID string, receiptPaths []string) string {
	history := "No existing receipt paths were found for this chain yet."
	if len(receiptPaths) > 0 {
		history = fmt.Sprintf("Relevant existing receipt paths to read first: %s.", strings.Join(receiptPaths, ", "))
	}
	roleConstraints := constrainedRoleInstruction(opts)
	if roleConstraints != "" {
		history = history + " " + roleConstraints
	}
	if len(opts.SourceSpecs) > 0 {
		return fmt.Sprintf("You are managing a chain execution. Source specs: %s. Chain ID: %s. Read the specs from the brain. %s Continue orchestrating from the current point.", strings.Join(opts.SourceSpecs, ", "), chainID, history)
	}
	return fmt.Sprintf("You are managing a chain execution. Task: %s. Chain ID: %s. %s Continue orchestrating from the current point.", strings.TrimSpace(opts.SourceTask), chainID, history)
}

func constrainedRoleInstruction(opts Options) string {
	if opts.Mode != ModeConstrained || len(opts.AllowedRoles) == 0 {
		return ""
	}
	return fmt.Sprintf("Constrained orchestration is enabled. When spawning agent steps, choose only from these configured role keys: %s. Do not spawn unlisted roles.", strings.Join(opts.AllowedRoles, ", "))
}

func buildOneStepTask(opts Options) string {
	step := opts.Step
	if strings.TrimSpace(step.Role) == "" {
		step.Role = opts.Role
	}
	return strings.TrimSpace(fmt.Sprintf(`Original work packet:
%s
Global sources:
%s

This step's dossier:
Note:
%s
Assigned sources:
%s

Complete the requested work and produce the required receipt.`, taskTextOrFallback(opts.SourceTask), lineListOrNone(opts.SourceSpecs), textOrNone(step.Note), lineListOrNone(step.Sources)))
}

func buildManualRosterTask(opts Options, chainID string, sequence int, step StepRequest, receiptPaths []string) string {
	receiptHistory := "No previous receipt paths are available yet."
	if len(receiptPaths) > 0 {
		receiptHistory = strings.Join(receiptPaths, "\n")
	}
	return fmt.Sprintf(`You are running manual roster step %d for role %s in chain %s.

Original work packet:
%s
Global sources:
%s

This step's dossier:
Note:
%s
Assigned sources:
%s

Receipt history:
%s

Complete only the work appropriate for this roster step and produce the required receipt.`, sequence, step.Role, chainID, taskTextOrFallback(opts.SourceTask), lineListOrNone(opts.SourceSpecs), textOrNone(step.Note), lineListOrNone(step.Sources), receiptHistory)
}

func manualRosterTaskContext(chainID string, sequence int, role string) string {
	return fmt.Sprintf("manual_roster:%s:%03d:%s", chainID, sequence, role)
}

func oneStepTerminalStatus(result spawnpkg.AgentStepResult) string {
	if result.Status == "failed" {
		return "failed"
	}
	if stepResultSafetyLimited(result) {
		return "failed"
	}
	if result.ExitCode == headlessExitEscalation {
		return "partial"
	}
	switch result.Verdict {
	case receipt.VerdictCompleted, receipt.VerdictCompletedWithConcerns, receipt.VerdictCompletedNoReceipt:
		return "completed"
	case receipt.VerdictFixRequired, receipt.VerdictBlocked, receipt.VerdictEscalate:
		return "partial"
	case receipt.VerdictSafetyLimit:
		return "failed"
	default:
		return "failed"
	}
}

func stepResultSafetyLimited(result spawnpkg.AgentStepResult) bool {
	return result.Verdict == receipt.VerdictSafetyLimit || result.ExitCode == headlessExitSafetyLimit
}

func shouldStopManualRoster(result spawnpkg.AgentStepResult) bool {
	return oneStepTerminalStatus(result) != "completed" || !manualRosterVerdictCanContinue(result)
}

func manualRosterVerdictCanContinue(result spawnpkg.AgentStepResult) bool {
	if result.Status == "failed" {
		return false
	}
	switch result.Verdict {
	case receipt.VerdictCompleted, receipt.VerdictCompletedWithConcerns, receipt.VerdictCompletedNoReceipt:
		return true
	default:
		return false
	}
}

func manualRosterTerminalStatus(results []spawnpkg.AgentStepResult) string {
	if len(results) == 0 {
		return "failed"
	}
	status := "completed"
	for _, result := range results {
		stepStatus := oneStepTerminalStatus(result)
		if stepStatus == "failed" {
			return "failed"
		}
		if stepStatus == "partial" {
			status = "partial"
		}
	}
	return status
}

func closeManualRoster(ctx context.Context, store *chain.Store, chainID string, opts Options, results []spawnpkg.AgentStepResult, watch WatchHandle, watchTimeout time.Duration) (*Result, error) {
	status := manualRosterTerminalStatus(results)
	for _, result := range results {
		if stepResultSafetyLimited(result) {
			_ = store.LogEvent(ctx, chainID, result.StepID, chain.EventSafetyLimitHit, map[string]any{"limit": "headless exit safety_limit", "exit_code": result.ExitCode, "step": result.Sequence})
		}
	}
	summary := manualRosterSummary(chainID, status, results)
	extra := map[string]any{"summary": summary, "mode": string(ModeManualRoster), "steps": len(results)}
	if len(results) > 0 {
		last := results[len(results)-1]
		extra["last_role_verdict"] = last.Verdict
	}
	if err := chain.ApplyTerminalChainClosure(ctx, store, chainID, chain.TerminalChainClosure{
		Status:    status,
		EventType: chain.EventChainCompleted,
		Summary:   &summary,
		Extra:     chainCompletionEventPayload(opts, extra),
	}); err != nil {
		return nil, err
	}
	return finishControlledChain(ctx, store, chainID, watch, watchTimeout)
}

func manualRosterSummary(chainID string, status string, results []spawnpkg.AgentStepResult) string {
	if len(results) == 0 {
		return fmt.Sprintf("manual roster chain %s finished with status %s before any steps ran", chainID, status)
	}
	last := results[len(results)-1]
	return fmt.Sprintf("manual roster chain %s finished with status %s after %d step(s); last verdict %s", chainID, status, len(results), last.Verdict)
}

func cloneStepRequests(steps []StepRequest) []StepRequest {
	out := make([]StepRequest, 0, len(steps))
	for _, step := range steps {
		out = append(out, StepRequest{
			Role:          step.Role,
			TaskContext:   step.TaskContext,
			Note:          step.Note,
			Sources:       append([]string(nil), step.Sources...),
			ReindexBefore: step.ReindexBefore,
		})
	}
	return out
}

func stepRequestsFromRoles(roles []string) []StepRequest {
	out := make([]StepRequest, 0, len(roles))
	for _, role := range roles {
		if role = strings.TrimSpace(role); role != "" {
			out = append(out, StepRequest{Role: role})
		}
	}
	return out
}

func stepRequestRoles(steps []StepRequest) []string {
	roles := make([]string, 0, len(steps))
	for _, step := range steps {
		if role := strings.TrimSpace(step.Role); role != "" {
			roles = append(roles, role)
		}
	}
	return roles
}

func stepPayloads(steps []StepRequest) []map[string]any {
	payloads := make([]map[string]any, 0, len(steps))
	for _, step := range steps {
		payloads = append(payloads, stepPayload(step))
	}
	return payloads
}

func stepPayload(step StepRequest) map[string]any {
	payload := map[string]any{"role": strings.TrimSpace(step.Role)}
	if note := strings.TrimSpace(step.Note); note != "" {
		payload["note"] = note
	}
	payload["sources"] = append([]string(nil), chaininput.NormalizeSpecs(step.Sources)...)
	return payload
}

func taskTextOrFallback(task string) string {
	task = strings.TrimSpace(task)
	if task == "" {
		return "No task text was provided."
	}
	return task
}

func textOrNone(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "None."
	}
	return value
}

func lineListOrNone(values []string) string {
	values = chaininput.NormalizeSpecs(values)
	if len(values) == 0 {
		return "None."
	}
	return strings.Join(values, "\n")
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

func exitCode(status string, events []chain.Event) int {
	switch status {
	case "completed", "paused":
		return 0
	case "partial":
		return 2
	case "cancelled":
		return 4
	case "failed":
		if eventsInclude(events, chain.EventSafetyLimitHit) {
			return 3
		}
		return 1
	default:
		return 0
	}
}

func eventsInclude(events []chain.Event, eventType chain.EventType) bool {
	for _, event := range events {
		if event.EventType == eventType {
			return true
		}
	}
	return false
}

func mustListEvents(ctx context.Context, store *chain.Store, chainID string) []chain.Event {
	events, err := store.ListEvents(ctx, chainID)
	if err != nil {
		return nil
	}
	return events
}

func mustListSteps(ctx context.Context, store *chain.Store, chainID string) []chain.Step {
	steps, err := store.ListSteps(ctx, chainID)
	if err != nil {
		return nil
	}
	return steps
}

func terminalStatus(ctx context.Context, store *chain.Store, chainID string) string {
	ch, err := store.GetChain(context.WithoutCancel(ctx), chainID)
	if err != nil {
		return ""
	}
	return ch.Status
}

func waitWatch(watch WatchHandle, timeout time.Duration) error {
	if watch == nil {
		return nil
	}
	return watch.Wait(timeout)
}

func finishControlledChain(ctx context.Context, store *chain.Store, chainID string, watch WatchHandle, watchTimeout time.Duration) (*Result, error) {
	status := terminalStatus(ctx, store, chainID)
	if err := waitWatch(watch, watchTimeout); err != nil {
		return nil, err
	}
	if code := exitCode(status, mustListEvents(ctx, store, chainID)); code != 0 {
		return nil, ExitError{Code: code, Err: fmt.Errorf("chain %s ended with status %s", chainID, status)}
	}
	return &Result{ChainID: chainID, Status: status}, nil
}

func stopIfRequested(ctx context.Context, store *chain.Store, chainID string, watch WatchHandle, watchTimeout time.Duration) (bool, *Result, error) {
	cleanupCtx := context.WithoutCancel(ctx)
	if err := finalizeRequestedChainStatus(cleanupCtx, store, chainID); err != nil {
		return false, nil, err
	}
	status := terminalStatus(cleanupCtx, store, chainID)
	if status == "paused" || status == "cancelled" {
		result, err := finishControlledChain(cleanupCtx, store, chainID, watch, watchTimeout)
		return true, result, err
	}
	return false, nil, nil
}

func emit(onMessage func(string), format string, args ...any) {
	if onMessage != nil {
		onMessage(fmt.Sprintf(format, args...))
	}
}
