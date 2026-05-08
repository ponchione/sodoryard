package tool

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
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
