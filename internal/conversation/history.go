package conversation

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	contextpkg "github.com/ponchione/sodoryard/internal/context"
	"github.com/ponchione/sodoryard/internal/db"
	"github.com/ponchione/sodoryard/internal/projectmemory"
)

// HistoryManager provides the first real conversation-history operations needed
// by the Layer 5 bootstrap path.
type HistoryManager struct {
	database *sql.DB
	queries  *db.Queries
	memory   ProjectMemoryStore
	seen     *SeenFiles
	now      func() time.Time
}

// NewHistoryManager constructs a DB-backed history manager. If seen is nil, a
// fresh session-scoped tracker is created.
func NewHistoryManager(database *sql.DB, seen *SeenFiles) *HistoryManager {
	if seen == nil {
		seen = NewSeenFiles()
	}
	return &HistoryManager{
		database: database,
		queries:  db.New(database),
		seen:     seen,
		now:      time.Now,
	}
}

// SetNowForTest overrides the clock used for persisted timestamps.
func (m *HistoryManager) SetNowForTest(now func() time.Time) {
	if m == nil || now == nil {
		return
	}
	m.now = now
}

// PersistUserMessage inserts the initial user message row for a turn before
// context assembly begins.
func (m *HistoryManager) PersistUserMessage(ctx context.Context, conversationID string, turnNumber int, message string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := m.validate(); err != nil {
		return err
	}
	if m.memory != nil {
		return m.memory.AppendUserMessage(ctx, projectmemory.AppendUserMessageArgs{
			ConversationID: conversationID,
			TurnNumber:     uint32(turnNumber),
			Content:        message,
			CreatedAtUS:    uint64(m.now().UTC().UnixMicro()),
		})
	}

	tx, err := m.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("conversation history: begin persist user message tx: %w", err)
	}
	defer tx.Rollback()

	q := m.queries.WithTx(tx)
	sequence, err := nextSequence(ctx, q, conversationID)
	if err != nil {
		return fmt.Errorf("conversation history: determine next sequence: %w", err)
	}

	timestamp := m.now().UTC().Format(time.RFC3339)
	if err := q.InsertUserMessage(ctx, db.InsertUserMessageParams{
		ConversationID: conversationID,
		Content:        sql.NullString{String: message, Valid: true},
		TurnNumber:     int64(turnNumber),
		Sequence:       sequence,
		CreatedAt:      timestamp,
	}); err != nil {
		return fmt.Errorf("conversation history: insert user message: %w", err)
	}
	if err := q.TouchConversationUpdatedAt(ctx, db.TouchConversationUpdatedAtParams{
		UpdatedAt: timestamp,
		ID:        conversationID,
	}); err != nil {
		return fmt.Errorf("conversation history: touch conversation updated_at: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("conversation history: commit persist user message tx: %w", err)
	}
	return nil
}

// ReconstructHistory returns the active message rows in provider order for the
// current conversation.
func (m *HistoryManager) ReconstructHistory(ctx context.Context, conversationID string) ([]db.Message, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := m.validate(); err != nil {
		return nil, err
	}
	if m.memory != nil {
		messages, err := m.memory.ListMessages(ctx, conversationID, false)
		if err != nil {
			return nil, fmt.Errorf("conversation history: list active messages: %w", err)
		}
		out := make([]db.Message, 0, len(messages))
		for _, message := range messages {
			out = append(out, dbMessageFromMemory(message))
		}
		return out, nil
	}
	messages, err := m.queries.ListActiveMessages(ctx, conversationID)
	if err != nil {
		return nil, fmt.Errorf("conversation history: list active messages: %w", err)
	}
	return messages, nil
}

// IterationMessage is the input shape for a single message within a completed
// iteration. The caller (agent loop) builds these from the assistant response
// and tool execution results before handing them to PersistIteration.
type IterationMessage struct {
	// Role is one of "assistant" or "tool".
	Role string
	// Content holds the message payload: JSON content-block array for assistant
	// messages, plain-text result for tool messages.
	Content string
	// ToolUseID is set only for role=tool messages and links back to the
	// tool_use block in the preceding assistant message.
	ToolUseID string
	// ToolName is set only for role=tool messages.
	ToolName string
}

