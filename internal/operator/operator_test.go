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
	launches  map[string]projectmemory.Launch
	presets   map[string]projectmemory.LaunchPreset
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

func (f *fakeBrainBackend) SaveLaunch(ctx context.Context, args projectmemory.SaveLaunchArgs) error {
	if f.launches == nil {
		f.launches = map[string]projectmemory.Launch{}
	}
	id := projectmemory.ProjectLaunchID(args.ProjectID, args.LaunchID)
	current := f.launches[id]
	createdAtUS := current.CreatedAtUS
	if createdAtUS == 0 {
		createdAtUS = args.UpdatedAtUS
	}
	f.launches[id] = projectmemory.Launch{
		ID:               id,
		ProjectID:        args.ProjectID,
		LaunchID:         args.LaunchID,
		Status:           args.Status,
		Mode:             args.Mode,
		Role:             args.Role,
		AllowedRolesJSON: args.AllowedRolesJSON,
		RosterJSON:       args.RosterJSON,
		SourceTask:       args.SourceTask,
		SourceSpecsJSON:  args.SourceSpecsJSON,
		CreatedAtUS:      createdAtUS,
		UpdatedAtUS:      args.UpdatedAtUS,
	}
	return nil
}

func (f *fakeBrainBackend) ReadLaunch(ctx context.Context, projectID string, launchID string) (projectmemory.Launch, bool, error) {
	launch, found := f.launches[projectmemory.ProjectLaunchID(projectID, launchID)]
	return launch, found, nil
}

func (f *fakeBrainBackend) SaveLaunchPreset(ctx context.Context, args projectmemory.SaveLaunchPresetArgs) error {
	if f.presets == nil {
		f.presets = map[string]projectmemory.LaunchPreset{}
	}
	id := projectmemory.ProjectLaunchPresetID(args.ProjectID, args.Name)
	current := f.presets[id]
	createdAtUS := current.CreatedAtUS
	if createdAtUS == 0 {
		createdAtUS = args.UpdatedAtUS
	}
	f.presets[id] = projectmemory.LaunchPreset{
		ID:               id,
		ProjectID:        args.ProjectID,
		PresetID:         "custom:" + args.Name,
		Name:             args.Name,
		Mode:             args.Mode,
		Role:             args.Role,
		AllowedRolesJSON: args.AllowedRolesJSON,
		RosterJSON:       args.RosterJSON,
		CreatedAtUS:      createdAtUS,
		UpdatedAtUS:      args.UpdatedAtUS,
	}
	return nil
}

