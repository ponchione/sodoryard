package spawn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ponchione/sodoryard/internal/brain"
	"github.com/ponchione/sodoryard/internal/chain"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/outputcap"
	"github.com/ponchione/sodoryard/internal/projectmemory"
	"github.com/ponchione/sodoryard/internal/receipt"
	"github.com/ponchione/sodoryard/internal/tool"
)

type SpawnAgentDeps struct {
	Store         *chain.Store
	Backend       brain.Backend
	Config        *appconfig.Config
	ChainID       string
	EngineBinary  string
	ProjectRoot   string
	SubprocessEnv []string
}

type SpawnAgentTool struct {
	Store         *chain.Store
	Backend       brain.Backend
	Config        *appconfig.Config
	ChainID       string
	EngineBinary  string
	ProjectRoot   string
	SubprocessEnv []string
	runCommand    func(context.Context, RunCommandInput) RunResult
	now           func() time.Time
}

const (
	defaultAgentRunTimeout     = 30 * time.Minute
	parentTimeoutGraceDuration = 10 * time.Second
)

type spawnAgentInput struct {
	Role          string `json:"role"`
	Task          string `json:"task"`
	TaskContext   string `json:"task_context,omitempty"`
	ReindexBefore bool   `json:"reindex_before,omitempty"`
}

type AgentStepInput struct {
	Role          string
	Task          string
	TaskContext   string
	ReindexBefore bool
}

type AgentStepResult struct {
	StepID       string
	Sequence     int
	ReceiptPath  string
	Verdict      receipt.Verdict
	Status       string
	TokensUsed   int
	TurnsUsed    int
	DurationSecs int
	ExitCode     int
}

type spawnStep struct {
	input                 spawnAgentInput
	roleName              string
	roleCfg               appconfig.AgentRoleConfig
	sequence              int
	stepID                string
	receiptPath           string
	task                  string
	chainRemainingTimeout time.Duration
}

type engineRunOutcome struct {
	exitCode     int
	err          error
	stdout       string
	stderr       string
	durationSecs int
}

type stepReceiptCompleter interface {
	CompleteStepWithReceipt(context.Context, projectmemory.CompleteStepWithReceiptArgs) error
}

func NewSpawnAgentTool(deps SpawnAgentDeps) *SpawnAgentTool {
	engineBinary := resolveEngineBinary(deps.EngineBinary)
	return &SpawnAgentTool{
		Store:         deps.Store,
		Backend:       deps.Backend,
		Config:        deps.Config,
		ChainID:       deps.ChainID,
		EngineBinary:  engineBinary,
		ProjectRoot:   deps.ProjectRoot,
		SubprocessEnv: deps.SubprocessEnv,
		runCommand:    RunCommand,
		now:           time.Now,
	}
}

func resolveEngineBinary(engineBinary string) string {
	engineBinary = strings.TrimSpace(engineBinary)
	if engineBinary == "" {
		engineBinary = "tidmouth"
	}
	if filepath.Base(engineBinary) != engineBinary {
		return engineBinary
	}
	executable, err := os.Executable()
	if err != nil {
		return engineBinary
	}
	sibling := filepath.Join(filepath.Dir(executable), engineBinary)
	if info, err := os.Stat(sibling); err == nil && !info.IsDir() {
		return sibling
	}
	return engineBinary
}

func (t *SpawnAgentTool) Name() string { return "spawn_agent" }
func (t *SpawnAgentTool) Description() string {
	return "Spawn a headless engine agent with the given role and task. Blocks until the engine completes. Returns the engine's receipt content."
}
func (t *SpawnAgentTool) ToolPurity() tool.Purity { return tool.Mutating }
func (t *SpawnAgentTool) Schema() json.RawMessage {
	return json.RawMessage(`{"name":"spawn_agent","description":"Spawn a headless engine agent with the given role and task. Blocks until the engine completes. Returns the engine's receipt content.","input_schema":{"type":"object","properties":{"role":{"type":"string","description":"Engine role config key or built-in persona name."},"task":{"type":"string","description":"Task description for the engine."},"task_context":{"type":"string","description":"Optional context identifier for resolver-loop tracking."},"reindex_before":{"type":"boolean","description":"Run code/brain reindexing before starting the engine.","default":false}},"required":["role","task"]}}`)
}

