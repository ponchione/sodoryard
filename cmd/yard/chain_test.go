package main

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/ponchione/sodoryard/internal/agent"
	"github.com/ponchione/sodoryard/internal/chain"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/conversation"
	rtpkg "github.com/ponchione/sodoryard/internal/runtime"
	"github.com/ponchione/sodoryard/internal/tool"
)

type blockingYardChainTurnRunner struct {
	started chan<- struct{}
	run     func(ctx context.Context, req agent.RunTurnRequest) (*agent.TurnResult, error)
}

func (b *blockingYardChainTurnRunner) RunTurn(ctx context.Context, req agent.RunTurnRequest) (*agent.TurnResult, error) {
	if b.started != nil {
		close(b.started)
	}
	if b.run != nil {
		return b.run(ctx, req)
	}
	<-ctx.Done()
	return nil, agent.ErrTurnCancelled
}

func (*blockingYardChainTurnRunner) Close() {}

func TestYardChainStartExposesMaxResolverLoopsFlag(t *testing.T) {
	configPath := "yard.yaml"
	cmd := newYardChainStartCmd(&configPath)
	flag := cmd.Flags().Lookup("max-resolver-loops")
	if flag == nil {
		t.Fatal("expected max-resolver-loops flag")
	}
	if flag.DefValue != "3" {
		t.Fatalf("default max-resolver-loops = %q, want 3", flag.DefValue)
	}
	if flag := cmd.Flags().Lookup("project"); flag == nil {
		t.Fatal("expected project flag")
	}
	if flag := cmd.Flags().Lookup("brain"); flag == nil {
		t.Fatal("expected brain flag")
	}
	if flag := cmd.Flags().Lookup("watch"); flag == nil || flag.DefValue != "true" {
		t.Fatalf("watch flag = %#v, want default true", flag)
	}
	if flag := cmd.Flags().Lookup("verbosity"); flag == nil || flag.DefValue != "normal" {
		t.Fatalf("verbosity flag = %#v, want default normal", flag)
	}
	resume := newYardChainResumeCmd(&configPath)
	if flag := resume.Flags().Lookup("watch"); flag == nil || flag.DefValue != "true" {
		t.Fatalf("resume watch flag = %#v, want default true", flag)
	}
	if flag := resume.Flags().Lookup("verbosity"); flag == nil || flag.DefValue != "normal" {
		t.Fatalf("resume verbosity flag = %#v, want default normal", flag)
	}
	logs := newYardChainLogsCmd(&configPath)
	if flag := logs.Flags().Lookup("verbosity"); flag == nil || flag.DefValue != "normal" {
		t.Fatalf("logs verbosity flag = %#v, want default normal", flag)
	}
}

func TestYardChainSpecFromFlagsUsesMaxResolverLoops(t *testing.T) {
	spec := yardChainSpecFromFlags("chain-1", yardChainFlags{Specs: "specs/a.md", MaxSteps: 7, MaxResolverLoops: 9, MaxDuration: time.Hour, TokenBudget: 123})
	if spec.MaxResolverLoops != 9 {
		t.Fatalf("MaxResolverLoops = %d, want 9", spec.MaxResolverLoops)
	}
}

func TestYardBuildChainTaskIncludesNoHistoryFallback(t *testing.T) {
	msg := yardBuildChainTask(yardChainFlags{Specs: "specs/a.md,specs/b.md"}, "chain-1", nil)
	if !strings.Contains(msg, "specs/a.md") || !strings.Contains(msg, "chain-1") {
		t.Fatalf("message = %q", msg)
	}
	if !strings.Contains(msg, "No existing receipt paths were found for this chain yet.") {
		t.Fatalf("message = %q, want no-history fallback", msg)
	}
	if strings.Contains(msg, "receipts/state") {
		t.Fatalf("message = %q, unexpected generic receipts/state wording", msg)
	}
}

func TestYardBuildChainTaskIncludesOnlyExistingReceiptPaths(t *testing.T) {
	msg := yardBuildChainTask(yardChainFlags{Task: "fix auth"}, "chain-1", []string{"receipts/planner/chain-1-step-001.md", "receipts/coder/chain-1-step-002.md"})
	if !strings.Contains(msg, "Relevant existing receipt paths to read first: receipts/planner/chain-1-step-001.md, receipts/coder/chain-1-step-002.md") {
		t.Fatalf("message = %q, want existing receipt paths", msg)
	}
	if strings.Contains(msg, "No existing receipt paths were found") {
		t.Fatalf("message = %q, unexpected no-history fallback", msg)
	}
}

