# L5-E02 — Conversation Manager

**Layer:** 5 — Agent Loop
**Epic:** 02
**Status:** ⬚ Not started
**Dependencies:** L5-E01 (types), Layer 0 Epic 04 (SQLite), Layer 0 Epic 05 (UUIDv7), Layer 0 Epic 06 (schema/sqlc), Layer 2 Epic 01 (provider types), Layer 2 Epics 06-07 (sub-call tracking, router)

---

## Description

Implement the conversation lifecycle manager in `internal/conversation/`. This covers creating new conversations, loading conversation history from SQLite, persisting messages (user message at turn start plus per-iteration atomic transactions for assistant/tool output), reconstructing the API-faithful message array for LLM calls, tracking seen files across a session, and auto-generating conversation titles. This is a shared package — both the agent loop (Layer 5) and the future REST API (Layer 6/7) consume it.

Title generation is included here because it's a conversation-level concern: after the first assistant response in a new conversation, fire an async lightweight LLM call (via the Provider interface) to generate a short title from the first user message. The sub-call is recorded with `purpose='title_generation'`.

---

## Definition of Done

- [ ] `internal/conversation/manager.go` provides `ConversationManager` with methods for:
  - `Create(projectID string, opts ...CreateOption) (*Conversation, error)` — creates a conversation with UUIDv7 ID, inserts into `conversations` table
  - `Get(conversationID string) (*Conversation, error)` — loads conversation metadata
  - `List(projectID string, limit, offset int) ([]Conversation, error)` — lists conversations ordered by `updated_at DESC`
  - `Delete(conversationID string) error` — cascade deletes conversation and all related records
  - `SetTitle(conversationID, title string) error` — manual title override
- [ ] `internal/conversation/history.go` provides conversation history operations:
  - `ReconstructHistory(conversationID string) ([]Message, error)` — the critical reconstruction query from [[08-data-model]]: `SELECT role, content, tool_use_id, tool_name FROM messages WHERE conversation_id = ? AND is_compressed = 0 ORDER BY sequence`. Returns API-faithful message structs ready to send to the provider
  - `PersistUserMessage(conversationID string, turnNumber int, message string) error` — inserts the turn's initial `role=user` row before context assembly begins so the user's message survives mid-turn failures or cancellation
  - `PersistIteration(conversationID string, turnNumber, iteration int, messages []Message) error` — atomic transaction that INSERTs assistant message + tool result messages + sub_call + tool_execution records per [[08-data-model]] §Persistence Transaction Model
  - `NextSequence(conversationID string) (float64, error)` — returns the next integer sequence value for a conversation
  - `CancelIteration(conversationID string, turnNumber, iteration int) error` — DELETEs messages, tool_executions, and sub_calls for the cancelled iteration per [[08-data-model]] §Cancellation Safety
- [ ] `internal/conversation/seen.go` provides `SeenFiles` tracker — an in-memory set per session tracking files that appeared in tool results. Methods: `Add(path string, turnNumber int)`, `Contains(path string) (bool, int)` (returns whether seen and which turn). Used by context assembly for the `[previously viewed in turn N]` annotation per [[06-context-assembly]] §Interaction with Conversation History
- [ ] `internal/conversation/title.go` provides `GenerateTitle(ctx, provider, conversationID, firstMessage string) error` — async title generation after the first turn. Uses `Provider.Complete()` with a short prompt, records a sub_call with `purpose='title_generation'`. Updates the conversation's `title` field. Failure is non-fatal (logged, conversation keeps null title)
- [ ] Message structs match the API-faithful storage model from [[08-data-model]] §Message Storage Model:
  - `role=user`: content is plain text
  - `role=assistant`: content is `json.RawMessage` (JSON array of content blocks, passed through without transformation)
  - `role=tool`: content is plain text, with `tool_use_id` and `tool_name`
- [ ] Sequence numbering uses `REAL` type per [[08-data-model]] §Sequence Numbering — normal messages get integer values (0.0, 1.0, 2.0, ...), with gaps reserved for compression summary insertion
- [ ] All database operations use sqlc-generated code from Layer 0 Epic 06
- [ ] Integration test: create a conversation, persist 3 iterations with mixed message types (user, assistant with tool_calls, tool results), reconstruct history, verify the message array matches what was persisted
- [ ] Integration test: cancel an iteration, verify only that iteration's messages are removed and earlier iterations survive
- [ ] Integration test: title generation fires asynchronously and updates the conversation record

---

## Architecture References

- [[08-data-model]] §Message Storage Model — API-faithful rows, content by role
- [[08-data-model]] §Persistence Transaction Model — per-iteration atomic commit
- [[08-data-model]] §Cancellation Safety — DELETE by conversation_id + turn_number + iteration
- [[08-data-model]] §Compression Model — REAL sequence numbering (this epic sets up the numbering; Layer 3 Epic 07 implements compression)
- [[08-data-model]] §Key Query Patterns — reconstruction query, conversation list query
- [[05-agent-loop]] §Turn Complete (step 8) — persistence requirements after each turn
- [[05-agent-loop]] §Cancellation §Post-Cancellation State — what survives vs what's discarded
- [[06-context-assembly]] §Interaction with Conversation History — seenFiles tracking, annotation approach

---

## Notes

- The `ConversationManager` is deliberately a separate package from the agent loop (`internal/conversation/` not `internal/agent/`). The REST API (Layer 6/7) needs the same CRUD operations without importing the full agent loop.
- The user message for a turn is persisted separately at turn start. `PersistIteration` handles the transaction boundary described in [[08-data-model]] §Persistence Transaction Model for completed assistant/tool iterations — all INSERTs in a single atomic transaction. The agent loop calls this after each completed iteration, not batched to end of turn.
- The `SeenFiles` tracker is session-scoped (in-memory), not persisted to SQLite. It resets when the conversation manager is instantiated. This is correct — it only needs to track what's been seen in the current running session.
- Title generation uses whatever provider/model is configured as default via the provider router. It's a simple call — extract a 5-8 word title from the user's first message. No extended thinking, no tool calling.
