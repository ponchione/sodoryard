# Task 03: SubCallStore Interface and SQLite Persistence

**Epic:** 06 — Sub-Call Tracking
**Status:** ⬚ Not started
**Dependencies:** Task 01 (TrackedProvider references SubCallStore and InsertSubCallParams), L0-E06 (schema & sqlc — `sub_calls` table DDL and sqlc-generated code)

---

## Description

Define the `SubCallStore` interface and the `InsertSubCallParams` struct in `internal/provider/tracking/store.go`. The `SubCallStore` interface is the persistence boundary for sub-call tracking; its sole method `InsertSubCall` maps directly to the sqlc-generated INSERT query for the `sub_calls` table. The `InsertSubCallParams` struct carries all 16 columns of the `sub_calls` table (minus the auto-increment `id`). This task also provides the `SQLiteSubCallStore` adapter that wraps the sqlc-generated `Queries` type to satisfy the `SubCallStore` interface, ensuring the tracking package depends only on its own interface rather than directly on the sqlc-generated code.

## Acceptance Criteria

- [ ] File `internal/provider/tracking/store.go` exists with `package tracking`
- [ ] The `SubCallStore` interface is defined with exactly one method:
  ```go
  type SubCallStore interface {
      InsertSubCall(ctx context.Context, params InsertSubCallParams) error
  }
  ```
- [ ] The `InsertSubCallParams` struct is defined with exactly these fields, mapping one-to-one with the `sub_calls` table columns (excluding the auto-increment `id`):
  ```go
  type InsertSubCallParams struct {
      ConversationID      *string // nullable — maps to sub_calls.conversation_id (TEXT, FK to conversations.id)
      MessageID           *int64  // nullable — maps to sub_calls.message_id (INTEGER, FK to messages.id)
      TurnNumber          *int    // nullable — maps to sub_calls.turn_number (INTEGER)
      Iteration           *int    // nullable — maps to sub_calls.iteration (INTEGER)
      Provider            string  // required — maps to sub_calls.provider (TEXT NOT NULL)
      Model               string  // required — maps to sub_calls.model (TEXT NOT NULL)
      Purpose             string  // required — maps to sub_calls.purpose (TEXT NOT NULL), one of "chat", "compression", "title_generation"
      TokensIn            int     // required — maps to sub_calls.tokens_in (INTEGER NOT NULL)
      TokensOut           int     // required — maps to sub_calls.tokens_out (INTEGER NOT NULL)
      CacheReadTokens     int     // required — maps to sub_calls.cache_read_tokens (INTEGER NOT NULL DEFAULT 0)
      CacheCreationTokens int     // required — maps to sub_calls.cache_creation_tokens (INTEGER NOT NULL DEFAULT 0)
      LatencyMs           int64   // required — maps to sub_calls.latency_ms (INTEGER NOT NULL)
      Success             int     // required — maps to sub_calls.success (INTEGER NOT NULL), 0 or 1
      ErrorMessage        *string // nullable — maps to sub_calls.error_message (TEXT)
      CreatedAt           string  // required — maps to sub_calls.created_at (TEXT NOT NULL), ISO 8601 format
  }
  ```
- [ ] The `SQLiteSubCallStore` adapter struct is defined:
  ```go
  type SQLiteSubCallStore struct {
      queries SubCallQueries
  }
  ```
  where `SubCallQueries` is a local interface matching the sqlc-generated method signature:
  ```go
  type SubCallQueries interface {
      InsertSubCall(ctx context.Context, arg db.InsertSubCallParams) error
  }
  ```
  `db` refers to the sqlc-generated package from L0-E06 (e.g., `internal/db` or `internal/store`; the exact import path depends on the sqlc configuration from L0-E06).
- [ ] The constructor for the adapter is defined:
  ```go
  func NewSQLiteSubCallStore(queries SubCallQueries) *SQLiteSubCallStore
  ```
- [ ] The `SQLiteSubCallStore` satisfies the `SubCallStore` interface by implementing:
  ```go
  func (s *SQLiteSubCallStore) InsertSubCall(ctx context.Context, params InsertSubCallParams) error
  ```
  This method maps `InsertSubCallParams` to the sqlc-generated `db.InsertSubCallParams` struct field-by-field and calls `s.queries.InsertSubCall(ctx, dbParams)`. Every field is mapped one-to-one; nullable fields (`ConversationID`, `MessageID`, `TurnNumber`, `Iteration`, `ErrorMessage`) use `sql.NullString`, `sql.NullInt64`, or `sql.NullInt32` as required by the sqlc-generated types.
- [ ] The mapping from `InsertSubCallParams` to `db.InsertSubCallParams` handles nullable fields correctly:
  - `*string` fields (`ConversationID`, `ErrorMessage`) map to `sql.NullString{String: *val, Valid: val != nil}`
  - `*int64` field (`MessageID`) maps to `sql.NullInt64{Int64: *val, Valid: val != nil}`
  - `*int` fields (`TurnNumber`, `Iteration`) map to `sql.NullInt64{Int64: int64(*val), Valid: val != nil}` (or `sql.NullInt32` depending on sqlc output; match whatever the sqlc-generated type uses)
- [ ] The `InsertSubCall` method on `SQLiteSubCallStore` returns the error from the sqlc call directly — it does NOT wrap, log, or swallow errors. Error handling policy (log and swallow) is enforced by `TrackedProvider`, not by the store adapter.
- [ ] Tracking failures never block inference: the `SubCallStore` interface is designed so that `TrackedProvider` can catch any error from `InsertSubCall` and handle it by logging. The store itself makes no guarantees about error suppression; that responsibility belongs to the caller (`TrackedProvider.Complete` and `TrackedProvider.Stream`).
- [ ] A compile-time interface check is present:
  ```go
  var _ SubCallStore = (*SQLiteSubCallStore)(nil)
  ```
- [ ] The file imports `context`, `database/sql`, and the sqlc-generated package (the exact import path matches the L0-E06 sqlc configuration)
- [ ] The file compiles with `go build ./internal/provider/tracking/...` with no errors
