//go:build sqlite_fts5
// +build sqlite_fts5

package chain

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestApprovalsFromEventsMergesDecisions(t *testing.T) {
	started := time.Date(2026, 5, 9, 10, 0, 0, 0, time.UTC)
	decided := started.Add(time.Minute)
	events := []Event{
		{
			ID:        1,
			ChainID:   "chain-1",
			StepID:    "step-1",
			EventType: EventApprovalRequired,
			EventData: `{"approval_id":"approval-1","tool_name":"shell","status":"pending","reason":"matched policy","risk_level":"high","tool_input":{"command":"git push --force"},"created_at":"2026-05-09T10:00:00Z"}`,
			CreatedAt: started,
		},
		{
			ID:        2,
			ChainID:   "chain-1",
			StepID:    "step-1",
			EventType: EventApprovalDecision,
			EventData: `{"approval_id":"approval-1","status":"approved","reason":"reviewed","decided_by":"operator","decided_at":"2026-05-09T10:01:00Z"}`,
			CreatedAt: decided,
		},
		{
			ID:        3,
			ChainID:   "chain-1",
			StepID:    "step-1",
			EventType: EventApprovalRequired,
			EventData: `{"approval_id":"approval-1","tool_name":"shell","status":"pending"}`,
			CreatedAt: decided.Add(time.Minute),
		},
	}

	approvals := ApprovalsFromEvents(events)
	if len(approvals) != 1 {
		t.Fatalf("approvals = %+v, want one", approvals)
	}
	got := approvals[0]
	if got.ID != "approval-1" || got.Status != ApprovalStatusApproved || got.ToolName != "shell" || got.DecisionReason != "reviewed" || got.DecidedBy != "operator" {
		t.Fatalf("approval = %+v, want merged approved shell approval", got)
	}
	if got.DecidedAt == nil || !got.DecidedAt.Equal(decided) {
		t.Fatalf("DecidedAt = %v, want %s", got.DecidedAt, decided)
	}
	if !strings.Contains(string(got.ToolInput), "git push --force") {
		t.Fatalf("ToolInput = %s, want command payload", got.ToolInput)
	}
}

func TestStoreRecordsApprovalDecision(t *testing.T) {
	ctx := context.Background()
	store := NewStore(newChainTestDB(t))
	store.clock = func() time.Time { return time.Date(2026, 5, 9, 11, 0, 0, 0, time.UTC) }
	chainID, err := store.StartChain(ctx, ChainSpec{ChainID: "chain-approval", MaxSteps: 5, MaxResolverLoops: 1, MaxDuration: time.Hour, TokenBudget: 100})
	if err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, "", EventApprovalRequired, map[string]any{
		"approval_id": "approval-1",
		"tool_name":   "shell",
		"status":      "pending",
	}); err != nil {
		t.Fatalf("LogEvent returned error: %v", err)
	}

	approval, err := store.RecordApprovalDecision(ctx, chainID, ApprovalDecisionInput{ApprovalID: "approval-1", Status: ApprovalStatusDenied, Reason: "too risky", DecidedBy: "cli"})
	if err != nil {
		t.Fatalf("RecordApprovalDecision returned error: %v", err)
	}
	if approval.Status != ApprovalStatusDenied || approval.DecisionReason != "too risky" || approval.DecidedBy != "cli" {
		t.Fatalf("approval = %+v, want denied decision", approval)
	}
	pending, err := store.PendingApprovals(ctx, chainID)
	if err != nil {
		t.Fatalf("PendingApprovals returned error: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending = %+v, want none", pending)
	}
}
