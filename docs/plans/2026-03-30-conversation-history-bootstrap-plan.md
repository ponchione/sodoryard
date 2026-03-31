# Conversation History Bootstrap Plan

> For Hermes: execute one narrow slice only, using strict TDD, one local commit, and no push.

Goal: land the first real `internal/conversation/history.go` implementation so the agent loop can use a DB-backed conversation history manager for `PersistUserMessage(...)` and `ReconstructHistory(...)` instead of only test stubs.

Architecture:
- Keep the slice focused on turn-start history only.
- Use sqlc-generated DB queries for message insert / sequence lookup / active-history reconstruction.
- Provide a small concrete history manager in `internal/conversation` that also exposes the existing in-memory `SeenFiles` tracker.

Tech stack: Go, SQLite, sqlc, `internal/db`, `internal/conversation`, `internal/agent`.

---

## Scope for this slice

Implement:
- `internal/conversation/history.go`
- targeted sqlc queries for:
  - next sequence
  - insert user message
  - touch conversation `updated_at`
  - list active messages as full `db.Message` rows
- DB-backed tests for conversation history
- one focused agent-loop integration test using the real history manager

Do not implement:
- `PersistIteration(...)`
- `CancelIteration(...)`
- full conversation CRUD
- title generation
- prompt building / provider streaming / tool execution

## Files likely to change
- Modify: `internal/db/query/conversation.sql`
- Regenerate: `internal/db/conversation.sql.go`
- Create: `internal/conversation/history.go`
- Create: `internal/conversation/history_test.go`
- Create or modify: one narrow build-tagged agent integration test proving real wiring

## TDD steps
1. Write failing DB-backed tests for `PersistUserMessage(...)` and `ReconstructHistory(...)`.
2. Run the targeted tests with `-tags sqlite_fts5` and confirm RED.
3. Add the minimal sqlc queries and regenerate code.
4. Implement the concrete history manager.
5. Add a focused agent integration test using the real history manager.
6. Re-run:
   - `go test -tags sqlite_fts5 ./internal/conversation/... ./internal/agent/... ./internal/db/...`
   - `go test ./internal/agent/... ./internal/conversation/...`
   - `go build ./internal/agent/... ./internal/conversation/...`
7. Commit only the focused files.

## Key behaviors to prove
- `PersistUserMessage(...)` inserts a `role=user` row with the next `REAL` sequence number
- first message in an empty conversation gets sequence `0.0`
- subsequent user messages increment sequence by `1.0`
- conversation `updated_at` is touched when the user message is persisted
- `ReconstructHistory(...)` returns only active messages in sequence order as full `db.Message` rows
- agent loop `RunTurn(...)` works against the real history manager for the turn-start path

## Suggested commit message
- `feat(conversation): add history manager bootstrap`
