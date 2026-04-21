package main

import (
	"bytes"
	"context"
	"fmt"
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

type fakeChainLoop struct{}

type blockingChainTurnRunner struct {
	started chan<- struct{}
	run     func(ctx context.Context, req agent.RunTurnRequest) (*agent.TurnResult, error)
}

func (fakeChainLoop) RunTurn(ctx context.Context, req agent.RunTurnRequest) (*agent.TurnResult, error) {
	return &agent.TurnResult{FinalText: "done", IterationCount: 1, Duration: time.Second}, nil
}

func (b *blockingChainTurnRunner) RunTurn(ctx context.Context, req agent.RunTurnRequest) (*agent.TurnResult, error) {
	if b.started != nil {
		close(b.started)
	}
	if b.run != nil {
		return b.run(ctx, req)
	}
	<-ctx.Done()
	return nil, agent.ErrTurnCancelled
}

func (fakeChainLoop) Close()            {}
func (*blockingChainTurnRunner) Close() {}

func TestRootIncludesPhase3Subcommands(t *testing.T) {
	cmd := newRootCmd()
	names := []string{}
	for _, child := range cmd.Commands() {
		names = append(names, child.Name())
	}
	for _, want := range []string{"chain", "status", "logs", "receipt", "cancel", "pause", "resume"} {
		if !contains(names, want) {
			t.Fatalf("missing subcommand %q in %v", want, names)
		}
	}
}

func TestChainCommandRequiresTaskOrSpecs(t *testing.T) {
	var buf bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"chain"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "one of --task or --specs is required") {
		t.Fatalf("error = %v, want task/specs requirement", err)
	}
}

func TestBuildChainTaskIncludesSpecsChainIDAndNoHistoryFallback(t *testing.T) {
	msg := buildChainTask(chainFlags{Specs: "specs/a.md,specs/b.md"}, "chain-1", nil)
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

func TestBuildChainTaskIncludesOnlyExistingReceiptPaths(t *testing.T) {
	msg := buildChainTask(chainFlags{Task: "fix auth"}, "chain-1", []string{"receipts/planner/chain-1-step-001.md", "receipts/coder/chain-1-step-002.md"})
	if !strings.Contains(msg, "Relevant existing receipt paths to read first: receipts/planner/chain-1-step-001.md, receipts/coder/chain-1-step-002.md") {
		t.Fatalf("message = %q, want existing receipt paths", msg)
	}
	if strings.Contains(msg, "No existing receipt paths were found") {
		t.Fatalf("message = %q, unexpected no-history fallback", msg)
	}
}

func TestChainSpecFromFlags(t *testing.T) {
	spec := chainSpecFromFlags("chain-1", chainFlags{Specs: "specs/a.md", Task: "ignored", MaxSteps: 7, MaxResolverLoops: 4, MaxDuration: time.Hour, TokenBudget: 123})
	if spec.ChainID != "chain-1" || len(spec.SourceSpecs) != 1 || spec.MaxSteps != 7 || spec.MaxResolverLoops != 4 || spec.TokenBudget != 123 {
		t.Fatalf("unexpected spec: %+v", spec)
	}
}

func TestPrepareExistingChainForExecutionRejectsPauseRequestedResume(t *testing.T) {
	cmd := newRootCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := prepareExistingChainForExecution(context.Background(), nil, &chain.Chain{ID: "chain-1", Status: "pause_requested"}, cmd)
	if err == nil || !strings.Contains(err.Error(), "cannot be resumed until paused") {
		t.Fatalf("error = %v, want pause_requested resume rejection", err)
	}
}

func TestPrepareExistingChainForExecutionStopsDuplicateRunningResume(t *testing.T) {
	cmd := newRootCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := prepareExistingChainForExecution(context.Background(), nil, &chain.Chain{ID: "chain-1", Status: "running"}, cmd)
	if err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("error = %v, want already running rejection", err)
	}
	if !strings.Contains(buf.String(), "already running") {
		t.Fatalf("output = %q, want already running message", buf.String())
	}
}

