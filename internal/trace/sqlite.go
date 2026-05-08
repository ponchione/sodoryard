package trace

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type SQLiteRecorder struct {
	db *sql.DB
}

func NewSQLiteRecorder(db *sql.DB) *SQLiteRecorder {
	if db == nil {
		return nil
	}
	return &SQLiteRecorder{db: db}
}

func (r *SQLiteRecorder) StartSpan(ctx context.Context, span Span) error {
	if r == nil || r.db == nil {
		return nil
	}
	attrs := "{}"
	if len(span.Attributes) > 0 {
		raw, err := json.Marshal(span.Attributes)
		if err != nil {
			return fmt.Errorf("start span: marshal attributes: %w", err)
		}
		attrs = string(raw)
	}
	_, err := r.db.ExecContext(ctx, `
INSERT INTO trace_spans (
    id, trace_id, parent_id, conversation_id, chain_id, step_id,
    turn_number, iteration, name, kind, status, started_at, attributes_json
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		span.ID,
		span.TraceID,
		nullString(span.ParentID),
		nullString(span.ConversationID),
		nullString(span.ChainID),
		nullString(span.StepID),
		span.TurnNumber,
		span.Iteration,
		span.Name,
		span.Kind,
		nonEmptyStatus(span.Status),
		formatTime(span.StartedAt),
		attrs,
	)
	if err != nil {
		return fmt.Errorf("start span: %w", err)
	}
	return nil
}

func (r *SQLiteRecorder) EndSpan(ctx context.Context, end SpanEnd) error {
	if r == nil || r.db == nil || strings.TrimSpace(end.ID) == "" {
		return nil
	}
	endedAt := end.EndedAt
	if endedAt.IsZero() {
		endedAt = time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx, `
UPDATE trace_spans
SET status = ?, ended_at = ?, duration_ms = ?, error = ?
WHERE id = ?`,
		nonEmptyStatus(end.Status),
		formatTime(endedAt),
		nonNegative(end.DurationMs),
		nullString(end.Error),
		end.ID,
	)
	if err != nil {
		return fmt.Errorf("end span: %w", err)
	}
	return nil
}

func (r *SQLiteRecorder) ListSpans(ctx context.Context, query Query) ([]Span, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	where := make([]string, 0, 3)
	args := make([]any, 0, 4)
	if strings.TrimSpace(query.ChainID) != "" {
		where = append(where, "chain_id = ?")
		args = append(args, strings.TrimSpace(query.ChainID))
	}
	if strings.TrimSpace(query.ConversationID) != "" {
		where = append(where, "conversation_id = ?")
		args = append(args, strings.TrimSpace(query.ConversationID))
	}
	if strings.TrimSpace(query.TraceID) != "" {
		where = append(where, "trace_id = ?")
		args = append(args, strings.TrimSpace(query.TraceID))
	}
	sqlText := `
SELECT id, trace_id, parent_id, conversation_id, chain_id, step_id,
       turn_number, iteration, name, kind, status, started_at, ended_at,
       duration_ms, attributes_json, error
FROM trace_spans`
	if len(where) > 0 {
		sqlText += "\nWHERE " + strings.Join(where, " AND ")
	}
	sqlText += "\nORDER BY started_at ASC, id ASC"
	if query.Limit > 0 {
		sqlText += "\nLIMIT ?"
		args = append(args, query.Limit)
	}
	rows, err := r.db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, fmt.Errorf("list spans: %w", err)
	}
	defer rows.Close()

	spans := make([]Span, 0)
	for rows.Next() {
		var span Span
		var parentID, conversationID, chainID, stepID, endedAt, errText sql.NullString
		var startedAt string
		var attrsJSON string
		if err := rows.Scan(
			&span.ID,
			&span.TraceID,
			&parentID,
			&conversationID,
			&chainID,
			&stepID,
			&span.TurnNumber,
			&span.Iteration,
			&span.Name,
			&span.Kind,
			&span.Status,
			&startedAt,
			&endedAt,
			&span.DurationMs,
			&attrsJSON,
			&errText,
		); err != nil {
			return nil, fmt.Errorf("list spans: scan: %w", err)
		}
		span.ParentID = stringFromNull(parentID)
		span.ConversationID = stringFromNull(conversationID)
		span.ChainID = stringFromNull(chainID)
		span.StepID = stringFromNull(stepID)
		span.Error = stringFromNull(errText)
		span.StartedAt = parseTime(startedAt)
		if endedAt.Valid {
			span.EndedAt = parseTime(endedAt.String)
		}
		span.Attributes = map[string]any{}
		if strings.TrimSpace(attrsJSON) != "" {
			_ = json.Unmarshal([]byte(attrsJSON), &span.Attributes)
		}
		spans = append(spans, span)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list spans: iterate: %w", err)
	}
	return spans, nil
}

func nullString(value string) sql.NullString {
	value = strings.TrimSpace(value)
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}

func stringFromNull(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func nonEmptyStatus(status string) string {
	status = strings.TrimSpace(status)
	if status == "" {
		return StatusRunning
	}
	return status
}

func nonNegative(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		value = time.Now().UTC()
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func parseTime(value string) time.Time {
	if t, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t
	}
	return time.Time{}
}
