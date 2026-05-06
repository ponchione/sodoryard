package chainrun

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ponchione/sodoryard/internal/agent"
	"github.com/ponchione/sodoryard/internal/chain"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/conversation"
	appdb "github.com/ponchione/sodoryard/internal/db"
	"github.com/ponchione/sodoryard/internal/receipt"
	rtpkg "github.com/ponchione/sodoryard/internal/runtime"
	spawnpkg "github.com/ponchione/sodoryard/internal/spawn"
	"github.com/ponchione/sodoryard/internal/tool"
)

func TestExitCodeMapsSpecStatuses(t *testing.T) {
	tests := []struct {
		status string
		events []chain.Event
		want   int
	}{
		{status: "completed", want: 0},
		{status: "partial", want: 2},
		{status: "cancelled", want: 4},
		{status: "failed", want: 1},
		{status: "failed", events: []chain.Event{{EventType: chain.EventSafetyLimitHit}}, want: 3},
	}

	for _, tc := range tests {
		if got := exitCode(tc.status, tc.events); got != tc.want {
			t.Fatalf("exitCode(%q) = %d, want %d", tc.status, got, tc.want)
		}
	}
}

func TestOneStepTerminalStatusHonorsHeadlessExitCodes(t *testing.T) {
	tests := []struct {
		name   string
		result spawnpkg.AgentStepResult
		want   string
	}{
		{
			name:   "completed receipt with ok process completes",
			result: spawnpkg.AgentStepResult{Status: "completed", Verdict: receipt.VerdictCompleted, ExitCode: 0},
			want:   "completed",
		},
		{
			name:   "completed receipt with safety-limit process fails",
			result: spawnpkg.AgentStepResult{Status: "completed", Verdict: receipt.VerdictCompleted, ExitCode: headlessExitSafetyLimit},
			want:   "failed",
		},
		{
			name:   "completed receipt with escalation process is partial",
			result: spawnpkg.AgentStepResult{Status: "completed", Verdict: receipt.VerdictCompleted, ExitCode: headlessExitEscalation},
			want:   "partial",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := oneStepTerminalStatus(tc.result); got != tc.want {
				t.Fatalf("oneStepTerminalStatus() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDefaultStepRunnerPassesMemoryEndpointEnv(t *testing.T) {
	expectedEnv := []string{"SODORYARD_MEMORY_ENDPOINT=unix:/tmp/memory.sock"}
	cfg := appconfig.Default()
	cfg.ProjectRoot = t.TempDir()
	deps := withDefaultDeps(Deps{})

	runner := deps.NewStepRunner(&rtpkg.OrchestratorRuntime{
		Config:            cfg,
		MemoryEndpointEnv: expectedEnv,
	}, "chain-env")
	spawnTool, ok := runner.(*spawnpkg.SpawnAgentTool)
	if !ok {
		t.Fatalf("NewStepRunner returned %T, want *spawn.SpawnAgentTool", runner)
	}
	if strings.Join(spawnTool.SubprocessEnv, "\n") != strings.Join(expectedEnv, "\n") {
		t.Fatalf("SubprocessEnv = %v, want %v", spawnTool.SubprocessEnv, expectedEnv)
	}
}

func TestBuildTaskIncludesReceiptHistory(t *testing.T) {
	msg := buildTask(Options{
		SourceTask:       "fix auth",
		MaxSteps:         10,
		MaxResolverLoops: 3,
		MaxDuration:      time.Hour,
		TokenBudget:      100,
	}, "chain-1", []string{"receipts/coder/chain-1-step-001.md"})

	if !containsAll(msg, "fix auth", "chain-1", "receipts/coder/chain-1-step-001.md") {
		t.Fatalf("message = %q, want task, chain id, and receipt history", msg)
	}
}

func TestChainSpecFromOptionsUsesMaxResolverLoops(t *testing.T) {
	spec := chainSpecFromOptions("chain-1", Options{
		SourceSpecs:      []string{"specs/a.md"},
		MaxSteps:         7,
		MaxResolverLoops: 9,
		MaxDuration:      time.Hour,
		TokenBudget:      123,
	})
	if spec.MaxResolverLoops != 9 {
		t.Fatalf("MaxResolverLoops = %d, want 9", spec.MaxResolverLoops)
	}
}

func TestBuildTaskIncludesNoHistoryFallback(t *testing.T) {
	msg := buildTask(Options{SourceSpecs: []string{"specs/a.md", "specs/b.md"}}, "chain-1", nil)
	if !containsAll(msg, "specs/a.md", "specs/b.md", "chain-1", "No existing receipt paths were found for this chain yet.") {
		t.Fatalf("message = %q, want specs, chain id, and no-history fallback", msg)
	}
	if strings.Contains(msg, "receipts/state") {
		t.Fatalf("message = %q, unexpected generic receipts/state wording", msg)
	}
}

func TestBuildTaskIncludesConstrainedRoleInstruction(t *testing.T) {
	msg := buildTask(Options{Mode: ModeConstrained, SourceTask: "fix auth", AllowedRoles: []string{"planner", "coder"}}, "chain-1", nil)
	if !containsAll(msg, "fix auth", "chain-1", "Constrained orchestration is enabled", "planner, coder", "Do not spawn unlisted roles") {
		t.Fatalf("message = %q, want constrained role instruction", msg)
	}
}

func TestBuildOneStepTaskIncludesSpecsWhenProvided(t *testing.T) {
	msg := buildOneStepTask(Options{SourceTask: "fix auth", SourceSpecs: []string{"specs/auth.md"}})
	if !containsAll(msg, "fix auth", "Source specs: specs/auth.md") {
		t.Fatalf("message = %q, want task and source specs", msg)
	}
}

func TestBuildTaskIncludesOnlyExistingReceiptPaths(t *testing.T) {
	msg := buildTask(Options{SourceTask: "fix auth"}, "chain-1", []string{"receipts/planner/chain-1-step-001.md", "receipts/coder/chain-1-step-002.md"})
	if !strings.Contains(msg, "Relevant existing receipt paths to read first: receipts/planner/chain-1-step-001.md, receipts/coder/chain-1-step-002.md") {
		t.Fatalf("message = %q, want existing receipt paths", msg)
	}
	if strings.Contains(msg, "No existing receipt paths were found") {
		t.Fatalf("message = %q, unexpected no-history fallback", msg)
	}
}

func TestPopulateOptionsFromExistingUsesStoredResumeInputs(t *testing.T) {
	t.Run("hydrates missing task and specs from stored chain", func(t *testing.T) {
		opts, err := populateOptionsFromExisting(Options{ChainID: "chain-1"}, &chain.Chain{
			ID:          "chain-1",
			SourceSpecs: []string{"specs/a.md", "specs/b.md"},
			SourceTask:  "stored task",
		})
		if err != nil {
			t.Fatalf("populateOptionsFromExisting returned error: %v", err)
		}
		if got := strings.Join(opts.SourceSpecs, ","); got != "specs/a.md,specs/b.md" {
			t.Fatalf("SourceSpecs = %q, want stored specs", got)
		}
		if opts.SourceTask != "stored task" {
			t.Fatalf("SourceTask = %q, want stored task", opts.SourceTask)
		}
	})

	t.Run("keeps explicit user inputs over stored values", func(t *testing.T) {
		opts, err := populateOptionsFromExisting(Options{
			ChainID:     "chain-1",
			SourceSpecs: []string{"specs/override.md"},
			SourceTask:  "explicit task",
		}, &chain.Chain{
			ID:          "chain-1",
			SourceSpecs: []string{"specs/a.md", "specs/b.md"},
			SourceTask:  "stored task",
		})
		if err != nil {
			t.Fatalf("populateOptionsFromExisting returned error: %v", err)
		}
		if got := strings.Join(opts.SourceSpecs, ","); got != "specs/override.md" {
			t.Fatalf("SourceSpecs = %q, want explicit specs", got)
		}
		if opts.SourceTask != "explicit task" {
			t.Fatalf("SourceTask = %q, want explicit task", opts.SourceTask)
		}
	})
}

func TestPrepareExistingChainForExecutionRejectsPauseRequestedResume(t *testing.T) {
	err := prepareExistingChainForExecution(context.Background(), nil, &chain.Chain{ID: "chain-1", Status: "pause_requested"})
	if err == nil || !strings.Contains(err.Error(), "cannot be resumed until paused") {
		t.Fatalf("error = %v, want pause_requested resume rejection", err)
	}
}

func TestPrepareExistingChainForExecutionStopsDuplicateRunningResume(t *testing.T) {
	err := prepareExistingChainForExecution(context.Background(), nil, &chain.Chain{ID: "chain-1", Status: "running"})
	if err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("error = %v, want already running rejection", err)
	}
}

func TestHandleInterruptionClosesStaleActiveExecutionForPausedChain(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newChainrunTestDB(t))
	chainID, err := store.StartChain(ctx, chain.ChainSpec{MaxSteps: 5, MaxResolverLoops: 1, MaxDuration: time.Hour, TokenBudget: 100})
	if err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, "", chain.EventChainStarted, map[string]any{"orchestrator_pid": 1111, "execution_id": "exec-1", "active_execution": true}); err != nil {
		t.Fatalf("LogEvent returned error: %v", err)
	}
	if err := store.SetChainStatus(ctx, chainID, "paused"); err != nil {
		t.Fatalf("SetChainStatus returned error: %v", err)
	}
	var out strings.Builder

	handled, err := handleInterruption(ctx, store, chainID, agent.ErrTurnCancelled, func(message string) {
		out.WriteString(message)
	})
	if err != nil {
		t.Fatalf("handleInterruption returned error: %v", err)
	}
	if !handled {
		t.Fatal("handleInterruption handled = false, want true")
	}
	if !strings.Contains(out.String(), "chain "+chainID+" paused") {
		t.Fatalf("output = %q, want paused message", out.String())
	}
	events, err := store.ListEvents(ctx, chainID)
	if err != nil {
		t.Fatalf("ListEvents returned error: %v", err)
	}
	if len(events) != 2 || events[1].EventType != chain.EventChainPaused {
		t.Fatalf("events = %+v, want start + one %s event", events, chain.EventChainPaused)
	}
	if !strings.Contains(events[1].EventData, `"execution_id":"exec-1"`) {
		t.Fatalf("EventData = %s, want execution_id exec-1", events[1].EventData)
	}
	if exec, ok := chain.LatestActiveExecution(events); ok || exec.ExecutionID != "" || exec.OrchestratorPID != 0 {
		t.Fatalf("LatestActiveExecution() = (%+v, %t), want empty,false", exec, ok)
	}
}

func TestFinalizeRequestedChainStatusLogsTerminalCancelEvent(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newChainrunTestDB(t))
	chainID, err := store.StartChain(ctx, chain.ChainSpec{MaxSteps: 5, MaxResolverLoops: 1, MaxDuration: time.Hour, TokenBudget: 100})
	if err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, "", chain.EventChainStarted, map[string]any{"orchestrator_pid": 1111, "execution_id": "exec-1", "active_execution": true}); err != nil {
		t.Fatalf("LogEvent returned error: %v", err)
	}
	if err := store.SetChainStatus(ctx, chainID, "cancel_requested"); err != nil {
		t.Fatalf("SetChainStatus returned error: %v", err)
	}

	if err := finalizeRequestedChainStatus(ctx, store, chainID); err != nil {
		t.Fatalf("finalizeRequestedChainStatus returned error: %v", err)
	}

	ch, err := store.GetChain(ctx, chainID)
	if err != nil {
		t.Fatalf("GetChain returned error: %v", err)
	}
	if ch.Status != "cancelled" {
		t.Fatalf("status = %q, want cancelled", ch.Status)
	}
	events, err := store.ListEvents(ctx, chainID)
	if err != nil {
		t.Fatalf("ListEvents returned error: %v", err)
	}
	if len(events) != 2 || events[1].EventType != chain.EventChainCancelled {
		t.Fatalf("events = %+v, want start + one %s event", events, chain.EventChainCancelled)
	}
	if !strings.Contains(events[1].EventData, `"execution_id":"exec-1"`) {
		t.Fatalf("EventData = %s, want execution_id exec-1", events[1].EventData)
	}
}