func TestHandleChainRunInterruptionClosesStaleActiveExecutionForCancelledChain(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newChainControlTestDB(t))
	chainID, err := store.StartChain(ctx, chain.ChainSpec{MaxSteps: 5, MaxResolverLoops: 1, MaxDuration: time.Hour, TokenBudget: 100})
	if err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, "", chain.EventChainStarted, map[string]any{"orchestrator_pid": 1111, "execution_id": "exec-1", "active_execution": true}); err != nil {
		t.Fatalf("LogEvent returned error: %v", err)
	}
	if err := store.SetChainStatus(ctx, chainID, "cancelled"); err != nil {
		t.Fatalf("SetChainStatus returned error: %v", err)
	}
	cmd := newRootCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	handled, err := handleChainRunInterruption(ctx, store, chainID, agent.ErrTurnCancelled, cmd)
	if err != nil {
		t.Fatalf("handleChainRunInterruption returned error: %v", err)
	}
	if !handled {
		t.Fatal("handleChainRunInterruption handled = false, want true")
	}
	if !strings.Contains(buf.String(), "chain "+chainID+" cancelled") {
		t.Fatalf("output = %q, want cancelled message", buf.String())
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
	if exec, ok := chain.LatestActiveExecution(events); ok || exec.ExecutionID != "" || exec.OrchestratorPID != 0 {
		t.Fatalf("LatestActiveExecution() = (%+v, %t), want empty,false", exec, ok)
	}
}

func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func TestSignalActiveChainProcessIgnoresStalePID(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newChainControlTestDB(t))
	chainID, err := store.StartChain(ctx, chain.ChainSpec{MaxSteps: 5, MaxResolverLoops: 1, MaxDuration: time.Hour, TokenBudget: 100})
	if err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, "", chain.EventChainStarted, map[string]any{"orchestrator_pid": 4242}); err != nil {
		t.Fatalf("LogEvent returned error: %v", err)
	}

	originalInterrupt := interruptChainPID
	interruptChainPID = func(pid int) error {
		if pid != 4242 {
			t.Fatalf("interrupt pid = %d, want 4242", pid)
		}
		return errChainPIDNotRunning
	}
	defer func() { interruptChainPID = originalInterrupt }()

	if err := signalActiveChainProcess(ctx, store, chainID); err != nil {
		t.Fatalf("signalActiveChainProcess returned error: %v", err)
	}
}

func TestSignalActiveChainProcessUsesLatestRegisteredOrchestratorPID(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newChainControlTestDB(t))
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

	originalInterrupt := interruptChainPID
	called := 0
	interruptChainPID = func(pid int) error {
		called++
		if pid != 4444 {
			t.Fatalf("interrupt pid = %d, want 4444", pid)
		}
		return nil
	}
	defer func() { interruptChainPID = originalInterrupt }()

	if err := signalActiveChainProcess(ctx, store, chainID); err != nil {
		t.Fatalf("signalActiveChainProcess returned error: %v", err)
	}
	if called != 1 {
		t.Fatalf("interrupt call count = %d, want 1", called)
	}
}

