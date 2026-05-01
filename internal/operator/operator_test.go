//go:build sqlite_fts5
// +build sqlite_fts5

package operator

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ponchione/sodoryard/internal/brain"
	"github.com/ponchione/sodoryard/internal/chain"
	"github.com/ponchione/sodoryard/internal/chainrun"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	appdb "github.com/ponchione/sodoryard/internal/db"
	rtpkg "github.com/ponchione/sodoryard/internal/runtime"
)

type fakeBrainBackend struct {
	docs      map[string]string
	readPaths []string
}

func (f *fakeBrainBackend) ReadDocument(ctx context.Context, path string) (string, error) {
	f.readPaths = append(f.readPaths, path)
	content, ok := f.docs[path]
	if !ok {
		return "", fmt.Errorf("missing document %s", path)
	}
	return content, nil
}

func (f *fakeBrainBackend) WriteDocument(ctx context.Context, path string, content string) error {
	f.docs[path] = content
	return nil
}

func (f *fakeBrainBackend) PatchDocument(ctx context.Context, path string, operation string, content string) error {
	return nil
}

func (f *fakeBrainBackend) SearchKeyword(ctx context.Context, query string) ([]brain.SearchHit, error) {
	return nil, nil
}

func (f *fakeBrainBackend) ListDocuments(ctx context.Context, directory string) ([]string, error) {
	return nil, nil
}

func TestRuntimeStatusCountsActiveChains(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newOperatorTestDB(t))
	if _, err := store.StartChain(ctx, chain.ChainSpec{ChainID: "running-chain", SourceTask: "run"}); err != nil {
		t.Fatalf("StartChain running returned error: %v", err)
	}
	if _, err := store.StartChain(ctx, chain.ChainSpec{ChainID: "paused-chain", SourceTask: "pause"}); err != nil {
		t.Fatalf("StartChain paused returned error: %v", err)
	}
	if err := store.SetChainStatus(ctx, "paused-chain", "paused"); err != nil {
		t.Fatalf("SetChainStatus paused returned error: %v", err)
	}
	if _, err := store.StartChain(ctx, chain.ChainSpec{ChainID: "done-chain", SourceTask: "done"}); err != nil {
		t.Fatalf("StartChain done returned error: %v", err)
	}
	if err := store.CompleteChain(ctx, "done-chain", "completed", "done"); err != nil {
		t.Fatalf("CompleteChain returned error: %v", err)
	}
	projectRoot := t.TempDir()
	svc := openOperatorTestService(t, projectRoot, store, &fakeBrainBackend{}, nil)

	status, err := svc.RuntimeStatus(ctx)
	if err != nil {
		t.Fatalf("RuntimeStatus returned error: %v", err)
	}
	if status.ProjectRoot != projectRoot {
		t.Fatalf("ProjectRoot = %q, want %q", status.ProjectRoot, projectRoot)
	}
	if status.ProjectName != filepath.Base(projectRoot) {
		t.Fatalf("ProjectName = %q, want %q", status.ProjectName, filepath.Base(projectRoot))
	}
	if status.Provider != "codex" || status.Model != "test-model" {
		t.Fatalf("provider/model = %q/%q, want codex/test-model", status.Provider, status.Model)
	}
	if status.ActiveChains != 2 {
		t.Fatalf("ActiveChains = %d, want 2", status.ActiveChains)
	}
}

func TestOpenReadOnlySkipsInjectedRuntimeBuilder(t *testing.T) {
	ctx := context.Background()
	projectRoot := t.TempDir()
	configPath := writeOperatorTestConfig(t, projectRoot)

	svc, err := Open(ctx, Options{
		ConfigPath: configPath,
		ReadOnly:   true,
		BuildRuntime: func(context.Context, *appconfig.Config) (*rtpkg.OrchestratorRuntime, error) {
			t.Fatal("BuildRuntime should not be called in read-only mode")
			return nil, nil
		},
	})
	if err != nil {
		t.Fatalf("Open read-only returned error: %v", err)
	}
	t.Cleanup(svc.Close)

	status, err := svc.RuntimeStatus(ctx)
	if err != nil {
		t.Fatalf("RuntimeStatus returned error: %v", err)
	}
	if status.ProjectRoot != projectRoot || status.Provider != "codex" || status.Model != "test-model" {
		t.Fatalf("RuntimeStatus = %+v, want config-derived status", status)
	}
}