func TestCloseErroredExecutionMarksFailedAndClearsActiveExecution(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newChainrunTestDB(t))
	chainID, err := store.StartChain(ctx, chain.ChainSpec{MaxSteps: 5, MaxResolverLoops: 1, MaxDuration: time.Hour, TokenBudget: 100})
	if err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, "", chain.EventChainStarted, map[string]any{"orchestrator_pid": 1111, "execution_id": "exec-1", "active_execution": true}); err != nil {
		t.Fatalf("LogEvent returned error: %v", err)
	}

	if err := closeErroredExecution(ctx, store, chainID, "boom"); err != nil {
		t.Fatalf("closeErroredExecution returned error: %v", err)
	}

	ch, err := store.GetChain(ctx, chainID)
	if err != nil {
		t.Fatalf("GetChain returned error: %v", err)
	}
	if ch.Status != "failed" {
		t.Fatalf("status = %q, want failed", ch.Status)
	}
	events, err := store.ListEvents(ctx, chainID)
	if err != nil {
		t.Fatalf("ListEvents returned error: %v", err)
	}
	if len(events) != 2 || events[1].EventType != chain.EventChainCompleted {
		t.Fatalf("events = %+v, want start + one %s event", events, chain.EventChainCompleted)
	}
	if !strings.Contains(events[1].EventData, `"execution_id":"exec-1"`) {
		t.Fatalf("EventData = %s, want execution_id exec-1", events[1].EventData)
	}
	if !strings.Contains(events[1].EventData, `"status":"failed"`) {
		t.Fatalf("EventData = %s, want failed status", events[1].EventData)
	}
	if exec, ok := chain.LatestActiveExecution(events); ok || exec.ExecutionID != "" || exec.OrchestratorPID != 0 {
		t.Fatalf("LatestActiveExecution() = (%+v, %t), want empty,false", exec, ok)
	}
}

