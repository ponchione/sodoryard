//go:build sqlite_fts5

package tool

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/ponchione/sodoryard/internal/db"
	"github.com/ponchione/sodoryard/internal/projectmemory"
)

func setupTestDB(t *testing.T) *db.Queries {
	t.Helper()
	sqlDB, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	if err := db.Init(context.Background(), sqlDB); err != nil {
		t.Fatalf("init schema: %v", err)
	}

	// Insert a project and conversation for FK references.
	_, err = sqlDB.Exec(`INSERT INTO projects(id, name, root_path, created_at, updated_at)
		VALUES ('proj-1', 'test', '/tmp/test', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	_, err = sqlDB.Exec(`INSERT INTO conversations(id, project_id, title, model, provider, created_at, updated_at)
		VALUES ('conv-1', 'proj-1', 'test', 'test-model', 'test-provider', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("insert conversation: %v", err)
	}

	return db.New(sqlDB)
}

func TestExecuteWithMetaPersistence(t *testing.T) {
	queries := setupTestDB(t)
	reg := NewRegistry()
	reg.Register(newMockTool("file_read", Pure))

	exec := NewExecutor(reg, ExecutorConfig{}, nil)
	exec.SetRecorder(NewToolExecutionRecorder(queries))

	calls := []ToolCall{
		{ID: "tc-1", Name: "file_read", Arguments: json.RawMessage(`{"path":"main.go"}`)},
	}
	meta := ExecutionMeta{
		ConversationID: "conv-1",
		TurnNumber:     1,
		Iteration:      1,
	}

	results := exec.ExecuteWithMeta(context.Background(), calls, meta)
	if len(results) != 1 || !results[0].Success {
		t.Fatalf("unexpected result: %+v", results)
	}

	// Verify the tool_executions row was written.
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

func TestExecuteWithMetaNilRecorder(t *testing.T) {
	reg := NewRegistry()
	reg.Register(newMockTool("file_read", Pure))

	exec := NewExecutor(reg, ExecutorConfig{}, nil)
	// No recorder set — should not panic.

	results := exec.ExecuteWithMeta(context.Background(), []ToolCall{
		{ID: "tc-1", Name: "file_read", Arguments: json.RawMessage(`{}`)},
	}, ExecutionMeta{ConversationID: "conv-1", TurnNumber: 1, Iteration: 1})

	if len(results) != 1 || !results[0].Success {
		t.Fatalf("unexpected result: %+v", results)
	}
}

func TestExecuteWithMetaPersistsRawAndNormalizedSizes(t *testing.T) {
	sqlDB, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer sqlDB.Close()
	if err := db.Init(context.Background(), sqlDB); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	if _, err := sqlDB.Exec(`INSERT INTO projects(id, name, root_path, created_at, updated_at)
		VALUES ('proj-1', 'test', '/tmp/test', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if _, err := sqlDB.Exec(`INSERT INTO conversations(id, project_id, title, model, provider, created_at, updated_at)
		VALUES ('conv-1', 'proj-1', 'test', 'test-model', 'test-provider', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert conversation: %v", err)
	}

	queries := db.New(sqlDB)
	reg := NewRegistry()
	shellTool := newMockTool("shell", Pure)
	shellTool.executeFn = func(ctx context.Context, projectRoot string, input json.RawMessage) (*ToolResult, error) {
		return &ToolResult{Success: true, Content: "\x1b[31mCompiling foo\x1b[0m\nCompiling bar\nDONE\n"}, nil
	}
	reg.Register(shellTool)

	exec := NewExecutor(reg, ExecutorConfig{}, nil)
	exec.SetRecorder(NewToolExecutionRecorder(queries))

	calls := []ToolCall{{ID: "tc-1", Name: "shell", Arguments: json.RawMessage(`{"cmd":"build"}`)}}
	meta := ExecutionMeta{ConversationID: "conv-1", TurnNumber: 1, Iteration: 1}
	results := exec.ExecuteWithMeta(context.Background(), calls, meta)
	if len(results) != 1 || !results[0].Success {
		t.Fatalf("unexpected result: %+v", results)
	}

	var outputSize, normalizedSize int64
	if err := sqlDB.QueryRow(`SELECT output_size, normalized_size FROM tool_executions WHERE conversation_id = 'conv-1' AND tool_use_id = 'tc-1'`).Scan(&outputSize, &normalizedSize); err != nil {
		t.Fatalf("query persisted sizes: %v", err)
	}
	if outputSize <= normalizedSize {
		t.Fatalf("output_size = %d, normalized_size = %d, want output_size > normalized_size after shell normalization", outputSize, normalizedSize)
	}
}

func TestExecuteWithMetaProjectMemoryRecorder(t *testing.T) {
	ctx := context.Background()
	backend, err := projectmemory.OpenBrainBackend(ctx, projectmemory.Config{DataDir: t.TempDir(), DurableAck: true})
	if err != nil {
		t.Fatalf("OpenBrainBackend: %v", err)
	}
	defer backend.Close()
	if err := backend.CreateConversation(ctx, projectmemory.CreateConversationArgs{
		ID:          "conv-1",
		ProjectID:   "project-1",
		Title:       "Tool Memory",
		CreatedAtUS: uint64(time.Now().UTC().UnixMicro()),
	}); err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}

	reg := NewRegistry()
	shellTool := newMockTool("shell", Pure)
	shellTool.executeFn = func(ctx context.Context, projectRoot string, input json.RawMessage) (*ToolResult, error) {
		return &ToolResult{Success: true, Content: "\x1b[31mCompiling foo\x1b[0m\nDONE\n"}, nil
	}
	reg.Register(shellTool)

	exec := NewExecutor(reg, ExecutorConfig{}, nil)
	exec.SetRecorder(NewProjectMemoryToolExecutionRecorder(backend))
	calls := []ToolCall{{ID: "tc-1", Name: "shell", Arguments: json.RawMessage(`{"cmd":"build"}`)}}
	results := exec.ExecuteWithMeta(ctx, calls, ExecutionMeta{ConversationID: "conv-1", TurnNumber: 2, Iteration: 1})
	if len(results) != 1 || !results[0].Success {
		t.Fatalf("unexpected result: %+v", results)
	}

	executions, err := backend.ListToolExecutions(ctx, "conv-1")
	if err != nil {
		t.Fatalf("ListToolExecutions: %v", err)
	}
	if len(executions) != 1 {
		t.Fatalf("executions = %+v, want one Shunter record", executions)
	}
	got := executions[0]
	if got.ToolUseID != "tc-1" || got.ToolName != "shell" || got.Status != "success" || got.TurnNumber != 2 || got.Iteration != 1 {
		t.Fatalf("execution metadata = %+v, want shell tc-1 turn 2 iteration 1", got)
	}
	if got.InputJSON != `{"cmd":"build"}` {
		t.Fatalf("InputJSON = %q, want raw args", got.InputJSON)
	}
	if got.OutputSize <= got.NormalizedSize {
		t.Fatalf("OutputSize = %d, NormalizedSize = %d, want raw output larger after normalization", got.OutputSize, got.NormalizedSize)
	}
}

func TestToolExecutionRecorderNilSafety(t *testing.T) {
	var recorder *ToolExecutionRecorder

	err := recorder.Record(context.Background(), ToolCall{ID: "tc-1", Name: "test"},
		ToolResult{Success: true}, ExecutionMeta{}, time.Now())
	if err != nil {
		t.Fatalf("nil recorder should return nil, got: %v", err)
	}
}