func TestListChainsAndDetail(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newOperatorTestDB(t))
	chainID, err := store.StartChain(ctx, chain.ChainSpec{ChainID: "chain-list", SourceSpecs: []string{"specs/plan.md"}, SourceTask: "build it"})
	if err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	stepOne, err := store.StartStep(ctx, chain.StepSpec{ChainID: chainID, SequenceNum: 1, Role: "planner", Task: "plan"})
	if err != nil {
		t.Fatalf("StartStep one returned error: %v", err)
	}
	if err := store.CompleteStep(ctx, chain.CompleteStepParams{StepID: stepOne, Status: "completed", Verdict: "accepted", ReceiptPath: "receipts/planner/chain-list-step-001.md", TokensUsed: 50}); err != nil {
		t.Fatalf("CompleteStep returned error: %v", err)
	}
	stepTwo, err := store.StartStep(ctx, chain.StepSpec{ChainID: chainID, SequenceNum: 2, Role: "coder", Task: "code"})
	if err != nil {
		t.Fatalf("StartStep two returned error: %v", err)
	}
	if err := store.StepRunning(ctx, stepTwo); err != nil {
		t.Fatalf("StepRunning returned error: %v", err)
	}
	if err := store.UpdateChainMetrics(ctx, chainID, chain.ChainMetrics{TotalSteps: 2, TotalTokens: 123}); err != nil {
		t.Fatalf("UpdateChainMetrics returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, stepTwo, chain.EventStepStarted, map[string]any{"role": "coder"}); err != nil {
		t.Fatalf("LogEvent returned error: %v", err)
	}
	svc := openOperatorTestService(t, t.TempDir(), store, &fakeBrainBackend{}, nil)

	summaries, err := svc.ListChains(ctx, 10)
	if err != nil {
		t.Fatalf("ListChains returned error: %v", err)
	}
	summary := requireSummary(t, summaries, chainID)
	if summary.SourceTask != "build it" || !reflect.DeepEqual(summary.SourceSpecs, []string{"specs/plan.md"}) {
		t.Fatalf("summary source = task %q specs %v, want build it/specs", summary.SourceTask, summary.SourceSpecs)
	}
	if summary.TotalSteps != 2 || summary.TotalTokens != 123 {
		t.Fatalf("summary metrics = steps %d tokens %d, want 2/123", summary.TotalSteps, summary.TotalTokens)
	}
	if summary.CurrentStep == nil || summary.CurrentStep.SequenceNum != 2 || summary.CurrentStep.Role != "coder" || summary.CurrentStep.Status != "running" {
		t.Fatalf("CurrentStep = %+v, want running coder step 2", summary.CurrentStep)
	}

	detail, err := svc.GetChainDetail(ctx, chainID)
	if err != nil {
		t.Fatalf("GetChainDetail returned error: %v", err)
	}
	if detail.Chain.ID != chainID || len(detail.Steps) != 2 || len(detail.RecentEvents) != 1 {
		t.Fatalf("detail = %+v, want chain, 2 steps, 1 event", detail)
	}
}