func TestYardReceiptPathForStepUsesMatchingStepReceipt(t *testing.T) {
	path, ok := yardReceiptPathForStep("chain-1", "2", []chain.Step{
		{SequenceNum: 1, ReceiptPath: "receipts/planner/chain-1-step-001.md"},
		{SequenceNum: 2, ReceiptPath: "receipts/coder/chain-1-step-002.md"},
	})
	if !ok {
		t.Fatal("yardReceiptPathForStep() ok = false, want true")
	}
	if path != "receipts/coder/chain-1-step-002.md" {
		t.Fatalf("yardReceiptPathForStep() path = %q, want coder receipt", path)
	}
}

func TestApplyYardChainOverrides(t *testing.T) {
	cfg := &appconfig.Config{ProjectRoot: "/old/project"}
	flags := yardChainFlags{ProjectRoot: "/new/project", Brain: "/new/brain"}

	applyYardChainOverrides(cfg, flags)

	if cfg.ProjectRoot != "/new/project" {
		t.Fatalf("ProjectRoot = %q, want /new/project", cfg.ProjectRoot)
	}
	if cfg.Brain.VaultPath != "/new/brain" {
		t.Fatalf("Brain.VaultPath = %q, want /new/brain", cfg.Brain.VaultPath)
	}
}

func TestYardParseSpecsTrimsWhitespaceAndDropsEmptyEntries(t *testing.T) {
	got := yardParseSpecs(" specs/a.md, , specs/b.md ,, ")
	want := []string{"specs/a.md", "specs/b.md"}
	if len(got) != len(want) {
		t.Fatalf("len(yardParseSpecs()) = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("yardParseSpecs()[%d] = %q, want %q (full=%v)", i, got[i], want[i], got)
		}
	}
}

