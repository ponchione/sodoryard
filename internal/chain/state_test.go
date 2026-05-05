//go:build sqlite_fts5
// +build sqlite_fts5

package chain

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	appdb "github.com/ponchione/sodoryard/internal/db"
	"github.com/ponchione/sodoryard/internal/projectmemory"
)

func TestStartChainCreatesRow(t *testing.T) {
	ctx := context.Background()
	db := newChainTestDB(t)
	store := StoreWithClock(db, func() time.Time { return time.Date(2026, 4, 11, 12, 0, 0, 0, time.UTC) })

	chainID, err := store.StartChain(ctx, ChainSpec{SourceSpecs: []string{"specs/auth.md"}, MaxSteps: 9, MaxResolverLoops: 2, MaxDuration: time.Hour, TokenBudget: 1234})
	if err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	if chainID == "" {
		t.Fatal("StartChain returned empty chain ID")
	}
	chain, err := store.GetChain(ctx, chainID)
	if err != nil {
		t.Fatalf("GetChain returned error: %v", err)
	}
	if chain.Status != "running" || chain.MaxSteps != 9 || chain.MaxResolverLoops != 2 || chain.TokenBudget != 1234 || len(chain.SourceSpecs) != 1 || chain.SourceSpecs[0] != "specs/auth.md" {
		t.Fatalf("unexpected chain: %+v", chain)
	}
}

func TestStartStepAndStepRunning(t *testing.T) {
	ctx := context.Background()
	db := newChainTestDB(t)
	store := NewStore(db)
	chainID, err := store.StartChain(ctx, ChainSpec{MaxSteps: 10, MaxResolverLoops: 2, MaxDuration: time.Hour, TokenBudget: 100})
	if err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	stepID, err := store.StartStep(ctx, StepSpec{ChainID: chainID, SequenceNum: 1, Role: "coder", Task: "do work", TaskContext: "task-1"})
	if err != nil {
		t.Fatalf("StartStep returned error: %v", err)
	}
	step, err := store.GetStep(ctx, stepID)
	if err != nil {
		t.Fatalf("GetStep returned error: %v", err)
	}
	if step.Status != "pending" {
		t.Fatalf("step status = %q, want pending", step.Status)
	}
	if err := store.StepRunning(ctx, stepID); err != nil {
		t.Fatalf("StepRunning returned error: %v", err)
	}
	step, err = store.GetStep(ctx, stepID)
	if err != nil {
		t.Fatalf("GetStep after running returned error: %v", err)
	}
	if step.Status != "running" || step.StartedAt == nil {
		t.Fatalf("unexpected running step: %+v", step)
	}
}

func TestCompleteStepUpdatesMetrics(t *testing.T) {
	ctx := context.Background()
	db := newChainTestDB(t)
	store := NewStore(db)
	chainID, _ := store.StartChain(ctx, ChainSpec{MaxSteps: 10, MaxResolverLoops: 2, MaxDuration: time.Hour, TokenBudget: 100})
	stepID, _ := store.StartStep(ctx, StepSpec{ChainID: chainID, SequenceNum: 1, Role: "coder", Task: "do work"})
	if err := store.StepRunning(ctx, stepID); err != nil {
		t.Fatalf("StepRunning returned error: %v", err)
	}
	exitCode := 0
	if err := store.CompleteStep(ctx, CompleteStepParams{StepID: stepID, Status: "completed", Verdict: "completed", ReceiptPath: "receipts/coder/x.md", TokensUsed: 12, TurnsUsed: 3, DurationSecs: 4, ExitCode: &exitCode}); err != nil {
		t.Fatalf("CompleteStep returned error: %v", err)
	}
	step, err := store.GetStep(ctx, stepID)
	if err != nil {
		t.Fatalf("GetStep returned error: %v", err)
	}
	if step.Status != "completed" || step.Verdict != "completed" || step.TokensUsed != 12 || step.TurnsUsed != 3 || step.DurationSecs != 4 || step.CompletedAt == nil {
		t.Fatalf("unexpected step: %+v", step)
	}
}