func (t *SpawnAgentTool) Execute(ctx context.Context, projectRoot string, raw json.RawMessage) (*tool.ToolResult, error) {
	var in spawnAgentInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("spawn_agent: parse input: %w", err)
	}

	_, content, err := t.RunStep(ctx, AgentStepInput{Role: in.Role, Task: in.Task, TaskContext: in.TaskContext, ReindexBefore: in.ReindexBefore})
	if err != nil {
		if content != "" {
			return &tool.ToolResult{Success: false, Content: content}, err
		}
		return nil, err
	}
	return &tool.ToolResult{Success: true, Content: content}, nil
}

func (t *SpawnAgentTool) RunStep(ctx context.Context, in AgentStepInput) (AgentStepResult, string, error) {
	step, err := t.prepareStep(ctx, spawnAgentInput{Role: in.Role, Task: in.Task, TaskContext: in.TaskContext, ReindexBefore: in.ReindexBefore})
	if err != nil {
		return AgentStepResult{}, "", err
	}
	outcome := t.runEngineStep(ctx, step)
	receiptContent, parsed, err := t.readStepReceipt(ctx, step, outcome)
	if err != nil {
		return AgentStepResult{StepID: step.stepID, Sequence: step.sequence, ReceiptPath: step.receiptPath, Status: "failed", DurationSecs: outcome.durationSecs, ExitCode: outcome.exitCode}, "", err
	}
	return t.recordStepOutcome(ctx, step, outcome, receiptContent, parsed)
}

func (t *SpawnAgentTool) prepareStep(ctx context.Context, in spawnAgentInput) (spawnStep, error) {
	roleName, roleCfg, err := t.Config.ResolveAgentRole(in.Role)
	if err != nil {
		if strings.Contains(err.Error(), "not found in config") {
			return spawnStep{}, fmt.Errorf("spawn_agent: role %q not defined in config", in.Role)
		}
		return spawnStep{}, fmt.Errorf("spawn_agent: %w", err)
	}
	chainRemainingTimeout, err := t.enforcePreSpawnLimits(ctx, roleName, in.TaskContext)
	if err != nil {
		return spawnStep{}, err
	}
	if stopErr := t.stopIfChainNotRunnable(ctx); stopErr != nil {
		return spawnStep{}, stopErr
	}
	if in.ReindexBefore {
		if err := t.reindex(ctx); err != nil {
			return spawnStep{}, fmt.Errorf("spawn_agent: reindex: %w", err)
		}
		chainRemainingTimeout, err = t.enforcePreSpawnLimits(ctx, roleName, in.TaskContext)
		if err != nil {
			return spawnStep{}, err
		}
	}
	if stopErr := t.stopIfChainNotRunnable(ctx); stopErr != nil {
		return spawnStep{}, stopErr
	}
	steps, err := t.Store.ListSteps(ctx, t.ChainID)
	if err != nil {
		return spawnStep{}, fmt.Errorf("spawn_agent: list steps: %w", err)
	}
	seq := len(steps) + 1
	receiptPath := receipt.StepPath(roleName, t.ChainID, seq)
	task := taskWithHarnessContext(in.Task, t.ChainID, seq, receiptPath)
	stepID, err := t.Store.StartStep(ctx, chain.StepSpec{ChainID: t.ChainID, SequenceNum: seq, Role: roleName, Task: in.Task, TaskContext: in.TaskContext})
	if err != nil {
		return spawnStep{}, fmt.Errorf("spawn_agent: create step: %w", err)
	}
	if err := t.Store.StepRunning(ctx, stepID); err != nil {
		return spawnStep{}, fmt.Errorf("spawn_agent: start step: %w", err)
	}
	_ = t.Store.LogEvent(ctx, t.ChainID, stepID, chain.EventStepStarted, map[string]any{"role": roleName, "task": in.Task, "receipt_path": receiptPath})
	return spawnStep{
		input:                 in,
		roleName:              roleName,
		roleCfg:               roleCfg,
		sequence:              seq,
		stepID:                stepID,
		receiptPath:           receiptPath,
		task:                  task,
		chainRemainingTimeout: chainRemainingTimeout,
	}, nil
}