// PersistIteration atomically inserts the message rows for a completed iteration
// (the assistant response plus any tool result messages) in a single SQLite
// transaction. Each message receives the next monotonic sequence number.
// If any insert fails the entire transaction rolls back — no partial iteration
// message data is left in the database.
//
// Tool execution analytics (`tool_executions`) and provider analytics
// (`sub_calls`) are persisted on separate paths today. They remain best-effort
// and intentionally non-atomic with message insertion, but PersistIteration
// backfills `sub_calls.message_id` within its transaction when the assistant row
// for that iteration is known.
func (m *HistoryManager) PersistIteration(ctx context.Context, conversationID string, turnNumber, iteration int, messages []IterationMessage) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := m.validate(); err != nil {
		return err
	}
	if len(messages) == 0 {
		return fmt.Errorf("conversation history: persist iteration: no messages provided")
	}
	if m.memory != nil {
		pmMessages := make([]projectmemory.PersistIterationMessage, 0, len(messages))
		for _, msg := range messages {
			pmMessages = append(pmMessages, projectmemory.PersistIterationMessage{
				Role:      msg.Role,
				Content:   msg.Content,
				ToolUseID: msg.ToolUseID,
				ToolName:  msg.ToolName,
			})
		}
		return m.memory.PersistIteration(ctx, projectmemory.PersistIterationArgs{
			ConversationID: conversationID,
			TurnNumber:     uint32(turnNumber),
			Iteration:      uint32(iteration),
			Messages:       pmMessages,
			CreatedAtUS:    uint64(m.now().UTC().UnixMicro()),
		})
	}

	tx, err := m.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("conversation history: begin persist iteration tx: %w", err)
	}
	defer tx.Rollback()

	q := m.queries.WithTx(tx)
	timestamp := m.now().UTC().Format(time.RFC3339)
	var assistantMessageID int64

	sequence, err := nextSequence(ctx, q, conversationID)
	if err != nil {
		return fmt.Errorf("conversation history: persist iteration: determine next sequence: %w", err)
	}
	for _, msg := range messages {
		params := db.InsertIterationMessageParams{
			ConversationID: conversationID,
			Role:           msg.Role,
			Content:        sql.NullString{String: msg.Content, Valid: msg.Content != ""},
			TurnNumber:     int64(turnNumber),
			Iteration:      int64(iteration),
			Sequence:       sequence,
			CreatedAt:      timestamp,
		}
		if msg.ToolUseID != "" {
			params.ToolUseID = sql.NullString{String: msg.ToolUseID, Valid: true}
		}
		if msg.ToolName != "" {
			params.ToolName = sql.NullString{String: msg.ToolName, Valid: true}
		}

		if err := q.InsertIterationMessage(ctx, params); err != nil {
			return fmt.Errorf("conversation history: persist iteration: insert %s message: %w", msg.Role, err)
		}
		if msg.Role == "assistant" {
			assistantMessageID, err = q.LatestAssistantMessageIDForIteration(ctx, db.LatestAssistantMessageIDForIterationParams{
				ConversationID: conversationID,
				TurnNumber:     int64(turnNumber),
				Iteration:      int64(iteration),
			})
			if err != nil {
				return fmt.Errorf("conversation history: persist iteration: lookup assistant message id: %w", err)
			}
		}
		sequence += 1.0
	}

	if assistantMessageID != 0 {
		if err := q.LinkIterationSubCallsToMessage(ctx, db.LinkIterationSubCallsToMessageParams{
			MessageID:      sql.NullInt64{Int64: assistantMessageID, Valid: true},
			ConversationID: sql.NullString{String: conversationID, Valid: true},
			TurnNumber:     sql.NullInt64{Int64: int64(turnNumber), Valid: true},
			Iteration:      sql.NullInt64{Int64: int64(iteration), Valid: true},
		}); err != nil {
			return fmt.Errorf("conversation history: persist iteration: link sub calls to assistant message: %w", err)
		}
	}

	if err := q.TouchConversationUpdatedAt(ctx, db.TouchConversationUpdatedAtParams{
		UpdatedAt: timestamp,
		ID:        conversationID,
	}); err != nil {
		return fmt.Errorf("conversation history: persist iteration: touch conversation updated_at: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("conversation history: commit persist iteration tx: %w", err)
	}
	return nil
}