func TestRunChainLaunchesBackgroundChildAndParentDoesNotRegisterActiveExecution(t *testing.T) {
	ctx := context.Background()
	cfgPath, projectRoot := writeChainRunConfig(t)
	db := newChainControlTestDB(t)
	store := chain.NewStore(db)

	originalBuildRuntime := buildChainRuntime
	originalLaunch := launchChainBackgroundChild
	defer func() {
		buildChainRuntime = originalBuildRuntime
		launchChainBackgroundChild = originalLaunch
	}()

	buildChainRuntime = func(ctx context.Context, cfg *appconfig.Config) (*orchestratorRuntime, error) {
		if cfg.ProjectRoot != projectRoot {
			t.Fatalf("ProjectRoot = %q, want %q", cfg.ProjectRoot, projectRoot)
		}
		if err := rtpkg.EnsureProjectRecord(ctx, db, cfg); err != nil {
			t.Fatalf("EnsureProjectRecord returned error: %v", err)
		}
		return &orchestratorRuntime{
			Config:              cfg,
			Logger:              slog.Default(),
			ConversationManager: conversation.NewManager(db, nil, slog.Default()),
			ContextAssembler:    rtpkg.NoopContextAssembler{},
			ChainStore:          store,
			Cleanup:             func() {},
		}, nil
	}

	const chainID = "chain-run-background-parent"
	launchChainBackgroundChild = func(configPath string, req backgroundChainExecutionRequest) (*backgroundChainChildHandle, error) {
		if configPath != cfgPath {
			t.Fatalf("configPath = %q, want %q", configPath, cfgPath)
		}
		if req.ChainID != chainID {
			t.Fatalf("req.ChainID = %q, want %q", req.ChainID, chainID)
		}
		if !req.IsNew || req.Resumed {
			t.Fatalf("request = %+v, want new non-resumed launch", req)
		}
		waitCh := make(chan error, 1)
		go func() {
			time.Sleep(25 * time.Millisecond)
			if err := store.LogEvent(context.Background(), chainID, "", chain.EventChainStarted, map[string]any{"orchestrator_pid": 7777, "execution_id": "exec-child", "active_execution": true}); err != nil {
				waitCh <- err
				return
			}
			waitCh <- nil
		}()
		return &backgroundChainChildHandle{wait: waitCh}, nil
	}

	var stdout, stderr bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	if err := runChain(ctx, cfgPath, chainFlags{Task: "launch child", ChainID: chainID, Watch: false}, cmd); err != nil {
		t.Fatalf("runChain returned error: %v", err)
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
	if !ok || exec.OrchestratorPID != 7777 {
		t.Fatalf("LatestActiveExecution() = (%+v, %t), want child pid 7777", exec, ok)
	}
	for _, event := range finalEvents {
		if strings.Contains(event.EventData, `"active_execution":true`) && strings.Contains(event.EventData, fmt.Sprintf(`"orchestrator_pid":%d`, os.Getpid())) {
			t.Fatalf("event data = %s, want parent process to avoid active_execution registration", event.EventData)
		}
	}
}

func TestRunChainWatchInterruptDetachesWithoutClosingBackgroundExecution(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfgPath, projectRoot := writeChainRunConfig(t)
	db := newChainControlTestDB(t)
	store := chain.NewStore(db)

	originalBuildRuntime := buildChainRuntime
	originalLaunch := launchChainBackgroundChild
	defer func() {
		buildChainRuntime = originalBuildRuntime
		launchChainBackgroundChild = originalLaunch
	}()

	buildChainRuntime = func(ctx context.Context, cfg *appconfig.Config) (*orchestratorRuntime, error) {
		if cfg.ProjectRoot != projectRoot {
			t.Fatalf("ProjectRoot = %q, want %q", cfg.ProjectRoot, projectRoot)
		}
		if err := rtpkg.EnsureProjectRecord(ctx, db, cfg); err != nil {
			t.Fatalf("EnsureProjectRecord returned error: %v", err)
		}
		return &orchestratorRuntime{
			Config:              cfg,
			Logger:              slog.Default(),
			ConversationManager: conversation.NewManager(db, nil, slog.Default()),
			ContextAssembler:    rtpkg.NoopContextAssembler{},
			ChainStore:          store,
			Cleanup:             func() {},
		}, nil
	}

	const chainID = "chain-run-detach"
	launchChainBackgroundChild = func(configPath string, req backgroundChainExecutionRequest) (*backgroundChainChildHandle, error) {
		waitCh := make(chan error)
		go func() {
			time.Sleep(25 * time.Millisecond)
			_ = store.LogEvent(context.Background(), chainID, "", chain.EventChainStarted, map[string]any{"orchestrator_pid": 8888, "execution_id": "exec-child", "active_execution": true})
			time.Sleep(25 * time.Millisecond)
			_ = store.LogEvent(context.Background(), chainID, "", chain.EventStepStarted, map[string]any{"role": "planner", "task": "background task"})
		}()
		return &backgroundChainChildHandle{wait: waitCh}, nil
	}

	var stdout, stderr bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	errCh := make(chan error, 1)
	go func() {
		errCh <- runChain(ctx, cfgPath, chainFlags{Task: "detach", ChainID: chainID, Watch: true}, cmd)
	}()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(stderr.String(), "step_started") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(stderr.String(), "step_started") {
		t.Fatalf("stderr = %q, want streamed watch output before detach", stderr.String())
	}
	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("runChain returned error: %v", err)
	}
	if !strings.Contains(stderr.String(), "detached from live output; chain "+chainID+" continues running") {
		t.Fatalf("stderr = %q, want detach message", stderr.String())
	}
	stored, err := store.GetChain(context.Background(), chainID)
	if err != nil {
		t.Fatalf("GetChain returned error: %v", err)
	}
	if stored.Status != "running" {
		t.Fatalf("status = %q, want running after detach", stored.Status)
	}
	finalEvents, err := store.ListEvents(context.Background(), chainID)
	if err != nil {
		t.Fatalf("ListEvents returned error: %v", err)
	}
	exec, ok := chain.LatestActiveExecution(finalEvents)
	if !ok || exec.OrchestratorPID != 8888 {
		t.Fatalf("LatestActiveExecution() = (%+v, %t), want child pid 8888 still active", exec, ok)
	}
}