func (t *SpawnAgentTool) runEngineStep(ctx context.Context, step spawnStep) engineRunOutcome {
	start := t.now()
	stdout := outputcap.NewBuffer(outputcap.DefaultLimit)
	stderr := outputcap.NewBuffer(outputcap.DefaultLimit)
	var enginePID int
	agentTimeout := resolveStepRunTimeout(step.roleCfg, step.chainRemainingTimeout)
	res := t.runCommand(ctx, RunCommandInput{
		Name:   t.EngineBinary,
		Args:   []string{"run", "--config", appconfig.ConfigFilename, "--role", step.roleName, "--task", step.task, "--chain-id", t.ChainID, "--receipt-path", step.receiptPath, "--timeout", agentTimeout.String()},
		Stdout: stdout,
		Stderr: stderr,
		OnStdoutLine: func(line string) {
			t.logStepOutput(ctx, step.stepID, "stdout", line)
		},
		OnStderrLine: func(line string) {
			t.logStepOutput(ctx, step.stepID, "stderr", line)
		},
		OnStart: func(pid int) {
			enginePID = pid
			t.logStepProcessStarted(ctx, step.stepID, step.roleName, pid)
		},
		Env:     t.SubprocessEnv,
		Dir:     t.ProjectRoot,
		Timeout: agentTimeout + parentTimeoutGraceDuration,
	})
	if enginePID > 0 {
		t.logStepProcessExited(ctx, step.stepID, enginePID, res.ExitCode)
	}
	durationSecs := int(t.now().Sub(start).Round(time.Second) / time.Second)
	return engineRunOutcome{
		exitCode:     res.ExitCode,
		err:          res.Err,
		stdout:       stdout.String(),
		stderr:       stderr.String(),
		durationSecs: durationSecs,
	}
}

func (t *SpawnAgentTool) readStepReceipt(ctx context.Context, step spawnStep, outcome engineRunOutcome) (string, receipt.Receipt, error) {
	receiptContent, readErr := t.Backend.ReadDocument(ctx, step.receiptPath)
	if readErr != nil {
		failMsg := fmt.Sprintf("missing receipt %s after exit_code=%d stdout=%q stderr=%q", step.receiptPath, outcome.exitCode, outcome.stdout, outcome.stderr)
		if writeErr := t.writeSyntheticSafetyReceipt(ctx, step.roleName, step.sequence, step.receiptPath, failMsg, outcome.durationSecs); writeErr != nil {
			failMsg = fmt.Sprintf("%s; failed to write safety receipt: %v", failMsg, writeErr)
		}
		_ = t.Store.FailStep(ctx, chain.CompleteStepParams{StepID: step.stepID, Verdict: string(receipt.VerdictSafetyLimit), ExitCode: intPtr(outcome.exitCode), ErrorMessage: failMsg, DurationSecs: outcome.durationSecs})
		_ = t.Store.LogEvent(ctx, t.ChainID, step.stepID, chain.EventStepFailed, map[string]any{"error": failMsg, "exit_code": outcome.exitCode})
		return "", receipt.Receipt{}, fmt.Errorf("spawn_agent: %s", failMsg)
	}
	parsed, err := receipt.Parse([]byte(receiptContent))
	if err != nil {
		failMsg := fmt.Sprintf("parse receipt %s: %v", step.receiptPath, err)
		_ = t.Store.FailStep(ctx, chain.CompleteStepParams{StepID: step.stepID, ExitCode: intPtr(outcome.exitCode), ErrorMessage: failMsg, DurationSecs: outcome.durationSecs})
		_ = t.Store.LogEvent(ctx, t.ChainID, step.stepID, chain.EventStepFailed, map[string]any{"error": failMsg, "exit_code": outcome.exitCode})
		return "", receipt.Receipt{}, fmt.Errorf("spawn_agent: %w", err)
	}
	return receiptContent, parsed, nil
}