func TestListEventsAndEventsSince(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newOperatorTestDB(t))
	chainID, err := store.StartChain(ctx, chain.ChainSpec{ChainID: "chain-events", SourceTask: "events"})
	if err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, "", chain.EventChainStarted, map[string]any{"task": "events"}); err != nil {
		t.Fatalf("LogEvent start returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, "", chain.EventResolverLoop, map[string]any{"count": 1}); err != nil {
		t.Fatalf("LogEvent loop returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, "", chain.EventChainCompleted, map[string]any{"status": "completed"}); err != nil {
		t.Fatalf("LogEvent complete returned error: %v", err)
	}
	svc := openOperatorTestService(t, t.TempDir(), store, &fakeBrainBackend{}, nil)

	events, err := svc.ListEvents(ctx, chainID)
	if err != nil {
		t.Fatalf("ListEvents returned error: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("event count = %d, want 3", len(events))
	}
	since, err := svc.ListEventsSince(ctx, chainID, events[0].ID)
	if err != nil {
		t.Fatalf("ListEventsSince returned error: %v", err)
	}
	if len(since) != 2 || since[0].EventType != chain.EventResolverLoop || since[1].EventType != chain.EventChainCompleted {
		t.Fatalf("events since = %+v, want resolver loop and completion", since)
	}
}

func TestReadReceiptResolvesOrchestratorAndStepPaths(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newOperatorTestDB(t))
	chainID, err := store.StartChain(ctx, chain.ChainSpec{ChainID: "chain-receipts", SourceTask: "receipts"})
	if err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	stepOne, err := store.StartStep(ctx, chain.StepSpec{ChainID: chainID, SequenceNum: 1, Role: "planner", Task: "plan"})
	if err != nil {
		t.Fatalf("StartStep one returned error: %v", err)
	}
	if err := store.CompleteStep(ctx, chain.CompleteStepParams{StepID: stepOne, Status: "completed", ReceiptPath: "receipts/planner/chain-receipts-step-001.md"}); err != nil {
		t.Fatalf("CompleteStep one returned error: %v", err)
	}
	stepTwo, err := store.StartStep(ctx, chain.StepSpec{ChainID: chainID, SequenceNum: 2, Role: "coder", Task: "code"})
	if err != nil {
		t.Fatalf("StartStep two returned error: %v", err)
	}
	if err := store.CompleteStep(ctx, chain.CompleteStepParams{StepID: stepTwo, Status: "completed", ReceiptPath: "receipts/coder/chain-receipts-step-002.md"}); err != nil {
		t.Fatalf("CompleteStep two returned error: %v", err)
	}
	backend := &fakeBrainBackend{docs: map[string]string{
		"receipts/orchestrator/chain-receipts.md":     "orchestrator receipt",
		"receipts/coder/chain-receipts-step-002.md":   "step 2 receipt",
		"receipts/planner/chain-receipts-step-001.md": "step 1 receipt",
	}}
	svc := openOperatorTestService(t, t.TempDir(), store, backend, nil)

	orchestrator, err := svc.ReadReceipt(ctx, chainID, "")
	if err != nil {
		t.Fatalf("ReadReceipt orchestrator returned error: %v", err)
	}
	if orchestrator.Path != "receipts/orchestrator/chain-receipts.md" || orchestrator.Content != "orchestrator receipt" {
		t.Fatalf("orchestrator receipt = %+v, want default path/content", orchestrator)
	}
	stepReceipt, err := svc.ReadReceipt(ctx, chainID, "2")
	if err != nil {
		t.Fatalf("ReadReceipt step returned error: %v", err)
	}
	if stepReceipt.Path != "receipts/coder/chain-receipts-step-002.md" || stepReceipt.Content != "step 2 receipt" {
		t.Fatalf("step receipt = %+v, want step path/content", stepReceipt)
	}
	fallback, err := svc.ReadReceipt(ctx, chainID, "99")
	if err != nil {
		t.Fatalf("ReadReceipt fallback returned error: %v", err)
	}
	if fallback.Path != "receipts/orchestrator/chain-receipts.md" || fallback.Content != "orchestrator receipt" {
		t.Fatalf("fallback receipt = %+v, want orchestrator path/content", fallback)
	}
}

func TestListAgentRolesAndValidateLaunch(t *testing.T) {
	ctx := context.Background()
	svc := openOperatorTestService(t, t.TempDir(), chain.NewStore(newOperatorTestDB(t)), &fakeBrainBackend{}, nil)

	roles, err := svc.ListAgentRoles(ctx)
	if err != nil {
		t.Fatalf("ListAgentRoles returned error: %v", err)
	}
	if !reflect.DeepEqual(roles, []AgentRoleSummary{{Name: "coder"}, {Name: "orchestrator"}}) {
		t.Fatalf("roles = %+v, want sorted coder/orchestrator", roles)
	}

	preview, err := svc.ValidateLaunch(ctx, LaunchRequest{Mode: LaunchModeOneStep, Role: "coder", SourceTask: "fix tests"})
	if err != nil {
		t.Fatalf("ValidateLaunch one-step returned error: %v", err)
	}
	if preview.Mode != LaunchModeOneStep || preview.Role != "coder" || preview.Summary != "Run one coder step" || preview.CompiledTask != "fix tests" {
		t.Fatalf("one-step preview = %+v, want coder preview", preview)
	}
	if len(preview.Warnings) != 1 || preview.Warnings[0].Message != "no source specs selected" {
		t.Fatalf("warnings = %+v, want no source specs warning", preview.Warnings)
	}

	orchestrator, err := svc.ValidateLaunch(ctx, LaunchRequest{Mode: LaunchModeOrchestrator, SourceSpecs: []string{" specs/a.md ", "specs/a.md"}})
	if err != nil {
		t.Fatalf("ValidateLaunch orchestrator returned error: %v", err)
	}
	if orchestrator.Mode != LaunchModeOrchestrator || orchestrator.Role != "orchestrator" || orchestrator.CompiledTask != "Specs: specs/a.md" {
		t.Fatalf("orchestrator preview = %+v, want normalized spec preview", orchestrator)
	}
}

func TestValidateLaunchRejectsMissingInputsAndUnknownRole(t *testing.T) {
	ctx := context.Background()
	svc := openOperatorTestService(t, t.TempDir(), chain.NewStore(newOperatorTestDB(t)), &fakeBrainBackend{}, nil)

	if _, err := svc.ValidateLaunch(ctx, LaunchRequest{Mode: LaunchModeOneStep, Role: "coder"}); err == nil || !strings.Contains(err.Error(), "one of task or specs is required") {
		t.Fatalf("ValidateLaunch missing inputs error = %v, want missing task/specs", err)
	}
	if _, err := svc.ValidateLaunch(ctx, LaunchRequest{Mode: LaunchModeOneStep, Role: "missing", SourceTask: "fix"}); err == nil || !strings.Contains(err.Error(), "resolve launch role") {
		t.Fatalf("ValidateLaunch unknown role error = %v, want role resolution error", err)
	}
}

func TestStartChainMapsLaunchRequestToChainrun(t *testing.T) {
	ctx := context.Background()
	projectRoot := t.TempDir()
	configPath := writeOperatorTestConfig(t, projectRoot)
	var gotCfg *appconfig.Config
	var gotOpts chainrun.Options
	var gotDeps chainrun.Deps
	svc, err := Open(ctx, Options{
		ConfigPath: configPath,
		ReadOnly:   true,
		ChainStarter: func(ctx context.Context, cfg *appconfig.Config, opts chainrun.Options, deps chainrun.Deps) (*chainrun.Result, error) {
			gotCfg = cfg
			gotOpts = opts
			gotDeps = deps
			opts.OnChainID("chain-launched")
			return &chainrun.Result{ChainID: "chain-launched", Status: "completed"}, nil
		},
		ProcessID: func() int { return 1234 },
		BuildRuntime: func(context.Context, *appconfig.Config) (*rtpkg.OrchestratorRuntime, error) {
			t.Fatal("BuildRuntime should not be called before ChainStarter uses it")
			return nil, nil
		},
	})
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(svc.Close)

	result, err := svc.StartChain(ctx, LaunchRequest{Mode: LaunchModeOneStep, Role: "coder", SourceTask: "ship it"})
	if err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	if result.ChainID != "chain-launched" || result.Status != "running" || result.Preview.Role != "coder" {
		t.Fatalf("result = %+v, want immediate running chain-launched coder result", result)
	}
	if gotCfg == nil || gotCfg.ProjectRoot != projectRoot {
		t.Fatalf("got cfg = %+v, want project root %s", gotCfg, projectRoot)
	}
	if gotOpts.Mode != chainrun.ModeOneStep || gotOpts.Role != "coder" || gotOpts.SourceTask != "ship it" {
		t.Fatalf("chainrun opts = %+v, want one-step coder task", gotOpts)
	}
	if gotOpts.MaxSteps != 100 || gotOpts.MaxResolverLoops != 3 || gotOpts.MaxDuration != 4*time.Hour || gotOpts.TokenBudget != 5_000_000 {
		t.Fatalf("chainrun defaults = steps %d loops %d duration %s budget %d", gotOpts.MaxSteps, gotOpts.MaxResolverLoops, gotOpts.MaxDuration, gotOpts.TokenBudget)
	}
	if gotDeps.ProcessID == nil || gotDeps.ProcessID() != 1234 {
		t.Fatalf("ProcessID dependency not propagated")
	}
}

func TestPauseResumeCancelStateTransitions(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newOperatorTestDB(t))
	if _, err := store.StartChain(ctx, chain.ChainSpec{ChainID: "pause-chain", SourceTask: "pause"}); err != nil {
		t.Fatalf("StartChain pause returned error: %v", err)
	}
	if _, err := store.StartChain(ctx, chain.ChainSpec{ChainID: "resume-chain", SourceTask: "resume"}); err != nil {
		t.Fatalf("StartChain resume returned error: %v", err)
	}
	if err := store.SetChainStatus(ctx, "resume-chain", "paused"); err != nil {
		t.Fatalf("SetChainStatus resume returned error: %v", err)
	}
	if _, err := store.StartChain(ctx, chain.ChainSpec{ChainID: "cancel-chain", SourceTask: "cancel"}); err != nil {
		t.Fatalf("StartChain cancel returned error: %v", err)
	}
	if err := store.SetChainStatus(ctx, "cancel-chain", "paused"); err != nil {
		t.Fatalf("SetChainStatus cancel returned error: %v", err)
	}
	svc := openOperatorTestService(t, t.TempDir(), store, &fakeBrainBackend{}, nil)

	paused, err := svc.PauseChain(ctx, "pause-chain")
	if err != nil {
		t.Fatalf("PauseChain returned error: %v", err)
	}
	requireChainStatus(t, ctx, store, "pause-chain", "pause_requested")
	if paused.PreviousStatus != "running" || paused.Status != "pause_requested" || paused.Message != "pause requested" {
		t.Fatalf("PauseChain result = %+v, want running -> pause_requested", paused)
	}

	resumed, err := svc.ResumeChain(ctx, "resume-chain")
	if err != nil {
		t.Fatalf("ResumeChain returned error: %v", err)
	}
	requireChainStatus(t, ctx, store, "resume-chain", "running")
	if resumed.PreviousStatus != "paused" || resumed.Status != "running" || resumed.EventType != chain.EventChainResumed {
		t.Fatalf("ResumeChain result = %+v, want paused -> running", resumed)
	}

	cancelled, err := svc.CancelChain(ctx, "cancel-chain")
	if err != nil {
		t.Fatalf("CancelChain returned error: %v", err)
	}
	requireChainStatus(t, ctx, store, "cancel-chain", "cancelled")
	if cancelled.PreviousStatus != "paused" || cancelled.Status != "cancelled" || cancelled.EventType != chain.EventChainCancelled {
		t.Fatalf("CancelChain result = %+v, want paused -> cancelled", cancelled)
	}
}

