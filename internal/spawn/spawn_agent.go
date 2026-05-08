package spawn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ponchione/sodoryard/internal/brain"
	"github.com/ponchione/sodoryard/internal/chain"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/id"
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
	defaultAgentRunTimeout            = 30 * time.Minute
	parentTimeoutGraceDuration        = 10 * time.Second
	sourceWriterLockGraceDuration     = time.Minute
	sourceWriterLockHeartbeatInterval = time.Minute
)

type spawnAgentInput struct {
	Role          string `json:"role"`
	Task          string `json:"task"`
	TaskContext   string `json:"task_context,omitempty"`
	ReindexBefore bool   `json:"reindex_before,omitempty"`
	MaxTurns      int    `json:"max_turns,omitempty"`
	MaxTokens     int    `json:"max_tokens,omitempty"`
}

type AgentStepInput struct {
	Role          string
	Task          string
	TaskContext   string
	ReindexBefore bool
	MaxTurns      int
	MaxTokens     int
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
	input                     spawnAgentInput
	roleName                  string
	roleCfg                   appconfig.AgentRoleConfig
	sequence                  int
	stepID                    string
	receiptPath               string
	task                      string
	sourceMutating            bool
	chainRemainingTimeout     time.Duration
	maxTurns                  int
	maxTokens                 int
	sourceWriterLockOwned     bool
	sourceWriterLockExpiresAt time.Time
}

type engineRunOutcome struct {
	exitCode     int
	err          error
	stdout       string
	stderr       string
	durationSecs int
}

type changedFileCapture struct {
	Paths []string
	Error string
}

type postStepGuardrailFacts struct {
	Role                                string   `json:"role"`
	Sequence                            int      `json:"sequence"`
	ReceiptPath                         string   `json:"receipt_path"`
	SourceMutating                      bool     `json:"source_mutating"`
	ExitCode                            int      `json:"exit_code"`
	DurationSecs                        int      `json:"duration_secs"`
	ReceiptPresent                      bool     `json:"receipt_present"`
	SyntheticReceiptWritten             bool     `json:"synthetic_receipt_written"`
	ReceiptSchemaValid                  bool     `json:"receipt_schema_valid"`
	ReceiptStepValid                    bool     `json:"receipt_step_valid"`
	ReceiptSectionsValid                bool     `json:"receipt_sections_valid"`
	ReceiptValid                        bool     `json:"receipt_valid"`
	ReceiptError                        string   `json:"receipt_error,omitempty"`
	ParsedVerdict                       string   `json:"parsed_verdict,omitempty"`
	TokensUsed                          int      `json:"tokens_used"`
	TurnsUsed                           int      `json:"turns_used"`
	ReceiptDurationSeconds              int      `json:"receipt_duration_seconds"`
	ClaimedValidationCommands           []string `json:"claimed_validation_commands"`
	ChangedFileClaimPresent             bool     `json:"changed_file_claim_present"`
	ClaimedChangedFiles                 []string `json:"claimed_changed_files"`
	ChangedFileClaimMatchesManifest     bool     `json:"changed_file_claim_matches_manifest"`
	ChangedFileClaimExtra               []string `json:"changed_file_claim_extra"`
	ChangedFileManifestUnclaimed        []string `json:"changed_file_manifest_unclaimed"`
	ChangedFileManifestPresent          bool     `json:"changed_file_manifest_present"`
	ChangedFileManifestError            string   `json:"changed_file_manifest_error,omitempty"`
	ChangedFileCount                    int      `json:"changed_file_count"`
	ChangedFiles                        []string `json:"changed_files"`
	SourceWriterLockReleaseAttempted    bool     `json:"source_writer_lock_release_attempted"`
	SourceWriterLockReleased            bool     `json:"source_writer_lock_released"`
	SourceWriterLockReleaseError        string   `json:"source_writer_lock_release_error,omitempty"`
	FindingCount                        int      `json:"finding_count"`
	OpenFindingCount                    int      `json:"open_finding_count"`
	ClosedFindingCount                  int      `json:"closed_finding_count"`
	AddressedFindingCount               int      `json:"addressed_finding_count"`
	FindingIDs                          []string `json:"finding_ids"`
	OpenFindingIDs                      []string `json:"open_finding_ids"`
	ClosedFindingIDs                    []string `json:"closed_finding_ids"`
	AddressedIDs                        []string `json:"addressed_ids"`
	SuspiciousVerdictFindingCombination bool     `json:"suspicious_verdict_finding_combination"`
	SuspiciousVerdictFindingReason      string   `json:"suspicious_verdict_finding_reason,omitempty"`
	RunError                            string   `json:"run_error,omitempty"`
}