func TestStartOneStepRunsSelectedRoleAndCompletesChain(t *testing.T) {
	ctx := context.Background()
	cfg := appconfig.Default()
	cfg.ProjectRoot = t.TempDir()
	cfg.AgentRoles = map[string]appconfig.AgentRoleConfig{
		"coder": {SystemPrompt: "builtin:coder"},
	}

	db := newChainrunTestDB(t)
	store := chain.NewStore(db)
	var gotInput spawnpkg.AgentStepInput
	deps := Deps{
		BuildRuntime: func(ctx context.Context, cfg *appconfig.Config) (*rtpkg.OrchestratorRuntime, error) {
			return &rtpkg.OrchestratorRuntime{Config: cfg, ChainStore: store, Cleanup: func() {}}, nil
		},
		NewStepRunner: func(rt *rtpkg.OrchestratorRuntime, chainID string) StepRunner {
			return fakeStepRunner{run: func(ctx context.Context, in spawnpkg.AgentStepInput) (spawnpkg.AgentStepResult, string, error) {
				gotInput = in
				stepID, err := store.StartStep(ctx, chain.StepSpec{ChainID: chainID, SequenceNum: 1, Role: "coder", Task: in.Task})
				if err != nil {
					t.Fatalf("StartStep returned error: %v", err)
				}
				if err := store.CompleteStep(ctx, chain.CompleteStepParams{StepID: stepID, Status: "completed", Verdict: "completed", ReceiptPath: "receipts/coder/one-step-chain-step-001.md", TokensUsed: 10, TurnsUsed: 2, DurationSecs: 3}); err != nil {
					t.Fatalf("CompleteStep returned error: %v", err)
				}
				if err := store.UpdateChainMetrics(ctx, chainID, chain.ChainMetrics{TotalSteps: 1, TotalTokens: 10, TotalDurationSecs: 3}); err != nil {
					t.Fatalf("UpdateChainMetrics returned error: %v", err)
				}
				return spawnpkg.AgentStepResult{StepID: stepID, Sequence: 1, ReceiptPath: "receipts/coder/one-step-chain-step-001.md", Verdict: "completed", Status: "completed", TokensUsed: 10, TurnsUsed: 2, DurationSecs: 3, ExitCode: 0}, "receipt", nil
			}}
		},
		NewChainID: func() string { return "one-step-chain" },
		ProcessID:  func() int { return 1234 },
	}

	result, err := Start(ctx, cfg, Options{Mode: ModeOneStep, Role: "coder", SourceTask: "implement one thing", MaxSteps: 10, MaxResolverLoops: 1, MaxDuration: time.Hour, TokenBudget: 100}, deps)
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("result status = %q, want completed", result.Status)
	}
	if gotInput.Role != "coder" || gotInput.Task != "implement one thing" {
		t.Fatalf("step input = %+v, want coder task", gotInput)
	}
	steps, err := store.ListSteps(ctx, "one-step-chain")
	if err != nil {
		t.Fatalf("ListSteps returned error: %v", err)
	}
	if len(steps) != 1 || steps[0].Role != "coder" {
		t.Fatalf("steps = %+v, want one coder step", steps)
	}
	stored, err := store.GetChain(ctx, "one-step-chain")
	if err != nil {
		t.Fatalf("GetChain returned error: %v", err)
	}
	if stored.Status != "completed" || stored.TotalSteps != 1 || stored.TotalTokens != 10 {
		t.Fatalf("stored chain = %+v, want completed with one-step metrics", stored)
	}
	events, err := store.ListEvents(ctx, "one-step-chain")
	if err != nil {
		t.Fatalf("ListEvents returned error: %v", err)
	}
	if exec, ok := chain.LatestActiveExecution(events); ok || exec.ExecutionID != "" || exec.OrchestratorPID != 0 {
		t.Fatalf("LatestActiveExecution() = (%+v, %t), want empty,false after terminal closure", exec, ok)
	}
}

