# Task 02: History Reconstruction and Message Persistence

**Epic:** 02 — Conversation Manager
**Status:** ⬚ Not started
**Dependencies:** Task 01, Layer 0 Epic 06 (schema/sqlc — messages table)

---

## Description

Implement the conversation history operations in `internal/conversation/history.go`. This is the most critical piece of the conversation manager: reconstructing the API-faithful message array from SQLite for LLM calls, persisting the initial user message at turn start, persisting completed assistant/tool iterations as atomic transactions, computing sequence numbers, and handling cancellation cleanup. The reconstruction query is the foundation of the entire conversation persistence model — it must produce exactly the message array the provider expects.

## Acceptance Criteria

- [ ] `Message` struct defined matching the API-faithful storage model: `Role string`, `Content json.RawMessage` (for assistant messages, holds the full content block array), `TextContent string` (convenience field for user/tool messages), `ToolUseID string` (for tool result messages), `ToolName string` (for tool result messages), `Sequence float64`, `TurnNumber int`, `Iteration int`, `IsCompressed bool`, `IsSummary bool`
- [ ] `ReconstructHistory(conversationID string) ([]Message, error)` — executes `SELECT role, content, tool_use_id, tool_name FROM messages WHERE conversation_id = ? AND is_compressed = 0 ORDER BY sequence`. Returns provider-ready message structs. User messages have plain text content, assistant messages have `json.RawMessage` content blocks, tool messages have text content with `tool_use_id`
- [ ] `PersistUserMessage(conversationID string, turnNumber int, message string) error` — inserts the turn's initial `role=user` row before context assembly begins. Uses the next sequence number for the conversation and is intentionally outside the per-iteration assistant/tool transaction so the user's message survives mid-turn failures
- [ ] `PersistIteration(conversationID string, turnNumber, iteration int, messages []Message) error` — wraps all INSERTs in a single SQLite transaction. Inserts the assistant message, all tool result messages, associated tool_execution records, and sub_call record. Each message gets the next sequence number. Transaction rolls back completely on any failure
- [ ] `NextSequence(conversationID string) (float64, error)` — queries `SELECT MAX(sequence) FROM messages WHERE conversation_id = ?` and returns `max + 1.0`. Returns `0.0` for an empty conversation
- [ ] `CancelIteration(conversationID string, turnNumber, iteration int) error` — DELETEs all messages, tool_executions, and sub_calls for the specified conversation + turn + iteration. Only the in-flight iteration is affected; completed iterations are untouched
- [ ] Sequence values are `REAL` (float64) type — normal messages use integer values (0.0, 1.0, 2.0, ...) with gaps reserved for compression summary insertion
- [ ] Package compiles with `go build ./internal/conversation/...`