type receiptFindingFactSet struct {
	FindingCount                        int
	OpenFindingCount                    int
	ClosedFindingCount                  int
	AddressedFindingCount               int
	FindingIDs                          []string
	OpenFindingIDs                      []string
	ClosedFindingIDs                    []string
	AddressedIDs                        []string
	LifecycleFacts                      []chain.FindingLifecycleFact
	SuspiciousVerdictFindingCombination bool
	SuspiciousVerdictFindingReason      string
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
	return json.RawMessage(`{"name":"spawn_agent","description":"Spawn a headless engine agent with the given role and task. Blocks until the engine completes. Returns the engine's receipt content.","input_schema":{"type":"object","properties":{"role":{"type":"string","description":"Engine role config key or built-in persona name."},"task":{"type":"string","description":"Task description for the engine."},"task_context":{"type":"string","description":"Optional context identifier for resolver-loop tracking."},"reindex_before":{"type":"boolean","description":"Run code/brain reindexing before starting the engine.","default":false},"max_turns":{"type":"integer","description":"Optional per-step override for the spawned headless agent's maximum model iterations."},"max_tokens":{"type":"integer","description":"Optional per-step override for the spawned headless agent's total token ceiling."}},"required":["role","task"]}}`)
}

func (t *SpawnAgentTool) Execute(ctx context.Context, projectRoot string, raw json.RawMessage) (*tool.ToolResult, error) {
	var in spawnAgentInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("spawn_agent: parse input: %w", err)
	}

	_, content, err := t.RunStep(ctx, AgentStepInput{Role: in.Role, Task: in.Task, TaskContext: in.TaskContext, ReindexBefore: in.ReindexBefore, MaxTurns: in.MaxTurns, MaxTokens: in.MaxTokens})
	if err != nil {
		if content != "" {
			return &tool.ToolResult{Success: false, Content: content}, err
		}
		return nil, err
	}
	return &tool.ToolResult{Success: true, Content: content}, nil
}

func (t *SpawnAgentTool) RunStep(ctx context.Context, in AgentStepInput) (result AgentStepResult, receiptContent string, err error) {
	step, err := t.prepareStep(ctx, spawnAgentInput{Role: in.Role, Task: in.Task, TaskContext: in.TaskContext, ReindexBefore: in.ReindexBefore, MaxTurns: in.MaxTurns, MaxTokens: in.MaxTokens})
	if err != nil {
		return AgentStepResult{}, "", err
	}
	facts := newPostStepGuardrailFacts(step)
	defer func() {
		factCtx := detachedLockContext(ctx)
		if step.sourceWriterLockOwned {
			facts.SourceWriterLockReleaseAttempted = true
			if releaseErr := t.releaseSourceWriterLock(factCtx, step); releaseErr != nil {
				facts.SourceWriterLockReleaseError = releaseErr.Error()
			} else {
				facts.SourceWriterLockReleased = true
			}
		}
		if err != nil {
			facts.RunError = err.Error()
		}
		t.logPostStepGuardrailFacts(factCtx, step, facts)
	}()
	outcome := t.runEngineStep(ctx, step)
	facts.ExitCode = outcome.exitCode
	facts.DurationSecs = outcome.durationSecs
	if step.sourceMutating {
		capture := t.captureChangedFiles(ctx, step)
		facts.ChangedFileManifestPresent = true
		facts.ChangedFiles = append([]string(nil), capture.Paths...)
		facts.ChangedFileCount = len(capture.Paths)
		facts.ChangedFileManifestError = capture.Error
	}
	receiptContent, parsed, err := t.readStepReceipt(ctx, step, outcome, facts)
	if err != nil {
		result = AgentStepResult{StepID: step.stepID, Sequence: step.sequence, ReceiptPath: step.receiptPath, Status: "failed", DurationSecs: outcome.durationSecs, ExitCode: outcome.exitCode}
		return result, "", err
	}
	result, receiptContent, err = t.recordStepOutcome(ctx, step, outcome, receiptContent, parsed)
	return result, receiptContent, err
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
	sourceMutating := appconfig.IsSourceWritingRole(roleName, roleCfg)
	steps, err := t.Store.ListSteps(ctx, t.ChainID)
	if err != nil {
		return spawnStep{}, fmt.Errorf("spawn_agent: list steps: %w", err)
	}
	seq := len(steps) + 1
	receiptPath := receipt.StepPath(roleName, t.ChainID, seq)
	stepID := id.New()
	ch, err := t.Store.GetChain(ctx, t.ChainID)
	if err != nil {
		return spawnStep{}, fmt.Errorf("spawn_agent: load chain briefing state: %w", err)
	}
	events, err := t.Store.ListEvents(ctx, t.ChainID)
	if err != nil {
		return spawnStep{}, fmt.Errorf("spawn_agent: load chain briefing events: %w", err)
	}
	briefing := chain.BuildStepBriefing(chain.StepBriefingInput{
		Chain:               *ch,
		Steps:               steps,
		Events:              events,
		CurrentStepSequence: seq,
		CurrentRole:         roleName,
		ReceiptPath:         receiptPath,
	})
	task := taskWithHarnessContext(in.Task, t.ChainID, seq, receiptPath, briefing)
	step := spawnStep{
		input:                 in,
		roleName:              roleName,
		roleCfg:               roleCfg,
		sequence:              seq,
		stepID:                stepID,
		receiptPath:           receiptPath,
		task:                  task,
		sourceMutating:        sourceMutating,
		chainRemainingTimeout: chainRemainingTimeout,
		maxTurns:              in.MaxTurns,
		maxTokens:             in.MaxTokens,
	}
	if sourceMutating {
		if err := t.acquireSourceWriterLock(ctx, &step); err != nil {
			return spawnStep{}, err
		}
	}
	releaseOnError := true
	defer func() {
		if releaseOnError && step.sourceWriterLockOwned {
			_ = t.releaseSourceWriterLock(detachedLockContext(ctx), step)
		}
	}()
	_, err = t.Store.StartStep(ctx, chain.StepSpec{StepID: stepID, ChainID: t.ChainID, SequenceNum: seq, Role: roleName, Task: in.Task, TaskContext: in.TaskContext})
	if err != nil {
		return spawnStep{}, fmt.Errorf("spawn_agent: create step: %w", err)
	}
	if err := t.Store.StepRunning(ctx, stepID); err != nil {
		return spawnStep{}, fmt.Errorf("spawn_agent: start step: %w", err)
	}
	_ = t.Store.LogEvent(ctx, t.ChainID, stepID, chain.EventStepStarted, map[string]any{"role": roleName, "task": in.Task, "receipt_path": receiptPath})
	releaseOnError = false
	return step, nil
}