func (f *fakeBrainBackend) ListLaunchPresets(ctx context.Context, projectID string) ([]projectmemory.LaunchPreset, error) {
	var presets []projectmemory.LaunchPreset
	for _, preset := range f.presets {
		if preset.ProjectID == projectID {
			presets = append(presets, preset)
		}
	}
	return presets, nil
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

func TestOpenReadOnlyUsesShunterMemoryWithoutYardDB(t *testing.T) {
	ctx := context.Background()
	projectRoot := t.TempDir()
	dataDir := filepath.Join(projectRoot, ".yard", "shunter", "project-memory")
	backend, err := projectmemory.OpenBrainBackend(ctx, projectmemory.Config{DataDir: dataDir, DurableAck: true})
	if err != nil {
		t.Fatalf("OpenBrainBackend returned error: %v", err)
	}
	if err := backend.StartChain(ctx, projectmemory.StartChainArgs{
		ID:          "readonly-shunter-chain",
		SourceTask:  "inspect Shunter state",
		CreatedAtUS: uint64(time.Now().UTC().UnixMicro()),
	}); err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	if err := backend.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	configPath := filepath.Join(t.TempDir(), "yard.yaml")
	content := fmt.Sprintf(`project_root: %q
memory:
  backend: shunter
  shunter_data_dir: .yard/shunter/project-memory
  durable_ack: true
brain:
  enabled: true
  backend: shunter
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
`, projectRoot)
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

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
	if svc.rt.Database != nil || svc.rt.Queries != nil {
		t.Fatalf("read-only Shunter runtime SQLite = (%v, %v), want nil", svc.rt.Database, svc.rt.Queries)
	}

	chains, err := svc.ListChains(ctx, 10)
	if err != nil {
		t.Fatalf("ListChains returned error: %v", err)
	}
	if len(chains) != 1 || chains[0].ID != "readonly-shunter-chain" {
		t.Fatalf("chains = %+v, want readonly-shunter-chain", chains)
	}
	if _, err := os.Stat(filepath.Join(projectRoot, ".yard", "yard.db")); !os.IsNotExist(err) {
		t.Fatalf("read-only Shunter Open touched yard.db: stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(projectRoot, ".brain")); !os.IsNotExist(err) {
		t.Fatalf("read-only Shunter Open touched .brain: stat err=%v", err)
	}
}

func TestRuntimeStatusIncludesReadinessMetadata(t *testing.T) {
	ctx := context.Background()
	projectRoot := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "yard.yaml")
	config := fmt.Sprintf(`project_root: %q
brain:
  enabled: true
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
	indexedTime, err := time.Parse(time.RFC3339, indexedAt)
	if err != nil {
		t.Fatalf("parse indexedAt: %v", err)
	}
	store := chain.NewStore(db)
	indexBackend := &fakeShunterIndexBrainBackend{
		brainState: projectmemory.BrainIndexState{
			ProjectID:       projectmemory.DefaultProjectID,
			LastIndexedAtUS: uint64(staleAt.Add(-time.Hour).UnixMicro()),
			Dirty:           true,
			DirtySinceUS:    uint64(staleAt.UnixMicro()),
			DirtyReason:     "brain_update",
		},
		brainFound: true,
		codeState: projectmemory.CodeIndexState{
			ProjectID:         projectmemory.DefaultProjectID,
			LastIndexedCommit: "abc123",
			LastIndexedAtUS:   uint64(indexedTime.UnixMicro()),
		},
		codeFound: true,
	}
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
				BrainBackend:   indexBackend,
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

func TestGetChainMetricsFlagsDogfoodingWarnings(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newOperatorTestDB(t))
	chainID, err := store.StartChain(ctx, chain.ChainSpec{
		ChainID:          "chain-metrics",
		SourceTask:       "observe dogfood run",
		MaxSteps:         2,
		MaxResolverLoops: 1,
		MaxDuration:      100 * time.Second,
		TokenBudget:      100,
	})
	if err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	stepOne, err := store.StartStep(ctx, chain.StepSpec{ChainID: chainID, SequenceNum: 1, Role: "planner", Task: "plan"})
	if err != nil {
		t.Fatalf("StartStep one returned error: %v", err)
	}
	exitZero := 0
	if err := store.CompleteStep(ctx, chain.CompleteStepParams{StepID: stepOne, Status: "completed", Verdict: "accepted", DurationSecs: 95, ExitCode: &exitZero}); err != nil {
		t.Fatalf("CompleteStep one returned error: %v", err)
	}
	stepTwo, err := store.StartStep(ctx, chain.StepSpec{ChainID: chainID, SequenceNum: 2, Role: "coder", Task: "code"})
	if err != nil {
		t.Fatalf("StartStep two returned error: %v", err)
	}
	exitOne := 1
	if err := store.FailStep(ctx, chain.CompleteStepParams{StepID: stepTwo, Verdict: "failed", ReceiptPath: "receipts/coder/chain-metrics-step-002.md", TokensUsed: 85, TurnsUsed: 2, ExitCode: &exitOne, ErrorMessage: "agent exited"}); err != nil {
		t.Fatalf("FailStep returned error: %v", err)
	}
	if err := store.UpdateChainMetrics(ctx, chainID, chain.ChainMetrics{TotalSteps: 2, TotalTokens: 85, TotalDurationSecs: 95, ResolverLoops: 1}); err != nil {
		t.Fatalf("UpdateChainMetrics returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, stepTwo, chain.EventStepProcessStarted, map[string]any{"process_id": 1234, "active_process": true}); err != nil {
		t.Fatalf("LogEvent process start returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, stepTwo, chain.EventStepOutput, map[string]any{"stream": "stderr", "line": "running"}); err != nil {
		t.Fatalf("LogEvent output returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, stepTwo, chain.EventStepFailed, map[string]any{"error": "agent exited"}); err != nil {
		t.Fatalf("LogEvent step failed returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, "", chain.EventSafetyLimitHit, map[string]any{"limit": "token_budget"}); err != nil {
		t.Fatalf("LogEvent safety returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, "", chain.EventReindexStarted, map[string]any{"reason": "post_step"}); err != nil {
		t.Fatalf("LogEvent reindex start returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, "", chain.EventReindexCompleted, map[string]any{"reason": "post_step"}); err != nil {
		t.Fatalf("LogEvent reindex complete returned error: %v", err)
	}
	if err := store.CompleteChain(ctx, chainID, "failed", "failed"); err != nil {
		t.Fatalf("CompleteChain returned error: %v", err)
	}
	svc := openOperatorTestService(t, t.TempDir(), store, &fakeBrainBackend{}, nil)

	report, err := svc.GetChainMetrics(ctx, chainID)
	if err != nil {
		t.Fatalf("GetChainMetrics returned error: %v", err)
	}
	if report.Health != "failing" || report.Status != "failed" {
		t.Fatalf("health/status = %s/%s, want failing/failed", report.Health, report.Status)
	}
	if report.TotalSteps != 2 || report.StepRows != 2 || report.CompletedSteps != 1 || report.FailedSteps != 1 {
		t.Fatalf("step counts = %+v, want one completed and one failed", report)
	}
	if report.TotalTokens != 85 || report.StepTokenTotal != 85 || report.TokenBudgetPct != 85 {
		t.Fatalf("token metrics = recorded %d step_sum %d pct %.1f, want 85/85/85.0", report.TotalTokens, report.StepTokenTotal, report.TokenBudgetPct)
	}
	if report.TotalDurationSecs != 95 || report.StepDurationSecs != 95 || report.DurationBudgetPct != 95 {
		t.Fatalf("duration metrics = recorded %d step_sum %d pct %.1f, want 95/95/95.0", report.TotalDurationSecs, report.StepDurationSecs, report.DurationBudgetPct)
	}
	if report.StepFailedEvents != 1 || report.SafetyLimitEvents != 1 || report.ReindexStartedEvents != 1 || report.ReindexDoneEvents != 1 || report.ProcessStartedEvents != 1 || report.ProcessExitedEvents != 0 {
		t.Fatalf("event counts = %+v, want failed/safety/reindex/process counts", report)
	}
	for _, want := range []string{
		"chain status is failed",
		"step 1 completed without a receipt path",
		"step 1 completed without token usage",
		"step 1 completed without turn count",
		"step 2 failed",
		"step 2 exited with code 1",
		"resolver loop budget exhausted",
		"token budget 85.0% used",
		"duration budget 95.0% used",
		"chain has 1 step_failed event(s)",
		"chain has 1 safety_limit_hit event(s)",
		"process events show started=1 exited=0",
	} {
		if !hasRuntimeWarning(report.Warnings, want) {
			t.Fatalf("warnings = %+v, want %q", report.Warnings, want)
		}
	}
}

func TestGetChainMetricsFlagsGuardrailInvariantWarnings(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newOperatorTestDB(t))
	chainID, err := store.StartChain(ctx, chain.ChainSpec{ChainID: "chain-guardrails", SourceTask: "guardrails", MaxSteps: 5, MaxResolverLoops: 1, MaxDuration: 100 * time.Second, TokenBudget: 100})
	if err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	stepID, err := store.StartStep(ctx, chain.StepSpec{ChainID: chainID, SequenceNum: 1, Role: "coder", Task: "code"})
	if err != nil {
		t.Fatalf("StartStep returned error: %v", err)
	}
	if err := store.CompleteStep(ctx, chain.CompleteStepParams{StepID: stepID, Status: "completed", Verdict: "completed", ReceiptPath: "receipts/coder/chain-guardrails-step-001.md", TokensUsed: 10, TurnsUsed: 1}); err != nil {
		t.Fatalf("CompleteStep returned error: %v", err)
	}
	if err := store.UpdateChainMetrics(ctx, chainID, chain.ChainMetrics{TotalSteps: 1, TotalTokens: 10}); err != nil {
		t.Fatalf("UpdateChainMetrics returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, stepID, chain.EventReceiptValidation, map[string]any{"warning": "missing section"}); err != nil {
		t.Fatalf("LogEvent receipt warning returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, "", chain.EventSourceWriterBlocked, map[string]any{"requested_role": "resolver"}); err != nil {
		t.Fatalf("LogEvent source writer block returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, stepID, chain.EventStepGuardrailFacts, map[string]any{
		"role":                                   "coder",
		"sequence":                               1,
		"source_mutating":                        true,
		"receipt_valid":                          false,
		"receipt_error":                          "receipt: missing required section: Validation",
		"changed_file_claim_present":             true,
		"claimed_changed_files":                  []string{"claimed.txt"},
		"changed_file_claim_matches_manifest":    false,
		"changed_file_claim_extra":               []string{"claimed.txt"},
		"changed_file_manifest_unclaimed":        []string{"actual.txt"},
		"changed_file_manifest_present":          false,
		"changed_file_count":                     1,
		"code_index_state_found":                 true,
		"code_index_dirty":                       false,
		"brain_index_state_found":                true,
		"brain_index_dirty":                      true,
		"brain_index_dirty_reason":               "complete_step_with_receipt",
		"source_writer_lock_release_attempted":   true,
		"source_writer_lock_released":            false,
		"source_writer_lock_release_error":       "lock held by other step",
		"suspicious_verdict_finding_combination": false,
	}); err != nil {
		t.Fatalf("LogEvent guardrail facts returned error: %v", err)
	}
	if err := store.CompleteChain(ctx, chainID, "completed", "done"); err != nil {
		t.Fatalf("CompleteChain returned error: %v", err)
	}
	svc := openOperatorTestService(t, t.TempDir(), store, &fakeBrainBackend{}, nil)

	report, err := svc.GetChainMetrics(ctx, chainID)
	if err != nil {
		t.Fatalf("GetChainMetrics returned error: %v", err)
	}
	if report.Health != "failing" || report.ReceiptWarningEvents != 1 || report.SourceWriterBlocks != 1 || report.StepGuardrailFactEvents != 1 || report.ChangedFileEvents != 0 {
		t.Fatalf("report = %+v, want failing guardrail counters", report)
	}
	for _, want := range []string{
		"chain has 1 receipt_validation_warning event(s)",
		"source writer guard blocked 1 spawn attempt(s)",
		"step 1 receipt guardrail facts show invalid receipt: receipt: missing required section: Validation",
		"step 1 changed-file receipt claim differs from harness manifest: extra=claimed.txt unclaimed=actual.txt",
		"step 1 changed files but code index state was not marked stale",
		"step 1 source writer lock release failed: lock held by other step",
		"step 1 source-writing role coder completed without changed-file manifest",
	} {
		if !hasRuntimeWarning(report.Warnings, want) {
			t.Fatalf("warnings = %+v, want %q", report.Warnings, want)
		}
	}
	detail, err := svc.GetChainDetail(ctx, chainID)
	if err != nil {
		t.Fatalf("GetChainDetail returned error: %v", err)
	}
	if detail.Guardrails.LockHealth.Blocked != 1 || len(detail.Guardrails.StepFacts) != 1 {
		t.Fatalf("guardrails = %+v, want lock block and step facts", detail.Guardrails)
	}
	facts := detail.Guardrails.StepFacts[0]
	if facts.SequenceNum != 1 || facts.Role != "coder" || facts.ReceiptValid || facts.ReceiptError != "receipt: missing required section: Validation" || facts.SourceWriterLockReleased {
		t.Fatalf("guardrail facts = %+v, want invalid receipt and unreleased lock", facts)
	}
	if !facts.ChangedFileClaimPresent || facts.ChangedFileClaimMatchesManifest || strings.Join(facts.ChangedFileClaimExtra, ",") != "claimed.txt" || strings.Join(facts.ChangedFileManifestUnclaimed, ",") != "actual.txt" {
		t.Fatalf("guardrail fact changed-file claim = %+v, want mismatch details", facts)
	}
	if !facts.CodeIndexStateFound || facts.CodeIndexDirty || !facts.BrainIndexStateFound || !facts.BrainIndexDirty || facts.BrainIndexDirtyReason != "complete_step_with_receipt" {
		t.Fatalf("guardrail fact index state = %+v, want clean code index and dirty brain index", facts)
	}
}

func TestGetChainMetricsWarnsWhenCodeIndexDirtyMarkingUnavailable(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newOperatorTestDB(t))
	chainID, err := store.StartChain(ctx, chain.ChainSpec{ChainID: "chain-index-mark-unavailable", SourceTask: "guardrails", MaxSteps: 10, MaxResolverLoops: 1, MaxDuration: time.Hour, TokenBudget: 100})
	if err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	stepID, err := store.StartStep(ctx, chain.StepSpec{ChainID: chainID, SequenceNum: 1, Role: "coder", Task: "code"})
	if err != nil {
		t.Fatalf("StartStep returned error: %v", err)
	}
	if err := store.CompleteStep(ctx, chain.CompleteStepParams{StepID: stepID, Status: "completed", Verdict: "completed", ReceiptPath: "receipts/coder/chain-index-mark-unavailable-step-001.md"}); err != nil {
		t.Fatalf("CompleteStep returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, stepID, chain.EventStepGuardrailFacts, map[string]any{
		"role":                          "coder",
		"sequence":                      1,
		"source_mutating":               true,
		"receipt_valid":                 true,
		"changed_file_manifest_present": true,
		"changed_file_count":            1,
		"changed_files":                 []string{"internal/example.go"},
	}); err != nil {
		t.Fatalf("LogEvent guardrail facts returned error: %v", err)
	}
	svc := openOperatorTestService(t, t.TempDir(), store, &fakeBrainBackend{}, nil)

	report, err := svc.GetChainMetrics(ctx, chainID)
	if err != nil {
		t.Fatalf("GetChainMetrics returned error: %v", err)
	}
	if !hasRuntimeWarning(report.Warnings, "step 1 changed files but code index dirty marking is unavailable") {
		t.Fatalf("warnings = %+v, want code index dirty marking unavailable warning", report.Warnings)
	}
}

func TestGetChainMetricsCombinesGuardrailEventChainFacts(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newOperatorTestDB(t))
	chainID, err := store.StartChain(ctx, chain.ChainSpec{ChainID: "chain-guardrail-smoke", SourceTask: "guardrail smoke", MaxSteps: 8, MaxResolverLoops: 3, MaxDuration: time.Hour, TokenBudget: 1000})
	if err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	completeOperatorTestStep(t, ctx, store, chainID, 1, "planner")
	coderID := completeOperatorTestStep(t, ctx, store, chainID, 2, "coder")
	auditorID := completeOperatorTestStep(t, ctx, store, chainID, 3, "correctness-auditor")
	resolverID := completeOperatorTestStep(t, ctx, store, chainID, 4, "resolver")
	if err := store.UpdateChainMetrics(ctx, chainID, chain.ChainMetrics{TotalSteps: 4, TotalTokens: 40, TotalDurationSecs: 4, ResolverLoops: 1}); err != nil {
		t.Fatalf("UpdateChainMetrics returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, coderID, chain.EventStepChangedFiles, map[string]any{"paths": []string{"internal/api.go", "internal/api_test.go"}, "count": 2}); err != nil {
		t.Fatalf("LogEvent coder changed files returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, coderID, chain.EventStepGuardrailFacts, map[string]any{
		"role":                                 "coder",
		"sequence":                             2,
		"source_mutating":                      true,
		"receipt_valid":                        true,
		"receipt_schema_valid":                 true,
		"receipt_sections_valid":               true,
		"changed_file_manifest_present":        true,
		"changed_file_count":                   2,
		"changed_files":                        []string{"internal/api.go", "internal/api_test.go"},
		"changed_file_claim_present":           true,
		"claimed_changed_files":                []string{"internal/api.go", "docs/claim.md"},
		"changed_file_claim_matches_manifest":  false,
		"changed_file_claim_extra":             []string{"docs/claim.md"},
		"changed_file_manifest_unclaimed":      []string{"internal/api_test.go"},
		"code_index_dirty_mark_supported":      true,
		"code_index_dirty_mark_attempted":      true,
		"code_index_dirty_marked":              true,
		"code_index_state_supported":           true,
		"code_index_state_found":               true,
		"code_index_dirty":                     true,
		"code_index_dirty_reason":              "source_write",
		"brain_index_state_supported":          true,
		"brain_index_state_found":              true,
		"brain_index_dirty":                    true,
		"brain_index_dirty_reason":             "complete_step_with_receipt",
		"source_writer_lock_release_attempted": true,
		"source_writer_lock_released":          true,
	}); err != nil {
		t.Fatalf("LogEvent coder guardrail facts returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, auditorID, chain.EventFindingLifecycleFacts, map[string]any{
		"role":    "correctness-auditor",
		"verdict": "fix_required",
		"facts": []map[string]any{{
			"id":           "FIND-correctness-001",
			"source_role":  "correctness-auditor",
			"action":       "opened",
			"status":       "open",
			"severity":     "high",
			"evidence":     "internal/api.go:42",
			"summary":      "nil panic",
			"required_fix": "guard nil",
		}},
	}); err != nil {
		t.Fatalf("LogEvent auditor lifecycle returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, resolverID, chain.EventStepChangedFiles, map[string]any{"paths": []string{"internal/api.go"}, "count": 1}); err != nil {
		t.Fatalf("LogEvent resolver changed files returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, resolverID, chain.EventFindingLifecycleFacts, map[string]any{
		"role":    "resolver",
		"verdict": "completed",
		"facts": []map[string]any{{
			"id":            "FIND-correctness-001",
			"action":        "addressed",
			"status":        "addressed",
			"resolution":    "fixed",
			"files_changed": []string{"internal/api.go"},
			"validation":    []string{"rtk make test"},
		}},
	}); err != nil {
		t.Fatalf("LogEvent resolver lifecycle returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, resolverID, chain.EventStepGuardrailFacts, map[string]any{
		"role":                                 "resolver",
		"sequence":                             4,
		"source_mutating":                      true,
		"receipt_valid":                        true,
		"receipt_schema_valid":                 true,
		"receipt_sections_valid":               true,
		"changed_file_manifest_present":        true,
		"changed_file_count":                   1,
		"changed_files":                        []string{"internal/api.go"},
		"changed_file_claim_present":           true,
		"claimed_changed_files":                []string{"internal/api.go"},
		"changed_file_claim_matches_manifest":  true,
		"code_index_dirty_mark_supported":      true,
		"code_index_dirty_mark_attempted":      true,
		"code_index_dirty_marked":              true,
		"code_index_state_supported":           true,
		"code_index_state_found":               true,
		"code_index_dirty":                     true,
		"code_index_dirty_reason":              "source_write",
		"source_writer_lock_release_attempted": true,
		"source_writer_lock_released":          true,
		"addressed_ids":                        []string{"FIND-correctness-001"},
	}); err != nil {
		t.Fatalf("LogEvent resolver guardrail facts returned error: %v", err)
	}
	if err := store.CompleteChain(ctx, chainID, "completed", "done"); err != nil {
		t.Fatalf("CompleteChain returned error: %v", err)
	}
	svc := openOperatorTestService(t, t.TempDir(), store, &fakeBrainBackend{}, nil)

	report, err := svc.GetChainMetrics(ctx, chainID)
	if err != nil {
		t.Fatalf("GetChainMetrics returned error: %v", err)
	}
	if report.Health != "attention" || report.ChangedFileEvents != 2 || report.StepGuardrailFactEvents != 2 || report.FindingLifecycleFactEvents != 2 {
		t.Fatalf("report = %+v, want attention with changed-file, guardrail, and lifecycle events", report)
	}
	for _, want := range []string{
		"step 2 changed-file receipt claim differs from harness manifest: extra=docs/claim.md unclaimed=internal/api_test.go",
		"open audit findings: FIND-correctness-001",
	} {
		if !hasRuntimeWarning(report.Warnings, want) {
			t.Fatalf("warnings = %+v, want %q", report.Warnings, want)
		}
	}
	if hasRuntimeWarning(report.Warnings, "changed files but code index state was not marked stale") ||
		hasRuntimeWarning(report.Warnings, "code index dirty marking is unavailable") {
		t.Fatalf("warnings = %+v, want no stale-marking warnings after successful dirty mark", report.Warnings)
	}
	if !equalStringSlices(report.OpenFindingIDs, []string{"FIND-correctness-001"}) ||
		!equalStringSlices(report.AddressedFindingIDs, []string{"FIND-correctness-001"}) {
		t.Fatalf("finding ids = open %v addressed %v, want FIND-correctness-001 in both", report.OpenFindingIDs, report.AddressedFindingIDs)
	}
	if len(report.FindingLifecycle) != 1 {
		t.Fatalf("finding lifecycle = %+v, want one finding", report.FindingLifecycle)
	}
	finding := report.FindingLifecycle[0]
	if finding.ID != "FIND-correctness-001" || finding.Status != "addressed" || finding.Severity != "high" || finding.Resolution != "fixed" ||
		!equalStringSlices(finding.FilesChanged, []string{"internal/api.go"}) || !equalStringSlices(finding.Validation, []string{"rtk make test"}) {
		t.Fatalf("finding lifecycle = %+v, want merged auditor and resolver facts", finding)
	}
	detail, err := svc.GetChainDetail(ctx, chainID)
	if err != nil {
		t.Fatalf("GetChainDetail returned error: %v", err)
	}
	if len(detail.Guardrails.StepFacts) != 2 {
		t.Fatalf("step facts = %+v, want coder and resolver facts", detail.Guardrails.StepFacts)
	}
	coderFacts := detail.Guardrails.StepFacts[0]
	if coderFacts.SequenceNum != 2 || !coderFacts.CodeIndexDirtyMarked || !coderFacts.CodeIndexDirty || coderFacts.CodeIndexDirtyReason != "source_write" {
		t.Fatalf("coder facts = %+v, want successful code index dirty mark", coderFacts)
	}
	if coderFacts.ChangedFileClaimMatchesManifest || strings.Join(coderFacts.ChangedFileClaimExtra, ",") != "docs/claim.md" || strings.Join(coderFacts.ChangedFileManifestUnclaimed, ",") != "internal/api_test.go" {
		t.Fatalf("coder claim facts = %+v, want mismatch details", coderFacts)
	}
}

func TestProjectLocksListAndForceReleaseAuditsOperatorAction(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newOperatorTestDB(t))
	chainID, err := store.StartChain(ctx, chain.ChainSpec{ChainID: "lock-chain", SourceTask: "lock"})
	if err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	stepID, err := store.StartStep(ctx, chain.StepSpec{ChainID: chainID, SequenceNum: 1, Role: "coder", Task: "write"})
	if err != nil {
		t.Fatalf("StartStep returned error: %v", err)
	}
	if _, err := store.AcquireProjectLock(ctx, chain.AcquireProjectLockParams{
		LockName:     chain.SourceWriterLockName,
		OwnerChainID: chainID,
		OwnerStepID:  stepID,
		OwnerRole:    "coder",
		ExpiresAt:    time.Now().Add(time.Hour),
		MetadataJSON: `{"task":"write"}`,
	}); err != nil {
		t.Fatalf("AcquireProjectLock returned error: %v", err)
	}
	svc, err := NewForRuntime(&rtpkg.OrchestratorRuntime{
		Config:       &appconfig.Config{ProjectRoot: t.TempDir()},
		ChainStore:   store,
		BrainBackend: &fakeBrainBackend{docs: map[string]string{}},
		Cleanup:      func() {},
	}, Options{})
	if err != nil {
		t.Fatalf("NewForRuntime returned error: %v", err)
	}
	t.Cleanup(svc.Close)

	locks, err := svc.ListProjectLocks(ctx)
	if err != nil {
		t.Fatalf("ListProjectLocks returned error: %v", err)
	}
	if len(locks) != 1 || locks[0].LockName != chain.SourceWriterLockName || locks[0].OwnerStepID != stepID || locks[0].Stale {
		t.Fatalf("locks = %+v, want active source writer lock", locks)
	}

	result, err := svc.ForceReleaseProjectLock(ctx, chain.SourceWriterLockName, "stale process")
	if err != nil {
		t.Fatalf("ForceReleaseProjectLock returned error: %v", err)
	}
	if !result.Released || result.OwnerChainID != chainID || result.OwnerStepID != stepID || result.OwnerRole != "coder" {
		t.Fatalf("result = %+v, want released coder lock", result)
	}
	if lock, found, err := store.GetProjectLock(ctx, chain.SourceWriterLockName); err != nil || found {
		t.Fatalf("lock after force release = %+v found=%t err=%v, want absent", lock, found, err)
	}
	events, err := store.ListEvents(ctx, chainID)
	if err != nil {
		t.Fatalf("ListEvents returned error: %v", err)
	}
	var auditEvent bool
	for _, event := range events {
		if event.EventType == chain.EventSourceWriterLockForceReleased &&
			strings.Contains(event.EventData, `"operator_initiated":true`) &&
			strings.Contains(event.EventData, `"reason":"stale process"`) {
			auditEvent = true
		}
	}
	if !auditEvent {
		t.Fatalf("events = %+v, want force-release audit event", events)
	}
}

func TestGetChainMetricsSummarizesReceiptFindingFacts(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newOperatorTestDB(t))
	chainID, err := store.StartChain(ctx, chain.ChainSpec{ChainID: "finding-chain", SourceTask: "findings"})
	if err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	auditStepID, err := store.StartStep(ctx, chain.StepSpec{ChainID: chainID, SequenceNum: 1, Role: "correctness-auditor", Task: "audit"})
	if err != nil {
		t.Fatalf("StartStep audit returned error: %v", err)
	}
	if err := store.CompleteStep(ctx, chain.CompleteStepParams{StepID: auditStepID, Status: "completed", Verdict: "completed", ReceiptPath: "receipts/correctness-auditor/finding-chain-step-001.md", TokensUsed: 1, TurnsUsed: 1}); err != nil {
		t.Fatalf("CompleteStep audit returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, auditStepID, chain.EventReceiptFindings, map[string]any{
		"role":             "correctness-auditor",
		"verdict":          "completed",
		"open_count":       1,
		"open_finding_ids": []string{"FIND-correctness-001"},
	}); err != nil {
		t.Fatalf("LogEvent finding returned error: %v", err)
	}
	resolverStepID, err := store.StartStep(ctx, chain.StepSpec{ChainID: chainID, SequenceNum: 2, Role: "resolver", Task: "resolve"})
	if err != nil {
		t.Fatalf("StartStep resolver returned error: %v", err)
	}
	if err := store.CompleteStep(ctx, chain.CompleteStepParams{StepID: resolverStepID, Status: "completed", Verdict: "completed", ReceiptPath: "receipts/resolver/finding-chain-step-002.md", TokensUsed: 1, TurnsUsed: 1}); err != nil {
		t.Fatalf("CompleteStep resolver returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, resolverStepID, chain.EventReceiptFindings, map[string]any{
		"role":            "resolver",
		"verdict":         "completed",
		"addressed_count": 0,
		"addressed_ids":   []string{},
	}); err != nil {
		t.Fatalf("LogEvent resolver finding returned error: %v", err)
	}
	svc := openOperatorTestService(t, t.TempDir(), store, &fakeBrainBackend{}, nil)

	report, err := svc.GetChainMetrics(ctx, chainID)
	if err != nil {
		t.Fatalf("GetChainMetrics returned error: %v", err)
	}
	if report.ReceiptFindingEvents != 2 || report.OpenFindingCount != 1 || len(report.OpenFindingIDs) != 1 || report.OpenFindingIDs[0] != "FIND-correctness-001" {
		t.Fatalf("report = %+v, want finding counts and open ID", report)
	}
	for _, want := range []string{
		"correctness-auditor reported 1 open finding(s) with verdict completed",
		"resolver receipt did not address any finding IDs",
	} {
		if !hasRuntimeWarning(report.Warnings, want) {
			t.Fatalf("warnings = %+v, want %q", report.Warnings, want)
		}
	}
}

func TestGetChainMetricsSummarizesFindingLifecycle(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newOperatorTestDB(t))
	chainID, err := store.StartChain(ctx, chain.ChainSpec{ChainID: "finding-lifecycle-chain", SourceTask: "findings"})
	if err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	steps := []struct {
		id      string
		seq     int
		role    string
		verdict string
		event   map[string]any
	}{
		{id: "step-planner", seq: 1, role: "planner", verdict: "completed"},
		{id: "step-coder", seq: 2, role: "coder", verdict: "completed"},
		{id: "step-audit-open", seq: 3, role: "correctness-auditor", verdict: "fix_required", event: map[string]any{"role": "correctness-auditor", "verdict": "fix_required", "open_finding_ids": []string{"FIND-correctness-001"}, "open_count": 1}},
		{id: "step-resolver-one", seq: 4, role: "resolver", verdict: "completed", event: map[string]any{"role": "resolver", "verdict": "completed", "addressed_ids": []string{"FIND-correctness-001"}, "addressed_count": 1}},
		{id: "step-audit-closed", seq: 5, role: "correctness-auditor", verdict: "completed", event: map[string]any{"role": "correctness-auditor", "verdict": "completed", "closed_finding_ids": []string{"FIND-correctness-001"}, "closed_count": 1}},
		{id: "step-audit-reopen", seq: 6, role: "correctness-auditor", verdict: "fix_required", event: map[string]any{"role": "correctness-auditor", "verdict": "fix_required", "open_finding_ids": []string{"FIND-correctness-001"}, "open_count": 1}},
		{id: "step-resolver-two", seq: 7, role: "resolver", verdict: "completed", event: map[string]any{"role": "resolver", "verdict": "completed", "addressed_ids": []string{"FIND-correctness-001"}, "addressed_count": 1}},
	}
	for _, spec := range steps {
		stepID, err := store.StartStep(ctx, chain.StepSpec{StepID: spec.id, ChainID: chainID, SequenceNum: spec.seq, Role: spec.role, Task: spec.role})
		if err != nil {
			t.Fatalf("StartStep %s returned error: %v", spec.id, err)
		}
		if err := store.CompleteStep(ctx, chain.CompleteStepParams{StepID: stepID, Status: "completed", Verdict: spec.verdict, ReceiptPath: fmt.Sprintf("receipts/%s/finding-lifecycle-chain-step-%03d.md", spec.role, spec.seq), TokensUsed: 1, TurnsUsed: 1}); err != nil {
			t.Fatalf("CompleteStep %s returned error: %v", spec.id, err)
		}
		if spec.event != nil {
			if err := store.LogEvent(ctx, chainID, stepID, chain.EventReceiptFindings, spec.event); err != nil {
				t.Fatalf("LogEvent %s returned error: %v", spec.id, err)
			}
		}
	}
	if err := store.UpdateChainMetrics(ctx, chainID, chain.ChainMetrics{TotalSteps: len(steps), TotalTokens: len(steps)}); err != nil {
		t.Fatalf("UpdateChainMetrics returned error: %v", err)
	}
	svc := openOperatorTestService(t, t.TempDir(), store, &fakeBrainBackend{}, nil)

	report, err := svc.GetChainMetrics(ctx, chainID)
	if err != nil {
		t.Fatalf("GetChainMetrics returned error: %v", err)
	}
	if report.OpenFindingCount != 1 || report.ClosedFindingCount != 0 || report.AddressedFindingCount != 1 {
		t.Fatalf("finding counts = open %d closed %d addressed %d, want 1/0/1", report.OpenFindingCount, report.ClosedFindingCount, report.AddressedFindingCount)
	}
	if !reflect.DeepEqual(report.OpenFindingIDs, []string{"FIND-correctness-001"}) ||
		!reflect.DeepEqual(report.AddressedFindingIDs, []string{"FIND-correctness-001"}) ||
		!reflect.DeepEqual(report.ReopenedFindingIDs, []string{"FIND-correctness-001"}) ||
		!reflect.DeepEqual(report.RepeatedResolverFindingIDs, []string{"FIND-correctness-001"}) {
		t.Fatalf("finding ID summaries = %+v", report)
	}
	if len(report.FindingLifecycle) != 1 {
		t.Fatalf("FindingLifecycle = %+v, want one entry", report.FindingLifecycle)
	}
	if got := report.FindingLifecycle[0]; got.ID != "FIND-correctness-001" || got.SourceRole != "correctness-auditor" || got.Status != "addressed" || got.AddressedCount != 2 || got.ReopenedCount != 1 {
		t.Fatalf("FindingLifecycle[0] = %+v, want legacy lifecycle counts", got)
	}
	for _, want := range []string{
		"open audit findings: FIND-correctness-001",
		"reopened audit findings: FIND-correctness-001",
		"flow: repeated resolver loop for FIND-correctness-001 (2 resolver receipts)",
	} {
		if !hasRuntimeWarning(report.Warnings, want) {
			t.Fatalf("warnings = %+v, want %q", report.Warnings, want)
		}
	}
}

func TestGetChainMetricsSummarizesFindingLifecycleFacts(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newOperatorTestDB(t))
	chainID, err := store.StartChain(ctx, chain.ChainSpec{ChainID: "finding-lifecycle-facts-chain", SourceTask: "findings"})
	if err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	steps := []struct {
		id      string
		seq     int
		role    string
		verdict string
		event   map[string]any
	}{
		{id: "step-audit-open", seq: 1, role: "correctness-auditor", verdict: "fix_required", event: map[string]any{"role": "correctness-auditor", "verdict": "fix_required", "facts": []map[string]any{{"id": "FIND-correctness-001", "source_role": "correctness-auditor", "action": "opened", "status": "open", "severity": "high"}}}},
		{id: "step-resolver-one", seq: 2, role: "resolver", verdict: "completed", event: map[string]any{"role": "resolver", "verdict": "completed", "facts": []map[string]any{{"id": "FIND-correctness-001", "action": "addressed", "status": "addressed", "resolution": "fixed"}}}},
		{id: "step-audit-closed", seq: 3, role: "correctness-auditor", verdict: "completed", event: map[string]any{"role": "correctness-auditor", "verdict": "completed", "facts": []map[string]any{{"id": "FIND-correctness-001", "source_role": "correctness-auditor", "action": "closed", "status": "closed"}}}},
		{id: "step-audit-reopen", seq: 4, role: "correctness-auditor", verdict: "fix_required", event: map[string]any{"role": "correctness-auditor", "verdict": "fix_required", "facts": []map[string]any{{"id": "FIND-correctness-001", "source_role": "correctness-auditor", "action": "reopened", "status": "open"}}}},
		{id: "step-resolver-two", seq: 5, role: "resolver", verdict: "completed", event: map[string]any{"role": "resolver", "verdict": "completed", "facts": []map[string]any{{"id": "FIND-correctness-001", "action": "addressed", "status": "addressed", "resolution": "fixed"}}}},
	}
	for _, spec := range steps {
		stepID, err := store.StartStep(ctx, chain.StepSpec{StepID: spec.id, ChainID: chainID, SequenceNum: spec.seq, Role: spec.role, Task: spec.role})
		if err != nil {
			t.Fatalf("StartStep %s returned error: %v", spec.id, err)
		}
		if err := store.CompleteStep(ctx, chain.CompleteStepParams{StepID: stepID, Status: "completed", Verdict: spec.verdict, ReceiptPath: fmt.Sprintf("receipts/%s/finding-lifecycle-facts-chain-step-%03d.md", spec.role, spec.seq), TokensUsed: 1, TurnsUsed: 1}); err != nil {
			t.Fatalf("CompleteStep %s returned error: %v", spec.id, err)
		}
		if err := store.LogEvent(ctx, chainID, stepID, chain.EventFindingLifecycleFacts, spec.event); err != nil {
			t.Fatalf("LogEvent %s returned error: %v", spec.id, err)
		}
	}
	if err := store.UpdateChainMetrics(ctx, chainID, chain.ChainMetrics{TotalSteps: len(steps), TotalTokens: len(steps)}); err != nil {
		t.Fatalf("UpdateChainMetrics returned error: %v", err)
	}
	svc := openOperatorTestService(t, t.TempDir(), store, &fakeBrainBackend{}, nil)

	report, err := svc.GetChainMetrics(ctx, chainID)
	if err != nil {
		t.Fatalf("GetChainMetrics returned error: %v", err)
	}
	if report.FindingLifecycleFactEvents != 5 || report.ReceiptFindingEvents != 0 {
		t.Fatalf("finding event counts = lifecycle %d receipt %d, want 5/0", report.FindingLifecycleFactEvents, report.ReceiptFindingEvents)
	}
	if !reflect.DeepEqual(report.OpenFindingIDs, []string{"FIND-correctness-001"}) ||
		!reflect.DeepEqual(report.AddressedFindingIDs, []string{"FIND-correctness-001"}) ||
		!reflect.DeepEqual(report.ReopenedFindingIDs, []string{"FIND-correctness-001"}) ||
		!reflect.DeepEqual(report.RepeatedResolverFindingIDs, []string{"FIND-correctness-001"}) {
		t.Fatalf("finding ID summaries = %+v", report)
	}
	if len(report.FindingLifecycle) != 1 {
		t.Fatalf("FindingLifecycle = %+v, want one entry", report.FindingLifecycle)
	}
	if got := report.FindingLifecycle[0]; got.ID != "FIND-correctness-001" || got.SourceRole != "correctness-auditor" || got.Status != "addressed" || got.Severity != "high" || got.Resolution != "fixed" || got.AddressedCount != 2 || got.ReopenedCount != 1 {
		t.Fatalf("FindingLifecycle[0] = %+v, want merged lifecycle details", got)
	}
	detail, err := svc.GetChainDetail(ctx, chainID)
	if err != nil {
		t.Fatalf("GetChainDetail returned error: %v", err)
	}
	if len(detail.Guardrails.Findings) != 1 || detail.Guardrails.Findings[0].ID != "FIND-correctness-001" || detail.Guardrails.Findings[0].Severity != "high" {
		t.Fatalf("detail guardrail findings = %+v, want lifecycle detail", detail.Guardrails.Findings)
	}
	for _, want := range []string{
		"open audit findings: FIND-correctness-001",
		"reopened audit findings: FIND-correctness-001",
		"flow: repeated resolver loop for FIND-correctness-001 (2 resolver receipts)",
	} {
		if !hasRuntimeWarning(report.Warnings, want) {
			t.Fatalf("warnings = %+v, want %q", report.Warnings, want)
		}
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

func completeOperatorTestStep(t *testing.T, ctx context.Context, store *chain.Store, chainID string, sequence int, role string) string {
	t.Helper()
	stepID, err := store.StartStep(ctx, chain.StepSpec{ChainID: chainID, SequenceNum: sequence, Role: role, Task: role + " task"})
	if err != nil {
		t.Fatalf("StartStep %s returned error: %v", role, err)
	}
	if err := store.CompleteStep(ctx, chain.CompleteStepParams{
		StepID:       stepID,
		Status:       "completed",
		Verdict:      "completed",
		ReceiptPath:  fmt.Sprintf("receipts/%s/%s-step-%03d.md", role, chainID, sequence),
		TokensUsed:   10,
		TurnsUsed:    1,
		DurationSecs: 1,
	}); err != nil {
		t.Fatalf("CompleteStep %s returned error: %v", role, err)
	}
	return stepID
}

func hasRuntimeWarning(warnings []RuntimeWarning, want string) bool {
	for _, warning := range warnings {
		if strings.Contains(warning.Message, want) {
			return true
		}
	}
	return false
}

func equalStringSlices(got []string, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