func TestStartOneStepMapsFixRequiredReceiptToPartialExit(t *testing.T) {
	ctx := context.Background()
	cfg := appconfig.Default()
	cfg.ProjectRoot = t.TempDir()
	cfg.AgentRoles = map[string]appconfig.AgentRoleConfig{
		"coder": {SystemPrompt: "builtin:coder"},
	}

	db := newChainrunTestDB(t)
	store := chain.NewStore(db)
	deps := Deps{
		BuildRuntime: func(ctx context.Context, cfg *appconfig.Config) (*rtpkg.OrchestratorRuntime, error) {
			return &rtpkg.OrchestratorRuntime{Config: cfg, ChainStore: store, Cleanup: func() {}}, nil
		},
		NewStepRunner: func(rt *rtpkg.OrchestratorRuntime, chainID string) StepRunner {
			return fakeStepRunner{run: func(ctx context.Context, in spawnpkg.AgentStepInput) (spawnpkg.AgentStepResult, string, error) {
				stepID, err := store.StartStep(ctx, chain.StepSpec{ChainID: chainID, SequenceNum: 1, Role: "coder", Task: in.Task})
				if err != nil {
					t.Fatalf("StartStep returned error: %v", err)
				}
				if err := store.CompleteStep(ctx, chain.CompleteStepParams{StepID: stepID, Status: "completed", Verdict: "fix_required", ReceiptPath: "receipts/coder/partial-step-001.md"}); err != nil {
					t.Fatalf("CompleteStep returned error: %v", err)
				}
				return spawnpkg.AgentStepResult{StepID: stepID, Sequence: 1, ReceiptPath: "receipts/coder/partial-step-001.md", Verdict: "fix_required", Status: "completed", ExitCode: 0}, "receipt", nil
			}}
		},
		NewChainID: func() string { return "partial-one-step" },
		ProcessID:  func() int { return 1234 },
	}

	_, err := Start(ctx, cfg, Options{Mode: ModeOneStep, Role: "coder", SourceTask: "audit", MaxSteps: 10, MaxResolverLoops: 1, MaxDuration: time.Hour, TokenBudget: 100}, deps)
	var exitErr ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("Start error = %T %[1]v, want ExitError", err)
	}
	if exitErr.ExitCode() != 2 {
		t.Fatalf("ExitCode = %d, want 2 for partial one-step chain", exitErr.ExitCode())
	}
	stored, loadErr := store.GetChain(ctx, "partial-one-step")
	if loadErr != nil {
		t.Fatalf("GetChain returned error: %v", loadErr)
	}
	if stored.Status != "partial" {
		t.Fatalf("status = %q, want partial", stored.Status)
	}
}