func (t *SpawnAgentTool) acquireSourceWriterLock(ctx context.Context, step *spawnStep) error {
	if t.Store == nil {
		return fmt.Errorf("spawn_agent: source writer guard: chain store is nil")
	}
	acquiredAt := t.now().UTC()
	expiresAt := acquiredAt.Add(sourceWriterLockTTL(step.roleCfg, step.chainRemainingTimeout))
	result, err := t.Store.AcquireProjectLock(ctx, chain.AcquireProjectLockParams{
		LockName:     chain.SourceWriterLockName,
		OwnerChainID: t.ChainID,
		OwnerStepID:  step.stepID,
		OwnerRole:    step.roleName,
		AcquiredAt:   acquiredAt,
		ExpiresAt:    expiresAt,
		MetadataJSON: mustMarshalString(map[string]any{
			"receipt_path":   step.receiptPath,
			"sequence":       step.sequence,
			"mutation_class": appconfig.MutationClassSourceWrite,
		}),
	})
	if err != nil {
		event := map[string]any{
			"requested_role":     step.roleName,
			"requested_step_id":  step.stepID,
			"requested_sequence": step.sequence,
			"lock_name":          chain.SourceWriterLockName,
			"mutation_class":     appconfig.MutationClassSourceWrite,
			"error":              err.Error(),
		}
		if lock, found, readErr := t.Store.GetProjectLock(ctx, chain.SourceWriterLockName); readErr == nil && found {
			event["owner_chain_id"] = lock.OwnerChainID
			event["owner_step_id"] = lock.OwnerStepID
			event["owner_role"] = lock.OwnerRole
			event["expires_at"] = lock.ExpiresAt.Format(time.RFC3339)
		}
		_ = t.Store.LogEvent(ctx, t.ChainID, "", chain.EventSourceWriterBlocked, event)
		return fmt.Errorf("spawn_agent: source writer guard: %w", err)
	}
	step.sourceWriterLockOwned = true
	step.sourceWriterLockExpiresAt = result.Lock.ExpiresAt
	_ = t.Store.LogEvent(ctx, t.ChainID, step.stepID, chain.EventSourceWriterLockAcquired, map[string]any{
		"lock_name":      chain.SourceWriterLockName,
		"owner_chain_id": t.ChainID,
		"owner_step_id":  step.stepID,
		"owner_role":     step.roleName,
		"expires_at":     result.Lock.ExpiresAt.Format(time.RFC3339),
	})
	if result.ReplacedLockOwnerStepID != "" {
		_ = t.Store.LogEvent(ctx, t.ChainID, step.stepID, chain.EventSourceWriterLockStaleReplaced, map[string]any{
			"lock_name":           chain.SourceWriterLockName,
			"previous_chain_id":   result.ReplacedLockOwnerChainID,
			"previous_step_id":    result.ReplacedLockOwnerStepID,
			"previous_role":       result.ReplacedLockOwnerRole,
			"previous_expires_at": result.ReplacedLockExpiredAt.Format(time.RFC3339),
		})
	}
	return nil
}