// CancelIteration atomically deletes all messages, tool_execution records,
// and sub_call records for a specific in-flight iteration. Completed iterations
// from earlier in the same turn are not affected. The user's initial message
// (persisted before iterations begin) is also preserved because it uses
// iteration=1 with role=user, while CancelIteration targets the assistant/tool
// iteration data.
//
// This is called when the user cancels a turn mid-iteration or when error
// recovery needs to discard a partial iteration before retrying.
func (m *HistoryManager) CancelIteration(ctx context.Context, conversationID string, turnNumber, iteration int) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := m.validate(); err != nil {
		return err
	}
	if m.memory != nil {
		return m.memory.CancelIteration(ctx, projectmemory.CancelIterationArgs{
			ConversationID: conversationID,
			TurnNumber:     uint32(turnNumber),
			Iteration:      uint32(iteration),
		})
	}

	tx, err := m.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("conversation history: begin cancel iteration tx: %w", err)
	}
	defer tx.Rollback()

	q := m.queries.WithTx(tx)

	if err := q.DeleteIterationMessages(ctx, db.DeleteIterationMessagesParams{
		ConversationID: conversationID,
		TurnNumber:     int64(turnNumber),
		Iteration:      int64(iteration),
	}); err != nil {
		return fmt.Errorf("conversation history: cancel iteration: delete messages: %w", err)
	}

	if err := q.DeleteIterationToolExecutions(ctx, db.DeleteIterationToolExecutionsParams{
		ConversationID: conversationID,
		TurnNumber:     int64(turnNumber),
		Iteration:      int64(iteration),
	}); err != nil {
		return fmt.Errorf("conversation history: cancel iteration: delete tool executions: %w", err)
	}

	if err := q.DeleteIterationSubCalls(ctx, db.DeleteIterationSubCallsParams{
		ConversationID: sql.NullString{String: conversationID, Valid: true},
		TurnNumber:     sql.NullInt64{Int64: int64(turnNumber), Valid: true},
		Iteration:      sql.NullInt64{Int64: int64(iteration), Valid: true},
	}); err != nil {
		return fmt.Errorf("conversation history: cancel iteration: delete sub calls: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("conversation history: commit cancel iteration tx: %w", err)
	}
	return nil
}

// DiscardTurn atomically deletes all persisted state for a turn, including the
// initial user message. It is intended for raw chat turns that fail or are
// canceled before a complete assistant response exists.
func (m *HistoryManager) DiscardTurn(ctx context.Context, conversationID string, turnNumber int) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := m.validate(); err != nil {
		return err
	}
	if m.memory != nil {
		return m.memory.DiscardTurn(ctx, projectmemory.DiscardTurnArgs{
			ConversationID: conversationID,
			TurnNumber:     uint32(turnNumber),
		})
	}

	tx, err := m.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("conversation history: begin discard turn tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM sub_calls WHERE conversation_id = ? AND turn_number = ?`, conversationID, turnNumber); err != nil {
		return fmt.Errorf("conversation history: discard turn: delete sub calls: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM tool_executions WHERE conversation_id = ? AND turn_number = ?`, conversationID, turnNumber); err != nil {
		return fmt.Errorf("conversation history: discard turn: delete tool executions: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM messages WHERE conversation_id = ? AND turn_number = ?`, conversationID, turnNumber); err != nil {
		return fmt.Errorf("conversation history: discard turn: delete messages: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("conversation history: commit discard turn tx: %w", err)
	}
	return nil
}

// SeenFiles exposes the session-scoped seen-files tracker used by Layer 3.
func (m *HistoryManager) SeenFiles(string) contextpkg.SeenFileLookup {
	if m == nil {
		return nil
	}
	return m.seen
}

func (m *HistoryManager) validate() error {
	if m == nil {
		return fmt.Errorf("conversation history: manager is nil")
	}
	if m.memory != nil {
		if m.now == nil {
			return fmt.Errorf("conversation history: clock is nil")
		}
		return nil
	}
	if m.database == nil {
		return fmt.Errorf("conversation history: database is nil")
	}
	if m.queries == nil {
		return fmt.Errorf("conversation history: queries are nil")
	}
	if m.now == nil {
		return fmt.Errorf("conversation history: clock is nil")
	}
	return nil
}

func dbMessageFromMemory(row projectmemory.Message) db.Message {
	return db.Message{
		ID:             int64(row.Sequence) + 1,
		ConversationID: row.ConversationID,
		Role:           row.Role,
		Content:        sql.NullString{String: row.Content, Valid: row.Content != ""},
		ToolUseID:      sql.NullString{String: row.ToolUseID, Valid: row.ToolUseID != ""},
		ToolName:       sql.NullString{String: row.ToolName, Valid: row.ToolName != ""},
		TurnNumber:     int64(row.TurnNumber),
		Iteration:      int64(row.Iteration),
		Sequence:       float64(row.Sequence),
		IsCompressed:   boolToInt64(row.Compressed),
		IsSummary:      boolToInt64(row.IsSummary),
		CreatedAt:      unixMicroTime(row.CreatedAtUS).UTC().Format(time.RFC3339),
	}
}

func boolToInt64(value bool) int64 {
	if value {
		return 1
	}
	return 0
}

func nextSequence(ctx context.Context, q *db.Queries, conversationID string) (float64, error) {
	value, err := q.NextMessageSequence(ctx, conversationID)
	if err != nil {
		return 0, err
	}
	switch v := value.(type) {
	case float64:
		return v, nil
	case int64:
		return float64(v), nil
	case int:
		return float64(v), nil
	case []byte:
		var parsed float64
		if _, err := fmt.Sscanf(string(v), "%f", &parsed); err != nil {
			return 0, fmt.Errorf("parse next sequence from bytes %q: %w", string(v), err)
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("unsupported next sequence type %T", value)
	}
}
