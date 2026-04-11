//go:build sqlite_fts5
// +build sqlite_fts5

package conversation

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ponchione/sodoryard/internal/db"
	sid "github.com/ponchione/sodoryard/internal/id"
)

func TestHistoryManagerPersistUserMessageAssignsSequenceAndTouchesConversation(t *testing.T) {
	ctx := context.Background()
	database := newHistoryTestDB(t)
	queries := db.New(database)
	conversationID := seedHistoryConversation(t, database)

	manager := NewHistoryManager(database, nil)
	manager.now = func() time.Time { return time.Unix(1700000800, 0).UTC() }

	if err := manager.PersistUserMessage(ctx, conversationID, 1, "fix auth"); err != nil {
		t.Fatalf("PersistUserMessage returned error: %v", err)
	}
	if err := manager.PersistUserMessage(ctx, conversationID, 2, "add tests"); err != nil {
		t.Fatalf("second PersistUserMessage returned error: %v", err)
	}

	rows, err := queries.ListTurnMessages(ctx, conversationID)
	if err != nil {
		t.Fatalf("ListTurnMessages returned error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("row count = %d, want 2", len(rows))
	}
	if rows[0].Sequence != 0.0 || rows[1].Sequence != 1.0 {
		t.Fatalf("sequences = (%v, %v), want (0.0, 1.0)", rows[0].Sequence, rows[1].Sequence)
	}
	if rows[0].Iteration != 1 || rows[1].Iteration != 1 {
		t.Fatalf("iterations = (%d, %d), want (1, 1)", rows[0].Iteration, rows[1].Iteration)
	}
	if rows[0].Content.String != "fix auth" || rows[1].Content.String != "add tests" {
		t.Fatalf("contents = (%q, %q), want (fix auth, add tests)", rows[0].Content.String, rows[1].Content.String)
	}

	var updatedAt string
	if err := database.QueryRowContext(ctx, `SELECT updated_at FROM conversations WHERE id = ?`, conversationID).Scan(&updatedAt); err != nil {
		t.Fatalf("query updated_at returned error: %v", err)
	}
	wantUpdatedAt := manager.now().Format(time.RFC3339)
	if updatedAt != wantUpdatedAt {
		t.Fatalf("updated_at = %q, want %q", updatedAt, wantUpdatedAt)
	}
}

func TestHistoryManagerPersistIterationInsertsAssistantAndToolMessages(t *testing.T) {
	ctx := context.Background()
	database := newHistoryTestDB(t)
	queries := db.New(database)
	conversationID := seedHistoryConversation(t, database)

	manager := NewHistoryManager(database, nil)
	manager.now = func() time.Time { return time.Unix(1700000850, 0).UTC() }

	// First persist a user message to establish sequence baseline.
	if err := manager.PersistUserMessage(ctx, conversationID, 1, "fix auth"); err != nil {
		t.Fatalf("PersistUserMessage returned error: %v", err)
	}

	// Now persist the first iteration: assistant + two tool results.
	iterMsgs := []IterationMessage{
		{
			Role:    "assistant",
			Content: `[{"type":"text","text":"I'll check the auth code."},{"type":"tool_use","id":"tc1","name":"file_read","input":{"path":"auth.go"}},{"type":"tool_use","id":"tc2","name":"search_text","input":{"pattern":"ValidateToken"}}]`,
		},
		{
			Role:      "tool",
			Content:   "package auth\n\nfunc ValidateToken...",
			ToolUseID: "tc1",
			ToolName:  "file_read",
		},
		{
			Role:      "tool",
			Content:   "auth.go:15: func ValidateToken...",
			ToolUseID: "tc2",
			ToolName:  "search_text",
		},
	}
	if err := manager.PersistIteration(ctx, conversationID, 1, 1, iterMsgs); err != nil {
		t.Fatalf("PersistIteration returned error: %v", err)
	}

	rows, err := queries.ListTurnMessages(ctx, conversationID)
	if err != nil {
		t.Fatalf("ListTurnMessages returned error: %v", err)
	}
	// 1 user + 3 iteration messages = 4
	if len(rows) != 4 {
		t.Fatalf("row count = %d, want 4", len(rows))
	}

	// Verify sequences are monotonically increasing.
	for i := 0; i < len(rows); i++ {
		if rows[i].Sequence != float64(i) {
			t.Fatalf("rows[%d].Sequence = %v, want %v", i, rows[i].Sequence, float64(i))
		}
	}

	// Verify the roles.
	wantRoles := []string{"user", "assistant", "tool", "tool"}
	for i, want := range wantRoles {
		if rows[i].Role != want {
			t.Fatalf("rows[%d].Role = %q, want %q", i, rows[i].Role, want)
		}
	}

	// Verify iteration numbers: user gets iteration=1 (from InsertUserMessage), iteration messages get iteration=1.
	for i := 1; i < len(rows); i++ {
		if rows[i].Iteration != 1 {
			t.Fatalf("rows[%d].Iteration = %d, want 1", i, rows[i].Iteration)
		}
	}

	// Verify tool metadata.
	if rows[2].ToolUseID.String != "tc1" || rows[2].ToolName.String != "file_read" {
		t.Fatalf("rows[2] tool fields = (%q, %q), want (tc1, file_read)", rows[2].ToolUseID.String, rows[2].ToolName.String)
	}
	if rows[3].ToolUseID.String != "tc2" || rows[3].ToolName.String != "search_text" {
		t.Fatalf("rows[3] tool fields = (%q, %q), want (tc2, search_text)", rows[3].ToolUseID.String, rows[3].ToolName.String)
	}

	// Verify conversation updated_at was touched.
	var updatedAt string
	if err := database.QueryRowContext(ctx, `SELECT updated_at FROM conversations WHERE id = ?`, conversationID).Scan(&updatedAt); err != nil {
		t.Fatalf("query updated_at returned error: %v", err)
	}
	wantUpdatedAt := manager.now().Format(time.RFC3339)
	if updatedAt != wantUpdatedAt {
		t.Fatalf("updated_at = %q, want %q", updatedAt, wantUpdatedAt)
	}
}

func TestHistoryManagerPersistIterationLinksChatSubCallsToAssistantMessage(t *testing.T) {
	ctx := context.Background()
	database := newHistoryTestDB(t)
	queries := db.New(database)
	conversationID := seedHistoryConversation(t, database)
	createdAt := time.Unix(1700000860, 0).UTC().Format(time.RFC3339)

	manager := NewHistoryManager(database, nil)
	manager.now = func() time.Time { return time.Unix(1700000860, 0).UTC() }

	if err := manager.PersistUserMessage(ctx, conversationID, 1, "fix auth"); err != nil {
		t.Fatalf("PersistUserMessage returned error: %v", err)
	}

	mustExecHistory(t, database, `INSERT INTO sub_calls(conversation_id, turn_number, iteration, provider, model, purpose, tokens_in, tokens_out, latency_ms, success, created_at)
		VALUES (?, 1, 1, 'anthropic', 'claude', 'chat', 1000, 200, 500, 1, ?)`, conversationID, createdAt)
	mustExecHistory(t, database, `INSERT INTO sub_calls(conversation_id, turn_number, iteration, provider, model, purpose, tokens_in, tokens_out, latency_ms, success, created_at)
		VALUES (?, 1, 1, 'anthropic', 'claude', 'compression', 10, 5, 20, 1, ?)`, conversationID, createdAt)

	iterMsgs := []IterationMessage{
		{Role: "assistant", Content: `[{"type":"text","text":"I'll check the auth code."}]`},
		{Role: "tool", Content: "file contents", ToolUseID: "tc1", ToolName: "file_read"},
	}
	if err := manager.PersistIteration(ctx, conversationID, 1, 1, iterMsgs); err != nil {
		t.Fatalf("PersistIteration returned error: %v", err)
	}

	rows, err := queries.ListTurnMessages(ctx, conversationID)
	if err != nil {
		t.Fatalf("ListTurnMessages returned error: %v", err)
	}
	if len(rows) < 2 || rows[1].Role != "assistant" {
		t.Fatalf("rows = %#v, want assistant message at index 1", rows)
	}
	assistantID := rows[1].ID

	var linkedMessageID sql.NullInt64
	if err := database.QueryRowContext(ctx, `SELECT message_id FROM sub_calls WHERE conversation_id = ? AND purpose = 'chat'`, conversationID).Scan(&linkedMessageID); err != nil {
		t.Fatalf("query linked chat sub_call returned error: %v", err)
	}
	if !linkedMessageID.Valid || linkedMessageID.Int64 != assistantID {
		t.Fatalf("chat sub_call message_id = %+v, want %d", linkedMessageID, assistantID)
	}

	var compressionMessageID sql.NullInt64
	if err := database.QueryRowContext(ctx, `SELECT message_id FROM sub_calls WHERE conversation_id = ? AND purpose = 'compression'`, conversationID).Scan(&compressionMessageID); err != nil {
		t.Fatalf("query compression sub_call returned error: %v", err)
	}
	if compressionMessageID.Valid {
		t.Fatalf("compression sub_call message_id = %+v, want NULL", compressionMessageID)
	}
}

func TestHistoryManagerPersistIterationLeavesChatSubCallsUnlinkedWithoutAssistantMessage(t *testing.T) {
	ctx := context.Background()
	database := newHistoryTestDB(t)
	conversationID := seedHistoryConversation(t, database)
	createdAt := time.Unix(1700000861, 0).UTC().Format(time.RFC3339)

	manager := NewHistoryManager(database, nil)
	manager.now = func() time.Time { return time.Unix(1700000861, 0).UTC() }

	if err := manager.PersistUserMessage(ctx, conversationID, 1, "fix auth"); err != nil {
		t.Fatalf("PersistUserMessage returned error: %v", err)
	}

	mustExecHistory(t, database, `INSERT INTO sub_calls(conversation_id, turn_number, iteration, provider, model, purpose, tokens_in, tokens_out, latency_ms, success, created_at)
		VALUES (?, 1, 1, 'anthropic', 'claude', 'chat', 1000, 200, 500, 1, ?)`, conversationID, createdAt)

	iterMsgs := []IterationMessage{
		{Role: "tool", Content: "file contents", ToolUseID: "tc1", ToolName: "file_read"},
	}
	if err := manager.PersistIteration(ctx, conversationID, 1, 1, iterMsgs); err != nil {
		t.Fatalf("PersistIteration returned error: %v", err)
	}

	var linkedMessageID sql.NullInt64
	if err := database.QueryRowContext(ctx, `SELECT message_id FROM sub_calls WHERE conversation_id = ? AND purpose = 'chat'`, conversationID).Scan(&linkedMessageID); err != nil {
		t.Fatalf("query chat sub_call returned error: %v", err)
	}
	if linkedMessageID.Valid {
		t.Fatalf("chat sub_call message_id = %+v, want NULL without assistant message", linkedMessageID)
	}
}

func TestHistoryManagerPersistIterationMultipleIterationsIncrementSequence(t *testing.T) {
	ctx := context.Background()
	database := newHistoryTestDB(t)
	queries := db.New(database)
	conversationID := seedHistoryConversation(t, database)

	manager := NewHistoryManager(database, nil)
	manager.now = func() time.Time { return time.Unix(1700000900, 0).UTC() }

	if err := manager.PersistUserMessage(ctx, conversationID, 1, "fix auth"); err != nil {
		t.Fatalf("PersistUserMessage returned error: %v", err)
	}

	// Iteration 1: assistant + tool
	iter1 := []IterationMessage{
		{Role: "assistant", Content: `[{"type":"text","text":"checking..."},{"type":"tool_use","id":"tc1","name":"file_read","input":{}}]`},
		{Role: "tool", Content: "file contents", ToolUseID: "tc1", ToolName: "file_read"},
	}
	if err := manager.PersistIteration(ctx, conversationID, 1, 1, iter1); err != nil {
		t.Fatalf("PersistIteration(iter1) returned error: %v", err)
	}

	// Iteration 2: assistant + tool
	iter2 := []IterationMessage{
		{Role: "assistant", Content: `[{"type":"text","text":"fixing now"},{"type":"tool_use","id":"tc2","name":"file_edit","input":{}}]`},
		{Role: "tool", Content: "edit applied", ToolUseID: "tc2", ToolName: "file_edit"},
	}
	if err := manager.PersistIteration(ctx, conversationID, 1, 2, iter2); err != nil {
		t.Fatalf("PersistIteration(iter2) returned error: %v", err)
	}

	// Iteration 3: final text-only assistant
	iter3 := []IterationMessage{
		{Role: "assistant", Content: `[{"type":"text","text":"Done. The issue was..."}]`},
	}
	if err := manager.PersistIteration(ctx, conversationID, 1, 3, iter3); err != nil {
		t.Fatalf("PersistIteration(iter3) returned error: %v", err)
	}

	rows, err := queries.ListTurnMessages(ctx, conversationID)
	if err != nil {
		t.Fatalf("ListTurnMessages returned error: %v", err)
	}
	// 1 user + 2 iter1 + 2 iter2 + 1 iter3 = 6
	if len(rows) != 6 {
		t.Fatalf("row count = %d, want 6", len(rows))
	}

	// Verify continuous sequences.
	for i := 0; i < len(rows); i++ {
		if rows[i].Sequence != float64(i) {
			t.Fatalf("rows[%d].Sequence = %v, want %v", i, rows[i].Sequence, float64(i))
		}
	}

	// Verify iteration assignments.
	wantIter := []int64{1, 1, 1, 2, 2, 3}
	for i, want := range wantIter {
		if rows[i].Iteration != want {
			t.Fatalf("rows[%d].Iteration = %d, want %d", i, rows[i].Iteration, want)
		}
	}
}

func TestHistoryManagerPersistIterationRejectsEmptyMessages(t *testing.T) {
	database := newHistoryTestDB(t)
	manager := NewHistoryManager(database, nil)

	err := manager.PersistIteration(context.Background(), "conv-1", 1, 1, nil)
	if err == nil {
		t.Fatal("PersistIteration(nil messages) error = nil, want error")
	}
	if !strings.Contains(err.Error(), "no messages provided") {
		t.Fatalf("error = %q, want 'no messages provided'", err.Error())
	}

	err = manager.PersistIteration(context.Background(), "conv-1", 1, 1, []IterationMessage{})
	if err == nil {
		t.Fatal("PersistIteration(empty messages) error = nil, want error")
	}
}

func TestHistoryManagerPersistIterationRollsBackOnInsertFailure(t *testing.T) {
	ctx := context.Background()
	database := newHistoryTestDB(t)
	queries := db.New(database)
	conversationID := seedHistoryConversation(t, database)

	manager := NewHistoryManager(database, nil)
	manager.now = func() time.Time { return time.Unix(1700000950, 0).UTC() }

	if err := manager.PersistUserMessage(ctx, conversationID, 1, "hello"); err != nil {
		t.Fatalf("PersistUserMessage returned error: %v", err)
	}

	// Insert an iteration message with an invalid role to trigger a CHECK constraint failure.
	badMsgs := []IterationMessage{
		{Role: "assistant", Content: `[{"type":"text","text":"ok"}]`},
		{Role: "invalid_role", Content: "this should fail the CHECK constraint"},
	}
	err := manager.PersistIteration(ctx, conversationID, 1, 1, badMsgs)
	if err == nil {
		t.Fatal("PersistIteration with bad role error = nil, want CHECK constraint error")
	}

	// Verify the assistant message from the failed transaction was NOT persisted.
	rows, err := queries.ListTurnMessages(ctx, conversationID)
	if err != nil {
		t.Fatalf("ListTurnMessages returned error: %v", err)
	}
	// Only the original user message should exist.
	if len(rows) != 1 {
		t.Fatalf("row count = %d, want 1 (only user message after rollback)", len(rows))
	}
	if rows[0].Role != "user" {
		t.Fatalf("rows[0].Role = %q, want user", rows[0].Role)
	}
}

func TestHistoryManagerCancelIterationDeletesOnlyTargetedIteration(t *testing.T) {
	ctx := context.Background()
	database := newHistoryTestDB(t)
	queries := db.New(database)
	conversationID := seedHistoryConversation(t, database)

	manager := NewHistoryManager(database, nil)
	manager.now = func() time.Time { return time.Unix(1700001000, 0).UTC() }

	// Persist user message + two iterations.
	if err := manager.PersistUserMessage(ctx, conversationID, 1, "fix auth"); err != nil {
		t.Fatalf("PersistUserMessage returned error: %v", err)
	}
	iter1 := []IterationMessage{
		{Role: "assistant", Content: `[{"type":"text","text":"checking"},{"type":"tool_use","id":"tc1","name":"file_read","input":{}}]`},
		{Role: "tool", Content: "file contents", ToolUseID: "tc1", ToolName: "file_read"},
	}
	if err := manager.PersistIteration(ctx, conversationID, 1, 1, iter1); err != nil {
		t.Fatalf("PersistIteration(1) returned error: %v", err)
	}
	iter2 := []IterationMessage{
		{Role: "assistant", Content: `[{"type":"text","text":"fixing"},{"type":"tool_use","id":"tc2","name":"file_edit","input":{}}]`},
		{Role: "tool", Content: "edit applied", ToolUseID: "tc2", ToolName: "file_edit"},
	}
	if err := manager.PersistIteration(ctx, conversationID, 1, 2, iter2); err != nil {
		t.Fatalf("PersistIteration(2) returned error: %v", err)
	}

	// Before cancel: 1 user + 2 iter1 + 2 iter2 = 5
	rowsBefore, err := queries.ListTurnMessages(ctx, conversationID)
	if err != nil {
		t.Fatalf("ListTurnMessages returned error: %v", err)
	}
	if len(rowsBefore) != 5 {
		t.Fatalf("before cancel: row count = %d, want 5", len(rowsBefore))
	}

	// Cancel iteration 2 only.
	if err := manager.CancelIteration(ctx, conversationID, 1, 2); err != nil {
		t.Fatalf("CancelIteration returned error: %v", err)
	}

	// After cancel: 1 user + 2 iter1 = 3
	rowsAfter, err := queries.ListTurnMessages(ctx, conversationID)
	if err != nil {
		t.Fatalf("ListTurnMessages returned error: %v", err)
	}
	if len(rowsAfter) != 3 {
		t.Fatalf("after cancel: row count = %d, want 3", len(rowsAfter))
	}
	// Verify roles of remaining rows.
	wantRoles := []string{"user", "assistant", "tool"}
	for i, want := range wantRoles {
		if rowsAfter[i].Role != want {
			t.Fatalf("rowsAfter[%d].Role = %q, want %q", i, rowsAfter[i].Role, want)
		}
	}
	// Iteration 1 messages should still have iteration=1.
	for i := 1; i < len(rowsAfter); i++ {
		if rowsAfter[i].Iteration != 1 {
			t.Fatalf("rowsAfter[%d].Iteration = %d, want 1", i, rowsAfter[i].Iteration)
		}
	}
}

func TestHistoryManagerCancelIterationDeletesToolExecutionsAndSubCalls(t *testing.T) {
	ctx := context.Background()
	database := newHistoryTestDB(t)
	conversationID := seedHistoryConversation(t, database)
	createdAt := time.Unix(1700001100, 0).UTC().Format(time.RFC3339)

	manager := NewHistoryManager(database, nil)
	manager.now = func() time.Time { return time.Unix(1700001100, 0).UTC() }

	// Persist user message + one iteration.
	if err := manager.PersistUserMessage(ctx, conversationID, 1, "fix auth"); err != nil {
		t.Fatalf("PersistUserMessage returned error: %v", err)
	}
	iter1 := []IterationMessage{
		{Role: "assistant", Content: `[{"type":"tool_use","id":"tc1","name":"file_read","input":{}}]`},
		{Role: "tool", Content: "file contents", ToolUseID: "tc1", ToolName: "file_read"},
	}
	if err := manager.PersistIteration(ctx, conversationID, 1, 1, iter1); err != nil {
		t.Fatalf("PersistIteration returned error: %v", err)
	}

	// Manually insert corresponding tool_execution and sub_call rows for iter 1.
	mustExecHistory(t, database, `INSERT INTO tool_executions(conversation_id, turn_number, iteration, tool_use_id, tool_name, success, duration_ms, created_at)
		VALUES (?, 1, 1, 'tc1', 'file_read', 1, 50, ?)`, conversationID, createdAt)
	mustExecHistory(t, database, `INSERT INTO sub_calls(conversation_id, turn_number, iteration, provider, model, purpose, tokens_in, tokens_out, latency_ms, success, created_at)
		VALUES (?, 1, 1, 'anthropic', 'claude', 'chat', 1000, 200, 500, 1, ?)`, conversationID, createdAt)

	// Verify they exist.
	var toolExecCount, subCallCount int
	database.QueryRowContext(ctx, `SELECT COUNT(*) FROM tool_executions WHERE conversation_id = ?`, conversationID).Scan(&toolExecCount)
	database.QueryRowContext(ctx, `SELECT COUNT(*) FROM sub_calls WHERE conversation_id = ?`, conversationID).Scan(&subCallCount)
	if toolExecCount != 1 {
		t.Fatalf("tool_executions count = %d, want 1", toolExecCount)
	}
	if subCallCount != 1 {
		t.Fatalf("sub_calls count = %d, want 1", subCallCount)
	}

	// Cancel iteration 1.
	if err := manager.CancelIteration(ctx, conversationID, 1, 1); err != nil {
		t.Fatalf("CancelIteration returned error: %v", err)
	}

	// Verify all three tables had iteration 1 data deleted.
	database.QueryRowContext(ctx, `SELECT COUNT(*) FROM tool_executions WHERE conversation_id = ?`, conversationID).Scan(&toolExecCount)
	database.QueryRowContext(ctx, `SELECT COUNT(*) FROM sub_calls WHERE conversation_id = ?`, conversationID).Scan(&subCallCount)
	if toolExecCount != 0 {
		t.Fatalf("after cancel: tool_executions count = %d, want 0", toolExecCount)
	}
	if subCallCount != 0 {
		t.Fatalf("after cancel: sub_calls count = %d, want 0", subCallCount)
	}

	// The user message survives because DeleteIterationMessages excludes
	// role='user'. Only assistant/tool messages are removed.
	var msgCount int
	database.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages WHERE conversation_id = ?`, conversationID).Scan(&msgCount)
	if msgCount != 1 {
		t.Fatalf("after cancel iter 1: message count = %d, want 1 (user message should survive)", msgCount)
	}
}

func TestHistoryManagerCancelIterationIsNoOpForNonExistentIteration(t *testing.T) {
	ctx := context.Background()
	database := newHistoryTestDB(t)
	queries := db.New(database)
	conversationID := seedHistoryConversation(t, database)

	manager := NewHistoryManager(database, nil)
	manager.now = func() time.Time { return time.Unix(1700001200, 0).UTC() }

	if err := manager.PersistUserMessage(ctx, conversationID, 1, "hello"); err != nil {
		t.Fatalf("PersistUserMessage returned error: %v", err)
	}

	// Cancel iteration 99 which doesn't exist — should succeed as a no-op.
	if err := manager.CancelIteration(ctx, conversationID, 1, 99); err != nil {
		t.Fatalf("CancelIteration(nonexistent) returned error: %v", err)
	}

	// User message still there.
	rows, err := queries.ListTurnMessages(ctx, conversationID)
	if err != nil {
		t.Fatalf("ListTurnMessages returned error: %v", err)
	}
	if len(rows) != 1 || rows[0].Role != "user" {
		t.Fatalf("rows = %#v, want single user message", rows)
	}
}

func TestHistoryManagerCancelIterationPreservesUserMessage(t *testing.T) {
	ctx := context.Background()
	database := newHistoryTestDB(t)
	queries := db.New(database)
	conversationID := seedHistoryConversation(t, database)

	manager := NewHistoryManager(database, nil)
	manager.now = func() time.Time { return time.Unix(1700001300, 0).UTC() }

	// User message is iteration=1 in InsertUserMessage.
	if err := manager.PersistUserMessage(ctx, conversationID, 1, "fix auth"); err != nil {
		t.Fatalf("PersistUserMessage returned error: %v", err)
	}

	// Persist iteration 1 (assistant + tool).
	iter1 := []IterationMessage{
		{Role: "assistant", Content: `[{"type":"text","text":"ok"}]`},
	}
	if err := manager.PersistIteration(ctx, conversationID, 1, 1, iter1); err != nil {
		t.Fatalf("PersistIteration returned error: %v", err)
	}

	// Cancel iteration 1 — should remove the assistant messages but NOT the
	// user message. The DeleteIterationMessages query has a `role != 'user'`
	// guard that protects user messages even though they share iteration=1
	// with assistant messages.
	if err := manager.CancelIteration(ctx, conversationID, 1, 1); err != nil {
		t.Fatalf("CancelIteration returned error: %v", err)
	}

	rows, err := queries.ListTurnMessages(ctx, conversationID)
	if err != nil {
		t.Fatalf("ListTurnMessages returned error: %v", err)
	}
	// The user message (iteration=1, role=user) survives the cancel because
	// DeleteIterationMessages excludes role='user'. Only the assistant
	// message is deleted.
	if len(rows) != 1 {
		t.Fatalf("after cancel iter 1: row count = %d, want 1 (user message should survive)", len(rows))
	}
	if rows[0].Role != "user" {
		t.Fatalf("surviving message role = %q, want \"user\"", rows[0].Role)
	}
}

func TestHistoryManagerReconstructHistoryReturnsOnlyActiveMessages(t *testing.T) {
	ctx := context.Background()
	database := newHistoryTestDB(t)
	conversationID := seedHistoryConversation(t, database)
	createdAt := time.Unix(1700000900, 0).UTC().Format(time.RFC3339)

	mustExecHistory(t, database, `INSERT INTO messages(conversation_id, role, content, turn_number, iteration, sequence, created_at)
		VALUES (?, 'user', ?, 1, 1, 0.0, ?)`, conversationID, "fix auth", createdAt)
	mustExecHistory(t, database, `INSERT INTO messages(conversation_id, role, content, turn_number, iteration, sequence, created_at)
		VALUES (?, 'assistant', ?, 1, 1, 1.0, ?)`, conversationID, `[{"type":"text","text":"checking"}]`, createdAt)
	mustExecHistory(t, database, `INSERT INTO messages(conversation_id, role, content, turn_number, iteration, sequence, is_compressed, is_summary, compressed_turn_start, compressed_turn_end, created_at)
		VALUES (?, 'assistant', ?, 9, 1, 1.5, 1, 1, 1, 8, ?)`, conversationID, `[{"type":"text","text":"summary"}]`, createdAt)
	mustExecHistory(t, database, `INSERT INTO messages(conversation_id, role, content, tool_use_id, tool_name, turn_number, iteration, sequence, created_at)
		VALUES (?, 'tool', ?, ?, ?, 1, 1, 2.0, ?)`, conversationID, "file contents", "toolu_1", "file_read", createdAt)

	manager := NewHistoryManager(database, nil)
	history, err := manager.ReconstructHistory(ctx, conversationID)
	if err != nil {
		t.Fatalf("ReconstructHistory returned error: %v", err)
	}
	if len(history) != 3 {
		t.Fatalf("history length = %d, want 3", len(history))
	}
	if history[0].Role != "user" || history[0].Sequence != 0.0 {
		t.Fatalf("history[0] = %#v, want user seq 0.0", history[0])
	}
	if history[1].Role != "assistant" || history[1].Sequence != 1.0 {
		t.Fatalf("history[1] = %#v, want assistant seq 1.0", history[1])
	}
	if history[2].Role != "tool" || history[2].ToolUseID.String != "toolu_1" || history[2].Sequence != 2.0 {
		t.Fatalf("history[2] = %#v, want tool seq 2.0", history[2])
	}
}

func newHistoryTestDB(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "conversation-history.db")
	database, err := db.OpenDB(ctx, dbPath)
	if err != nil {
		t.Fatalf("OpenDB returned error: %v", err)
	}
	if err := db.Init(ctx, database); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func seedHistoryConversation(t *testing.T, database *sql.DB) string {
	t.Helper()
	projectID := sid.New()
	conversationID := sid.New()
	createdAt := time.Unix(1700000700, 0).UTC().Format(time.RFC3339)

	mustExecHistory(t, database, `INSERT INTO projects(id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, projectID, "proj", "/tmp/proj", createdAt, createdAt)
	mustExecHistory(t, database, `INSERT INTO conversations(id, project_id, title, model, provider, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, conversationID, projectID, "Test", "claude", "anthropic", createdAt, createdAt)
	return conversationID
}

func mustExecHistory(t *testing.T, database *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := database.Exec(query, args...); err != nil {
		t.Fatalf("exec failed for %q: %v", query, err)
	}
}
