//go:build sqlite_fts5
// +build sqlite_fts5

package trace

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	appdb "github.com/ponchione/sodoryard/internal/db"
)

func TestNoopRecorderBehavior(t *testing.T) {
	ctx := ContextWithScope(context.Background(), Scope{ChainID: "chain-1"})
	ctx, span := StartSpan(ctx, NoopRecorder{}, SpanStart{Name: "noop", Kind: KindProvider})
	if span.ID() != "" || span.TraceID() != "" {
		t.Fatalf("noop span = id %q trace %q, want empty", span.ID(), span.TraceID())
	}
	span.End(ctx, StatusOK, nil)
	spans, err := NoopRecorder{}.ListSpans(ctx, Query{ChainID: "chain-1"})
	if err != nil {
		t.Fatalf("ListSpans returned error: %v", err)
	}
	if len(spans) != 0 {
		t.Fatalf("spans = %+v, want none", spans)
	}
}

func TestScopeFromContextReturnsCurrentScope(t *testing.T) {
	ctx := ContextWithScope(context.Background(), Scope{
		TraceID:        "trace-1",
		ParentSpanID:   "span-parent",
		ConversationID: "conv-1",
		ChainID:        "chain-1",
		StepID:         "step-1",
		TurnNumber:     2,
		Iteration:      3,
	})
	scope := ScopeFromContext(ctx)
	if scope.TraceID != "trace-1" || scope.ParentSpanID != "span-parent" || scope.ConversationID != "conv-1" || scope.ChainID != "chain-1" || scope.StepID != "step-1" || scope.TurnNumber != 2 || scope.Iteration != 3 {
		t.Fatalf("scope = %+v, want attached scope", scope)
	}
}

func TestSQLiteRecorderStartEndAndParentGrouping(t *testing.T) {
	ctx := context.Background()
	db := newTraceTestDB(t)
	recorder := NewSQLiteRecorder(db)

	start := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	ctx = ContextWithScope(ctx, Scope{ChainID: "chain-1", ConversationID: "conv-1", TurnNumber: 1})
	ctx, parent := StartSpan(ctx, recorder, SpanStart{Name: "provider.stream", Kind: KindProvider, StartedAt: start})
	if parent.ID() == "" || parent.TraceID() == "" {
		t.Fatal("parent span did not start")
	}
	_, child := StartSpan(ctx, recorder, SpanStart{Name: "tool.file_read", Kind: KindTool, Iteration: 2})
	child.End(ctx, StatusError, context.DeadlineExceeded)
	parent.End(ctx, StatusOK, nil)

	spans, err := recorder.ListSpans(ctx, Query{ChainID: "chain-1"})
	if err != nil {
		t.Fatalf("ListSpans returned error: %v", err)
	}
	if len(spans) != 2 {
		t.Fatalf("span count = %d, want 2", len(spans))
	}
	if spans[0].TraceID != spans[1].TraceID || spans[1].ParentID != spans[0].ID {
		t.Fatalf("span grouping = parent %+v child %+v", spans[0], spans[1])
	}
	if spans[1].Status != StatusCancelled || spans[1].Error == "" {
		t.Fatalf("child status/error = %q/%q, want cancelled with error", spans[1].Status, spans[1].Error)
	}
	if spans[0].DurationMs < 0 || spans[0].EndedAt.IsZero() {
		t.Fatalf("parent duration/ended_at = %d/%v, want completed span", spans[0].DurationMs, spans[0].EndedAt)
	}
}

func newTraceTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := appdb.InitIfNeeded(context.Background(), db); err != nil {
		t.Fatalf("InitIfNeeded returned error: %v", err)
	}
	if err := appdb.EnsureTraceSchema(context.Background(), db); err != nil {
		t.Fatalf("EnsureTraceSchema returned error: %v", err)
	}
	return db
}
