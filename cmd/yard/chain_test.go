package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/ponchione/sodoryard/internal/agent"
	"github.com/ponchione/sodoryard/internal/brain"
	"github.com/ponchione/sodoryard/internal/chain"
	"github.com/ponchione/sodoryard/internal/chainrun"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/conversation"
	"github.com/ponchione/sodoryard/internal/operator"
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

type yardChainTestBrainBackend struct {
	docs map[string]string
}

func (b *yardChainTestBrainBackend) ReadDocument(ctx context.Context, path string) (string, error) {
	content, ok := b.docs[path]
	if !ok {
		return "", fmt.Errorf("missing document %s", path)
	}
	return content, nil
}

func (b *yardChainTestBrainBackend) WriteDocument(ctx context.Context, path string, content string) error {
	b.docs[path] = content
	return nil
}

func (b *yardChainTestBrainBackend) PatchDocument(ctx context.Context, path string, operation string, content string) error {
	return nil
}

func (b *yardChainTestBrainBackend) SearchKeyword(ctx context.Context, query string) ([]brain.SearchHit, error) {
	return nil, nil
}

func (b *yardChainTestBrainBackend) ListDocuments(ctx context.Context, directory string) ([]string, error) {
	return nil, nil
}

func withYardOperatorTestRuntime(t *testing.T, projectRoot string, store *chain.Store, backend brain.Backend) {
	t.Helper()
	originalBuildRuntime := buildYardChainRuntime
	originalReadOnlyOperator := openYardReadOnlyOperator
	buildYardChainRuntime = func(ctx context.Context, cfg *appconfig.Config) (*rtpkg.OrchestratorRuntime, error) {
		if cfg.ProjectRoot != projectRoot {
			t.Fatalf("ProjectRoot = %q, want %q", cfg.ProjectRoot, projectRoot)
		}
		return &rtpkg.OrchestratorRuntime{
			Config:       cfg,
			ChainStore:   store,
			BrainBackend: backend,
			Cleanup:      func() {},
		}, nil
	}
	openYardReadOnlyOperator = func(ctx context.Context, configPath string) (*operator.Service, error) {
		return operator.Open(ctx, operator.Options{
			ConfigPath:      configPath,
			BuildRuntime:    buildYardChainRuntime,
			ProcessSignaler: signalYardOperatorProcess,
		})
	}
	t.Cleanup(func() {
		buildYardChainRuntime = originalBuildRuntime
		openYardReadOnlyOperator = originalReadOnlyOperator
	})
}

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
	if flag := cmd.Flags().Lookup("role"); flag == nil {
		t.Fatal("expected role flag")
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

func TestYardChainReceiptCommandPrintsStepReceipt(t *testing.T) {
	ctx := context.Background()
	cfgPath, projectRoot := writeYardRunConfig(t)
	store := chain.NewStore(newYardChainControlTestDB(t))
	chainID, err := store.StartChain(ctx, chain.ChainSpec{ChainID: "chain-receipt", SourceTask: "receipt"})
	if err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	stepID, err := store.StartStep(ctx, chain.StepSpec{ChainID: chainID, SequenceNum: 2, Role: "coder", Task: "code"})
	if err != nil {
		t.Fatalf("StartStep returned error: %v", err)
	}
	if err := store.CompleteStep(ctx, chain.CompleteStepParams{StepID: stepID, Status: "completed", ReceiptPath: "receipts/coder/chain-receipt-step-002.md"}); err != nil {
		t.Fatalf("CompleteStep returned error: %v", err)
	}
	withYardOperatorTestRuntime(t, projectRoot, store, &yardChainTestBrainBackend{docs: map[string]string{
		"receipts/orchestrator/chain-receipt.md":   "orchestrator receipt",
		"receipts/coder/chain-receipt-step-002.md": "step receipt",
	}})

	var out bytes.Buffer
	cmd := newYardChainReceiptCmd(&cfgPath)
	cmd.SetContext(ctx)
	cmd.SetOut(&out)
	cmd.SetArgs([]string{chainID, "2"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if out.String() != "step receipt" {
		t.Fatalf("stdout = %q, want step receipt", out.String())
	}
}

func TestYardChainStatusCommandPrintsExistingFormats(t *testing.T) {
	ctx := context.Background()
	cfgPath, projectRoot := writeYardRunConfig(t)
	store := chain.NewStore(newYardChainControlTestDB(t))
	chainID, err := store.StartChain(ctx, chain.ChainSpec{ChainID: "chain-status", SourceTask: "status"})
	if err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	stepID, err := store.StartStep(ctx, chain.StepSpec{ChainID: chainID, SequenceNum: 1, Role: "planner", Task: "plan"})
	if err != nil {
		t.Fatalf("StartStep returned error: %v", err)
	}
	if err := store.CompleteStep(ctx, chain.CompleteStepParams{StepID: stepID, Status: "completed", Verdict: "accepted", ReceiptPath: "receipts/planner/chain-status-step-001.md", TokensUsed: 40}); err != nil {
		t.Fatalf("CompleteStep returned error: %v", err)
	}
	if err := store.UpdateChainMetrics(ctx, chainID, chain.ChainMetrics{TotalSteps: 1, TotalTokens: 40}); err != nil {
		t.Fatalf("UpdateChainMetrics returned error: %v", err)
	}
	withYardOperatorTestRuntime(t, projectRoot, store, &yardChainTestBrainBackend{docs: map[string]string{}})

	var listOut bytes.Buffer
	listCmd := newYardChainStatusCmd(&cfgPath)
	listCmd.SetContext(ctx)
	listCmd.SetOut(&listOut)
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list Execute returned error: %v", err)
	}
	if listOut.String() != "chain-status\trunning\tsteps=1\ttokens=40\n" {
		t.Fatalf("list stdout = %q, want existing list format", listOut.String())
	}

	var detailOut bytes.Buffer
	detailCmd := newYardChainStatusCmd(&cfgPath)
	detailCmd.SetContext(ctx)
	detailCmd.SetOut(&detailOut)
	detailCmd.SetArgs([]string{chainID})
	want := "chain=chain-status status=running steps=1 tokens=40 duration=0 summary=\n" +
		"step=1 role=planner status=completed verdict=accepted receipt=receipts/planner/chain-status-step-001.md\n"
	if err := detailCmd.Execute(); err != nil {
		t.Fatalf("detail Execute returned error: %v", err)
	}
	if detailOut.String() != want {
		t.Fatalf("detail stdout = %q, want %q", detailOut.String(), want)
	}
}

func TestYardChainLogsCommandPrintsRenderedOperatorEvents(t *testing.T) {
	ctx := context.Background()
	cfgPath, projectRoot := writeYardRunConfig(t)
	store := chain.NewStore(newYardChainControlTestDB(t))
	chainID, err := store.StartChain(ctx, chain.ChainSpec{ChainID: "chain-logs", SourceTask: "logs"})
	if err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, "", chain.EventStepStarted, map[string]any{"role": "coder", "task": "fix logs", "receipt_path": "receipts/coder/chain-logs-step-001.md"}); err != nil {
		t.Fatalf("LogEvent returned error: %v", err)
	}
	events, err := store.ListEvents(ctx, chainID)
	if err != nil {
		t.Fatalf("ListEvents returned error: %v", err)
	}
	withYardOperatorTestRuntime(t, projectRoot, store, &yardChainTestBrainBackend{docs: map[string]string{}})

	var out bytes.Buffer
	cmd := newYardChainLogsCmd(&cfgPath)
	cmd.SetContext(ctx)
	cmd.SetOut(&out)
	cmd.SetArgs([]string{chainID})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	want := formatChainEvent(events[0], chainRenderOptions{Verbosity: chainVerbosityNormal})
	if out.String() != want {
		t.Fatalf("stdout = %q, want %q", out.String(), want)
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
	err := <-errCh
	var exitErr chainrun.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("yardRunChain error = %T %[1]v, want chainrun.ExitError", err)
	}
	if exitErr.ExitCode() != 4 {
		t.Fatalf("exit code = %d, want 4", exitErr.ExitCode())
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
