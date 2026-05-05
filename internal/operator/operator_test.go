//go:build sqlite_fts5
// +build sqlite_fts5

package operator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ponchione/sodoryard/internal/brain"
	brainindexstate "github.com/ponchione/sodoryard/internal/brain/indexstate"
	"github.com/ponchione/sodoryard/internal/chain"
	"github.com/ponchione/sodoryard/internal/chainrun"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	appdb "github.com/ponchione/sodoryard/internal/db"
	"github.com/ponchione/sodoryard/internal/projectmemory"
	"github.com/ponchione/sodoryard/internal/provider"
	"github.com/ponchione/sodoryard/internal/provider/router"
	rtpkg "github.com/ponchione/sodoryard/internal/runtime"
)

type fakeBrainBackend struct {
	docs      map[string]string
	readPaths []string
}

type fakeAuthProvider struct {
	status *provider.AuthStatus
}

func (f fakeAuthProvider) Name() string { return "codex" }

func (f fakeAuthProvider) Complete(context.Context, *provider.Request) (*provider.Response, error) {
	return nil, fmt.Errorf("not implemented")
}

func (f fakeAuthProvider) Stream(context.Context, *provider.Request) (<-chan provider.StreamEvent, error) {
	return nil, fmt.Errorf("not implemented")
}

func (f fakeAuthProvider) Models(context.Context) ([]provider.Model, error) {
	return []provider.Model{{ID: "test-model", Provider: "codex"}}, nil
}

