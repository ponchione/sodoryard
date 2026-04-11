// Package tracking provides a cross-cutting sub-call tracking layer that wraps
// any provider to record every LLM invocation to SQLite.
package tracking

import (
	"context"
	"database/sql"

	db "github.com/ponchione/sodoryard/internal/db"
)

// SubCallStore is the persistence boundary for sub-call tracking. Its sole
// method maps directly to the sqlc-generated INSERT query for the sub_calls table.
type SubCallStore interface {
	InsertSubCall(ctx context.Context, params InsertSubCallParams) error
}

// InsertSubCallParams carries all columns of the sub_calls table (excluding the
// auto-increment id). Nullable fields use Go pointers; the SQLiteSubCallStore
// adapter converts them to sql.Null* types for the sqlc layer.
type InsertSubCallParams struct {
	ConversationID      *string // nullable — maps to sub_calls.conversation_id (TEXT, FK to conversations.id)
	MessageID           *int64  // nullable — maps to sub_calls.message_id (INTEGER, FK to messages.id)
	TurnNumber          *int    // nullable — maps to sub_calls.turn_number (INTEGER)
	Iteration           *int    // nullable — maps to sub_calls.iteration (INTEGER)
	Provider            string  // required — maps to sub_calls.provider (TEXT NOT NULL)
	Model               string  // required — maps to sub_calls.model (TEXT NOT NULL)
	Purpose             string  // required — maps to sub_calls.purpose (TEXT NOT NULL)
	TokensIn            int     // required — maps to sub_calls.tokens_in (INTEGER NOT NULL)
	TokensOut           int     // required — maps to sub_calls.tokens_out (INTEGER NOT NULL)
	CacheReadTokens     int     // required — maps to sub_calls.cache_read_tokens (INTEGER NOT NULL DEFAULT 0)
	CacheCreationTokens int     // required — maps to sub_calls.cache_creation_tokens (INTEGER NOT NULL DEFAULT 0)
	LatencyMs           int64   // required — maps to sub_calls.latency_ms (INTEGER NOT NULL)
	Success             int     // required — maps to sub_calls.success (INTEGER NOT NULL), 0 or 1
	ErrorMessage        *string // nullable — maps to sub_calls.error_message (TEXT)
	CreatedAt           string  // required — maps to sub_calls.created_at (TEXT NOT NULL), ISO 8601 format
}

// SubCallQueries is a local interface matching the sqlc-generated method
// signature for inserting sub-call records.
type SubCallQueries interface {
	InsertSubCall(ctx context.Context, arg db.InsertSubCallParams) error
}

// SQLiteSubCallStore adapts the sqlc-generated Queries type to satisfy the
// SubCallStore interface, converting tracking-layer types to sqlc types.
type SQLiteSubCallStore struct {
	queries SubCallQueries
}

// Compile-time interface check.
var _ SubCallStore = (*SQLiteSubCallStore)(nil)

// NewSQLiteSubCallStore creates a new SQLiteSubCallStore wrapping the given
// sqlc-generated queries interface.
func NewSQLiteSubCallStore(queries SubCallQueries) *SQLiteSubCallStore {
	return &SQLiteSubCallStore{queries: queries}
}

// InsertSubCall maps InsertSubCallParams to the sqlc-generated db.InsertSubCallParams
// and delegates to the sqlc layer. Errors are returned directly — error handling
// policy (log and swallow) is enforced by TrackedProvider, not by the store adapter.
func (s *SQLiteSubCallStore) InsertSubCall(ctx context.Context, params InsertSubCallParams) error {
	dbParams := db.InsertSubCallParams{
		Provider:            params.Provider,
		Model:               params.Model,
		Purpose:             params.Purpose,
		TokensIn:            int64(params.TokensIn),
		TokensOut:           int64(params.TokensOut),
		CacheReadTokens:     int64(params.CacheReadTokens),
		CacheCreationTokens: int64(params.CacheCreationTokens),
		LatencyMs:           params.LatencyMs,
		Success:             int64(params.Success),
		CreatedAt:           params.CreatedAt,
	}

	// Map nullable *string fields to sql.NullString.
	if params.ConversationID != nil {
		dbParams.ConversationID = sql.NullString{String: *params.ConversationID, Valid: true}
	}
	if params.ErrorMessage != nil {
		dbParams.ErrorMessage = sql.NullString{String: *params.ErrorMessage, Valid: true}
	}

	// Map nullable *int64 field to sql.NullInt64.
	if params.MessageID != nil {
		dbParams.MessageID = sql.NullInt64{Int64: *params.MessageID, Valid: true}
	}

	// Map nullable *int fields to sql.NullInt64.
	if params.TurnNumber != nil {
		dbParams.TurnNumber = sql.NullInt64{Int64: int64(*params.TurnNumber), Valid: true}
	}
	if params.Iteration != nil {
		dbParams.Iteration = sql.NullInt64{Int64: int64(*params.Iteration), Valid: true}
	}

	return s.queries.InsertSubCall(ctx, dbParams)
}
