//go:build sqlite_fts5

package tool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ponchione/sodoryard/internal/provider"
)

func TestAdapterExecuteRecordsToolExecutionWhenContextMetaPresent(t *testing.T) {
	queries := setupTestDB(t)
	reg := NewRegistry()
	reg.Register(newMockTool("file_read", Pure))

	exec := NewExecutor(reg, ExecutorConfig{}, nil)
	exec.SetRecorder(NewToolExecutionRecorder(queries))
	adapter := NewAgentLoopAdapter(exec)

	ctx := ContextWithExecutionMeta(context.Background(), ExecutionMeta{
		ConversationID: "conv-1",
		TurnNumber:     1,
		Iteration:      2,
	})

	pr, err := adapter.Execute(ctx, provider.ToolCall{
		ID:    "tc-1",
		Name:  "file_read",
		Input: json.RawMessage(`{"path":"main.go"}`),
	})
	if err != nil {
		t.Fatalf("adapter execute error: %v", err)
	}
	if pr == nil || pr.IsError {
		t.Fatalf("adapter result = %+v, want successful provider tool result", pr)
	}

	rows, err := queries.GetConversationToolUsage(context.Background(), "conv-1")
	if err != nil {
		t.Fatalf("query tool usage: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 tool usage row, got %d", len(rows))
	}
	if rows[0].ToolName != "file_read" {
		t.Fatalf("tool_name = %q, want file_read", rows[0].ToolName)
	}
	if rows[0].CallCount != 1 {
		t.Fatalf("call_count = %d, want 1", rows[0].CallCount)
	}
}
