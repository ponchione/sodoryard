package spawn

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ponchione/sodoryard/internal/brain"
	"github.com/ponchione/sodoryard/internal/chain"
	appconfig "github.com/ponchione/sodoryard/internal/config"
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

type spawnAgentInput struct {
	Role          string `json:"role"`
	Task          string `json:"task"`
	TaskContext   string `json:"task_context,omitempty"`
	ReindexBefore bool   `json:"reindex_before,omitempty"`
}

func NewSpawnAgentTool(deps SpawnAgentDeps) *SpawnAgentTool {
	engineBinary := deps.EngineBinary
	if strings.TrimSpace(engineBinary) == "" {
		engineBinary = "tidmouth"
	}
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

func (t *SpawnAgentTool) Name() string { return "spawn_agent" }
func (t *SpawnAgentTool) Description() string {
	return "Spawn a headless engine agent with the given role and task. Blocks until the engine completes. Returns the engine's receipt content."
}
func (t *SpawnAgentTool) ToolPurity() tool.Purity { return tool.Mutating }
func (t *SpawnAgentTool) Schema() json.RawMessage {
	return json.RawMessage(`{"name":"spawn_agent","description":"Spawn a headless engine agent with the given role and task. Blocks until the engine completes. Returns the engine's receipt content.","input_schema":{"type":"object","properties":{"role":{"type":"string","description":"Engine role name from the config."},"task":{"type":"string","description":"Task description for the engine."},"task_context":{"type":"string","description":"Optional context identifier for resolver-loop tracking."},"reindex_before":{"type":"boolean","description":"Run code/brain reindexing before starting the engine.","default":false}},"required":["role","task"]}}`)
}

func (t *SpawnAgentTool) Execute(ctx context.Context, projectRoot string, raw json.RawMessage) (*tool.ToolResult, error) {
	var in spawnAgentInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("spawn_agent: parse input: %w", err)
	}
	if _, ok := t.Config.AgentRoles[in.Role]; !ok {
		return nil, fmt.Errorf("spawn_agent: role %q not defined in config", in.Role)
	}
	if err := t.Store.CheckLimits(ctx, t.ChainID, chain.LimitCheckInput{Role: in.Role, TaskContext: in.TaskContext}); err != nil {
		if errors.Is(err, chain.ErrChainNotRunning) {
			if stopErr := t.stopIfChainNotRunnable(ctx); stopErr != nil {
				return nil, stopErr
			}
		}
		_ = t.Store.LogEvent(ctx, t.ChainID, "", chain.EventSafetyLimitHit, map[string]any{"role": in.Role, "limit": err.Error()})
		return nil, fmt.Errorf("spawn_agent: %w", err)
	}
	if stopErr := t.stopIfChainNotRunnable(ctx); stopErr != nil {
		return nil, stopErr
	}
	if in.ReindexBefore {
		if err := t.reindex(ctx); err != nil {
			return nil, fmt.Errorf("spawn_agent: reindex: %w", err)
		}
	}
	if stopErr := t.stopIfChainNotRunnable(ctx); stopErr != nil {
		return nil, stopErr
	}
	steps, err := t.Store.ListSteps(ctx, t.ChainID)
	if err != nil {
		return nil, fmt.Errorf("spawn_agent: list steps: %w", err)
	}
	seq := len(steps) + 1
	receiptPath := fmt.Sprintf("receipts/%s/%s-step-%03d.md", in.Role, t.ChainID, seq)
	stepID, err := t.Store.StartStep(ctx, chain.StepSpec{ChainID: t.ChainID, SequenceNum: seq, Role: in.Role, Task: in.Task, TaskContext: in.TaskContext})
	if err != nil {
		return nil, fmt.Errorf("spawn_agent: create step: %w", err)
	}
	if err := t.Store.StepRunning(ctx, stepID); err != nil {
		return nil, fmt.Errorf("spawn_agent: start step: %w", err)
	}
	_ = t.Store.LogEvent(ctx, t.ChainID, stepID, chain.EventStepStarted, map[string]any{"role": in.Role, "task": in.Task, "receipt_path": receiptPath})
	start := t.now()
	var stdout, stderr bytes.Buffer
	res := t.runCommand(ctx, RunCommandInput{
		Name:    t.EngineBinary,
		Args:    []string{"run", "--config", appconfig.ConfigFilename, "--role", in.Role, "--task", in.Task, "--chain-id", t.ChainID, "--receipt-path", receiptPath, "--quiet"},
		Stdout:  &stdout,
		Stderr:  &stderr,
		Env:     t.SubprocessEnv,
		Dir:     t.ProjectRoot,
		Timeout: 30 * time.Minute,
	})
	durationSecs := int(t.now().Sub(start).Round(time.Second) / time.Second)
	receiptContent, readErr := t.Backend.ReadDocument(ctx, receiptPath)
	if readErr != nil {
		failMsg := fmt.Sprintf("missing receipt %s after exit_code=%d stdout=%q stderr=%q", receiptPath, res.ExitCode, stdout.String(), stderr.String())
		_ = t.Store.FailStep(ctx, chain.CompleteStepParams{StepID: stepID, Verdict: string(receipt.VerdictSafetyLimit), ExitCode: intPtr(res.ExitCode), ErrorMessage: failMsg, DurationSecs: durationSecs})
		_ = t.Store.LogEvent(ctx, t.ChainID, stepID, chain.EventStepFailed, map[string]any{"error": failMsg, "exit_code": res.ExitCode})
		return nil, fmt.Errorf("spawn_agent: %s", failMsg)
	}
	parsed, err := receipt.Parse([]byte(receiptContent))
	if err != nil {
		failMsg := fmt.Sprintf("parse receipt %s: %v", receiptPath, err)
		_ = t.Store.FailStep(ctx, chain.CompleteStepParams{StepID: stepID, ExitCode: intPtr(res.ExitCode), ErrorMessage: failMsg, DurationSecs: durationSecs})
		_ = t.Store.LogEvent(ctx, t.ChainID, stepID, chain.EventStepFailed, map[string]any{"error": failMsg, "exit_code": res.ExitCode})
		return nil, fmt.Errorf("spawn_agent: %w", err)
	}
	stepStatus := statusFromVerdict(parsed.Verdict)
	if res.ExitCode != 0 && stepStatus == "completed" {
		stepStatus = "failed"
	}
	completeParams := chain.CompleteStepParams{StepID: stepID, Status: stepStatus, Verdict: string(parsed.Verdict), ReceiptPath: receiptPath, TokensUsed: parsed.TokensUsed, TurnsUsed: parsed.TurnsUsed, DurationSecs: durationSecs, ExitCode: intPtr(res.ExitCode)}
	if stepStatus == "failed" && res.ExitCode != 0 {
		completeParams.ErrorMessage = strings.TrimSpace(stderr.String())
	}
	if err := t.Store.CompleteStep(ctx, completeParams); err != nil {
		return nil, fmt.Errorf("spawn_agent: complete step: %w", err)
	}
	ch, err := t.Store.GetChain(ctx, t.ChainID)
	if err != nil {
		return nil, fmt.Errorf("spawn_agent: load chain: %w", err)
	}
	metrics := chain.ChainMetrics{TotalSteps: ch.TotalSteps + 1, TotalTokens: ch.TotalTokens + parsed.TokensUsed, TotalDurationSecs: ch.TotalDurationSecs + durationSecs, ResolverLoops: ch.ResolverLoops}
	if in.Role == "resolver" {
		metrics.ResolverLoops++
		_ = t.Store.LogEvent(ctx, t.ChainID, stepID, chain.EventResolverLoop, map[string]any{"task_context": in.TaskContext, "count": metrics.ResolverLoops})
	}
	if err := t.Store.UpdateChainMetrics(ctx, t.ChainID, metrics); err != nil {
		return nil, fmt.Errorf("spawn_agent: update chain metrics: %w", err)
	}
	eventType := chain.EventStepCompleted
	if stepStatus == "failed" {
		eventType = chain.EventStepFailed
	}
	_ = t.Store.LogEvent(ctx, t.ChainID, stepID, eventType, map[string]any{"verdict": parsed.Verdict, "tokens_used": parsed.TokensUsed, "turns_used": parsed.TurnsUsed, "duration_secs": durationSecs, "exit_code": res.ExitCode})
	if res.Err != nil {
		return nil, fmt.Errorf("spawn_agent: run command: %w", res.Err)
	}
	return &tool.ToolResult{Success: true, Content: receiptContent}, nil
}