func TestProjectMemoryStoreRoundTripsChainState(t *testing.T) {
	ctx := context.Background()
	backend, err := projectmemory.OpenBrainBackend(ctx, projectmemory.Config{DataDir: filepath.Join(t.TempDir(), "memory"), DurableAck: true})
	if err != nil {
		t.Fatalf("OpenBrainBackend: %v", err)
	}
	defer backend.Close()
	store := NewProjectMemoryStore(backend)

	chainID, err := store.StartChain(ctx, ChainSpec{
		ChainID:          "pm-chain",
		SourceSpecs:      []string{"specs/chain.md"},
		SourceTask:       "persist chain",
		MaxSteps:         4,
		MaxResolverLoops: 2,
		MaxDuration:      time.Hour,
		TokenBudget:      500,
	})
	if err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	stepID, err := store.StartStep(ctx, StepSpec{StepID: "pm-step", ChainID: chainID, SequenceNum: 1, Role: "resolver", Task: "resolve", TaskContext: "ctx-a"})
	if err != nil {
		t.Fatalf("StartStep returned error: %v", err)
	}
	if err := store.StepRunning(ctx, stepID); err != nil {
		t.Fatalf("StepRunning returned error: %v", err)
	}
	exitCode := 0
	if err := store.CompleteStep(ctx, CompleteStepParams{StepID: stepID, Status: "completed", Verdict: "completed", ReceiptPath: "receipts/resolver/pm-chain-step-001.md", TokensUsed: 21, TurnsUsed: 2, DurationSecs: 6, ExitCode: &exitCode}); err != nil {
		t.Fatalf("CompleteStep returned error: %v", err)
	}
	if err := store.UpdateChainMetrics(ctx, chainID, ChainMetrics{TotalSteps: 1, TotalTokens: 21, TotalDurationSecs: 6, ResolverLoops: 1}); err != nil {
		t.Fatalf("UpdateChainMetrics returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, stepID, EventStepCompleted, map[string]any{"verdict": "completed"}); err != nil {
		t.Fatalf("LogEvent returned error: %v", err)
	}

	got, err := store.GetChain(ctx, chainID)
	if err != nil {
		t.Fatalf("GetChain returned error: %v", err)
	}
	if got.SourceTask != "persist chain" || got.MaxSteps != 4 || got.TotalTokens != 21 || got.ResolverLoops != 1 || len(got.SourceSpecs) != 1 {
		t.Fatalf("chain = %+v, want Shunter-backed state", got)
	}
	steps, err := store.ListSteps(ctx, chainID)
	if err != nil {
		t.Fatalf("ListSteps returned error: %v", err)
	}
	if len(steps) != 1 || steps[0].ID != stepID || steps[0].ExitCode == nil || *steps[0].ExitCode != 0 {
		t.Fatalf("steps = %+v, want completed Shunter step", steps)
	}
	count, err := store.CountResolverStepsForContext(ctx, chainID, "ctx-a")
	if err != nil {
		t.Fatalf("CountResolverStepsForContext returned error: %v", err)
	}
	if count != 1 {
		t.Fatalf("resolver count = %d, want 1", count)
	}
	events, err := store.ListEventsSince(ctx, chainID, 0)
	if err != nil {
		t.Fatalf("ListEventsSince returned error: %v", err)
	}
	if len(events) != 1 || events[0].ID != 1 || events[0].EventType != EventStepCompleted {
		t.Fatalf("events = %+v, want step_completed sequence 1", events)
	}
}

func TestFailStepSetsFailedState(t *testing.T) {
	ctx := context.Background()
	db := newChainTestDB(t)
	store := NewStore(db)
	chainID, _ := store.StartChain(ctx, ChainSpec{MaxSteps: 10, MaxResolverLoops: 2, MaxDuration: time.Hour, TokenBudget: 100})
	stepID, _ := store.StartStep(ctx, StepSpec{ChainID: chainID, SequenceNum: 1, Role: "coder", Task: "do work"})
	if err := store.FailStep(ctx, CompleteStepParams{StepID: stepID, Verdict: "escalate", ErrorMessage: "boom", DurationSecs: 1}); err != nil {
		t.Fatalf("FailStep returned error: %v", err)
	}
	step, err := store.GetStep(ctx, stepID)
	if err != nil {
		t.Fatalf("GetStep returned error: %v", err)
	}
	if step.Status != "failed" || step.ErrorMessage != "boom" {
		t.Fatalf("unexpected failed step: %+v", step)
	}
}

func TestConcurrentChainAccessDifferentChains(t *testing.T) {
	ctx := context.Background()
	db := newChainTestDB(t)
	store := NewStore(db)
	chainA, _ := store.StartChain(ctx, ChainSpec{MaxSteps: 10, MaxResolverLoops: 2, MaxDuration: time.Hour, TokenBudget: 100})
	chainB, _ := store.StartChain(ctx, ChainSpec{MaxSteps: 10, MaxResolverLoops: 2, MaxDuration: time.Hour, TokenBudget: 100})

	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	for i, chainID := range []string{chainA, chainB} {
		wg.Add(1)
		go func(chainID string, n int) {
			defer wg.Done()
			stepID, err := store.StartStep(ctx, StepSpec{ChainID: chainID, SequenceNum: 1, Role: "coder", Task: "work"})
			if err != nil {
				errCh <- err
				return
			}
			exitCode := n
			if err := store.CompleteStep(ctx, CompleteStepParams{StepID: stepID, Status: "completed", Verdict: "completed", TokensUsed: n + 1, ExitCode: &exitCode}); err != nil {
				errCh <- err
			}
		}(chainID, i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent update returned error: %v", err)
		}
	}
}

func newChainTestDB(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "chain.db")
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

func requireLimitError(t *testing.T, err error, target error) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("error = %v, want %v", err, target)
	}
}