func TestBuildManualRosterTaskIncludesOriginalWorkPacketAndPreviousReceipts(t *testing.T) {
	msg := buildManualRosterTask(Options{SourceTask: "fix auth", SourceSpecs: []string{"specs/auth.md"}}, "chain-1", 2, "coder", []string{"receipts/planner/chain-1-step-001.md"})
	if !containsAll(msg, "manual roster step 2", "role coder", "chain chain-1", "fix auth", "Source specs: specs/auth.md", "receipts/planner/chain-1-step-001.md") {
		t.Fatalf("message = %q, want roster context, original work packet, and previous receipt", msg)
	}
}

func TestStartManualRosterRunsRolesInOrderWithReceiptHistory(t *testing.T) {
	ctx := context.Background()
	cfg := appconfig.Default()
	cfg.ProjectRoot = t.TempDir()
	cfg.AgentRoles = map[string]appconfig.AgentRoleConfig{
		"planner": {SystemPrompt: "builtin:planner"},
		"coder":   {SystemPrompt: "builtin:coder"},
	}

	db := newChainrunTestDB(t)
	store := chain.NewStore(db)
	var inputs []spawnpkg.AgentStepInput
	deps := Deps{
		BuildRuntime: func(ctx context.Context, cfg *appconfig.Config) (*rtpkg.OrchestratorRuntime, error) {
			return &rtpkg.OrchestratorRuntime{Config: cfg, ChainStore: store, Cleanup: func() {}}, nil
		},
		NewStepRunner: func(rt *rtpkg.OrchestratorRuntime, chainID string) StepRunner {
			return fakeStepRunner{run: func(ctx context.Context, in spawnpkg.AgentStepInput) (spawnpkg.AgentStepResult, string, error) {
				inputs = append(inputs, in)
				sequence := len(inputs)
				stepID, receiptPath := completeFakeRosterStep(t, ctx, store, chainID, sequence, in.Role, in.Task, receipt.VerdictCompleted)
				return spawnpkg.AgentStepResult{StepID: stepID, Sequence: sequence, ReceiptPath: receiptPath, Verdict: receipt.VerdictCompleted, Status: "completed", TokensUsed: 10, DurationSecs: 1, ExitCode: 0}, "receipt", nil
			}}
		},
		NewChainID: func() string { return "manual-roster-chain" },
		ProcessID:  func() int { return 1234 },
	}

	result, err := Start(ctx, cfg, Options{
		Mode:             ModeManualRoster,
		Roster:           []StepRequest{{Role: "planner"}, {Role: "coder"}},
		SourceTask:       "ship manual roster",
		MaxSteps:         10,
		MaxResolverLoops: 1,
		MaxDuration:      time.Hour,
		TokenBudget:      100,
	}, deps)
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("result status = %q, want completed", result.Status)
	}
	if len(inputs) != 2 || inputs[0].Role != "planner" || inputs[1].Role != "coder" {
		t.Fatalf("inputs = %+v, want planner then coder", inputs)
	}
	if !strings.Contains(inputs[0].Task, "No previous receipt paths are available yet.") {
		t.Fatalf("first task = %q, want no previous receipts", inputs[0].Task)
	}
	if !strings.Contains(inputs[1].Task, "receipts/planner/manual-roster-chain-step-001.md") {
		t.Fatalf("second task = %q, want first receipt path", inputs[1].Task)
	}
	steps, err := store.ListSteps(ctx, "manual-roster-chain")
	if err != nil {
		t.Fatalf("ListSteps returned error: %v", err)
	}
	if len(steps) != 2 || steps[0].Role != "planner" || steps[1].Role != "coder" {
		t.Fatalf("steps = %+v, want planner then coder", steps)
	}
	stored, err := store.GetChain(ctx, "manual-roster-chain")
	if err != nil {
		t.Fatalf("GetChain returned error: %v", err)
	}
	if stored.Status != "completed" || stored.TotalSteps != 2 || stored.TotalTokens != 20 {
		t.Fatalf("stored chain = %+v, want completed with roster metrics", stored)
	}
}

