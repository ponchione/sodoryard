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