func TestRunChainBackgroundWorkerRegistersActiveExecutionWithOwnPID(t *testing.T) {
	ctx := context.Background()
	cfgPath, projectRoot := writeChainRunConfig(t)
	db := newChainControlTestDB(t)
	store := chain.NewStore(db)
	const chainID = "chain-worker-registers"
	if _, err := store.StartChain(ctx, chainSpecFromFlags(chainID, chainFlags{Task: "worker task", MaxSteps: 100, MaxResolverLoops: 3, MaxDuration: 4 * time.Hour, TokenBudget: 5_000_000})); err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, "", chain.EventChainStarted, map[string]any{"task": "worker task"}); err != nil {
		t.Fatalf("LogEvent returned error: %v", err)
	}

	originalBuildRuntime := buildChainRuntime
	originalBuildRegistry := buildChainRegistry
	originalNewTurnRunner := newChainTurnRunner
	defer func() {
		buildChainRuntime = originalBuildRuntime
		buildChainRegistry = originalBuildRegistry
		newChainTurnRunner = originalNewTurnRunner
	}()

	buildChainRuntime = func(ctx context.Context, cfg *appconfig.Config) (*orchestratorRuntime, error) {
		if cfg.ProjectRoot != projectRoot {
			t.Fatalf("ProjectRoot = %q, want %q", cfg.ProjectRoot, projectRoot)
		}
		if err := rtpkg.EnsureProjectRecord(ctx, db, cfg); err != nil {
			t.Fatalf("EnsureProjectRecord returned error: %v", err)
		}
		return &orchestratorRuntime{
			Config:              cfg,
			Logger:              slog.Default(),
			ConversationManager: conversation.NewManager(db, nil, slog.Default()),
			ContextAssembler:    rtpkg.NoopContextAssembler{},
			ChainStore:          store,
			Cleanup:             func() {},
		}, nil
	}
	buildChainRegistry = func(rt *orchestratorRuntime, roleCfg appconfig.AgentRoleConfig, chainID string) (*tool.Registry, error) {
		return tool.NewRegistry(), nil
	}
	newChainTurnRunner = func(deps agent.AgentLoopDeps) chainTurnRunner {
		return &blockingChainTurnRunner{run: func(ctx context.Context, req agent.RunTurnRequest) (*agent.TurnResult, error) {
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

	if err := runChainBackgroundWorker(ctx, cfgPath, backgroundChainExecutionRequest{ChainID: chainID, IsNew: true}, cmd); err != nil {
		t.Fatalf("runChainBackgroundWorker returned error: %v", err)
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

func TestRunChainCancelRequestedInterruptionClosesActiveExecution(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfgPath, projectRoot := writeChainRunConfig(t)
	db := newChainControlTestDB(t)
	store := chain.NewStore(db)
	const chainID = "chain-run-cancel"
	if _, err := store.StartChain(ctx, chainSpecFromFlags(chainID, chainFlags{Task: "prove cancellation cleanup", MaxSteps: 100, MaxResolverLoops: 3, MaxDuration: 4 * time.Hour, TokenBudget: 5_000_000})); err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, "", chain.EventChainStarted, map[string]any{"task": "prove cancellation cleanup"}); err != nil {
		t.Fatalf("LogEvent returned error: %v", err)
	}

	originalBuildRuntime := buildChainRuntime
	originalBuildRegistry := buildChainRegistry
	originalNewTurnRunner := newChainTurnRunner
	defer func() {
		buildChainRuntime = originalBuildRuntime
		buildChainRegistry = originalBuildRegistry
		newChainTurnRunner = originalNewTurnRunner
	}()

	buildChainRuntime = func(ctx context.Context, cfg *appconfig.Config) (*orchestratorRuntime, error) {
		if cfg.ProjectRoot != projectRoot {
			t.Fatalf("ProjectRoot = %q, want %q", cfg.ProjectRoot, projectRoot)
		}
		if err := rtpkg.EnsureProjectRecord(ctx, db, cfg); err != nil {
			t.Fatalf("EnsureProjectRecord returned error: %v", err)
		}
		return &orchestratorRuntime{
			Config:              cfg,
			Logger:              slog.Default(),
			ConversationManager: conversation.NewManager(db, nil, slog.Default()),
			ContextAssembler:    rtpkg.NoopContextAssembler{},
			ChainStore:          store,
			Cleanup:             func() {},
		}, nil
	}
	buildChainRegistry = func(rt *orchestratorRuntime, roleCfg appconfig.AgentRoleConfig, chainID string) (*tool.Registry, error) {
		return tool.NewRegistry(), nil
	}

	started := make(chan struct{})
	newChainTurnRunner = func(deps agent.AgentLoopDeps) chainTurnRunner {
		return &blockingChainTurnRunner{
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
		errCh <- runChainBackgroundWorker(ctx, cfgPath, backgroundChainExecutionRequest{ChainID: chainID, IsNew: true}, cmd)
	}()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for blocked chain turn to start")
	}

	events, err := waitForChainActiveExecution(ctx, store, chainID)
	if err != nil {
		t.Fatalf("waitForChainActiveExecution returned error: %v", err)
	}
	activeExec, ok := chain.LatestActiveExecution(events)
	if !ok || activeExec.ExecutionID == "" {
		t.Fatalf("LatestActiveExecution() = (%+v, %t), want populated active execution", activeExec, ok)
	}
	if err := store.SetChainStatus(context.Background(), chainID, "cancel_requested"); err != nil {
		t.Fatalf("SetChainStatus returned error: %v", err)
	}
	cancel()

	if err := <-errCh; err != nil {
		t.Fatalf("runChain returned error: %v", err)
	}
	if !strings.Contains(out.String(), "chain "+chainID+" cancelled") {
		t.Fatalf("output = %q, want cancelled message", out.String())
	}
	stored, err := store.GetChain(context.Background(), chainID)
	if err != nil {
		t.Fatalf("GetChain returned error: %v", err)
	}
	if stored.Status != "cancelled" {
		t.Fatalf("status = %q, want cancelled", stored.Status)
	}
	finalEvents, err := store.ListEvents(context.Background(), chainID)
	if err != nil {
		t.Fatalf("ListEvents returned error: %v", err)
	}
	if exec, ok := chain.LatestActiveExecution(finalEvents); ok || exec.ExecutionID != "" || exec.OrchestratorPID != 0 {
		t.Fatalf("LatestActiveExecution() = (%+v, %t), want empty,false", exec, ok)
	}
	last := finalEvents[len(finalEvents)-1]
	if last.EventType != chain.EventChainCancelled {
		t.Fatalf("last event type = %s, want %s", last.EventType, chain.EventChainCancelled)
	}
	if !strings.Contains(last.EventData, `"execution_id":"`+activeExec.ExecutionID+`"`) {
		t.Fatalf("last event data = %s, want execution_id %s", last.EventData, activeExec.ExecutionID)
	}
}

func waitForChainActiveExecution(ctx context.Context, store *chain.Store, chainID string) ([]chain.Event, error) {
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

func writeChainRunConfig(t *testing.T) (string, string) {
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

func TestFormatChainEventRendersStepOutputCompactly(t *testing.T) {
	event := chain.Event{ID: 42, CreatedAt: time.Date(2026, 4, 21, 1, 2, 3, 0, time.UTC), EventType: chain.EventStepOutput, EventData: `{"stream":"stderr","line":"thinking hard"}`}
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

var _ = fakeChainLoop{}
var _ = appconfig.Config{}