func TestStartManualRosterStopsBeforeNextStepWhenPauseRequested(t *testing.T) {
	ctx := context.Background()
	cfg := appconfig.Default()
	cfg.ProjectRoot = t.TempDir()
	cfg.AgentRoles = map[string]appconfig.AgentRoleConfig{
		"planner": {SystemPrompt: "builtin:planner"},
		"coder":   {SystemPrompt: "builtin:coder"},
	}

	db := newChainrunTestDB(t)
	store := chain.NewStore(db)
	runs := 0
	deps := Deps{
		BuildRuntime: func(ctx context.Context, cfg *appconfig.Config) (*rtpkg.OrchestratorRuntime, error) {
			return &rtpkg.OrchestratorRuntime{Config: cfg, ChainStore: store, Cleanup: func() {}}, nil
		},
		NewStepRunner: func(rt *rtpkg.OrchestratorRuntime, chainID string) StepRunner {
			return fakeStepRunner{run: func(ctx context.Context, in spawnpkg.AgentStepInput) (spawnpkg.AgentStepResult, string, error) {
				runs++
				stepID, receiptPath := completeFakeRosterStep(t, ctx, store, chainID, runs, in.Role, in.Task, receipt.VerdictCompleted)
				if err := store.SetChainStatus(ctx, chainID, "pause_requested"); err != nil {
					t.Fatalf("SetChainStatus returned error: %v", err)
				}
				return spawnpkg.AgentStepResult{StepID: stepID, Sequence: runs, ReceiptPath: receiptPath, Verdict: receipt.VerdictCompleted, Status: "completed", TokensUsed: 10, DurationSecs: 1, ExitCode: 0}, "receipt", nil
			}}
		},
		NewChainID: func() string { return "paused-manual-roster" },
		ProcessID:  func() int { return 1234 },
	}

	result, err := Start(ctx, cfg, Options{
		Mode:             ModeManualRoster,
		Roster:           []StepRequest{{Role: "planner"}, {Role: "coder"}},
		SourceTask:       "pause after first",
		MaxSteps:         10,
		MaxResolverLoops: 1,
		MaxDuration:      time.Hour,
		TokenBudget:      100,
	}, deps)
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if result.Status != "paused" {
		t.Fatalf("result status = %q, want paused", result.Status)
	}
	if runs != 1 {
		t.Fatalf("runs = %d, want one scheduled step", runs)
	}
	stored, err := store.GetChain(ctx, "paused-manual-roster")
	if err != nil {
		t.Fatalf("GetChain returned error: %v", err)
	}
	if stored.Status != "paused" {
		t.Fatalf("stored status = %q, want paused", stored.Status)
	}
}

func TestStartManualRosterStopsBeforeNextStepWhenCancelRequested(t *testing.T) {
	ctx := context.Background()
	cfg := appconfig.Default()
	cfg.ProjectRoot = t.TempDir()
	cfg.AgentRoles = map[string]appconfig.AgentRoleConfig{
		"planner": {SystemPrompt: "builtin:planner"},
		"coder":   {SystemPrompt: "builtin:coder"},
	}

	db := newChainrunTestDB(t)
	store := chain.NewStore(db)
	runs := 0
	deps := Deps{
		BuildRuntime: func(ctx context.Context, cfg *appconfig.Config) (*rtpkg.OrchestratorRuntime, error) {
			return &rtpkg.OrchestratorRuntime{Config: cfg, ChainStore: store, Cleanup: func() {}}, nil
		},
		NewStepRunner: func(rt *rtpkg.OrchestratorRuntime, chainID string) StepRunner {
			return fakeStepRunner{run: func(ctx context.Context, in spawnpkg.AgentStepInput) (spawnpkg.AgentStepResult, string, error) {
				runs++
				stepID, receiptPath := completeFakeRosterStep(t, ctx, store, chainID, runs, in.Role, in.Task, receipt.VerdictCompleted)
				if err := store.SetChainStatus(ctx, chainID, "cancel_requested"); err != nil {
					t.Fatalf("SetChainStatus returned error: %v", err)
				}
				return spawnpkg.AgentStepResult{StepID: stepID, Sequence: runs, ReceiptPath: receiptPath, Verdict: receipt.VerdictCompleted, Status: "completed", TokensUsed: 10, DurationSecs: 1, ExitCode: 0}, "receipt", nil
			}}
		},
		NewChainID: func() string { return "cancelled-manual-roster" },
		ProcessID:  func() int { return 1234 },
	}

	_, err := Start(ctx, cfg, Options{
		Mode:             ModeManualRoster,
		Roster:           []StepRequest{{Role: "planner"}, {Role: "coder"}},
		SourceTask:       "cancel after first",
		MaxSteps:         10,
		MaxResolverLoops: 1,
		MaxDuration:      time.Hour,
		TokenBudget:      100,
	}, deps)
	var exitErr ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("Start error = %T %[1]v, want ExitError", err)
	}
	if exitErr.ExitCode() != 4 {
		t.Fatalf("ExitCode = %d, want cancel exit 4", exitErr.ExitCode())
	}
	if runs != 1 {
		t.Fatalf("runs = %d, want one scheduled step", runs)
	}
	stored, loadErr := store.GetChain(ctx, "cancelled-manual-roster")
	if loadErr != nil {
		t.Fatalf("GetChain returned error: %v", loadErr)
	}
	if stored.Status != "cancelled" {
		t.Fatalf("stored status = %q, want cancelled", stored.Status)
	}
}

