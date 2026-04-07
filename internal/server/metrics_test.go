//go:build sqlite_fts5
// +build sqlite_fts5

package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	appdb "github.com/ponchione/sirtopham/internal/db"
	sid "github.com/ponchione/sirtopham/internal/id"
)

func TestContextSignalStreamEndpointReturnsOrderedSignalFlow(t *testing.T) {
	database := newMetricsTestDB(t)
	queries := appdb.New(database)
	conversationID := seedMetricsTestConversation(t, database)
	createdAt := time.Now().UTC().Format(time.RFC3339)

	needsJSON := `{"semantic_queries":["runtime brain proof canary"],"explicit_files":["internal/context/retrieval.go"],"explicit_symbols":["HeuristicQueryExtractor"],"momentum_files":["internal/context/query.go"],"momentum_module":"internal/context","prefer_brain_context":true,"include_conventions":true,"include_git_context":true,"git_context_depth":3}`
	signalsJSON := `[{"type":"brain_intent","source":"project brain","value":"prefer_brain_context"},{"type":"file_ref","source":"internal/context/retrieval.go","value":"internal/context/retrieval.go"}]`
	mustExecMetrics(t, database, `INSERT INTO context_reports(conversation_id, turn_number, analysis_latency_ms, retrieval_latency_ms, total_latency_ms,
		needs_json, signals_json, rag_results_json, brain_results_json, graph_results_json, explicit_files_json,
		budget_total, budget_used, budget_breakdown_json, included_count, excluded_count,
		agent_used_search_tool, agent_read_files_json, context_hit_rate, created_at)
		VALUES (?, 1, 1, 2, 3, ?, ?, '[]', '[]', '[]', '[]', 1000, 250, '{"brain":125}', 2, 0, 0, '[]', 1.0, ?)`, conversationID, needsJSON, signalsJSON, createdAt)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(Config{}, logger)
	NewMetricsHandler(srv, queries, logger)

	req := httptest.NewRequest(http.MethodGet, "/api/metrics/conversation/"+conversationID+"/context/1/signals", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	var resp contextSignalStreamResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ConversationID != conversationID {
		t.Fatalf("conversation_id = %q, want %q", resp.ConversationID, conversationID)
	}
	if resp.TurnNumber != 1 {
		t.Fatalf("turn_number = %d, want 1", resp.TurnNumber)
	}
	if len(resp.Stream) != 10 {
		t.Fatalf("len(stream) = %d, want 10; stream=%+v", len(resp.Stream), resp.Stream)
	}

	assertSignalStreamEntry(t, resp.Stream[0], 0, "signal", "brain_intent", "project brain", "prefer_brain_context")
	assertSignalStreamEntry(t, resp.Stream[1], 1, "signal", "file_ref", "internal/context/retrieval.go", "internal/context/retrieval.go")
	assertSignalStreamEntry(t, resp.Stream[2], 2, "semantic_query", "", "", "runtime brain proof canary")
	assertSignalStreamEntry(t, resp.Stream[3], 3, "explicit_file", "", "", "internal/context/retrieval.go")
	assertSignalStreamEntry(t, resp.Stream[4], 4, "explicit_symbol", "", "", "HeuristicQueryExtractor")
	assertSignalStreamEntry(t, resp.Stream[5], 5, "momentum_file", "", "", "internal/context/query.go")
	assertSignalStreamEntry(t, resp.Stream[6], 6, "momentum_module", "", "", "internal/context")
	assertSignalStreamEntry(t, resp.Stream[7], 7, "flag", "prefer_brain_context", "", "true")
	assertSignalStreamEntry(t, resp.Stream[8], 8, "flag", "include_conventions", "", "true")
	assertSignalStreamEntry(t, resp.Stream[9], 9, "flag", "include_git_context", "", "depth=3")
}

func assertSignalStreamEntry(t *testing.T, got contextSignalStreamEntry, wantIndex int, wantKind string, wantType string, wantSource string, wantValue string) {
	t.Helper()
	if got.Index != wantIndex || got.Kind != wantKind || got.Type != wantType || got.Source != wantSource || got.Value != wantValue {
		t.Fatalf("stream entry = %+v, want index=%d kind=%q type=%q source=%q value=%q", got, wantIndex, wantKind, wantType, wantSource, wantValue)
	}
}

func newMetricsTestDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := appdb.Init(context.Background(), database); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	return database
}

func seedMetricsTestConversation(t *testing.T, database *sql.DB) string {
	t.Helper()
	projectID := sid.New()
	conversationID := sid.New()
	createdAt := time.Now().UTC().Format(time.RFC3339)
	mustExecMetrics(t, database, `INSERT INTO projects(id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, projectID, "proj", "/tmp/proj", createdAt, createdAt)
	mustExecMetrics(t, database, `INSERT INTO conversations(id, project_id, title, model, provider, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, conversationID, projectID, "Signal Flow", "qwen-coder", "local", createdAt, createdAt)
	return conversationID
}

func mustExecMetrics(t *testing.T, database *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := database.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}
