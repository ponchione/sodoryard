package tool

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/ponchione/sodoryard/internal/approval"
	tracepkg "github.com/ponchione/sodoryard/internal/trace"
)

type toolHookStub struct {
	name      string
	events    *[]string
	beforeErr error
	afterErr  error
}

func (h toolHookStub) BeforeTool(ctx context.Context, call ToolCall, def Tool) (context.Context, error) {
	*h.events = append(*h.events, "before:"+h.name+":"+call.Name+":"+def.ToolPurity().String())
	if h.beforeErr != nil {
		return ctx, h.beforeErr
	}
	return ctx, nil
}

func (h toolHookStub) AfterTool(ctx context.Context, call ToolCall, result ToolResult) (ToolResult, error) {
	*h.events = append(*h.events, "after:"+h.name+":"+call.ID)
	return result, h.afterErr
}

func TestExecutorRunsToolHooksInAroundOrder(t *testing.T) {
	reg := NewRegistry()
	reg.Register(newMockTool("file_read", Pure))
	var events []string
	exec := NewExecutor(reg, ExecutorConfig{}, nil)
	exec.SetHooks(
		toolHookStub{name: "a", events: &events},
		toolHookStub{name: "b", events: &events},
	)

	results := exec.Execute(context.Background(), []ToolCall{
		{ID: "tc-1", Name: "file_read", Arguments: json.RawMessage(`{}`)},
	})
	if len(results) != 1 || !results[0].Success {
		t.Fatalf("results = %+v, want one success", results)
	}
	want := []string{"before:a:file_read:pure", "before:b:file_read:pure", "after:b:tc-1", "after:a:tc-1"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
}

func TestExecutorToolHookBeforeErrorFailsClosed(t *testing.T) {
	reg := NewRegistry()
	tool := newMockTool("file_write", Mutating)
	var executed bool
	tool.executeFn = func(ctx context.Context, _ string, _ json.RawMessage) (*ToolResult, error) {
		executed = true
		return &ToolResult{Success: true, Content: "done"}, nil
	}
	reg.Register(tool)
	blocked := errors.New("requires approval")
	var events []string
	exec := NewExecutor(reg, ExecutorConfig{}, nil)
	exec.SetHooks(
		toolHookStub{name: "audit", events: &events},
		toolHookStub{name: "approval", events: &events, beforeErr: blocked},
	)

	results := exec.Execute(context.Background(), []ToolCall{
		{ID: "tc-1", Name: "file_write", Arguments: json.RawMessage(`{}`)},
	})
	if executed {
		t.Fatal("tool executed despite before hook error")
	}
	if len(results) != 1 || results[0].Success || results[0].Error != blocked.Error() {
		t.Fatalf("results = %+v, want blocked failure", results)
	}
	want := []string{"before:audit:file_write:mutating", "before:approval:file_write:mutating", "after:audit:tc-1"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
}

func TestExecutorToolHookAfterErrorFailsClosed(t *testing.T) {
	reg := NewRegistry()
	reg.Register(newMockTool("file_read", Pure))
	afterErr := errors.New("postcheck failed")
	var events []string
	exec := NewExecutor(reg, ExecutorConfig{}, nil)
	exec.SetHooks(toolHookStub{name: "postcheck", events: &events, afterErr: afterErr})

	results := exec.Execute(context.Background(), []ToolCall{
		{ID: "tc-1", Name: "file_read", Arguments: json.RawMessage(`{}`)},
	})
	if len(results) != 1 || results[0].Success || results[0].Error != afterErr.Error() {
		t.Fatalf("results = %+v, want hook failure", results)
	}
	if !reflect.DeepEqual(events, []string{"before:postcheck:file_read:pure", "after:postcheck:tc-1"}) {
		t.Fatalf("events = %v", events)
	}
}

func TestExecutorShellApprovalHookBlocksBeforeExecutionWithMetadata(t *testing.T) {
	reg := NewRegistry()
	shellTool := newMockTool("shell", Mutating)
	var executed bool
	shellTool.executeFn = func(ctx context.Context, _ string, _ json.RawMessage) (*ToolResult, error) {
		executed = true
		return &ToolResult{Success: true, Content: "ran"}, nil
	}
	reg.Register(shellTool)
	exec := NewExecutor(reg, ExecutorConfig{ShellApprovalPatterns: []string{"git push --force"}}, nil)

	ctx := tracepkg.ContextWithScope(context.Background(), tracepkg.Scope{
		ConversationID: "conv-approval",
		ChainID:        "chain-approval",
		StepID:         "step-approval",
		TurnNumber:     2,
		Iteration:      3,
	})
	results := exec.Execute(ctx, []ToolCall{
		{ID: "tc-approval", Name: "shell", Arguments: json.RawMessage(`{"command":"git push --force origin main"}`)},
	})
	if executed {
		t.Fatal("shell executed despite approval requirement")
	}
	if len(results) != 1 || results[0].Success || results[0].Error != ErrApprovalRequired.Error() {
		t.Fatalf("results = %+v, want approval-required failure", results)
	}
	if !strings.Contains(results[0].Content, "Approval required") || !strings.Contains(results[0].Content, "git push --force") {
		t.Fatalf("content = %q, want approval explanation", results[0].Content)
	}
	details := decodeToolResultDetails(t, results[0].Details)
	if details["kind"] != "approval_required" || details["approval_id"] != "approval-tc-approval" || details["status"] != ApprovalStatusPending {
		t.Fatalf("details = %#v, want approval metadata", details)
	}
	if details["tool_name"] != "shell" || details["risk_level"] != ApprovalRiskHigh {
		t.Fatalf("details = %#v, want shell high-risk metadata", details)
	}
	if details["chain_id"] != "chain-approval" || details["step_id"] != "step-approval" || details["conversation_id"] != "conv-approval" || detailInt(t, details, "turn_number") != 2 || detailInt(t, details, "iteration") != 3 {
		t.Fatalf("details = %#v, want trace scope metadata", details)
	}
}

func TestExecutorShellApprovalHookAllowsNonMatchingShellCommand(t *testing.T) {
	reg := NewRegistry()
	shellTool := newMockTool("shell", Mutating)
	shellTool.executeFn = func(ctx context.Context, _ string, _ json.RawMessage) (*ToolResult, error) {
		return &ToolResult{Success: true, Content: "ran"}, nil
	}
	reg.Register(shellTool)
	exec := NewExecutor(reg, ExecutorConfig{ShellApprovalPatterns: []string{"git push --force"}}, nil)

	results := exec.Execute(context.Background(), []ToolCall{
		{ID: "tc-ok", Name: "shell", Arguments: json.RawMessage(`{"command":"git status --short"}`)},
	})
	if len(results) != 1 || !results[0].Success || results[0].Content != "ran" {
		t.Fatalf("results = %+v, want shell command to execute", results)
	}
}

func TestExecutorShellApprovalHookAllowsApprovedMatchingInput(t *testing.T) {
	reg := NewRegistry()
	shellTool := newMockTool("shell", Mutating)
	var executed bool
	shellTool.executeFn = func(ctx context.Context, _ string, _ json.RawMessage) (*ToolResult, error) {
		executed = true
		return &ToolResult{Success: true, Content: "ran approved command"}, nil
	}
	reg.Register(shellTool)
	exec := NewExecutor(reg, ExecutorConfig{
		ShellApprovalPatterns: []string{"git push --force"},
		ApprovalDecisions: []approval.Decision{{
			ID:        "approval-original",
			ToolName:  "shell",
			ToolInput: json.RawMessage(`{"command":"git push --force origin main"}`),
			Status:    ApprovalStatusApproved,
			Reason:    "reviewed",
		}},
	}, nil)

	results := exec.Execute(context.Background(), []ToolCall{
		{ID: "tc-retry", Name: "shell", Arguments: json.RawMessage(`{"command":"git push --force origin main"}`)},
	})
	if !executed {
		t.Fatal("approved shell command did not execute")
	}
	if len(results) != 1 || !results[0].Success || results[0].Content != "ran approved command" {
		t.Fatalf("results = %+v, want approved command to execute", results)
	}
}

func TestExecutorShellApprovalHookDeniesDeniedMatchingInput(t *testing.T) {
	reg := NewRegistry()
	shellTool := newMockTool("shell", Mutating)
	var executed bool
	shellTool.executeFn = func(ctx context.Context, _ string, _ json.RawMessage) (*ToolResult, error) {
		executed = true
		return &ToolResult{Success: true, Content: "ran"}, nil
	}
	reg.Register(shellTool)
	exec := NewExecutor(reg, ExecutorConfig{
		ShellApprovalPatterns: []string{"git push --force"},
		ApprovalDecisions: []approval.Decision{{
			ID:        "approval-denied",
			ToolName:  "shell",
			ToolInput: json.RawMessage(`{"command":"git push --force origin main"}`),
			Status:    ApprovalStatusDenied,
			Reason:    "too risky",
		}},
	}, nil)

	results := exec.Execute(context.Background(), []ToolCall{
		{ID: "tc-retry", Name: "shell", Arguments: json.RawMessage(`{"command":"git push --force origin main"}`)},
	})
	if executed {
		t.Fatal("denied shell command executed")
	}
	if len(results) != 1 || results[0].Success || results[0].Error != ErrApprovalDenied.Error() {
		t.Fatalf("results = %+v, want approval denied failure", results)
	}
	if !strings.Contains(results[0].Content, "Approval denied") || !strings.Contains(results[0].Content, "too risky") {
		t.Fatalf("content = %q, want denial reason", results[0].Content)
	}
	details := decodeToolResultDetails(t, results[0].Details)
	if details["kind"] != approval.KindDenied || details["approval_id"] != "approval-denied" || details["status"] != ApprovalStatusDenied {
		t.Fatalf("details = %#v, want approval denied metadata", details)
	}
}