func TestStartManualRosterStopsAndMarksPartialForFixRequired(t *testing.T) {
	ctx := context.Background()
	cfg := appconfig.Default()
	cfg.ProjectRoot = t.TempDir()
	cfg.AgentRoles = map[string]appconfig.AgentRoleConfig{
		"planner": {SystemPrompt: "builtin:planner"},
		"coder":   {SystemPrompt: "builtin:coder"},
	}

	db := newChainrunTestDB(t)
	store := chain.NewStore(db)
	runs := 0
	deps := Deps{
		BuildRuntime: func(ctx context.Context, cfg *appconfig.Config) (*rtpkg.OrchestratorRuntime, error) {
			return &rtpkg.OrchestratorRuntime{Config: cfg, ChainStore: store, Cleanup: func() {}}, nil
		},
		NewStepRunner: func(rt *rtpkg.OrchestratorRuntime, chainID string) StepRunner {
			return fakeStepRunner{run: func(ctx context.Context, in spawnpkg.AgentStepInput) (spawnpkg.AgentStepResult, string, error) {
				runs++
				stepID, receiptPath := completeFakeRosterStep(t, ctx, store, chainID, runs, in.Role, in.Task, receipt.VerdictFixRequired)
				return spawnpkg.AgentStepResult{StepID: stepID, Sequence: runs, ReceiptPath: receiptPath, Verdict: receipt.VerdictFixRequired, Status: "completed", TokensUsed: 10, DurationSecs: 1, ExitCode: 0}, "receipt", nil
			}}
		},
		NewChainID: func() string { return "partial-manual-roster" },
		ProcessID:  func() int { return 1234 },
	}

	_, err := Start(ctx, cfg, Options{
		Mode:             ModeManualRoster,
		Roster:           []StepRequest{{Role: "planner"}, {Role: "coder"}},
		SourceTask:       "stop after non-success",
		MaxSteps:         10,
		MaxResolverLoops: 1,
		MaxDuration:      time.Hour,
		TokenBudget:      100,
	}, deps)
	var exitErr ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("Start error = %T %[1]v, want ExitError", err)
	}
	if exitErr.ExitCode() != 2 {
		t.Fatalf("ExitCode = %d, want partial exit 2", exitErr.ExitCode())
	}
	if runs != 1 {
		t.Fatalf("runs = %d, want one scheduled step", runs)
	}
	stored, loadErr := store.GetChain(ctx, "partial-manual-roster")
	if loadErr != nil {
		t.Fatalf("GetChain returned error: %v", loadErr)
	}
	if stored.Status != "partial" {
		t.Fatalf("stored status = %q, want partial", stored.Status)
	}
}

func TestStartReturnsCancelExitCodeForHandledInterruption(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := appconfig.Default()
	cfg.ProjectRoot = t.TempDir()
	cfg.Routing.Default.Provider = "test"
	cfg.Routing.Default.Model = "test-model"
	cfg.Providers = map[string]appconfig.ProviderConfig{
		"test": {Type: "openai-compatible", Model: "test-model", ContextLength: 128},
	}
	cfg.AgentRoles = map[string]appconfig.AgentRoleConfig{
		"orchestrator": {SystemPrompt: "builtin:orchestrator", MaxTurns: 3},
	}

	db := newChainrunTestDB(t)
	store := chain.NewStore(db)
	deps := Deps{
		BuildRuntime: func(ctx context.Context, cfg *appconfig.Config) (*rtpkg.OrchestratorRuntime, error) {
			if err := rtpkg.EnsureProjectRecord(ctx, db, cfg); err != nil {
				return nil, err
			}
			return &rtpkg.OrchestratorRuntime{
				Config:              cfg,
				Logger:              slog.Default(),
				Database:            db,
				Queries:             appdb.New(db),
				ConversationManager: conversation.NewManager(db, nil, slog.Default()),
				ContextAssembler:    rtpkg.NoopContextAssembler{},
				ChainStore:          store,
				Cleanup:             func() {},
			}, nil
		},
		BuildRegistry: func(*rtpkg.OrchestratorRuntime, appconfig.AgentRoleConfig, string) (*tool.Registry, error) {
			return tool.NewRegistry(), nil
		},
		NewTurnRunner: func(agent.AgentLoopDeps) TurnRunner {
			return fakeTurnRunner{run: func(runCtx context.Context, req agent.RunTurnRequest) (*agent.TurnResult, error) {
				cancel()
				<-runCtx.Done()
				return nil, agent.ErrTurnCancelled
			}}
		},
		NewChainID: func() string { return "cancel-exit-code" },
		ProcessID:  func() int { return 1234 },
	}

	_, err := Start(ctx, cfg, Options{SourceTask: "cancel me", MaxSteps: 10, MaxResolverLoops: 1, MaxDuration: time.Hour, TokenBudget: 100}, deps)
	var exitErr ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("Start error = %T %[1]v, want ExitError", err)
	}
	if exitErr.ExitCode() != 4 {
		t.Fatalf("ExitCode = %d, want 4", exitErr.ExitCode())
	}
	stored, loadErr := store.GetChain(context.Background(), "cancel-exit-code")
	if loadErr != nil {
		t.Fatalf("GetChain returned error: %v", loadErr)
	}
	if stored.Status != "cancelled" {
		t.Fatalf("status = %q, want cancelled", stored.Status)
	}
}