func (t *SpawnAgentTool) releaseSourceWriterLock(ctx context.Context, step spawnStep) error {
	err := t.Store.ReleaseProjectLock(ctx, chain.ReleaseProjectLockParams{
		LockName:     chain.SourceWriterLockName,
		OwnerChainID: t.ChainID,
		OwnerStepID:  step.stepID,
	})
	if err != nil {
		_ = t.Store.LogEvent(ctx, t.ChainID, step.stepID, chain.EventSourceWriterLockReleaseFailed, map[string]any{
			"lock_name":      chain.SourceWriterLockName,
			"owner_chain_id": t.ChainID,
			"owner_step_id":  step.stepID,
			"owner_role":     step.roleName,
			"error":          err.Error(),
		})
		return err
	}
	_ = t.Store.LogEvent(ctx, t.ChainID, step.stepID, chain.EventSourceWriterLockReleased, map[string]any{
		"lock_name":      chain.SourceWriterLockName,
		"owner_chain_id": t.ChainID,
		"owner_step_id":  step.stepID,
		"owner_role":     step.roleName,
	})
	return nil
}

func (t *SpawnAgentTool) heartbeatSourceWriterLock(ctx context.Context, step spawnStep) error {
	expiresAt := t.now().UTC().Add(sourceWriterLockTTL(step.roleCfg, step.chainRemainingTimeout))
	return t.Store.HeartbeatProjectLock(ctx, chain.HeartbeatProjectLockParams{
		LockName:     chain.SourceWriterLockName,
		OwnerChainID: t.ChainID,
		OwnerStepID:  step.stepID,
		ExpiresAt:    expiresAt,
	})
}

func sourceWriterLockTTL(roleCfg appconfig.AgentRoleConfig, chainRemainingTimeout time.Duration) time.Duration {
	return resolveStepRunTimeout(roleCfg, chainRemainingTimeout) + parentTimeoutGraceDuration + sourceWriterLockGraceDuration
}