func (t *SpawnAgentTool) stopIfChainNotRunnable(ctx context.Context) error {
	ch, err := t.Store.GetChain(ctx, t.ChainID)
	if err != nil {
		return fmt.Errorf("spawn_agent: load chain control state: %w", err)
	}
	switch ch.Status {
	case "running":
		return nil
	case "paused":
		return tool.ErrChainComplete
	case "cancelled":
		return tool.ErrChainComplete
	default:
		return fmt.Errorf("spawn_agent: chain %s is %s", t.ChainID, ch.Status)
	}
}

func (t *SpawnAgentTool) reindex(ctx context.Context) error {
	start := t.now()
	_ = t.Store.LogEvent(ctx, t.ChainID, "", chain.EventReindexStarted, nil)
	res := t.runCommand(ctx, RunCommandInput{Name: t.EngineBinary, Args: []string{"index", "--config", appconfig.ConfigFilename, "--quiet"}, Env: t.SubprocessEnv, Dir: t.ProjectRoot, Timeout: 10 * time.Minute})
	if res.Err != nil || res.ExitCode != 0 {
		return fmt.Errorf("index exited %d: %v", res.ExitCode, res.Err)
	}
	_ = t.Store.LogEvent(ctx, t.ChainID, "", chain.EventReindexCompleted, map[string]any{"duration_secs": int(t.now().Sub(start).Round(time.Second) / time.Second)})
	return nil
}

func statusFromVerdict(v receipt.Verdict) string {
	switch v {
	case receipt.VerdictCompleted, receipt.VerdictCompletedNoReceipt:
		return "completed"
	default:
		return "failed"
	}
}

func intPtr(v int) *int { return &v }