func (t *SpawnAgentTool) recordStepOutcome(ctx context.Context, step spawnStep, outcome engineRunOutcome, receiptContent string, parsed receipt.Receipt) (AgentStepResult, string, error) {
	result := AgentStepResult{
		StepID:       step.stepID,
		Sequence:     step.sequence,
		ReceiptPath:  step.receiptPath,
		Verdict:      parsed.Verdict,
		TokensUsed:   parsed.TokensUsed,
		TurnsUsed:    parsed.TurnsUsed,
		DurationSecs: outcome.durationSecs,
		ExitCode:     outcome.exitCode,
	}
	if outcome.err != nil || infrastructureExitCode(outcome.exitCode) {
		failMsg := strings.TrimSpace(outcome.stderr)
		if failMsg == "" {
			failMsg = fmt.Sprintf("engine exited %d", outcome.exitCode)
		}
		result.Status = "failed"
		_ = t.Store.FailStep(ctx, chain.CompleteStepParams{StepID: step.stepID, Verdict: string(parsed.Verdict), ReceiptPath: step.receiptPath, TokensUsed: parsed.TokensUsed, TurnsUsed: parsed.TurnsUsed, DurationSecs: outcome.durationSecs, ExitCode: intPtr(outcome.exitCode), ErrorMessage: failMsg})
		_ = t.Store.LogEvent(ctx, t.ChainID, step.stepID, chain.EventStepFailed, map[string]any{"error": failMsg, "exit_code": outcome.exitCode})
		if outcome.err != nil {
			return result, "", fmt.Errorf("spawn_agent: run command: %w", outcome.err)
		}
		return result, "", fmt.Errorf("spawn_agent: %s", failMsg)
	}
	stepStatus := statusFromVerdict(parsed.Verdict)
	result.Status = stepStatus
	completeParams := chain.CompleteStepParams{StepID: step.stepID, Status: stepStatus, Verdict: string(parsed.Verdict), ReceiptPath: step.receiptPath, TokensUsed: parsed.TokensUsed, TurnsUsed: parsed.TurnsUsed, DurationSecs: outcome.durationSecs, ExitCode: intPtr(outcome.exitCode)}
	ch, err := t.Store.GetChain(ctx, t.ChainID)
	if err != nil {
		return result, "", fmt.Errorf("spawn_agent: load chain: %w", err)
	}
	metrics := chain.ChainMetrics{TotalSteps: ch.TotalSteps + 1, TotalTokens: ch.TotalTokens + parsed.TokensUsed, TotalDurationSecs: ch.TotalDurationSecs + outcome.durationSecs, ResolverLoops: ch.ResolverLoops}
	events := make([]projectmemory.CompleteStepWithReceiptEvent, 0, 2)
	if step.roleName == "resolver" {
		metrics.ResolverLoops++
		events = append(events, stepReceiptEvent(step.stepID, chain.EventResolverLoop, map[string]any{"task_context": step.input.TaskContext, "count": metrics.ResolverLoops}))
	}
	eventType := chain.EventStepCompleted
	if stepStatus == "failed" {
		eventType = chain.EventStepFailed
	}
	events = append(events, stepReceiptEvent(step.stepID, eventType, map[string]any{"verdict": parsed.Verdict, "tokens_used": parsed.TokensUsed, "turns_used": parsed.TurnsUsed, "duration_secs": outcome.durationSecs, "exit_code": outcome.exitCode}))
	if err := t.completeStepWithReceipt(ctx, step, completeParams, metrics, receiptContent, events); err != nil {
		return result, "", err
	}
	if limitErr := t.postStepLimitError(ch, metrics); limitErr != nil {
		summary := limitErr.Error()
		_ = t.Store.LogEvent(ctx, t.ChainID, step.stepID, chain.EventSafetyLimitHit, map[string]any{"role": step.roleName, "limit": summary})
		if err := chain.ApplyTerminalChainClosure(ctx, t.Store, t.ChainID, chain.TerminalChainClosure{
			Status:    "failed",
			EventType: chain.EventChainCompleted,
			Summary:   &summary,
			Extra:     map[string]any{"summary": summary, "reason": "safety_limit"},
		}); err != nil {
			return result, "", fmt.Errorf("spawn_agent: close chain after safety limit: %w", err)
		}
		return result, summary, fmt.Errorf("%w: %v", tool.ErrChainComplete, limitErr)
	}
	return result, receiptContent, nil
}

