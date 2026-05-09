package chain

import (
	"context"
	"testing"
)

func TestNextControlStatus(t *testing.T) {
	tests := []struct {
		name   string
		cur    string
		target string
		want   string
	}{
		{name: "pause running becomes requested", cur: "running", target: "paused", want: "pause_requested"},
		{name: "cancel running becomes requested", cur: "running", target: "cancelled", want: "cancel_requested"},
		{name: "cancel waiting approval becomes requested", cur: StatusWaitingApproval, target: "cancelled", want: "cancel_requested"},
		{name: "resume waiting approval becomes running", cur: StatusWaitingApproval, target: "running", want: "running"},
		{name: "cancel paused is immediate", cur: "paused", target: "cancelled", want: "cancelled"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NextControlStatus(tc.cur, tc.target)
			if err != nil {
				t.Fatalf("NextControlStatus() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("NextControlStatus() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNextControlStatusRejectsResumeWhilePauseStillRequested(t *testing.T) {
	if got, err := NextControlStatus("pause_requested", "running"); err == nil || got != "" {
		t.Fatalf("NextControlStatus(pause_requested, running) = (%q, %v), want error", got, err)
	}
}

func TestFinalizeControlStatus(t *testing.T) {
	if got, ok := FinalizeControlStatus("pause_requested"); !ok || got != "paused" {
		t.Fatalf("FinalizeControlStatus(pause_requested) = (%q, %t), want (paused, true)", got, ok)
	}
	if got, ok := FinalizeControlStatus("cancel_requested"); !ok || got != "cancelled" {
		t.Fatalf("FinalizeControlStatus(cancel_requested) = (%q, %t), want (cancelled, true)", got, ok)
	}
	if got, ok := FinalizeControlStatus("running"); ok || got != "" {
		t.Fatalf("FinalizeControlStatus(running) = (%q, %t), want (\"\", false)", got, ok)
	}
}

func TestFinalizeControlEventType(t *testing.T) {
	if got, ok := FinalizeControlEventType("pause_requested"); !ok || got != EventChainPaused {
		t.Fatalf("FinalizeControlEventType(pause_requested) = (%q, %t), want (%q, true)", got, ok, EventChainPaused)
	}
	if got, ok := FinalizeControlEventType("cancel_requested"); !ok || got != EventChainCancelled {
		t.Fatalf("FinalizeControlEventType(cancel_requested) = (%q, %t), want (%q, true)", got, ok, EventChainCancelled)
	}
	if got, ok := FinalizeControlEventType("running"); ok || got != "" {
		t.Fatalf("FinalizeControlEventType(running) = (%q, %t), want (\"\", false)", got, ok)
	}
}

func TestShouldStopScheduling(t *testing.T) {
	for _, status := range []string{"paused", "cancelled", "pause_requested", "cancel_requested", StatusWaitingApproval} {
		if !ShouldStopScheduling(status) {
			t.Fatalf("ShouldStopScheduling(%q) = false, want true", status)
		}
	}
	if ShouldStopScheduling("running") {
		t.Fatal("ShouldStopScheduling(running) = true, want false")
	}
}

func TestBuildTerminalEventPayloadIncludesStatusAndExecutionID(t *testing.T) {
	events := []Event{
		{EventType: EventChainStarted, EventData: `{"orchestrator_pid":1111,"execution_id":"exec-1","active_execution":true}`},
	}
	payload := BuildTerminalEventPayload(events, "paused", map[string]any{"finalized_from": "pause_requested"})
	if payload["status"] != "paused" {
		t.Fatalf("status payload = %#v, want paused", payload["status"])
	}
	if payload["execution_id"] != "exec-1" {
		t.Fatalf("execution_id payload = %#v, want exec-1", payload["execution_id"])
	}
	if payload["finalized_from"] != "pause_requested" {
		t.Fatalf("finalized_from payload = %#v, want pause_requested", payload["finalized_from"])
	}
}

func TestBuildTerminalEventPayloadOmitsExecutionIDWithoutActiveExecution(t *testing.T) {
	payload := BuildTerminalEventPayload(nil, "failed", map[string]any{"summary": "boom"})
	if payload["status"] != "failed" {
		t.Fatalf("status payload = %#v, want failed", payload["status"])
	}
	if _, ok := payload["execution_id"]; ok {
		t.Fatalf("payload = %#v, want no execution_id", payload)
	}
	if payload["summary"] != "boom" {
		t.Fatalf("summary payload = %#v, want boom", payload["summary"])
	}
}

func TestApplyTerminalChainClosureUpdatesStatusAndLogsTerminalEvent(t *testing.T) {
	ctx := context.Background()
	store := NewStore(newChainTestDB(t))
	chainID, err := store.StartChain(ctx, ChainSpec{MaxSteps: 5, MaxResolverLoops: 1, MaxDuration: 1, TokenBudget: 10})
	if err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, "", EventChainStarted, map[string]any{"orchestrator_pid": 1111, "execution_id": "exec-1", "active_execution": true}); err != nil {
		t.Fatalf("LogEvent returned error: %v", err)
	}

	if err := ApplyTerminalChainClosure(ctx, store, chainID, TerminalChainClosure{Status: "paused", EventType: EventChainPaused, Extra: map[string]any{"finalized_from": "pause_requested"}}); err != nil {
		t.Fatalf("ApplyTerminalChainClosure returned error: %v", err)
	}

	ch, err := store.GetChain(ctx, chainID)
	if err != nil {
		t.Fatalf("GetChain returned error: %v", err)
	}
	if ch.Status != "paused" {
		t.Fatalf("status = %q, want paused", ch.Status)
	}
	events, err := store.ListEvents(ctx, chainID)
	if err != nil {
		t.Fatalf("ListEvents returned error: %v", err)
	}
	if len(events) != 2 || events[1].EventType != EventChainPaused {
		t.Fatalf("events = %+v, want start + one %s event", events, EventChainPaused)
	}
	if events[1].EventData != `{"execution_id":"exec-1","finalized_from":"pause_requested","status":"paused"}` {
		t.Fatalf("EventData = %s, want stable paused payload", events[1].EventData)
	}
}

func TestApplyTerminalChainClosureCompletesChainWithSummary(t *testing.T) {
	ctx := context.Background()
	store := NewStore(newChainTestDB(t))
	chainID, err := store.StartChain(ctx, ChainSpec{MaxSteps: 5, MaxResolverLoops: 1, MaxDuration: 1, TokenBudget: 10})
	if err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, "", EventChainStarted, map[string]any{"orchestrator_pid": 1111, "execution_id": "exec-1", "active_execution": true}); err != nil {
		t.Fatalf("LogEvent returned error: %v", err)
	}

	summary := "done"
	if err := ApplyTerminalChainClosure(ctx, store, chainID, TerminalChainClosure{Status: "completed", EventType: EventChainCompleted, Summary: &summary, Extra: map[string]any{"summary": summary}}); err != nil {
		t.Fatalf("ApplyTerminalChainClosure returned error: %v", err)
	}

	ch, err := store.GetChain(ctx, chainID)
	if err != nil {
		t.Fatalf("GetChain returned error: %v", err)
	}
	if ch.Status != "completed" || ch.Summary != summary {
		t.Fatalf("chain = %+v, want completed with summary", ch)
	}
	events, err := store.ListEvents(ctx, chainID)
	if err != nil {
		t.Fatalf("ListEvents returned error: %v", err)
	}
	if len(events) != 2 || events[1].EventType != EventChainCompleted {
		t.Fatalf("events = %+v, want start + one %s event", events, EventChainCompleted)
	}
	if events[1].EventData != `{"execution_id":"exec-1","status":"completed","summary":"done"}` {
		t.Fatalf("EventData = %s, want stable completed payload", events[1].EventData)
	}
}

func TestLatestActiveExecutionSkipsTerminalizedExecutionID(t *testing.T) {
	ctx := context.Background()
	store := NewStore(newChainTestDB(t))
	chainID, err := store.StartChain(ctx, ChainSpec{MaxSteps: 5, MaxResolverLoops: 1, MaxDuration: 1, TokenBudget: 10})
	if err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, "", EventChainStarted, map[string]any{"orchestrator_pid": 1111, "execution_id": "exec-1", "active_execution": true}); err != nil {
		t.Fatalf("LogEvent returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, "", EventChainCompleted, map[string]any{"execution_id": "exec-1"}); err != nil {
		t.Fatalf("LogEvent returned error: %v", err)
	}

	events, err := store.ListEvents(ctx, chainID)
	if err != nil {
		t.Fatalf("ListEvents returned error: %v", err)
	}
	if exec, ok := LatestActiveExecution(events); ok || exec.ExecutionID != "" || exec.OrchestratorPID != 0 {
		t.Fatalf("LatestActiveExecution() = (%+v, %t), want empty,false", exec, ok)
	}
}

func TestLatestActiveExecutionReturnsLatestNonTerminalRegistration(t *testing.T) {
	ctx := context.Background()
	store := NewStore(newChainTestDB(t))
	chainID, err := store.StartChain(ctx, ChainSpec{MaxSteps: 5, MaxResolverLoops: 1, MaxDuration: 1, TokenBudget: 10})
	if err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, "", EventChainStarted, map[string]any{"orchestrator_pid": 1111, "execution_id": "exec-1", "active_execution": true}); err != nil {
		t.Fatalf("LogEvent returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, "", EventChainCompleted, map[string]any{"execution_id": "exec-1"}); err != nil {
		t.Fatalf("LogEvent returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, "", EventChainResumed, map[string]any{"orchestrator_pid": 2222, "execution_id": "exec-2", "active_execution": true}); err != nil {
		t.Fatalf("LogEvent returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, "", EventChainPaused, map[string]any{"orchestrator_pid": 9999}); err != nil {
		t.Fatalf("LogEvent returned error: %v", err)
	}

	events, err := store.ListEvents(ctx, chainID)
	if err != nil {
		t.Fatalf("ListEvents returned error: %v", err)
	}
	exec, ok := LatestActiveExecution(events)
	if !ok {
		t.Fatal("LatestActiveExecution() ok = false, want true")
	}
	if exec.ExecutionID != "exec-2" || exec.OrchestratorPID != 2222 {
		t.Fatalf("LatestActiveExecution() = %+v, want exec-2/2222", exec)
	}
}

func TestLatestActiveExecutionAllowsInProcessExecutionWithoutSignalPID(t *testing.T) {
	ctx := context.Background()
	store := NewStore(newChainTestDB(t))
	chainID, err := store.StartChain(ctx, ChainSpec{MaxSteps: 5, MaxResolverLoops: 1, MaxDuration: 1, TokenBudget: 10})
	if err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, "", EventChainStarted, map[string]any{"orchestrator_pid": 0, "execution_id": "exec-embedded", "active_execution": true}); err != nil {
		t.Fatalf("LogEvent returned error: %v", err)
	}

	events, err := store.ListEvents(ctx, chainID)
	if err != nil {
		t.Fatalf("ListEvents returned error: %v", err)
	}
	exec, ok := LatestActiveExecution(events)
	if !ok {
		t.Fatal("LatestActiveExecution() ok = false, want true")
	}
	if exec.ExecutionID != "exec-embedded" || exec.OrchestratorPID != 0 {
		t.Fatalf("LatestActiveExecution() = %+v, want exec-embedded/0", exec)
	}
}

func TestLatestActiveStepProcessSkipsExitedProcess(t *testing.T) {
	ctx := context.Background()
	store := NewStore(newChainTestDB(t))
	chainID, err := store.StartChain(ctx, ChainSpec{MaxSteps: 5, MaxResolverLoops: 1, MaxDuration: 1, TokenBudget: 10})
	if err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	stepID, err := store.StartStep(ctx, StepSpec{ChainID: chainID, SequenceNum: 1, Role: "coder", Task: "do work"})
	if err != nil {
		t.Fatalf("StartStep returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, stepID, EventStepProcessStarted, map[string]any{"process_id": 1111, "active_process": true}); err != nil {
		t.Fatalf("LogEvent returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, stepID, EventStepProcessExited, map[string]any{"process_id": 1111, "exit_code": 0}); err != nil {
		t.Fatalf("LogEvent returned error: %v", err)
	}

	events, err := store.ListEvents(ctx, chainID)
	if err != nil {
		t.Fatalf("ListEvents returned error: %v", err)
	}
	if proc, ok := LatestActiveStepProcess(events); ok || proc.ProcessID != 0 {
		t.Fatalf("LatestActiveStepProcess() = (%+v, %t), want empty,false", proc, ok)
	}
}

func TestLatestActiveStepProcessReturnsLatestRunningProcess(t *testing.T) {
	ctx := context.Background()
	store := NewStore(newChainTestDB(t))
	chainID, err := store.StartChain(ctx, ChainSpec{MaxSteps: 5, MaxResolverLoops: 1, MaxDuration: 1, TokenBudget: 10})
	if err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	firstStepID, err := store.StartStep(ctx, StepSpec{ChainID: chainID, SequenceNum: 1, Role: "coder", Task: "do work"})
	if err != nil {
		t.Fatalf("StartStep returned error: %v", err)
	}
	secondStepID, err := store.StartStep(ctx, StepSpec{ChainID: chainID, SequenceNum: 2, Role: "auditor", Task: "audit work"})
	if err != nil {
		t.Fatalf("StartStep returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, firstStepID, EventStepProcessStarted, map[string]any{"process_id": 1111, "active_process": true}); err != nil {
		t.Fatalf("LogEvent returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, firstStepID, EventStepCompleted, map[string]any{"verdict": "completed"}); err != nil {
		t.Fatalf("LogEvent returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, secondStepID, EventStepProcessStarted, map[string]any{"process_id": 2222, "active_process": true}); err != nil {
		t.Fatalf("LogEvent returned error: %v", err)
	}

	events, err := store.ListEvents(ctx, chainID)
	if err != nil {
		t.Fatalf("ListEvents returned error: %v", err)
	}
	proc, ok := LatestActiveStepProcess(events)
	if !ok {
		t.Fatal("LatestActiveStepProcess() ok = false, want true")
	}
	if proc.StepID != secondStepID || proc.ProcessID != 2222 {
		t.Fatalf("LatestActiveStepProcess() = %+v, want %s/2222", proc, secondStepID)
	}
}