func TestValidateYardChainFlagsRejectsInvalidNumericFlags(t *testing.T) {
	tests := []struct {
		name    string
		flags   yardChainFlags
		wantErr string
	}{
		{name: "missing task and specs", flags: yardChainFlags{}, wantErr: "one of --task or --specs is required"},
		{name: "nonpositive max steps", flags: yardChainFlags{Task: "x", MaxSteps: 0, MaxResolverLoops: 0, MaxDuration: time.Second, TokenBudget: 1}, wantErr: "--max-steps must be > 0"},
		{name: "negative resolver loops", flags: yardChainFlags{Task: "x", MaxSteps: 1, MaxResolverLoops: -1, MaxDuration: time.Second, TokenBudget: 1}, wantErr: "--max-resolver-loops must be >= 0"},
		{name: "nonpositive max duration", flags: yardChainFlags{Task: "x", MaxSteps: 1, MaxResolverLoops: 0, MaxDuration: 0, TokenBudget: 1}, wantErr: "--max-duration must be > 0"},
		{name: "nonpositive token budget", flags: yardChainFlags{Task: "x", MaxSteps: 1, MaxResolverLoops: 0, MaxDuration: time.Second, TokenBudget: 0}, wantErr: "--token-budget must be > 0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateYardChainFlags(tc.flags); err == nil || err.Error() != tc.wantErr {
				t.Fatalf("validateYardChainFlags() error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestValidateYardChainFlagsAcceptsZeroResolverLoops(t *testing.T) {
	flags := yardChainFlags{Task: "x", MaxSteps: 1, MaxResolverLoops: 0, MaxDuration: time.Second, TokenBudget: 1}
	if err := validateYardChainFlags(flags); err != nil {
		t.Fatalf("validateYardChainFlags() error = %v, want nil", err)
	}
}

func TestValidateYardChainFlagsAcceptsChainIDOnlyForResume(t *testing.T) {
	flags := yardChainFlags{ChainID: "chain-1", MaxSteps: 1, MaxResolverLoops: 0, MaxDuration: time.Second, TokenBudget: 1}
	if err := validateYardChainFlags(flags); err != nil {
		t.Fatalf("validateYardChainFlags() error = %v, want nil", err)
	}
}

func TestValidateYardChainStatusTransition(t *testing.T) {
	if err := validateYardChainStatusTransition("paused", "running", "chain-1"); err != nil {
		t.Fatalf("resume paused chain error = %v, want nil", err)
	}
	if err := validateYardChainStatusTransition("pause_requested", "running", "chain-1"); err == nil {
		t.Fatal("expected pause_requested chain resume transition to fail")
	}
	if err := validateYardChainStatusTransition("completed", "running", "chain-1"); err == nil {
		t.Fatal("expected completed chain resume to fail")
	}
	if err := validateYardChainStatusTransition("running", "paused", "chain-1"); err != nil {
		t.Fatalf("pause running chain error = %v, want nil", err)
	}
	if err := validateYardChainStatusTransition("pause_requested", "cancelled", "chain-1"); err != nil {
		t.Fatalf("cancel pause_requested chain error = %v, want nil", err)
	}
}

func TestPrepareYardExistingChainForExecutionRejectsPauseRequestedResume(t *testing.T) {
	cmd := newYardChainCmd(new(string))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := prepareYardExistingChainForExecution(context.Background(), nil, &chain.Chain{ID: "chain-1", Status: "pause_requested"}, cmd)
	if err == nil || !strings.Contains(err.Error(), "cannot be resumed until paused") {
		t.Fatalf("error = %v, want pause_requested resume rejection", err)
	}
}

func TestPrepareYardExistingChainForExecutionStopsDuplicateRunningResume(t *testing.T) {
	cmd := newYardChainCmd(new(string))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := prepareYardExistingChainForExecution(context.Background(), nil, &chain.Chain{ID: "chain-1", Status: "running"}, cmd)
	if err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("error = %v, want already running rejection", err)
	}
	if !strings.Contains(buf.String(), "already running") {
		t.Fatalf("output = %q, want already running message", buf.String())
	}
}

func TestHandleYardChainRunInterruptionClosesStaleActiveExecutionForPausedChain(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newYardChainControlTestDB(t))
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
	cmd := newYardChainCmd(new(string))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	handled, err := handleYardChainRunInterruption(ctx, store, chainID, agent.ErrTurnCancelled, cmd)
	if err != nil {
		t.Fatalf("handleYardChainRunInterruption returned error: %v", err)
	}
	if !handled {
		t.Fatal("handleYardChainRunInterruption handled = false, want true")
	}
	if !strings.Contains(buf.String(), "chain "+chainID+" paused") {
		t.Fatalf("output = %q, want paused message", buf.String())
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

func TestSignalYardActiveChainProcessIgnoresStalePID(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newYardChainControlTestDB(t))
	chainID, err := store.StartChain(ctx, chain.ChainSpec{MaxSteps: 5, MaxResolverLoops: 1, MaxDuration: time.Hour, TokenBudget: 100})
	if err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, "", chain.EventChainStarted, map[string]any{"orchestrator_pid": 4242}); err != nil {
		t.Fatalf("LogEvent returned error: %v", err)
	}

	originalInterrupt := interruptYardChainPID
	interruptYardChainPID = func(pid int) error {
		if pid != 4242 {
			t.Fatalf("interrupt pid = %d, want 4242", pid)
		}
		return errYardChainPIDNotRunning
	}
	defer func() { interruptYardChainPID = originalInterrupt }()

	if err := signalYardActiveChainProcess(ctx, store, chainID); err != nil {
		t.Fatalf("signalYardActiveChainProcess returned error: %v", err)
	}
}

func TestSignalYardActiveChainProcessUsesLatestRegisteredOrchestratorPID(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newYardChainControlTestDB(t))
	chainID, err := store.StartChain(ctx, chain.ChainSpec{MaxSteps: 5, MaxResolverLoops: 1, MaxDuration: time.Hour, TokenBudget: 100})
	if err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, "", chain.EventChainStarted, map[string]any{"orchestrator_pid": 1111, "execution_id": "exec-1", "active_execution": true}); err != nil {
		t.Fatalf("LogEvent returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, "", chain.EventStepStarted, map[string]any{"orchestrator_pid": 9999}); err != nil {
		t.Fatalf("LogEvent returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, "", chain.EventChainResumed, map[string]any{"orchestrator_pid": 2222, "execution_id": "exec-2", "active_execution": true}); err != nil {
		t.Fatalf("LogEvent returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, "", chain.EventChainPaused, map[string]any{"orchestrator_pid": 3333, "execution_id": "exec-2"}); err != nil {
		t.Fatalf("LogEvent returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, "", chain.EventChainResumed, map[string]any{"orchestrator_pid": 4444, "execution_id": "exec-3", "active_execution": true}); err != nil {
		t.Fatalf("LogEvent returned error: %v", err)
	}

	originalInterrupt := interruptYardChainPID
	called := 0
	interruptYardChainPID = func(pid int) error {
		called++
		if pid != 4444 {
			t.Fatalf("interrupt pid = %d, want 4444", pid)
		}
		return nil
	}
	defer func() { interruptYardChainPID = originalInterrupt }()

	if err := signalYardActiveChainProcess(ctx, store, chainID); err != nil {
		t.Fatalf("signalYardActiveChainProcess returned error: %v", err)
	}
	if called != 1 {
		t.Fatalf("interrupt call count = %d, want 1", called)
	}
}

func TestYardRunChainWatchFalseRunsInForegroundWithoutHiddenChild(t *testing.T) {
	ctx := context.Background()
	cfgPath, projectRoot := writeYardRunConfig(t)
	db := newYardChainControlTestDB(t)
	store := chain.NewStore(db)

	originalBuildRuntime := buildYardChainRuntime
	originalBuildRegistry := buildYardChainRegistry
	originalNewTurnRunner := newYardChainTurnRunner
	defer func() {
		buildYardChainRuntime = originalBuildRuntime
		buildYardChainRegistry = originalBuildRegistry
		newYardChainTurnRunner = originalNewTurnRunner
	}()

	buildYardChainRuntime = func(ctx context.Context, cfg *appconfig.Config) (*rtpkg.OrchestratorRuntime, error) {
		if cfg.ProjectRoot != projectRoot {
			t.Fatalf("ProjectRoot = %q, want %q", cfg.ProjectRoot, projectRoot)
		}
		if err := rtpkg.EnsureProjectRecord(ctx, db, cfg); err != nil {
			t.Fatalf("EnsureProjectRecord returned error: %v", err)
		}
		return &rtpkg.OrchestratorRuntime{Config: cfg, Logger: slog.Default(), ConversationManager: conversation.NewManager(db, nil, slog.Default()), ContextAssembler: rtpkg.NoopContextAssembler{}, ChainStore: store, Cleanup: func() {}}, nil
	}
	buildYardChainRegistry = func(rt *rtpkg.OrchestratorRuntime, roleCfg appconfig.AgentRoleConfig, chainID string) (*tool.Registry, error) {
		return tool.NewRegistry(), nil
	}

	const chainID = "yard-run-foreground"
	newYardChainTurnRunner = func(deps agent.AgentLoopDeps) chainTurnRunner {
		return &blockingYardChainTurnRunner{run: func(ctx context.Context, req agent.RunTurnRequest) (*agent.TurnResult, error) {
			if err := store.SetChainStatus(ctx, chainID, "completed"); err != nil {
				t.Fatalf("SetChainStatus returned error: %v", err)
			}
			return &agent.TurnResult{FinalText: "done", IterationCount: 1, Duration: time.Second}, nil
		}}
	}

	var stdout, stderr bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	if err := yardRunChain(ctx, cfgPath, yardChainFlags{Task: "foreground run", ChainID: chainID, Watch: false}, cmd); err != nil {
		t.Fatalf("yardRunChain returned error: %v", err)
	}
	if stdout.String() != chainID+"\n" {
		t.Fatalf("stdout = %q, want only chain id", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want no watch output", stderr.String())
	}
	finalEvents, err := store.ListEvents(context.Background(), chainID)
	if err != nil {
		t.Fatalf("ListEvents returned error: %v", err)
	}
	exec, ok := chain.LatestActiveExecution(finalEvents)
	if !ok || exec.OrchestratorPID != os.Getpid() {
		t.Fatalf("LatestActiveExecution() = (%+v, %t), want current pid %d", exec, ok, os.Getpid())
	}
}

func TestYardRunChainWatchInterruptCancelsForegroundExecution(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfgPath, projectRoot := writeYardRunConfig(t)
	db := newYardChainControlTestDB(t)
	store := chain.NewStore(db)

	originalBuildRuntime := buildYardChainRuntime
	originalBuildRegistry := buildYardChainRegistry
	originalNewTurnRunner := newYardChainTurnRunner
	defer func() {
		buildYardChainRuntime = originalBuildRuntime
		buildYardChainRegistry = originalBuildRegistry
		newYardChainTurnRunner = originalNewTurnRunner
	}()

	buildYardChainRuntime = func(ctx context.Context, cfg *appconfig.Config) (*rtpkg.OrchestratorRuntime, error) {
		if cfg.ProjectRoot != projectRoot {
			t.Fatalf("ProjectRoot = %q, want %q", cfg.ProjectRoot, projectRoot)
		}
		if err := rtpkg.EnsureProjectRecord(ctx, db, cfg); err != nil {
			t.Fatalf("EnsureProjectRecord returned error: %v", err)
		}
		return &rtpkg.OrchestratorRuntime{Config: cfg, Logger: slog.Default(), ConversationManager: conversation.NewManager(db, nil, slog.Default()), ContextAssembler: rtpkg.NoopContextAssembler{}, ChainStore: store, Cleanup: func() {}}, nil
	}
	buildYardChainRegistry = func(rt *rtpkg.OrchestratorRuntime, roleCfg appconfig.AgentRoleConfig, chainID string) (*tool.Registry, error) {
		return tool.NewRegistry(), nil
	}

	const chainID = "yard-run-interrupt"
	started := make(chan struct{})
	newYardChainTurnRunner = func(deps agent.AgentLoopDeps) chainTurnRunner {
		return &blockingYardChainTurnRunner{started: started, run: func(ctx context.Context, req agent.RunTurnRequest) (*agent.TurnResult, error) {
			if err := store.LogEvent(context.Background(), chainID, "", chain.EventStepStarted, map[string]any{"role": "planner", "task": "foreground task"}); err != nil {
				t.Fatalf("LogEvent returned error: %v", err)
			}
			<-ctx.Done()
			return nil, agent.ErrTurnCancelled
		}}
	}

	var stdout, stderr bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	errCh := make(chan error, 1)
	go func() {
		errCh <- yardRunChain(ctx, cfgPath, yardChainFlags{Task: "interrupt", ChainID: chainID, Watch: true}, cmd)
	}()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for blocked yard chain turn to start")
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(stderr.String(), "step_started") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(stderr.String(), "step_started") {
		t.Fatalf("stderr = %q, want streamed watch output before interrupt", stderr.String())
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("yardRunChain returned error: %v", err)
	}
	if strings.Contains(stderr.String(), "detached from live output") {
		t.Fatalf("stderr = %q, did not want detach message", stderr.String())
	}
	stored, err := store.GetChain(context.Background(), chainID)
	if err != nil {
		t.Fatalf("GetChain returned error: %v", err)
	}
	if stored.Status == "running" {
		t.Fatalf("status = %q, want non-running status after interrupt", stored.Status)
	}
	if stored.Status != "cancelled" {
		t.Fatalf("status = %q, want cancelled after interrupt", stored.Status)
	}
	finalEvents, err := store.ListEvents(context.Background(), chainID)
	if err != nil {
		t.Fatalf("ListEvents returned error: %v", err)
	}
	if exec, ok := chain.LatestActiveExecution(finalEvents); ok || exec.ExecutionID != "" || exec.OrchestratorPID != 0 {
		t.Fatalf("LatestActiveExecution() = (%+v, %t), want empty,false", exec, ok)
	}
}

func TestYardRunChainRegistersActiveExecutionWithOwnPID(t *testing.T) {
	ctx := context.Background()
	cfgPath, projectRoot := writeYardRunConfig(t)
	db := newYardChainControlTestDB(t)
	store := chain.NewStore(db)
	const chainID = "yard-registers-own-pid"

	originalBuildRuntime := buildYardChainRuntime
	originalBuildRegistry := buildYardChainRegistry
	originalNewTurnRunner := newYardChainTurnRunner
	defer func() {
		buildYardChainRuntime = originalBuildRuntime
		buildYardChainRegistry = originalBuildRegistry
		newYardChainTurnRunner = originalNewTurnRunner
	}()

	buildYardChainRuntime = func(ctx context.Context, cfg *appconfig.Config) (*rtpkg.OrchestratorRuntime, error) {
		if cfg.ProjectRoot != projectRoot {
			t.Fatalf("ProjectRoot = %q, want %q", cfg.ProjectRoot, projectRoot)
		}
		if err := rtpkg.EnsureProjectRecord(ctx, db, cfg); err != nil {
			t.Fatalf("EnsureProjectRecord returned error: %v", err)
		}
		return &rtpkg.OrchestratorRuntime{Config: cfg, Logger: slog.Default(), ConversationManager: conversation.NewManager(db, nil, slog.Default()), ContextAssembler: rtpkg.NoopContextAssembler{}, ChainStore: store, Cleanup: func() {}}, nil
	}
	buildYardChainRegistry = func(rt *rtpkg.OrchestratorRuntime, roleCfg appconfig.AgentRoleConfig, chainID string) (*tool.Registry, error) {
		return tool.NewRegistry(), nil
	}
	newYardChainTurnRunner = func(deps agent.AgentLoopDeps) chainTurnRunner {
		return &blockingYardChainTurnRunner{run: func(ctx context.Context, req agent.RunTurnRequest) (*agent.TurnResult, error) {
			if err := store.SetChainStatus(ctx, chainID, "completed"); err != nil {
				t.Fatalf("SetChainStatus returned error: %v", err)
			}
			return &agent.TurnResult{FinalText: "done", IterationCount: 1, Duration: time.Second}, nil
		}}
	}

	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	if err := yardRunChain(ctx, cfgPath, yardChainFlags{Task: "worker task", ChainID: chainID, Watch: false}, cmd); err != nil {
		t.Fatalf("yardRunChain returned error: %v", err)
	}
	events, err := store.ListEvents(context.Background(), chainID)
	if err != nil {
		t.Fatalf("ListEvents returned error: %v", err)
	}
	exec, ok := chain.LatestActiveExecution(events)
	if !ok || exec.OrchestratorPID != os.Getpid() {
		t.Fatalf("LatestActiveExecution() = (%+v, %t), want current pid %d", exec, ok, os.Getpid())
	}
}

func TestYardRunChainPrintsChainIDEarlyAndStreamsWatchOutput(t *testing.T) {
	ctx := context.Background()
	cfgPath, projectRoot := writeYardRunConfig(t)
	db := newYardChainControlTestDB(t)
	store := chain.NewStore(db)

	originalBuildRuntime := buildYardChainRuntime
	originalBuildRegistry := buildYardChainRegistry
	originalNewTurnRunner := newYardChainTurnRunner
	defer func() {
		buildYardChainRuntime = originalBuildRuntime
		buildYardChainRegistry = originalBuildRegistry
		newYardChainTurnRunner = originalNewTurnRunner
	}()

	buildYardChainRuntime = func(ctx context.Context, cfg *appconfig.Config) (*rtpkg.OrchestratorRuntime, error) {
		if cfg.ProjectRoot != projectRoot {
			t.Fatalf("ProjectRoot = %q, want %q", cfg.ProjectRoot, projectRoot)
		}
		if err := rtpkg.EnsureProjectRecord(ctx, db, cfg); err != nil {
			t.Fatalf("EnsureProjectRecord returned error: %v", err)
		}
		return &rtpkg.OrchestratorRuntime{Config: cfg, Logger: slog.Default(), ConversationManager: conversation.NewManager(db, nil, slog.Default()), ContextAssembler: rtpkg.NoopContextAssembler{}, ChainStore: store, Cleanup: func() {}}, nil
	}
	buildYardChainRegistry = func(rt *rtpkg.OrchestratorRuntime, roleCfg appconfig.AgentRoleConfig, chainID string) (*tool.Registry, error) {
		return tool.NewRegistry(), nil
	}

	const chainID = "yard-run-watch"
	newYardChainTurnRunner = func(deps agent.AgentLoopDeps) chainTurnRunner {
		return &blockingYardChainTurnRunner{run: func(ctx context.Context, req agent.RunTurnRequest) (*agent.TurnResult, error) {
			if err := store.LogEvent(context.Background(), chainID, "", chain.EventStepStarted, map[string]any{"role": "planner", "task": "watch task"}); err != nil {
				t.Fatalf("LogEvent returned error: %v", err)
			}
			if err := store.SetChainStatus(ctx, chainID, "completed"); err != nil {
				t.Fatalf("SetChainStatus returned error: %v", err)
			}
			return &agent.TurnResult{FinalText: "done", IterationCount: 1, Duration: time.Second}, nil
		}}
	}

	var stdout, stderr bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	if err := yardRunChain(ctx, cfgPath, yardChainFlags{Task: "watch task", ChainID: chainID, Watch: true}, cmd); err != nil {
		t.Fatalf("yardRunChain returned error: %v", err)
	}
	if stdout.String() != chainID+"\n" {
		t.Fatalf("stdout = %q, want chain id before watch completion", stdout.String())
	}
	if !strings.Contains(stderr.String(), "step_started") || !strings.Contains(stderr.String(), "role=planner") {
		t.Fatalf("stderr = %q, want streamed step_started output", stderr.String())
	}
}

func TestYardRunChainWatchFalseSuppressesStderrProgress(t *testing.T) {
	ctx := context.Background()
	cfgPath, projectRoot := writeYardRunConfig(t)
	db := newYardChainControlTestDB(t)
	store := chain.NewStore(db)

	originalBuildRuntime := buildYardChainRuntime
	originalBuildRegistry := buildYardChainRegistry
	originalNewTurnRunner := newYardChainTurnRunner
	defer func() {
		buildYardChainRuntime = originalBuildRuntime
		buildYardChainRegistry = originalBuildRegistry
		newYardChainTurnRunner = originalNewTurnRunner
	}()

	buildYardChainRuntime = func(ctx context.Context, cfg *appconfig.Config) (*rtpkg.OrchestratorRuntime, error) {
		if cfg.ProjectRoot != projectRoot {
			t.Fatalf("ProjectRoot = %q, want %q", cfg.ProjectRoot, projectRoot)
		}
		if err := rtpkg.EnsureProjectRecord(ctx, db, cfg); err != nil {
			t.Fatalf("EnsureProjectRecord returned error: %v", err)
		}
		return &rtpkg.OrchestratorRuntime{Config: cfg, Logger: slog.Default(), ConversationManager: conversation.NewManager(db, nil, slog.Default()), ContextAssembler: rtpkg.NoopContextAssembler{}, ChainStore: store, Cleanup: func() {}}, nil
	}
	buildYardChainRegistry = func(rt *rtpkg.OrchestratorRuntime, roleCfg appconfig.AgentRoleConfig, chainID string) (*tool.Registry, error) {
		return tool.NewRegistry(), nil
	}

	const chainID = "yard-run-no-watch"
	newYardChainTurnRunner = func(deps agent.AgentLoopDeps) chainTurnRunner {
		return &blockingYardChainTurnRunner{run: func(ctx context.Context, req agent.RunTurnRequest) (*agent.TurnResult, error) {
			if err := store.LogEvent(context.Background(), chainID, "", chain.EventStepStarted, map[string]any{"role": "planner", "task": "watch task"}); err != nil {
				t.Fatalf("LogEvent returned error: %v", err)
			}
			if err := store.SetChainStatus(ctx, chainID, "completed"); err != nil {
				t.Fatalf("SetChainStatus returned error: %v", err)
			}
			return &agent.TurnResult{FinalText: "done", IterationCount: 1, Duration: time.Second}, nil
		}}
	}

	var stdout, stderr bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	if err := yardRunChain(ctx, cfgPath, yardChainFlags{Task: "watch task", ChainID: chainID, Watch: false}, cmd); err != nil {
		t.Fatalf("yardRunChain returned error: %v", err)
	}
	if stdout.String() != chainID+"\n" {
		t.Fatalf("stdout = %q, want only chain id", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want no streamed output when watch=false", stderr.String())
	}
}

func TestYardRunChainPauseRequestedInterruptionClosesActiveExecution(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfgPath, projectRoot := writeYardRunConfig(t)
	db := newYardChainControlTestDB(t)
	store := chain.NewStore(db)
	const chainID = "yard-run-pause"

	originalBuildRuntime := buildYardChainRuntime
	originalBuildRegistry := buildYardChainRegistry
	originalNewTurnRunner := newYardChainTurnRunner
	defer func() {
		buildYardChainRuntime = originalBuildRuntime
		buildYardChainRegistry = originalBuildRegistry
		newYardChainTurnRunner = originalNewTurnRunner
	}()

	buildYardChainRuntime = func(ctx context.Context, cfg *appconfig.Config) (*rtpkg.OrchestratorRuntime, error) {
		if cfg.ProjectRoot != projectRoot {
			t.Fatalf("ProjectRoot = %q, want %q", cfg.ProjectRoot, projectRoot)
		}
		if err := rtpkg.EnsureProjectRecord(ctx, db, cfg); err != nil {
			t.Fatalf("EnsureProjectRecord returned error: %v", err)
		}
		return &rtpkg.OrchestratorRuntime{
			Config:              cfg,
			Logger:              slog.Default(),
			ConversationManager: conversation.NewManager(db, nil, slog.Default()),
			ContextAssembler:    rtpkg.NoopContextAssembler{},
			ChainStore:          store,
			Cleanup:             func() {},
		}, nil
	}
	buildYardChainRegistry = func(rt *rtpkg.OrchestratorRuntime, roleCfg appconfig.AgentRoleConfig, chainID string) (*tool.Registry, error) {
		return tool.NewRegistry(), nil
	}

	started := make(chan struct{})
	newYardChainTurnRunner = func(deps agent.AgentLoopDeps) chainTurnRunner {
		return &blockingYardChainTurnRunner{
			started: started,
			run: func(ctx context.Context, req agent.RunTurnRequest) (*agent.TurnResult, error) {
				<-ctx.Done()
				return nil, agent.ErrTurnCancelled
			},
		}
	}

	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	errCh := make(chan error, 1)
	go func() {
		errCh <- yardRunChain(ctx, cfgPath, yardChainFlags{Task: "prove pause cleanup", ChainID: chainID, Watch: false}, cmd)
	}()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for blocked yard chain turn to start")
	}

	events, err := waitForYardChainActiveExecution(ctx, store, chainID)
	if err != nil {
		t.Fatalf("waitForYardChainActiveExecution returned error: %v", err)
	}
	activeExec, ok := chain.LatestActiveExecution(events)
	if !ok || activeExec.ExecutionID == "" {
		t.Fatalf("LatestActiveExecution() = (%+v, %t), want populated active execution", activeExec, ok)
	}
	if err := store.SetChainStatus(context.Background(), chainID, "pause_requested"); err != nil {
		t.Fatalf("SetChainStatus returned error: %v", err)
	}
	cancel()

	if err := <-errCh; err != nil {
		t.Fatalf("yardRunChain returned error: %v", err)
	}
	if !strings.Contains(out.String(), "chain "+chainID+" paused") {
		t.Fatalf("output = %q, want paused message", out.String())
	}
	stored, err := store.GetChain(context.Background(), chainID)
	if err != nil {
		t.Fatalf("GetChain returned error: %v", err)
	}
	if stored.Status != "paused" {
		t.Fatalf("status = %q, want paused", stored.Status)
	}
	finalEvents, err := store.ListEvents(context.Background(), chainID)
	if err != nil {
		t.Fatalf("ListEvents returned error: %v", err)
	}
	if exec, ok := chain.LatestActiveExecution(finalEvents); ok || exec.ExecutionID != "" || exec.OrchestratorPID != 0 {
		t.Fatalf("LatestActiveExecution() = (%+v, %t), want empty,false", exec, ok)
	}
	last := finalEvents[len(finalEvents)-1]
	if last.EventType != chain.EventChainPaused {
		t.Fatalf("last event type = %s, want %s", last.EventType, chain.EventChainPaused)
	}
	if !strings.Contains(last.EventData, `"execution_id":"`+activeExec.ExecutionID+`"`) {
		t.Fatalf("last event data = %s, want execution_id %s", last.EventData, activeExec.ExecutionID)
	}
}

func waitForYardChainActiveExecution(ctx context.Context, store *chain.Store, chainID string) ([]chain.Event, error) {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		events, err := store.ListEvents(ctx, chainID)
		if err == nil {
			if _, ok := chain.LatestActiveExecution(events); ok {
				return events, nil
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	return nil, context.DeadlineExceeded
}

func writeYardRunConfig(t *testing.T) (string, string) {
	t.Helper()
	projectRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectRoot, ".brain"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.brain) returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "orchestrator-prompt.md"), []byte("You are the orchestrator."), 0o644); err != nil {
		t.Fatalf("WriteFile(prompt) returned error: %v", err)
	}
	configPath := filepath.Join(projectRoot, "yard.yaml")
	config := "project_root: " + projectRoot + "\n" +
		"brain:\n  enabled: false\n" +
		"local_services:\n  enabled: false\n" +
		"agent_roles:\n  orchestrator:\n    system_prompt: orchestrator-prompt.md\n"
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatalf("WriteFile(config) returned error: %v", err)
	}
	return configPath, projectRoot
}

func TestRenderYardChainEventsSkipsSuppressedOutputAndReturnsLastID(t *testing.T) {
	events := []chain.Event{
		{ID: 42, CreatedAt: time.Date(2026, 4, 21, 1, 2, 3, 0, time.UTC), EventType: chain.EventStepOutput, EventData: `{"stream":"stderr","line":"status: waiting_for_llm"}`},
		{ID: 43, CreatedAt: time.Date(2026, 4, 21, 1, 2, 4, 0, time.UTC), EventType: chain.EventStepStarted, EventData: `{"role":"coder","task":"fix auth","receipt_path":"receipts/coder/chain-step-001.md"}`},
	}
	var out bytes.Buffer
	got := renderYardChainEvents(&out, events, chainRenderOptions{Verbosity: chainVerbosityNormal})
	if got != 43 {
		t.Fatalf("renderYardChainEvents() last id = %d, want 43", got)
	}
	want := "43\t2026-04-21T01:02:04Z\tstep_started\trole=coder task=\"fix auth\" receipt_path=receipts/coder/chain-step-001.md\n"
	if out.String() != want {
		t.Fatalf("renderYardChainEvents() output = %q, want %q", out.String(), want)
	}
}

func TestFormatChainEventRendersStepOutputCompactly(t *testing.T) {
	event := chain.Event{
		ID:        42,
		CreatedAt: time.Date(2026, 4, 21, 1, 2, 3, 0, time.UTC),
		EventType: chain.EventStepOutput,
		EventData: `{"stream":"stderr","line":"thinking hard"}`,
	}
	got := formatChainEvent(event, chainRenderOptions{Verbosity: chainVerbosityNormal})
	want := "42\t2026-04-21T01:02:03Z\tstep_output\t[stderr] thinking hard\n"
	if got != want {
		t.Fatalf("formatChainEvent() = %q, want %q", got, want)
	}
}

func TestFormatChainEventSuppressesNormalModeBootstrapChatter(t *testing.T) {
	event := chain.Event{ID: 43, CreatedAt: time.Date(2026, 4, 21, 1, 2, 3, 0, time.UTC), EventType: chain.EventStepOutput, EventData: `{"stream":"stderr","line":"status: waiting_for_llm"}`}
	if got := formatChainEvent(event, chainRenderOptions{Verbosity: chainVerbosityNormal}); got != "" {
		t.Fatalf("formatChainEvent() = %q, want empty for suppressed normal-mode chatter", got)
	}
	want := "43\t2026-04-21T01:02:03Z\tstep_output\t[stderr] status: waiting_for_llm\n"
	if got := formatChainEvent(event, chainRenderOptions{Verbosity: chainVerbosityDebug}); got != want {
		t.Fatalf("debug formatChainEvent() = %q, want %q", got, want)
	}
}

func TestFormatChainEventRendersStepStartedCompactly(t *testing.T) {
	event := chain.Event{ID: 7, CreatedAt: time.Date(2026, 4, 21, 1, 2, 3, 0, time.UTC), EventType: chain.EventStepStarted, EventData: `{"role":"coder","task":"fix auth","receipt_path":"receipts/coder/chain-step-001.md"}`}
	got := formatChainEvent(event, chainRenderOptions{Verbosity: chainVerbosityNormal})
	want := "7\t2026-04-21T01:02:03Z\tstep_started\trole=coder task=\"fix auth\" receipt_path=receipts/coder/chain-step-001.md\n"
	if got != want {
		t.Fatalf("formatChainEvent() = %q, want %q", got, want)
	}
}

func TestFormatChainEventFallsBackToRawPayloadForNonStepOutput(t *testing.T) {
	event := chain.Event{
		ID:        7,
		CreatedAt: time.Date(2026, 4, 21, 1, 2, 3, 0, time.UTC),
		EventType: chain.EventStepStarted,
		EventData: `not-json`,
	}
	got := formatChainEvent(event, chainRenderOptions{Verbosity: chainVerbosityNormal})
	want := "7\t2026-04-21T01:02:03Z\tstep_started\tnot-json\n"
	if got != want {
		t.Fatalf("formatChainEvent() = %q, want %q", got, want)
	}
}