func TestStartAppliesOrchestratorRoleTimeout(t *testing.T) {
	ctx := context.Background()
	cfg := appconfig.Default()
	cfg.ProjectRoot = t.TempDir()
	cfg.Routing.Default.Provider = "test"
	cfg.Routing.Default.Model = "test-model"
	cfg.Providers = map[string]appconfig.ProviderConfig{
		"test": {Type: "openai-compatible", Model: "test-model", ContextLength: 128},
	}
	cfg.AgentRoles = map[string]appconfig.AgentRoleConfig{
		"orchestrator": {SystemPrompt: "builtin:orchestrator", MaxTurns: 3, Timeout: appconfig.Duration(20 * time.Millisecond)},
	}

	db := newChainrunTestDB(t)
	store := chain.NewStore(db)
	deps := Deps{
		BuildRuntime: func(ctx context.Context, cfg *appconfig.Config) (*rtpkg.OrchestratorRuntime, error) {
			if err := rtpkg.EnsureProjectRecord(ctx, db, cfg); err != nil {
				return nil, err
			}
			return &rtpkg.OrchestratorRuntime{
				Config:              cfg,
				Logger:              slog.Default(),
				Database:            db,
				Queries:             appdb.New(db),
				ConversationManager: conversation.NewManager(db, nil, slog.Default()),
				ContextAssembler:    rtpkg.NoopContextAssembler{},
				ChainStore:          store,
				Cleanup:             func() {},
			}, nil
		},
		BuildRegistry: func(*rtpkg.OrchestratorRuntime, appconfig.AgentRoleConfig, string) (*tool.Registry, error) {
			return tool.NewRegistry(), nil
		},
		NewTurnRunner: func(agent.AgentLoopDeps) TurnRunner {
			return fakeTurnRunner{run: func(runCtx context.Context, req agent.RunTurnRequest) (*agent.TurnResult, error) {
				<-runCtx.Done()
				if !errors.Is(runCtx.Err(), context.DeadlineExceeded) {
					t.Fatalf("RunTurn context error = %v, want deadline exceeded", runCtx.Err())
				}
				return nil, agent.ErrTurnCancelled
			}}
		},
		NewChainID: func() string { return "timeout-exit-code" },
		ProcessID:  func() int { return 1234 },
	}

	_, err := Start(ctx, cfg, Options{SourceTask: "timeout", MaxSteps: 10, MaxResolverLoops: 1, MaxDuration: time.Hour, TokenBudget: 100}, deps)
	var exitErr ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("Start error = %T %[1]v, want ExitError", err)
	}
	if exitErr.ExitCode() != 3 {
		t.Fatalf("ExitCode = %d, want 3 after orchestrator role timeout", exitErr.ExitCode())
	}
	stored, loadErr := store.GetChain(context.Background(), "timeout-exit-code")
	if loadErr != nil {
		t.Fatalf("GetChain returned error: %v", loadErr)
	}
	if stored.Status != "failed" {
		t.Fatalf("status = %q, want failed after timeout", stored.Status)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(s, part) {
			return false
		}
	}
	return true
}

type fakeTurnRunner struct {
	run func(context.Context, agent.RunTurnRequest) (*agent.TurnResult, error)
}

func (f fakeTurnRunner) RunTurn(ctx context.Context, req agent.RunTurnRequest) (*agent.TurnResult, error) {
	return f.run(ctx, req)
}

func (f fakeTurnRunner) Close() {}

type fakeStepRunner struct {
	run func(context.Context, spawnpkg.AgentStepInput) (spawnpkg.AgentStepResult, string, error)
}

func (f fakeStepRunner) RunStep(ctx context.Context, in spawnpkg.AgentStepInput) (spawnpkg.AgentStepResult, string, error) {
	return f.run(ctx, in)
}

func completeFakeRosterStep(t *testing.T, ctx context.Context, store *chain.Store, chainID string, sequence int, role string, task string, verdict receipt.Verdict) (string, string) {
	t.Helper()
	stepID, err := store.StartStep(ctx, chain.StepSpec{ChainID: chainID, SequenceNum: sequence, Role: role, Task: task})
	if err != nil {
		t.Fatalf("StartStep returned error: %v", err)
	}
	receiptPath := receipt.StepPath(role, chainID, sequence)
	if err := store.CompleteStep(ctx, chain.CompleteStepParams{StepID: stepID, Status: "completed", Verdict: string(verdict), ReceiptPath: receiptPath, TokensUsed: 10, DurationSecs: 1}); err != nil {
		t.Fatalf("CompleteStep returned error: %v", err)
	}
	ch, err := store.GetChain(ctx, chainID)
	if err != nil {
		t.Fatalf("GetChain returned error: %v", err)
	}
	if err := store.UpdateChainMetrics(ctx, chainID, chain.ChainMetrics{TotalSteps: ch.TotalSteps + 1, TotalTokens: ch.TotalTokens + 10, TotalDurationSecs: ch.TotalDurationSecs + 1}); err != nil {
		t.Fatalf("UpdateChainMetrics returned error: %v", err)
	}
	return stepID, receiptPath
}

func newChainrunTestDB(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()
	database, err := appdb.OpenDB(ctx, filepath.Join(t.TempDir(), "chainrun.db"))
	if err != nil {
		t.Fatalf("OpenDB returned error: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := appdb.InitIfNeeded(ctx, database); err != nil {
		t.Fatalf("InitIfNeeded returned error: %v", err)
	}
	if err := appdb.EnsureChainSchema(ctx, database); err != nil {
		t.Fatalf("EnsureChainSchema returned error: %v", err)
	}
	return database
}
