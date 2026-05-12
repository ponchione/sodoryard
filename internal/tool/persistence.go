package tool

import (
	"context"
	"database/sql"
	"time"

	"github.com/ponchione/sodoryard/internal/db"
	"github.com/ponchione/sodoryard/internal/projectmemory"
)

// ExecutionMeta carries the conversation context needed for tool_executions
// persistence. Passed to ExecuteWithMeta to record analytics rows.
type ExecutionMeta struct {
	ConversationID string
	TurnNumber     int
	Iteration      int
}

type toolExecutionStore interface {
	Record(ctx context.Context, call ToolCall, result ToolResult, meta ExecutionMeta, now time.Time) error
}

// ToolExecutionRecorder persists tool execution analytics. When nil on the
// Executor, persistence is silently skipped (useful for testing).
type ToolExecutionRecorder struct {
	store toolExecutionStore
}

// NewToolExecutionRecorder creates a recorder backed by sqlc queries.
func NewToolExecutionRecorder(queries *db.Queries) *ToolExecutionRecorder {
	if queries == nil {
		return nil
	}
	return &ToolExecutionRecorder{store: sqliteToolExecutionStore{queries: queries}}
}

// NewProjectMemoryToolExecutionRecorder creates a Shunter-backed recorder.
func NewProjectMemoryToolExecutionRecorder(recorder projectmemory.ToolExecutionRecorder) *ToolExecutionRecorder {
	if recorder == nil {
		return nil
	}
	return &ToolExecutionRecorder{store: projectMemoryToolExecutionStore{recorder: recorder}}
}

// Record inserts a tool_executions row. Errors are returned to the caller
// (the executor logs and swallows them).
func (r *ToolExecutionRecorder) Record(ctx context.Context, call ToolCall, result ToolResult, meta ExecutionMeta, now time.Time) error {
	if r == nil || r.store == nil {
		return nil
	}
	return r.store.Record(ctx, call, result, meta, now)
}

type sqliteToolExecutionStore struct {
	queries *db.Queries
}

func (s sqliteToolExecutionStore) Record(ctx context.Context, call ToolCall, result ToolResult, meta ExecutionMeta, now time.Time) error {
	if s.queries == nil {
		return nil
	}

	var errStr sql.NullString
	if result.Error != "" {
		errStr = sql.NullString{String: result.Error, Valid: true}
	}

	var inputStr sql.NullString
	if len(call.Arguments) > 0 {
		inputStr = sql.NullString{String: string(call.Arguments), Valid: true}
	}

	var success int64
	if result.Success {
		success = 1
	}

	outputSize, normalizedSize := toolExecutionSizes(call, result)

	return s.queries.InsertToolExecution(ctx, db.InsertToolExecutionParams{
		ConversationID: meta.ConversationID,
		TurnNumber:     int64(meta.TurnNumber),
		Iteration:      int64(meta.Iteration),
		ToolUseID:      call.ID,
		ToolName:       call.Name,
		Input:          inputStr,
		OutputSize:     sql.NullInt64{Int64: int64(outputSize), Valid: true},
		NormalizedSize: sql.NullInt64{Int64: int64(normalizedSize), Valid: true},
		Error:          errStr,
		Success:        success,
		DurationMs:     result.DurationMs,
		CreatedAt:      now.UTC().Format(time.RFC3339),
	})
}

type projectMemoryToolExecutionStore struct {
	recorder projectmemory.ToolExecutionRecorder
}

func (s projectMemoryToolExecutionStore) Record(ctx context.Context, call ToolCall, result ToolResult, meta ExecutionMeta, now time.Time) error {
	if s.recorder == nil {
		return nil
	}
	outputSize, normalizedSize := toolExecutionSizes(call, result)
	completedAtUS := uint64(now.UTC().UnixMicro())
	durationMs := nonNegativeUint64(result.DurationMs)
	startedAtUS := completedAtUS
	if durationMs > 0 {
		durationUS := durationMs * 1000
		if completedAtUS > durationUS {
			startedAtUS = completedAtUS - durationUS
		}
	}
	status := "error"
	if result.Success {
		status = "success"
	}
	turnNumber := nonNegativeUint32(meta.TurnNumber)
	iteration := nonNegativeUint32(meta.Iteration)
	return s.recorder.RecordToolExecution(ctx, projectmemory.RecordToolExecutionArgs{
		ID:             projectmemory.ToolExecutionID(meta.ConversationID, turnNumber, iteration, call.ID, call.Name),
		ConversationID: meta.ConversationID,
		TurnNumber:     turnNumber,
		Iteration:      iteration,
		ToolUseID:      call.ID,
		ToolName:       call.Name,
		Status:         status,
		StartedAtUS:    startedAtUS,
		CompletedAtUS:  completedAtUS,
		DurationMs:     durationMs,
		InputJSON:      string(call.Arguments),
		OutputSize:     outputSize,
		NormalizedSize: normalizedSize,
		Error:          result.Error,
		MetadataJSON:   "{}",
	})
}

func toolExecutionSizes(call ToolCall, result ToolResult) (uint64, uint64) {
	outputSize := result.OutputSize
	if outputSize <= 0 && result.Content != "" {
		outputSize = len(result.Content)
	}
	normalizedSize := result.NormalizedSize
	if normalizedSize <= 0 {
		normalizedSize = len(result.Content)
		if result.Success {
			normalizedSize = len(NormalizeToolResult(call.Name, result.Content))
		}
	}
	return nonNegativeUint64(outputSize), nonNegativeUint64(normalizedSize)
}

func nonNegativeUint64[T ~int | ~int64](value T) uint64 {
	if value <= 0 {
		return 0
	}
	return uint64(value)
}

func nonNegativeUint32(value int) uint32 {
	if value <= 0 {
		return 0
	}
	return uint32(value)
}