func TestCancelChainSignalsInjectedActiveProcesses(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newOperatorTestDB(t))
	chainID, err := store.StartChain(ctx, chain.ChainSpec{ChainID: "signal-chain", SourceTask: "cancel"})
	if err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	stepID, err := store.StartStep(ctx, chain.StepSpec{ChainID: chainID, SequenceNum: 1, Role: "coder", Task: "code"})
	if err != nil {
		t.Fatalf("StartStep returned error: %v", err)
	}
	if err := store.StepRunning(ctx, stepID); err != nil {
		t.Fatalf("StepRunning returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, "", chain.EventChainStarted, map[string]any{"orchestrator_pid": 1111, "execution_id": "exec-1", "active_execution": true}); err != nil {
		t.Fatalf("LogEvent chain start returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, stepID, chain.EventStepProcessStarted, map[string]any{"process_id": 2222, "active_process": true}); err != nil {
		t.Fatalf("LogEvent step process returned error: %v", err)
	}
	var signaled []int
	signaler := func(pid int) error {
		signaled = append(signaled, pid)
		if pid == 2222 {
			return ErrProcessNotRunning
		}
		return nil
	}
	svc := openOperatorTestService(t, t.TempDir(), store, &fakeBrainBackend{}, signaler)

	result, err := svc.CancelChain(ctx, chainID)
	if err != nil {
		t.Fatalf("CancelChain returned error: %v", err)
	}
	requireChainStatus(t, ctx, store, chainID, "cancel_requested")
	if !reflect.DeepEqual(signaled, []int{2222, 1111}) {
		t.Fatalf("signaled pids = %v, want step then orchestrator", signaled)
	}
	if !reflect.DeepEqual(result.SignaledPIDs, []int{2222, 1111}) || len(result.Warnings) != 0 {
		t.Fatalf("CancelChain result = %+v, want signaled pids and no warnings for ErrProcessNotRunning", result)
	}
}

func openOperatorTestService(t *testing.T, projectRoot string, store *chain.Store, backend *fakeBrainBackend, signaler func(int) error) *Service {
	t.Helper()
	configPath := writeOperatorTestConfig(t, projectRoot)
	if backend == nil {
		backend = &fakeBrainBackend{docs: map[string]string{}}
	}
	svc, err := Open(context.Background(), Options{
		ConfigPath: configPath,
		BuildRuntime: func(ctx context.Context, cfg *appconfig.Config) (*rtpkg.OrchestratorRuntime, error) {
			if cfg.ProjectRoot != projectRoot {
				t.Fatalf("ProjectRoot = %q, want %q", cfg.ProjectRoot, projectRoot)
			}
			return &rtpkg.OrchestratorRuntime{
				Config:       cfg,
				ChainStore:   store,
				BrainBackend: backend,
				Cleanup:      func() {},
			}, nil
		},
		ProcessSignaler: signaler,
	})
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(svc.Close)
	return svc
}

func writeOperatorTestConfig(t *testing.T, projectRoot string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "yard.yaml")
	content := fmt.Sprintf(`project_root: %q
brain:
  enabled: false
local_services:
  enabled: false
routing:
  default:
    provider: codex
    model: test-model
providers:
  codex:
    type: codex
    model: test-model
agent_roles:
  coder:
    system_prompt: prompts/coder.md
  orchestrator:
    system_prompt: prompts/orchestrator.md
`, projectRoot)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func newOperatorTestDB(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "operator.db")
	db, err := appdb.OpenDB(ctx, path)
	if err != nil {
		t.Fatalf("OpenDB returned error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := appdb.InitIfNeeded(ctx, db); err != nil {
		t.Fatalf("InitIfNeeded returned error: %v", err)
	}
	if err := appdb.EnsureChainSchema(ctx, db); err != nil {
		t.Fatalf("EnsureChainSchema returned error: %v", err)
	}
	return db
}

func requireSummary(t *testing.T, summaries []ChainSummary, chainID string) ChainSummary {
	t.Helper()
	for _, summary := range summaries {
		if summary.ID == chainID {
			return summary
		}
	}
	t.Fatalf("summary for chain %q not found in %+v", chainID, summaries)
	return ChainSummary{}
}

func requireChainStatus(t *testing.T, ctx context.Context, store *chain.Store, chainID string, want string) {
	t.Helper()
	ch, err := store.GetChain(ctx, chainID)
	if err != nil {
		t.Fatalf("GetChain returned error: %v", err)
	}
	if ch.Status != want {
		t.Fatalf("chain %s status = %q, want %q", chainID, ch.Status, want)
	}
}
