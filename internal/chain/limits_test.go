//go:build sqlite_fts5
// +build sqlite_fts5

package chain

import (
	"context"
	"testing"
	"time"
)

func TestCheckLimitsRejectsNonRunningChain(t *testing.T) {
	ctx := context.Background()
	db := newChainTestDB(t)
	store := NewStore(db)
	chainID, _ := store.StartChain(ctx, ChainSpec{MaxSteps: 1, MaxResolverLoops: 1, MaxDuration: time.Hour, TokenBudget: 10})
	if err := store.SetChainStatus(ctx, chainID, "paused"); err != nil {
		t.Fatalf("SetChainStatus returned error: %v", err)
	}
	requireLimitError(t, store.CheckLimits(ctx, chainID, LimitCheckInput{}), ErrChainNotRunning)
}

func TestCheckLimitsRejectsMaxSteps(t *testing.T) {
	ctx := context.Background()
	db := newChainTestDB(t)
	store := NewStore(db)
	chainID, _ := store.StartChain(ctx, ChainSpec{MaxSteps: 1, MaxResolverLoops: 1, MaxDuration: time.Hour, TokenBudget: 10})
	if err := store.UpdateChainMetrics(ctx, chainID, ChainMetrics{TotalSteps: 1}); err != nil {
		t.Fatalf("UpdateChainMetrics returned error: %v", err)
	}
	requireLimitError(t, store.CheckLimits(ctx, chainID, LimitCheckInput{}), ErrMaxStepsExceeded)
}

func TestCheckLimitsRejectsTokenBudget(t *testing.T) {
	ctx := context.Background()
	db := newChainTestDB(t)
	store := NewStore(db)
	chainID, _ := store.StartChain(ctx, ChainSpec{MaxSteps: 2, MaxResolverLoops: 1, MaxDuration: time.Hour, TokenBudget: 10})
	if err := store.UpdateChainMetrics(ctx, chainID, ChainMetrics{TotalTokens: 10}); err != nil {
		t.Fatalf("UpdateChainMetrics returned error: %v", err)
	}
	requireLimitError(t, store.CheckLimits(ctx, chainID, LimitCheckInput{}), ErrTokenBudgetExceeded)
}

func TestCheckLimitsRejectsMaxDuration(t *testing.T) {
	ctx := context.Background()
	db := newChainTestDB(t)
	clockNow := time.Date(2026, 4, 11, 14, 0, 0, 0, time.UTC)
	store := StoreWithClock(db, func() time.Time { return clockNow })
	chainID, _ := store.StartChain(ctx, ChainSpec{MaxSteps: 2, MaxResolverLoops: 1, MaxDuration: time.Second, TokenBudget: 10})
	if _, err := db.ExecContext(ctx, `UPDATE chains SET started_at = ? WHERE id = ?`, clockNow.Add(-2*time.Second).Format(time.RFC3339), chainID); err != nil {
		t.Fatalf("set started_at returned error: %v", err)
	}
	requireLimitError(t, store.CheckLimits(ctx, chainID, LimitCheckInput{}), ErrMaxDurationExceeded)
}

func TestCheckLimitsRejectsResolverLoopCap(t *testing.T) {
	ctx := context.Background()
	db := newChainTestDB(t)
	store := NewStore(db)
	chainID, _ := store.StartChain(ctx, ChainSpec{MaxSteps: 5, MaxResolverLoops: 1, MaxDuration: time.Hour, TokenBudget: 10})
	stepID, _ := store.StartStep(ctx, StepSpec{ChainID: chainID, SequenceNum: 1, Role: "resolver", Task: "resolve", TaskContext: "task-1"})
	if err := store.CompleteStep(ctx, CompleteStepParams{StepID: stepID, Status: "completed"}); err != nil {
		t.Fatalf("CompleteStep returned error: %v", err)
	}
	requireLimitError(t, store.CheckLimits(ctx, chainID, LimitCheckInput{Role: "resolver", TaskContext: "task-1"}), ErrResolverLoopCapHit)
}