func (t *SpawnAgentTool) completeStepWithReceipt(ctx context.Context, step spawnStep, completeParams chain.CompleteStepParams, metrics chain.ChainMetrics, receiptContent string, events []projectmemory.CompleteStepWithReceiptEvent) error {
	if completer, ok := t.Backend.(stepReceiptCompleter); ok && completer != nil {
		now := t.now
		if now == nil {
			now = time.Now
		}
		args := projectmemory.CompleteStepWithReceiptArgs{
			StepID:            step.stepID,
			ChainID:           t.ChainID,
			Status:            completeParams.Status,
			Verdict:           completeParams.Verdict,
			ReceiptPath:       step.receiptPath,
			ReceiptContent:    receiptContent,
			TokensUsed:        uint64(nonNegativeInt(completeParams.TokensUsed)),
			TurnsUsed:         uint64(nonNegativeInt(completeParams.TurnsUsed)),
			DurationSecs:      uint64(nonNegativeInt(completeParams.DurationSecs)),
			HasExitCode:       completeParams.ExitCode != nil,
			Error:             completeParams.ErrorMessage,
			CompletedAtUS:     uint64(now().UTC().UnixMicro()),
			TotalSteps:        uint64(nonNegativeInt(metrics.TotalSteps)),
			TotalTokens:       uint64(nonNegativeInt(metrics.TotalTokens)),
			TotalDurationSecs: uint64(nonNegativeInt(metrics.TotalDurationSecs)),
			ResolverLoops:     uint64(nonNegativeInt(metrics.ResolverLoops)),
			Events:            events,
		}
		if completeParams.ExitCode != nil {
			args.ExitCode = int64(*completeParams.ExitCode)
		}
		if err := completer.CompleteStepWithReceipt(ctx, args); err != nil {
			return fmt.Errorf("spawn_agent: complete step with receipt: %w", err)
		}
		return nil
	}
	if err := t.Store.CompleteStep(ctx, completeParams); err != nil {
		return fmt.Errorf("spawn_agent: complete step: %w", err)
	}
	for _, event := range events[:len(events)-1] {
		_ = t.Store.LogEvent(ctx, t.ChainID, event.StepID, chain.EventType(event.EventType), json.RawMessage(event.PayloadJSON))
	}
	if err := t.Store.UpdateChainMetrics(ctx, t.ChainID, metrics); err != nil {
		return fmt.Errorf("spawn_agent: update chain metrics: %w", err)
	}
	lastEvent := events[len(events)-1]
	_ = t.Store.LogEvent(ctx, t.ChainID, lastEvent.StepID, chain.EventType(lastEvent.EventType), json.RawMessage(lastEvent.PayloadJSON))
	return nil
}

func stepReceiptEvent(stepID string, eventType chain.EventType, payload any) projectmemory.CompleteStepWithReceiptEvent {
	payloadJSON := "{}"
	if payload != nil {
		if data, err := json.Marshal(payload); err == nil {
			payloadJSON = string(data)
		}
	}
	return projectmemory.CompleteStepWithReceiptEvent{
		StepID:      stepID,
		EventType:   string(eventType),
		PayloadJSON: payloadJSON,
		CreatedAtUS: uint64(time.Now().UTC().UnixMicro()),
	}
}

