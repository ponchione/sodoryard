package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ponchione/sirtopham/internal/provider"
)

func TestAdapterConvertsTypes(t *testing.T) {
	reg := NewRegistry()
	m := newMockTool("file_read", Pure)
	m.executeFn = func(ctx context.Context, projectRoot string, input json.RawMessage) (*ToolResult, error) {
		// Verify the projectRoot was passed through.
		if projectRoot != "/tmp/project" {
			t.Errorf("projectRoot = %q, want /tmp/project", projectRoot)
		}
		return &ToolResult{
			Success: true,
			Content: "file contents here",
		}, nil
	}
	reg.Register(m)

	exec := NewExecutor(reg, ExecutorConfig{ProjectRoot: "/tmp/project"}, nil)
	adapter := NewAgentLoopAdapter(exec)

	// Call with provider types.
	pr, err := adapter.Execute(context.Background(), provider.ToolCall{
		ID:    "tc-1",
		Name:  "file_read",
		Input: json.RawMessage(`{"path":"main.go"}`),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pr.ToolUseID != "tc-1" {
		t.Fatalf("ToolUseID = %q, want tc-1", pr.ToolUseID)
	}
	if pr.Content != "file contents here" {
		t.Fatalf("Content = %q, want 'file contents here'", pr.Content)
	}
	if pr.IsError {
		t.Fatal("IsError = true, want false")
	}
}

func TestAdapterUnknownTool(t *testing.T) {
	reg := NewRegistry()
	reg.Register(newMockTool("file_read", Pure))

	exec := NewExecutor(reg, ExecutorConfig{}, nil)
	adapter := NewAgentLoopAdapter(exec)

	pr, err := adapter.Execute(context.Background(), provider.ToolCall{
		ID:    "tc-1",
		Name:  "nonexistent",
		Input: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !pr.IsError {
		t.Fatal("IsError = false, want true for unknown tool")
	}
	if !strings.Contains(pr.Content, "Unknown tool") {
		t.Fatalf("Content should mention 'Unknown tool', got: %s", pr.Content)
	}
}

func TestAdapterToolFailure(t *testing.T) {
	reg := NewRegistry()
	m := newMockTool("failing_tool", Pure)
	m.executeFn = func(ctx context.Context, _ string, _ json.RawMessage) (*ToolResult, error) {
		return &ToolResult{
			Success: false,
			Content: "permission denied: /etc/shadow",
			Error:   "permission denied",
		}, nil
	}
	reg.Register(m)

	exec := NewExecutor(reg, ExecutorConfig{}, nil)
	adapter := NewAgentLoopAdapter(exec)

	pr, err := adapter.Execute(context.Background(), provider.ToolCall{
		ID:    "tc-1",
		Name:  "failing_tool",
		Input: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !pr.IsError {
		t.Fatal("IsError = false, want true for failed tool")
	}
	if !strings.Contains(pr.Content, "permission denied") {
		t.Fatalf("Content = %q, want to contain 'permission denied'", pr.Content)
	}
}