func (f fakeAuthProvider) AuthStatus(context.Context) (*provider.AuthStatus, error) {
	return f.status, nil
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

type fakeShunterIndexBrainBackend struct {
	fakeBrainBackend
	brainState projectmemory.BrainIndexState
	brainFound bool
	codeState  projectmemory.CodeIndexState
	codeFound  bool
	err        error
}

func (f *fakeShunterIndexBrainBackend) ReadBrainIndexState(context.Context) (projectmemory.BrainIndexState, bool, error) {
	return f.brainState, f.brainFound, f.err
}

func (f *fakeShunterIndexBrainBackend) ReadCodeIndexState(context.Context) (projectmemory.CodeIndexState, bool, error) {
	return f.codeState, f.codeFound, f.err
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

func TestRuntimeStatusReadsShunterBrainIndexState(t *testing.T) {
	ctx := context.Background()
	projectRoot := t.TempDir()
	db := newOperatorTestDB(t)
	cfg := appconfig.Default()
	cfg.ProjectRoot = projectRoot
	cfg.Brain.Enabled = true
	cfg.Brain.Backend = "shunter"
	cfg.Memory.Backend = "shunter"
	cfg.Routing.Default.Model = "test-model"
	store := chain.NewStore(db)
	staleAt := time.Date(2026, 5, 1, 12, 30, 0, 0, time.UTC)
	backend := &fakeShunterIndexBrainBackend{
		brainState: projectmemory.BrainIndexState{
			LastIndexedAtUS: uint64(time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC).UnixMicro()),
			Dirty:           true,
			DirtySinceUS:    uint64(staleAt.UnixMicro()),
			DirtyReason:     "write_document",
		},
		brainFound: true,
		codeState: projectmemory.CodeIndexState{
			LastIndexedCommit: "abc123",
			LastIndexedAtUS:   uint64(time.Date(2026, 5, 1, 11, 0, 0, 0, time.UTC).UnixMicro()),
		},
		codeFound: true,
	}
	svc, err := NewForRuntime(&rtpkg.OrchestratorRuntime{
		Config:       cfg,
		Database:     db,
		ChainStore:   store,
		BrainBackend: backend,
		Cleanup:      func() {},
	}, Options{})
	if err != nil {
		t.Fatalf("NewForRuntime returned error: %v", err)
	}
	t.Cleanup(svc.Close)

	status, err := svc.RuntimeStatus(ctx)
	if err != nil {
		t.Fatalf("RuntimeStatus returned error: %v", err)
	}
	if status.BrainIndex.Status != brainindexstate.StatusStale || status.BrainIndex.StaleSince != staleAt.Format(time.RFC3339) || status.BrainIndex.StaleReason != "write_document" {
		t.Fatalf("BrainIndex = %+v, want Shunter stale write_document state", status.BrainIndex)
	}
	if status.CodeIndex.Status != "indexed" || status.CodeIndex.LastIndexedCommit != "abc123" || status.CodeIndex.LastIndexedAt == "" {
		t.Fatalf("CodeIndex = %+v, want Shunter indexed abc123 state", status.CodeIndex)
	}
	if _, err := os.Stat(brainindexstate.Path(projectRoot)); !os.IsNotExist(err) {
		t.Fatalf("brain index state file stat err = %v, want not-exist", err)
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
	if _, err := os.Stat(filepath.Join(projectRoot, ".yard")); !os.IsNotExist(err) {
		t.Fatalf("read-only Open created .yard state: stat err=%v", err)
	}
}

func TestRuntimeStatusIncludesReadinessMetadata(t *testing.T) {
	ctx := context.Background()
	projectRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectRoot, ".brain"), 0o755); err != nil {
		t.Fatalf("create brain dir: %v", err)
	}
	configPath := filepath.Join(t.TempDir(), "yard.yaml")
	config := fmt.Sprintf(`project_root: %q
brain:
  enabled: true
  vault_path: ".brain"
local_services:
  enabled: true
  mode: manual
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
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	db := newOperatorTestDB(t)
	indexedAt := "2026-05-01T12:00:00Z"
	if _, err := db.ExecContext(ctx, `INSERT INTO projects(id, name, root_path, last_indexed_commit, last_indexed_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, projectRoot, filepath.Base(projectRoot), projectRoot, "abc123", indexedAt, indexedAt, indexedAt); err != nil {
		t.Fatalf("insert project metadata: %v", err)
	}
	staleAt := time.Date(2026, 5, 1, 12, 30, 0, 0, time.UTC)
	if err := brainindexstate.MarkStale(projectRoot, "brain_update", staleAt); err != nil {
		t.Fatalf("mark brain index stale: %v", err)
	}
	store := chain.NewStore(db)
	providerRouter := newOperatorTestRouter(t, &provider.AuthStatus{
		Provider:       "codex",
		Mode:           "oauth",
		Source:         "private_store",
		HasAccessToken: true,
	})
	svc, err := Open(ctx, Options{
		ConfigPath: configPath,
		BuildRuntime: func(ctx context.Context, cfg *appconfig.Config) (*rtpkg.OrchestratorRuntime, error) {
			return &rtpkg.OrchestratorRuntime{
				Config:         cfg,
				Database:       db,
				ProviderRouter: providerRouter,
				ChainStore:     store,
				BrainBackend:   &fakeBrainBackend{},
				Cleanup:        func() {},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(svc.Close)

	status, err := svc.RuntimeStatus(ctx)
	if err != nil {
		t.Fatalf("RuntimeStatus returned error: %v", err)
	}
	if status.AuthStatus != "ready (oauth, private_store)" {
		t.Fatalf("AuthStatus = %q, want ready auth detail", status.AuthStatus)
	}
	if status.CodeIndex.Status != "indexed" || status.CodeIndex.LastIndexedAt != indexedAt || status.CodeIndex.LastIndexedCommit != "abc123" {
		t.Fatalf("CodeIndex = %+v, want indexed metadata", status.CodeIndex)
	}
	if status.BrainIndex.Status != brainindexstate.StatusStale || status.BrainIndex.StaleSince != staleAt.Format(time.RFC3339) || status.BrainIndex.StaleReason != "brain_update" {
		t.Fatalf("BrainIndex = %+v, want stale brain_update metadata", status.BrainIndex)
	}
	if status.LocalServicesStatus != "manual" {
		t.Fatalf("LocalServicesStatus = %q, want manual", status.LocalServicesStatus)
	}
}

func TestRuntimeStatusIncludesStartupWarnings(t *testing.T) {
	ctx := context.Background()
	projectRoot := t.TempDir()
	configPath := writeOperatorTestConfig(t, projectRoot)
	db := newOperatorTestDB(t)
	store := chain.NewStore(db)
	svc, err := Open(ctx, Options{
		ConfigPath:      configPath,
		StartupWarnings: []RuntimeWarning{{Message: "opened operator in degraded read-only mode"}},
		BuildRuntime: func(ctx context.Context, cfg *appconfig.Config) (*rtpkg.OrchestratorRuntime, error) {
			return &rtpkg.OrchestratorRuntime{
				Config:       cfg,
				Database:     db,
				ChainStore:   store,
				BrainBackend: &fakeBrainBackend{},
				Cleanup:      func() {},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(svc.Close)

	status, err := svc.RuntimeStatus(ctx)
	if err != nil {
		t.Fatalf("RuntimeStatus returned error: %v", err)
	}
	if !hasRuntimeWarning(status.Warnings, "opened operator in degraded read-only mode") {
		t.Fatalf("Warnings = %+v, want startup warning", status.Warnings)
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
	detail, err := svc.GetChainDetail(ctx, chainID)
	if err != nil {
		t.Fatalf("GetChainDetail returned error: %v", err)
	}
	wantReceipts := []ReceiptSummary{
		{Label: "orchestrator", Path: "receipts/orchestrator/chain-receipts.md"},
		{Label: "step 1 planner", Step: "1", Path: "receipts/planner/chain-receipts-step-001.md"},
		{Label: "step 2 coder", Step: "2", Path: "receipts/coder/chain-receipts-step-002.md"},
	}
	if !reflect.DeepEqual(detail.Receipts, wantReceipts) {
		t.Fatalf("detail receipts = %+v, want %+v", detail.Receipts, wantReceipts)
	}
}

func TestReadReceiptFallsBackToStepReceiptWhenOrchestratorReceiptIsMissing(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newOperatorTestDB(t))
	chainID, err := store.StartChain(ctx, chain.ChainSpec{ChainID: "one-step-receipts", SourceTask: "receipts"})
	if err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	stepID, err := store.StartStep(ctx, chain.StepSpec{ChainID: chainID, SequenceNum: 1, Role: "coder", Task: "code"})
	if err != nil {
		t.Fatalf("StartStep returned error: %v", err)
	}
	if err := store.CompleteStep(ctx, chain.CompleteStepParams{StepID: stepID, Status: "completed", ReceiptPath: "receipts/coder/one-step-receipts-step-001.md"}); err != nil {
		t.Fatalf("CompleteStep returned error: %v", err)
	}
	backend := &fakeBrainBackend{docs: map[string]string{
		"receipts/coder/one-step-receipts-step-001.md": "one-step receipt",
	}}
	svc := openOperatorTestService(t, t.TempDir(), store, backend, nil)

	receipt, err := svc.ReadReceipt(ctx, chainID, "")
	if err != nil {
		t.Fatalf("ReadReceipt returned error: %v", err)
	}
	if receipt.Path != "receipts/coder/one-step-receipts-step-001.md" || receipt.Content != "one-step receipt" {
		t.Fatalf("receipt = %+v, want fallback step receipt", receipt)
	}
	detail, err := svc.GetChainDetail(ctx, chainID)
	if err != nil {
		t.Fatalf("GetChainDetail returned error: %v", err)
	}
	wantReceipts := []ReceiptSummary{{Label: "step 1 coder", Step: "1", Path: "receipts/coder/one-step-receipts-step-001.md"}}
	if !reflect.DeepEqual(detail.Receipts, wantReceipts) {
		t.Fatalf("detail receipts = %+v, want %+v", detail.Receipts, wantReceipts)
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

	manual, err := svc.ValidateLaunch(ctx, LaunchRequest{Mode: LaunchModeManualRoster, Roster: []string{" coder ", "orchestrator"}, SourceTask: "ship roster"})
	if err != nil {
		t.Fatalf("ValidateLaunch manual roster returned error: %v", err)
	}
	if manual.Mode != LaunchModeManualRoster || manual.Role != "coder,orchestrator" || !reflect.DeepEqual(manual.Roster, []string{"coder", "orchestrator"}) || manual.Summary != "Run manual roster: coder -> orchestrator" {
		t.Fatalf("manual preview = %+v, want normalized roster preview", manual)
	}

	constrained, err := svc.ValidateLaunch(ctx, LaunchRequest{Mode: LaunchModeConstrained, AllowedRoles: []string{" coder ", "coder"}, SourceTask: "ship constrained"})
	if err != nil {
		t.Fatalf("ValidateLaunch constrained returned error: %v", err)
	}
	if constrained.Mode != LaunchModeConstrained || constrained.Role != "orchestrator" || !reflect.DeepEqual(constrained.AllowedRoles, []string{"coder"}) || constrained.Summary != "Run constrained orchestration with roles: coder" || !strings.Contains(constrained.CompiledTask, "Allowed roles: coder") {
		t.Fatalf("constrained preview = %+v, want normalized constrained preview", constrained)
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
	if _, err := svc.ValidateLaunch(ctx, LaunchRequest{Mode: LaunchModeManualRoster, SourceTask: "fix"}); err == nil || !strings.Contains(err.Error(), "manual roster requires at least one role") {
		t.Fatalf("ValidateLaunch missing roster error = %v, want missing roster error", err)
	}
	if _, err := svc.ValidateLaunch(ctx, LaunchRequest{Mode: LaunchModeConstrained, SourceTask: "fix"}); err == nil || !strings.Contains(err.Error(), "constrained orchestration requires at least one allowed role") {
		t.Fatalf("ValidateLaunch missing constrained roles error = %v, want missing allowed roles error", err)
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
	if gotDeps.ProcessID == nil || gotDeps.ProcessID() != 0 {
		t.Fatalf("ProcessID dependency returned nonzero, want embedded starts to register without a signalable PID")
	}
}

func TestStartChainMapsManualRosterLaunchRequestToChainrun(t *testing.T) {
	ctx := context.Background()
	projectRoot := t.TempDir()
	configPath := writeOperatorTestConfig(t, projectRoot)
	var gotOpts chainrun.Options
	svc, err := Open(ctx, Options{
		ConfigPath: configPath,
		ReadOnly:   true,
		ChainStarter: func(ctx context.Context, cfg *appconfig.Config, opts chainrun.Options, deps chainrun.Deps) (*chainrun.Result, error) {
			gotOpts = opts
			opts.OnChainID("manual-launched")
			return &chainrun.Result{ChainID: "manual-launched", Status: "completed"}, nil
		},
	})
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(svc.Close)

	result, err := svc.StartChain(ctx, LaunchRequest{Mode: LaunchModeManualRoster, Roster: []string{"coder", "orchestrator"}, SourceTask: "ship roster"})
	if err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	if result.ChainID != "manual-launched" || result.Preview.Summary != "Run manual roster: coder -> orchestrator" {
		t.Fatalf("result = %+v, want manual roster preview", result)
	}
	if gotOpts.Mode != chainrun.ModeManualRoster || gotOpts.Role != "coder,orchestrator" || len(gotOpts.Roster) != 2 || gotOpts.Roster[0].Role != "coder" || gotOpts.Roster[1].Role != "orchestrator" {
		t.Fatalf("chainrun opts = %+v, want manual roster mapped to step requests", gotOpts)
	}
}

func TestStartChainMapsConstrainedLaunchRequestToChainrun(t *testing.T) {
	ctx := context.Background()
	projectRoot := t.TempDir()
	configPath := writeOperatorTestConfig(t, projectRoot)
	var gotOpts chainrun.Options
	svc, err := Open(ctx, Options{
		ConfigPath: configPath,
		ReadOnly:   true,
		ChainStarter: func(ctx context.Context, cfg *appconfig.Config, opts chainrun.Options, deps chainrun.Deps) (*chainrun.Result, error) {
			gotOpts = opts
			opts.OnChainID("constrained-launched")
			return &chainrun.Result{ChainID: "constrained-launched", Status: "completed"}, nil
		},
	})
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(svc.Close)

	result, err := svc.StartChain(ctx, LaunchRequest{Mode: LaunchModeConstrained, AllowedRoles: []string{"coder"}, SourceTask: "ship constrained"})
	if err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	if result.ChainID != "constrained-launched" || result.Preview.Summary != "Run constrained orchestration with roles: coder" {
		t.Fatalf("result = %+v, want constrained preview", result)
	}
	if gotOpts.Mode != chainrun.ModeConstrained || gotOpts.Role != "orchestrator" || !reflect.DeepEqual(gotOpts.AllowedRoles, []string{"coder"}) || gotOpts.SourceTask != "ship constrained" {
		t.Fatalf("chainrun opts = %+v, want constrained orchestrator mapped with allowed roles", gotOpts)
	}
}

func TestLaunchDraftSaveLoadRoundTripsCurrentDraft(t *testing.T) {
	ctx := context.Background()
	projectRoot := t.TempDir()
	configPath := writeOperatorTestConfig(t, projectRoot)
	db := newOperatorTestDB(t)
	store := chain.NewStore(db)
	svc, err := Open(ctx, Options{
		ConfigPath: configPath,
		BuildRuntime: func(ctx context.Context, cfg *appconfig.Config) (*rtpkg.OrchestratorRuntime, error) {
			return &rtpkg.OrchestratorRuntime{
				Config:       cfg,
				Database:     db,
				ChainStore:   store,
				BrainBackend: &fakeBrainBackend{},
				Cleanup:      func() {},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(svc.Close)

	initial, found, err := svc.LoadLaunchDraft(ctx)
	if err != nil {
		t.Fatalf("LoadLaunchDraft initial returned error: %v", err)
	}
	if found {
		t.Fatalf("initial draft = %+v, want no draft", initial)
	}

	saved, err := svc.SaveLaunchDraft(ctx, LaunchRequest{
		Mode:         LaunchModeConstrained,
		Role:         "coder",
		AllowedRoles: []string{"coder", "planner"},
		SourceTask:   "persist launch",
		SourceSpecs:  []string{"docs/specs/a.md", "docs/specs/b.md"},
	})
	if err != nil {
		t.Fatalf("SaveLaunchDraft returned error: %v", err)
	}
	if saved.ID != "current" || saved.UpdatedAt == "" {
		t.Fatalf("saved draft metadata = %+v, want current id and updated timestamp", saved)
	}

	loaded, found, err := svc.LoadLaunchDraft(ctx)
	if err != nil {
		t.Fatalf("LoadLaunchDraft returned error: %v", err)
	}
	if !found {
		t.Fatal("LoadLaunchDraft found=false, want saved draft")
	}
	if loaded.Request.Mode != LaunchModeConstrained || loaded.Request.Role != "coder" || loaded.Request.SourceTask != "persist launch" {
		t.Fatalf("loaded draft request = %+v, want constrained coder task", loaded.Request)
	}
	if !reflect.DeepEqual(loaded.Request.AllowedRoles, []string{"coder", "planner"}) || !reflect.DeepEqual(loaded.Request.SourceSpecs, []string{"docs/specs/a.md", "docs/specs/b.md"}) {
		t.Fatalf("loaded draft slices = allowed %v specs %v", loaded.Request.AllowedRoles, loaded.Request.SourceSpecs)
	}

	if _, err := svc.SaveLaunchDraft(ctx, LaunchRequest{Mode: LaunchModeOneStep, Role: "coder", SourceTask: "replacement"}); err != nil {
		t.Fatalf("SaveLaunchDraft replacement returned error: %v", err)
	}
	loaded, found, err = svc.LoadLaunchDraft(ctx)
	if err != nil {
		t.Fatalf("LoadLaunchDraft replacement returned error: %v", err)
	}
	if !found || loaded.Request.Mode != LaunchModeOneStep || loaded.Request.SourceTask != "replacement" || len(loaded.Request.AllowedRoles) != 0 {
		t.Fatalf("loaded replacement = %+v, want overwritten one-step draft", loaded)
	}
}

func TestLaunchPresetSaveListRoundTripsCustomPreset(t *testing.T) {
	ctx := context.Background()
	projectRoot := t.TempDir()
	configPath := writeOperatorTestConfig(t, projectRoot)
	db := newOperatorTestDB(t)
	store := chain.NewStore(db)
	svc, err := Open(ctx, Options{
		ConfigPath: configPath,
		BuildRuntime: func(ctx context.Context, cfg *appconfig.Config) (*rtpkg.OrchestratorRuntime, error) {
			return &rtpkg.OrchestratorRuntime{
				Config:       cfg,
				Database:     db,
				ChainStore:   store,
				BrainBackend: &fakeBrainBackend{},
				Cleanup:      func() {},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(svc.Close)

	initial, err := svc.ListLaunchPresets(ctx)
	if err != nil {
		t.Fatalf("ListLaunchPresets initial returned error: %v", err)
	}
	if len(initial) != 0 {
		t.Fatalf("initial presets = %+v, want none", initial)
	}

	saved, err := svc.SaveLaunchPreset(ctx, "audit pair", LaunchRequest{
		Mode:        LaunchModeManualRoster,
		Role:        "coder",
		Roster:      []string{"coder", "orchestrator"},
		SourceTask:  "do not persist",
		SourceSpecs: []string{"docs/specs/nope.md"},
	})
	if err != nil {
		t.Fatalf("SaveLaunchPreset returned error: %v", err)
	}
	if saved.Name != "audit pair" || saved.Request.SourceTask != "" || len(saved.Request.SourceSpecs) != 0 {
		t.Fatalf("saved preset = %+v, want name without task/specs", saved)
	}
	if saved.Request.Mode != LaunchModeManualRoster || saved.Request.Role != "coder,orchestrator" || !reflect.DeepEqual(saved.Request.Roster, []string{"coder", "orchestrator"}) {
		t.Fatalf("saved preset request = %+v, want manual roster", saved.Request)
	}

	presets, err := svc.ListLaunchPresets(ctx)
	if err != nil {
		t.Fatalf("ListLaunchPresets returned error: %v", err)
	}
	if len(presets) != 1 || presets[0].Name != "audit pair" || !reflect.DeepEqual(presets[0].Request.Roster, []string{"coder", "orchestrator"}) {
		t.Fatalf("presets = %+v, want saved audit pair", presets)
	}

	if _, err := svc.SaveLaunchPreset(ctx, "audit pair", LaunchRequest{Mode: LaunchModeConstrained, AllowedRoles: []string{"coder"}, SourceTask: "ignored"}); err != nil {
		t.Fatalf("SaveLaunchPreset update returned error: %v", err)
	}
	presets, err = svc.ListLaunchPresets(ctx)
	if err != nil {
		t.Fatalf("ListLaunchPresets after update returned error: %v", err)
	}
	if len(presets) != 1 || presets[0].Request.Mode != LaunchModeConstrained || !reflect.DeepEqual(presets[0].Request.AllowedRoles, []string{"coder"}) {
		t.Fatalf("updated presets = %+v, want constrained replacement", presets)
	}
}

func TestLaunchDraftAndPresetsUseProjectMemoryInShunterMode(t *testing.T) {
	ctx := context.Background()
	projectRoot := t.TempDir()
	cfg := appconfig.Default()
	cfg.ProjectRoot = projectRoot
	cfg.Memory.Backend = "shunter"
	cfg.AgentRoles = map[string]appconfig.AgentRoleConfig{
		"coder":        {SystemPrompt: "prompts/coder.md"},
		"orchestrator": {SystemPrompt: "prompts/orchestrator.md"},
	}
	backend, err := projectmemory.OpenBrainBackend(ctx, projectmemory.Config{DataDir: filepath.Join(projectRoot, "memory"), DurableAck: true})
	if err != nil {
		t.Fatalf("OpenBrainBackend: %v", err)
	}
	defer backend.Close()
	svc, err := NewForRuntime(&rtpkg.OrchestratorRuntime{
		Config:        cfg,
		BrainBackend:  backend,
		MemoryBackend: backend,
		Cleanup:       func() {},
	}, Options{})
	if err != nil {
		t.Fatalf("NewForRuntime returned error: %v", err)
	}
	t.Cleanup(svc.Close)

	saved, err := svc.SaveLaunchDraft(ctx, LaunchRequest{
		Mode:         LaunchModeConstrained,
		Role:         "coder",
		AllowedRoles: []string{"coder", "orchestrator"},
		SourceTask:   "persist shunter launch",
	})
	if err != nil {
		t.Fatalf("SaveLaunchDraft returned error: %v", err)
	}
	if saved.ID != "current" || saved.UpdatedAt == "" {
		t.Fatalf("saved draft = %+v, want current id", saved)
	}
	loaded, found, err := svc.LoadLaunchDraft(ctx)
	if err != nil {
		t.Fatalf("LoadLaunchDraft returned error: %v", err)
	}
	if !found || loaded.Request.Mode != LaunchModeConstrained || loaded.Request.SourceTask != "persist shunter launch" || !reflect.DeepEqual(loaded.Request.AllowedRoles, []string{"coder", "orchestrator"}) {
		t.Fatalf("loaded draft = %+v found=%t, want Shunter launch draft", loaded, found)
	}

	preset, err := svc.SaveLaunchPreset(ctx, "shunter audit pair", LaunchRequest{Mode: LaunchModeManualRoster, Roster: []string{"coder", "orchestrator"}, SourceTask: "do not persist"})
	if err != nil {
		t.Fatalf("SaveLaunchPreset returned error: %v", err)
	}
	if preset.ID != "custom:shunter audit pair" || preset.Request.SourceTask != "" {
		t.Fatalf("preset = %+v, want custom preset without source task", preset)
	}
	presets, err := svc.ListLaunchPresets(ctx)
	if err != nil {
		t.Fatalf("ListLaunchPresets returned error: %v", err)
	}
	if len(presets) != 1 || presets[0].Name != "shunter audit pair" || !reflect.DeepEqual(presets[0].Request.Roster, []string{"coder", "orchestrator"}) {
		t.Fatalf("presets = %+v, want Shunter custom preset", presets)
	}
	if _, statErr := os.Stat(cfg.DatabasePath()); !os.IsNotExist(statErr) {
		t.Fatalf("database stat err = %v, want no SQLite launch dependency in Shunter mode", statErr)
	}
}

func TestStartChainCancelsRunnerWhenCallerContextEndsBeforeChainID(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	projectRoot := t.TempDir()
	configPath := writeOperatorTestConfig(t, projectRoot)
	started := make(chan struct{})
	cancelled := make(chan struct{})
	svc, err := Open(context.Background(), Options{
		ConfigPath: configPath,
		ReadOnly:   true,
		ChainStarter: func(ctx context.Context, cfg *appconfig.Config, opts chainrun.Options, deps chainrun.Deps) (*chainrun.Result, error) {
			close(started)
			<-ctx.Done()
			close(cancelled)
			return nil, ctx.Err()
		},
	})
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(svc.Close)

	errCh := make(chan error, 1)
	go func() {
		_, err := svc.StartChain(ctx, LaunchRequest{Mode: LaunchModeOneStep, Role: "coder", SourceTask: "ship it"})
		errCh <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for chain starter")
	}
	cancel()
	select {
	case err := <-errCh:
		if err == nil || !errors.Is(err, context.Canceled) {
			t.Fatalf("StartChain error = %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("StartChain did not return after caller cancellation")
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("runner context was not cancelled")
	}
}

func TestCancelChainCancelsInProcessRunnerWithoutSignalingOwnPID(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newOperatorTestDB(t))
	projectRoot := t.TempDir()
	configPath := writeOperatorTestConfig(t, projectRoot)
	cancelled := make(chan struct{})
	var signaled []int
	svc, err := Open(ctx, Options{
		ConfigPath: configPath,
		BuildRuntime: func(ctx context.Context, cfg *appconfig.Config) (*rtpkg.OrchestratorRuntime, error) {
			return &rtpkg.OrchestratorRuntime{Config: cfg, ChainStore: store, BrainBackend: &fakeBrainBackend{}, Cleanup: func() {}}, nil
		},
		ChainStarter: func(ctx context.Context, cfg *appconfig.Config, opts chainrun.Options, deps chainrun.Deps) (*chainrun.Result, error) {
			chainID, err := store.StartChain(context.Background(), chain.ChainSpec{ChainID: "embedded-chain", SourceTask: opts.SourceTask})
			if err != nil {
				return nil, err
			}
			if err := store.LogEvent(context.Background(), chainID, "", chain.EventChainStarted, map[string]any{"orchestrator_pid": deps.ProcessID(), "execution_id": "exec-embedded", "active_execution": true}); err != nil {
				return nil, err
			}
			opts.OnChainID(chainID)
			<-ctx.Done()
			close(cancelled)
			return &chainrun.Result{ChainID: chainID, Status: "cancelled"}, ctx.Err()
		},
		ProcessID: func() int { return 1234 },
		ProcessSignaler: func(pid int) error {
			signaled = append(signaled, pid)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(svc.Close)

	started, err := svc.StartChain(ctx, LaunchRequest{Mode: LaunchModeOneStep, Role: "coder", SourceTask: "ship it"})
	if err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	if started.ChainID != "embedded-chain" {
		t.Fatalf("ChainID = %q, want embedded-chain", started.ChainID)
	}
	result, err := svc.CancelChain(ctx, started.ChainID)
	if err != nil {
		t.Fatalf("CancelChain returned error: %v", err)
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("in-process runner context was not cancelled")
	}
	if len(signaled) != 0 || len(result.SignaledPIDs) != 0 {
		t.Fatalf("signaled = %v result=%+v, want no OS signal for embedded runner", signaled, result)
	}
	requireChainStatus(t, ctx, store, started.ChainID, "cancel_requested")
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
		ChainStarter: func(ctx context.Context, cfg *appconfig.Config, opts chainrun.Options, deps chainrun.Deps) (*chainrun.Result, error) {
			chainID := opts.ChainID
			if chainID == "" {
				chainID = "operator-test-chain"
				if _, err := store.StartChain(context.Background(), chain.ChainSpec{ChainID: chainID, SourceTask: opts.SourceTask, SourceSpecs: opts.SourceSpecs}); err != nil {
					return nil, err
				}
			} else {
				if err := store.SetChainStatus(context.Background(), chainID, "running"); err != nil {
					return nil, err
				}
				_ = store.LogEvent(context.Background(), chainID, "", chain.EventChainResumed, map[string]any{"resumed_by": "test"})
			}
			if opts.OnChainID != nil {
				opts.OnChainID(chainID)
			}
			return &chainrun.Result{ChainID: chainID, Status: "running"}, nil
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

func newOperatorTestRouter(t *testing.T, status *provider.AuthStatus) *router.Router {
	t.Helper()
	r, err := router.NewRouter(router.RouterConfig{
		Default: router.RouteTarget{Provider: "codex", Model: "test-model"},
	}, nil, nil)
	if err != nil {
		t.Fatalf("NewRouter returned error: %v", err)
	}
	if err := r.RegisterProvider(fakeAuthProvider{status: status}); err != nil {
		t.Fatalf("RegisterProvider returned error: %v", err)
	}
	return r
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

func hasRuntimeWarning(warnings []RuntimeWarning, want string) bool {
	for _, warning := range warnings {
		if strings.Contains(warning.Message, want) {
			return true
		}
	}
	return false
}