func nonNegativeInt(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

func (t *SpawnAgentTool) enforcePreSpawnLimits(ctx context.Context, roleName string, taskContext string) (time.Duration, error) {
	if err := t.Store.CheckLimits(ctx, t.ChainID, chain.LimitCheckInput{Role: roleName, TaskContext: taskContext}); err != nil {
		if errors.Is(err, chain.ErrChainNotRunning) {
			if stopErr := t.stopIfChainNotRunnable(ctx); stopErr != nil {
				return 0, stopErr
			}
		}
		_ = t.Store.LogEvent(ctx, t.ChainID, "", chain.EventSafetyLimitHit, map[string]any{"role": roleName, "limit": err.Error()})
		return 0, fmt.Errorf("spawn_agent: %w", err)
	}
	remainingTimeout, err := t.Store.RemainingDuration(ctx, t.ChainID)
	if err != nil {
		return 0, fmt.Errorf("spawn_agent: remaining chain duration: %w", err)
	}
	if remainingTimeout <= 0 {
		limitErr := fmt.Errorf("%w (remaining=%s)", chain.ErrMaxDurationExceeded, remainingTimeout)
		_ = t.Store.LogEvent(ctx, t.ChainID, "", chain.EventSafetyLimitHit, map[string]any{"role": roleName, "limit": limitErr.Error()})
		return 0, fmt.Errorf("spawn_agent: %w", limitErr)
	}
	return remainingTimeout, nil
}

func (t *SpawnAgentTool) postStepLimitError(ch *chain.Chain, metrics chain.ChainMetrics) error {
	if metrics.TotalSteps > ch.MaxSteps {
		return fmt.Errorf("%w (current=%d max=%d)", chain.ErrMaxStepsExceeded, metrics.TotalSteps, ch.MaxSteps)
	}
	if metrics.TotalTokens > ch.TokenBudget {
		return fmt.Errorf("%w (current=%d budget=%d)", chain.ErrTokenBudgetExceeded, metrics.TotalTokens, ch.TokenBudget)
	}
	if metrics.TotalDurationSecs > ch.MaxDurationSecs {
		return fmt.Errorf("%w (duration=%ds max=%ds)", chain.ErrMaxDurationExceeded, metrics.TotalDurationSecs, ch.MaxDurationSecs)
	}
	return nil
}

func (t *SpawnAgentTool) stopIfChainNotRunnable(ctx context.Context) error {
	ch, err := t.Store.GetChain(ctx, t.ChainID)
	if err != nil {
		return fmt.Errorf("spawn_agent: load chain control state: %w", err)
	}
	if chain.ShouldStopScheduling(ch.Status) {
		return tool.ErrChainComplete
	}
	if ch.Status == "running" {
		return nil
	}
	return fmt.Errorf("spawn_agent: chain %s is %s", t.ChainID, ch.Status)
}

func (t *SpawnAgentTool) reindex(ctx context.Context) error {
	start := t.now()
	_ = t.Store.LogEvent(ctx, t.ChainID, "", chain.EventReindexStarted, map[string]any{"indexes": []string{"code", "brain"}})
	if err := t.runReindexCommand(ctx, "code", t.EngineBinary, []string{"index", "--config", appconfig.ConfigFilename, "--quiet"}); err != nil {
		return err
	}
	if t.Config != nil && t.Config.Brain.Enabled {
		if err := t.runReindexCommand(ctx, "brain", yardBinaryForEngine(t.EngineBinary), []string{"brain", "index", "--config", appconfig.ConfigFilename, "--quiet"}); err != nil {
			return err
		}
	}
	_ = t.Store.LogEvent(ctx, t.ChainID, "", chain.EventReindexCompleted, map[string]any{"duration_secs": int(t.now().Sub(start).Round(time.Second) / time.Second)})
	return nil
}

func (t *SpawnAgentTool) runReindexCommand(ctx context.Context, indexName string, commandName string, args []string) error {
	res := t.runCommand(ctx, RunCommandInput{Name: commandName, Args: args, Env: t.SubprocessEnv, Dir: t.ProjectRoot, Timeout: 10 * time.Minute})
	if res.Err != nil || res.ExitCode != 0 {
		return fmt.Errorf("%s index exited %d: %v", indexName, res.ExitCode, res.Err)
	}
	return nil
}

func yardBinaryForEngine(engineBinary string) string {
	engineBinary = strings.TrimSpace(engineBinary)
	if engineBinary == "" {
		return "yard"
	}
	if filepath.Base(engineBinary) != "tidmouth" {
		return "yard"
	}
	dir := filepath.Dir(engineBinary)
	if dir == "." || dir == "" {
		return "yard"
	}
	return filepath.Join(dir, "yard")
}

func (t *SpawnAgentTool) logStepOutput(ctx context.Context, stepID string, stream string, line string) {
	if strings.TrimSpace(line) == "" {
		return
	}
	_ = t.Store.LogEvent(ctx, t.ChainID, stepID, chain.EventStepOutput, map[string]any{"stream": stream, "line": line})
}

func (t *SpawnAgentTool) logStepProcessStarted(ctx context.Context, stepID string, role string, pid int) {
	if pid <= 0 {
		return
	}
	_ = t.Store.LogEvent(ctx, t.ChainID, stepID, chain.EventStepProcessStarted, map[string]any{"process_id": pid, "role": role, "active_process": true})
}

func (t *SpawnAgentTool) logStepProcessExited(ctx context.Context, stepID string, pid int, exitCode int) {
	if pid <= 0 {
		return
	}
	_ = t.Store.LogEvent(ctx, t.ChainID, stepID, chain.EventStepProcessExited, map[string]any{"process_id": pid, "exit_code": exitCode})
}

func (t *SpawnAgentTool) writeSyntheticSafetyReceipt(ctx context.Context, role string, step int, receiptPath string, reason string, durationSecs int) error {
	timestamp := t.now().UTC().Format(time.RFC3339)
	body := strings.TrimSpace(reason)
	if body == "" {
		body = "The engine did not produce a receipt before the harness stopped it."
	}
	content := fmt.Sprintf(`---
agent: %s
chain_id: %s
step: %d
verdict: %s
timestamp: %s
turns_used: 0
tokens_used: 0
duration_seconds: %d
---

%s
`, role, t.ChainID, step, receipt.VerdictSafetyLimit, timestamp, durationSecs, body)
	return t.Backend.WriteDocument(ctx, receiptPath, content)
}

func statusFromVerdict(v receipt.Verdict) string {
	switch v {
	case receipt.VerdictCompleted, receipt.VerdictCompletedWithConcerns, receipt.VerdictCompletedNoReceipt, receipt.VerdictFixRequired, receipt.VerdictBlocked, receipt.VerdictEscalate, receipt.VerdictSafetyLimit:
		return "completed"
	default:
		return "failed"
	}
}

func resolveAgentRunTimeout(roleCfg appconfig.AgentRoleConfig) time.Duration {
	timeout := roleCfg.Timeout.Duration()
	if timeout <= 0 {
		return defaultAgentRunTimeout
	}
	return timeout
}

func resolveStepRunTimeout(roleCfg appconfig.AgentRoleConfig, chainRemainingTimeout time.Duration) time.Duration {
	timeout := resolveAgentRunTimeout(roleCfg)
	if chainRemainingTimeout > 0 && chainRemainingTimeout < timeout {
		return chainRemainingTimeout
	}
	return timeout
}

func infrastructureExitCode(code int) bool {
	return code != 0 && code != 2 && code != 3
}

func intPtr(v int) *int { return &v }

func taskWithHarnessContext(task string, chainID string, step int, receiptPath string) string {
	return fmt.Sprintf(`%s

Harness context:
- Chain ID: %s
- Step number: %d
- Receipt path: %s

Before finishing, write your receipt to the exact brain path above. If you cannot complete the task, still write the receipt there with the appropriate verdict and concerns.

Receipt frontmatter must be valid YAML and include these required fields:
---
agent: <role name>
chain_id: %s
step: %d
verdict: completed
timestamp: <current UTC time in RFC3339 format>
turns_used: 0
tokens_used: 0
duration_seconds: 0
---

Use the actual verdict and usage numbers when known. Do not use created_at; the required completion-time field is timestamp.

The receipt body must include the concrete outcome of the task. If the task asks a question, put the answer in the Summary section rather than only saying that you found it.`, strings.TrimSpace(task), chainID, step, receiptPath, chainID, step)
}
