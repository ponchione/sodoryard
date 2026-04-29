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
	rtpkg "github.com/ponchione/sodoryard/internal/runtime"
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
