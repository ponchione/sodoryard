//go:build sqlite_fts5
// +build sqlite_fts5

package chain

import (
	"context"
	"testing"
	"time"
)

func TestLogEventRoundTripsPayload(t *testing.T) {
	ctx := context.Background()
	db := newChainTestDB(t)
	store := NewStore(db)
	chainID, _ := store.StartChain(ctx, ChainSpec{MaxSteps: 2, MaxResolverLoops: 1, MaxDuration: time.Hour, TokenBudget: 10})
	stepID, _ := store.StartStep(ctx, StepSpec{ChainID: chainID, SequenceNum: 1, Role: "coder", Task: "do work"})
	if err := store.LogEvent(ctx, chainID, stepID, EventStepStarted, map[string]any{"role": "coder", "task": "do work"}); err != nil {
		t.Fatalf("LogEvent returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, "", EventChainCompleted, map[string]any{"status": "success"}); err != nil {
		t.Fatalf("LogEvent returned error: %v", err)
	}
	events, err := store.ListEvents(ctx, chainID)
	if err != nil {
		t.Fatalf("ListEvents returned error: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("event count = %d, want 2", len(events))
	}
	if events[0].EventType != EventStepStarted || events[0].StepID != stepID || events[0].EventData == "" {
		t.Fatalf("unexpected first event: %+v", events[0])
	}
	since, err := store.ListEventsSince(ctx, chainID, events[0].ID)
	if err != nil {
		t.Fatalf("ListEventsSince returned error: %v", err)
	}
	if len(since) != 1 || since[0].EventType != EventChainCompleted {
		t.Fatalf("unexpected events since: %+v", since)
	}
}