func (t *SpawnAgentTool) runEngineStep(ctx context.Context, step spawnStep) engineRunOutcome {
	start := t.now()
	stdout := outputcap.NewBuffer(outputcap.DefaultLimit)
	stderr := outputcap.NewBuffer(outputcap.DefaultLimit)
	var enginePID int
	agentTimeout := resolveStepRunTimeout(step.roleCfg, step.chainRemainingTimeout)
	stopHeartbeat := t.startSourceWriterLockHeartbeat(ctx, step)
	defer stopHeartbeat()
	res := t.runCommand(ctx, RunCommandInput{
		Name:   t.EngineBinary,
		Args:   buildEngineRunArgs(step, t.ChainID, agentTimeout),
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

func (t *SpawnAgentTool) startSourceWriterLockHeartbeat(ctx context.Context, step spawnStep) func() {
	if !step.sourceWriterLockOwned || t.Store == nil {
		return func() {}
	}
	heartbeatCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(sourceWriterLockHeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeatCtx.Done():
				return
			case <-ticker.C:
				lockCtx, cancelLock := context.WithTimeout(context.Background(), 30*time.Second)
				if err := t.heartbeatSourceWriterLock(lockCtx, step); err != nil {
					_ = t.Store.LogEvent(lockCtx, t.ChainID, step.stepID, chain.EventSourceWriterLockHeartbeatFailed, map[string]any{
						"lock_name":      chain.SourceWriterLockName,
						"owner_chain_id": t.ChainID,
						"owner_step_id":  step.stepID,
						"owner_role":     step.roleName,
						"operation":      "heartbeat",
						"error":          err.Error(),
					})
				}
				cancelLock()
			}
		}
	}()
	return func() {
		cancel()
		<-done
	}
}

func (t *SpawnAgentTool) captureChangedFiles(ctx context.Context, step spawnStep) changedFileCapture {
	captureCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	stdout := outputcap.NewBuffer(outputcap.DefaultLimit)
	stderr := outputcap.NewBuffer(outputcap.DefaultLimit)
	cmd := exec.CommandContext(captureCtx, "git", "status", "--short", "--untracked-files=all")
	cmd.Dir = t.ProjectRoot
	if len(t.SubprocessEnv) > 0 {
		cmd.Env = append(os.Environ(), t.SubprocessEnv...)
	}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	paths := parseGitStatusChangedFiles(stdout.String())
	payload := map[string]any{
		"paths": paths,
		"count": len(paths),
	}
	var captureErr string
	if err != nil {
		captureErr = strings.TrimSpace(err.Error())
		payload["error"] = captureErr
		if text := strings.TrimSpace(stderr.String()); text != "" {
			payload["stderr"] = text
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			payload["exit_code"] = exitErr.ExitCode()
		}
	}
	_ = t.Store.LogEvent(ctx, t.ChainID, step.stepID, chain.EventStepChangedFiles, payload)
	return changedFileCapture{Paths: paths, Error: captureErr}
}

func parseGitStatusChangedFiles(status string) []string {
	seen := map[string]struct{}{}
	for _, line := range strings.Split(status, "\n") {
		if len(line) < 4 {
			continue
		}
		pathPart := strings.TrimSpace(line[3:])
		if pathPart == "" {
			continue
		}
		if before, after, ok := strings.Cut(pathPart, " -> "); ok {
			addGitStatusPath(seen, before)
			addGitStatusPath(seen, after)
			continue
		}
		addGitStatusPath(seen, pathPart)
	}
	paths := make([]string, 0, len(seen))
	for path := range seen {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func addGitStatusPath(seen map[string]struct{}, raw string) {
	path := strings.TrimSpace(raw)
	if path == "" {
		return
	}
	if unquoted, err := strconv.Unquote(path); err == nil {
		path = unquoted
	}
	if path != "" {
		seen[path] = struct{}{}
	}
}

func buildEngineRunArgs(step spawnStep, chainID string, agentTimeout time.Duration) []string {
	args := []string{"run", "--config", appconfig.ConfigFilename, "--role", step.roleName, "--task", step.task, "--chain-id", chainID, "--receipt-path", step.receiptPath, "--timeout", agentTimeout.String()}
	if step.maxTurns > 0 {
		args = append(args, "--max-turns", fmt.Sprintf("%d", step.maxTurns))
	}
	if step.maxTokens > 0 {
		args = append(args, "--max-tokens", fmt.Sprintf("%d", step.maxTokens))
	}
	return args
}

func (t *SpawnAgentTool) readStepReceipt(ctx context.Context, step spawnStep, outcome engineRunOutcome, facts *postStepGuardrailFacts) (string, receipt.Receipt, error) {
	receiptContent, readErr := t.Backend.ReadDocument(ctx, step.receiptPath)
	if readErr != nil {
		failMsg := fmt.Sprintf("missing receipt %s after exit_code=%d stdout=%q stderr=%q", step.receiptPath, outcome.exitCode, outcome.stdout, outcome.stderr)
		receiptPath := step.receiptPath
		if writeErr := t.writeSyntheticSafetyReceipt(ctx, step.roleName, step.sequence, step.receiptPath, failMsg, outcome.durationSecs); writeErr != nil {
			failMsg = fmt.Sprintf("%s; failed to write safety receipt: %v", failMsg, writeErr)
			receiptPath = ""
		} else if facts != nil {
			facts.SyntheticReceiptWritten = true
		}
		if facts != nil {
			facts.ReceiptPresent = false
			facts.ReceiptError = failMsg
		}
		_ = t.Store.FailStep(ctx, chain.CompleteStepParams{StepID: step.stepID, Verdict: string(receipt.VerdictSafetyLimit), ReceiptPath: receiptPath, ExitCode: intPtr(outcome.exitCode), ErrorMessage: failMsg, DurationSecs: outcome.durationSecs})
		_ = t.Store.LogEvent(ctx, t.ChainID, step.stepID, chain.EventStepFailed, map[string]any{"error": failMsg, "exit_code": outcome.exitCode})
		return "", receipt.Receipt{}, fmt.Errorf("spawn_agent: %s", failMsg)
	}
	if facts != nil {
		facts.ReceiptPresent = true
	}
	parsed, err := receipt.Parse([]byte(receiptContent))
	if err != nil {
		failMsg := fmt.Sprintf("parse receipt %s: %v", step.receiptPath, err)
		if facts != nil {
			facts.ReceiptError = err.Error()
		}
		_ = t.Store.FailStep(ctx, chain.CompleteStepParams{StepID: step.stepID, ExitCode: intPtr(outcome.exitCode), ErrorMessage: failMsg, DurationSecs: outcome.durationSecs})
		_ = t.Store.LogEvent(ctx, t.ChainID, step.stepID, chain.EventStepFailed, map[string]any{"error": failMsg, "exit_code": outcome.exitCode})
		return "", receipt.Receipt{}, fmt.Errorf("spawn_agent: %w", err)
	}
	if facts != nil {
		facts.ReceiptSchemaValid = true
		facts.ParsedVerdict = string(parsed.Verdict)
		facts.TokensUsed = parsed.TokensUsed
		facts.TurnsUsed = parsed.TurnsUsed
		facts.ReceiptDurationSeconds = parsed.DurationSeconds
		facts.ClaimedValidationCommands = receipt.ParseValidationCommands(parsed.RawBody)
		facts.ChangedFileClaimPresent = receipt.HasSection(parsed.RawBody, "Changed Files")
		facts.ClaimedChangedFiles = receipt.ParseChangedFiles(parsed.RawBody)
		if step.sourceMutating {
			facts.ChangedFileClaimExtra, facts.ChangedFileManifestUnclaimed = diffStringSets(facts.ClaimedChangedFiles, facts.ChangedFiles)
			facts.ChangedFileClaimMatchesManifest = len(facts.ChangedFileClaimExtra) == 0 && len(facts.ChangedFileManifestUnclaimed) == 0
		}
	}
	if err := receipt.ValidateForStep(parsed, receipt.StepValidation{Agent: step.roleName, ChainID: t.ChainID, Step: step.sequence}); err != nil {
		failMsg := fmt.Sprintf("validate receipt %s: %v", step.receiptPath, err)
		if facts != nil {
			facts.ReceiptError = err.Error()
		}
		_ = t.Store.FailStep(ctx, chain.CompleteStepParams{StepID: step.stepID, Verdict: string(parsed.Verdict), ReceiptPath: step.receiptPath, TokensUsed: parsed.TokensUsed, TurnsUsed: parsed.TurnsUsed, ExitCode: intPtr(outcome.exitCode), ErrorMessage: failMsg, DurationSecs: outcome.durationSecs})
		_ = t.Store.LogEvent(ctx, t.ChainID, step.stepID, chain.EventStepFailed, map[string]any{"error": failMsg, "exit_code": outcome.exitCode})
		return "", receipt.Receipt{}, fmt.Errorf("spawn_agent: %w", err)
	}
	if facts != nil {
		facts.ReceiptStepValid = true
	}
	if err := receipt.ValidateRequiredSections(parsed.RawBody, receipt.RequiredSectionsForRole(step.roleName)); err != nil {
		failMsg := fmt.Sprintf("validate receipt sections %s: %v", step.receiptPath, err)
		if facts != nil {
			facts.ReceiptError = err.Error()
		}
		_ = t.Store.FailStep(ctx, chain.CompleteStepParams{StepID: step.stepID, Verdict: string(parsed.Verdict), ReceiptPath: step.receiptPath, TokensUsed: parsed.TokensUsed, TurnsUsed: parsed.TurnsUsed, ExitCode: intPtr(outcome.exitCode), ErrorMessage: failMsg, DurationSecs: outcome.durationSecs})
		_ = t.Store.LogEvent(ctx, t.ChainID, step.stepID, chain.EventReceiptValidation, map[string]any{"role": step.roleName, "receipt_path": step.receiptPath, "error": err.Error()})
		_ = t.Store.LogEvent(ctx, t.ChainID, step.stepID, chain.EventStepFailed, map[string]any{"error": failMsg, "exit_code": outcome.exitCode})
		return "", receipt.Receipt{}, fmt.Errorf("spawn_agent: %w", err)
	}
	if facts != nil {
		facts.ReceiptSectionsValid = true
		facts.ReceiptValid = true
	}
	t.logReceiptFindingFacts(ctx, step, parsed, facts)
	return receiptContent, parsed, nil
}

func (t *SpawnAgentTool) logReceiptFindingFacts(ctx context.Context, step spawnStep, parsed receipt.Receipt, facts *postStepGuardrailFacts) {
	if t == nil || t.Store == nil {
		return
	}
	findingFacts, ok := buildReceiptFindingFacts(step.roleName, parsed)
	if !ok {
		return
	}
	if facts != nil {
		facts.FindingCount = findingFacts.FindingCount
		facts.OpenFindingCount = findingFacts.OpenFindingCount
		facts.ClosedFindingCount = findingFacts.ClosedFindingCount
		facts.AddressedFindingCount = findingFacts.AddressedFindingCount
		facts.FindingIDs = append([]string(nil), findingFacts.FindingIDs...)
		facts.OpenFindingIDs = append([]string(nil), findingFacts.OpenFindingIDs...)
		facts.ClosedFindingIDs = append([]string(nil), findingFacts.ClosedFindingIDs...)
		facts.AddressedIDs = append([]string(nil), findingFacts.AddressedIDs...)
		facts.SuspiciousVerdictFindingCombination = findingFacts.SuspiciousVerdictFindingCombination
		facts.SuspiciousVerdictFindingReason = findingFacts.SuspiciousVerdictFindingReason
	}
	payload := map[string]any{
		"role":         step.roleName,
		"receipt_path": step.receiptPath,
		"verdict":      string(parsed.Verdict),
	}
	switch step.roleName {
	case "correctness-auditor", "quality-auditor", "performance-auditor", "security-auditor", "integration-auditor":
		payload["finding_count"] = findingFacts.FindingCount
		payload["open_count"] = findingFacts.OpenFindingCount
		payload["closed_count"] = findingFacts.ClosedFindingCount
		payload["finding_ids"] = findingFacts.FindingIDs
		payload["open_finding_ids"] = findingFacts.OpenFindingIDs
		payload["closed_finding_ids"] = findingFacts.ClosedFindingIDs
	case "resolver":
		payload["addressed_count"] = findingFacts.AddressedFindingCount
		payload["addressed_ids"] = findingFacts.AddressedIDs
	default:
		return
	}
	_ = t.Store.LogEvent(ctx, t.ChainID, step.stepID, chain.EventReceiptFindings, payload)
	_ = t.Store.LogEvent(ctx, t.ChainID, step.stepID, chain.EventFindingLifecycleFacts, chain.FindingLifecycleFactsPayload{
		Role:        step.roleName,
		ReceiptPath: step.receiptPath,
		Verdict:     string(parsed.Verdict),
		Facts:       findingFacts.LifecycleFacts,
	})
}

func buildReceiptFindingFacts(roleName string, parsed receipt.Receipt) (receiptFindingFactSet, bool) {
	var facts receiptFindingFactSet
	switch roleName {
	case "correctness-auditor", "quality-auditor", "performance-auditor", "security-auditor", "integration-auditor":
		findings := receipt.ParseAuditFindings(parsed.RawBody, roleName)
		for _, finding := range findings {
			if strings.TrimSpace(finding.ID) == "" {
				continue
			}
			lifecycleFact := chain.FindingLifecycleFact{
				ID:          finding.ID,
				SourceRole:  roleName,
				Action:      "opened",
				Status:      "open",
				Severity:    finding.Severity,
				Evidence:    finding.Evidence,
				Summary:     finding.Summary,
				RequiredFix: finding.RequiredFix,
			}
			facts.FindingIDs = append(facts.FindingIDs, finding.ID)
			if finding.Status == "closed" {
				facts.ClosedFindingIDs = append(facts.ClosedFindingIDs, finding.ID)
				lifecycleFact.Action = "closed"
				lifecycleFact.Status = "closed"
			} else if finding.Status == "reopened" {
				facts.OpenFindingIDs = append(facts.OpenFindingIDs, finding.ID)
				lifecycleFact.Action = "reopened"
				lifecycleFact.Status = "open"
			} else {
				facts.OpenFindingIDs = append(facts.OpenFindingIDs, finding.ID)
			}
			facts.LifecycleFacts = append(facts.LifecycleFacts, lifecycleFact)
		}
		facts.FindingIDs = uniqueSorted(facts.FindingIDs)
		facts.OpenFindingIDs = uniqueSorted(facts.OpenFindingIDs)
		facts.ClosedFindingIDs = uniqueSorted(facts.ClosedFindingIDs)
		facts.FindingCount = len(facts.FindingIDs)
		facts.OpenFindingCount = len(facts.OpenFindingIDs)
		facts.ClosedFindingCount = len(facts.ClosedFindingIDs)
		if facts.OpenFindingCount > 0 && parsed.Verdict != receipt.VerdictFixRequired {
			facts.SuspiciousVerdictFindingCombination = true
			facts.SuspiciousVerdictFindingReason = fmt.Sprintf("%s reported %d open finding(s) with verdict %s", roleName, facts.OpenFindingCount, parsed.Verdict)
		}
		return facts, true
	case "resolver":
		resolutions := receipt.ParseFindingResolutions(parsed.RawBody)
		for _, resolution := range resolutions {
			if strings.TrimSpace(resolution.ID) != "" {
				facts.AddressedIDs = append(facts.AddressedIDs, resolution.ID)
				facts.LifecycleFacts = append(facts.LifecycleFacts, chain.FindingLifecycleFact{
					ID:           resolution.ID,
					Action:       "addressed",
					Status:       "addressed",
					Resolution:   resolution.Resolution,
					FilesChanged: append([]string(nil), resolution.FilesChanged...),
					Validation:   append([]string(nil), resolution.Validation...),
				})
			}
		}
		facts.AddressedIDs = uniqueSorted(facts.AddressedIDs)
		facts.AddressedFindingCount = len(facts.AddressedIDs)
		if facts.AddressedFindingCount == 0 {
			facts.SuspiciousVerdictFindingCombination = true
			facts.SuspiciousVerdictFindingReason = "resolver receipt did not address any finding IDs"
		}
		return facts, true
	default:
		return receiptFindingFactSet{}, false
	}
}

func newPostStepGuardrailFacts(step spawnStep) *postStepGuardrailFacts {
	return &postStepGuardrailFacts{
		Role:                         step.roleName,
		Sequence:                     step.sequence,
		ReceiptPath:                  step.receiptPath,
		SourceMutating:               step.sourceMutating,
		ClaimedValidationCommands:    []string{},
		ClaimedChangedFiles:          []string{},
		ChangedFileClaimExtra:        []string{},
		ChangedFileManifestUnclaimed: []string{},
		ChangedFiles:                 []string{},
		FindingIDs:                   []string{},
		OpenFindingIDs:               []string{},
		ClosedFindingIDs:             []string{},
		AddressedIDs:                 []string{},
	}
}

func (t *SpawnAgentTool) logPostStepGuardrailFacts(ctx context.Context, step spawnStep, facts *postStepGuardrailFacts) {
	if t == nil || t.Store == nil || facts == nil {
		return
	}
	facts.Role = step.roleName
	facts.Sequence = step.sequence
	facts.ReceiptPath = step.receiptPath
	facts.SourceMutating = step.sourceMutating
	facts.ChangedFiles = uniqueSorted(facts.ChangedFiles)
	facts.ChangedFileCount = len(facts.ChangedFiles)
	facts.ClaimedChangedFiles = uniqueSorted(facts.ClaimedChangedFiles)
	facts.ChangedFileClaimExtra = uniqueSorted(facts.ChangedFileClaimExtra)
	facts.ChangedFileManifestUnclaimed = uniqueSorted(facts.ChangedFileManifestUnclaimed)
	facts.FindingIDs = uniqueSorted(facts.FindingIDs)
	facts.OpenFindingIDs = uniqueSorted(facts.OpenFindingIDs)
	facts.ClosedFindingIDs = uniqueSorted(facts.ClosedFindingIDs)
	facts.AddressedIDs = uniqueSorted(facts.AddressedIDs)
	facts.ClaimedValidationCommands = uniqueStringsPreserveOrder(facts.ClaimedValidationCommands)
	_ = t.Store.LogEvent(ctx, t.ChainID, step.stepID, chain.EventStepGuardrailFacts, facts)
}

func uniqueSorted(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func diffStringSets(left []string, right []string) ([]string, []string) {
	leftSet := map[string]struct{}{}
	rightSet := map[string]struct{}{}
	for _, value := range left {
		value = strings.TrimSpace(value)
		if value != "" {
			leftSet[value] = struct{}{}
		}
	}
	for _, value := range right {
		value = strings.TrimSpace(value)
		if value != "" {
			rightSet[value] = struct{}{}
		}
	}
	leftOnly := make([]string, 0)
	for value := range leftSet {
		if _, ok := rightSet[value]; !ok {
			leftOnly = append(leftOnly, value)
		}
	}
	rightOnly := make([]string, 0)
	for value := range rightSet {
		if _, ok := leftSet[value]; !ok {
			rightOnly = append(rightOnly, value)
		}
	}
	sort.Strings(leftOnly)
	sort.Strings(rightOnly)
	return leftOnly, rightOnly
}

func uniqueStringsPreserveOrder(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
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

func detachedLockContext(ctx context.Context) context.Context {
	if ctx != nil {
		return context.WithoutCancel(ctx)
	}
	return context.Background()
}

func mustMarshalString(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
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

## Summary
The harness wrote this safety receipt because the step did not produce a valid receipt.

## Changes
No source changes were recorded by this synthetic receipt.

## Validation
The harness could not validate the step output.

## Concerns
%s

## Next Steps
Inspect the step failure and decide whether to retry, resolve manually, or stop the chain.
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

func taskWithHarnessContext(task string, chainID string, step int, receiptPath string, briefing string) string {
	briefing = strings.TrimSpace(briefing)
	briefingBlock := ""
	if briefing != "" {
		briefingBlock = "\n\nCurrent chain briefing:\n" + briefing
	}
	return fmt.Sprintf(`%s

Harness context:
- Chain ID: %s
- Step number: %d
- Receipt path: %s
%s

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

The receipt body must include these markdown sections:
- ## Summary
- ## Changes
- ## Changed Files (source-writing roles only; list every created, modified, or deleted path, or "None.")
- ## Validation
- ## Concerns
- ## Next Steps

Auditor receipts must also include ## Findings. Resolver receipts must also include ## Findings Addressed.

The receipt body must include the concrete outcome of the task. If the task asks a question, put the answer in the Summary section rather than only saying that you found it.`, strings.TrimSpace(task), chainID, step, receiptPath, briefingBlock, chainID, step)
}
