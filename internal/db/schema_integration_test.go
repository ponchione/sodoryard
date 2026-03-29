//go:build sqlite_fts5
// +build sqlite_fts5

package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	sid "github.com/ponchione/sirtopham/internal/id"
)

func TestInitCreatesTablesAndRoundTrips(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	if err := Init(ctx, db); err != nil {
		t.Fatalf("Init second run returned error: %v", err)
	}
	assertTableCount(t, db, "projects", 0)

	projectID := sid.New()
	conversationID := sid.New()
	createdAt := time.Now().UTC().Format(time.RFC3339)

	mustExec(t, db, `INSERT INTO projects(id, name, root_path, language, last_indexed_commit, last_indexed_at, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, projectID, "sirtopham", "/tmp/sirtopham", "go", "abc123", createdAt, createdAt, createdAt)
	mustExec(t, db, `INSERT INTO conversations(id, project_id, title, model, provider, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?)`, conversationID, projectID, "Layer 0", "claude-sonnet-4-6", "anthropic", createdAt, createdAt)

	mustExec(t, db, `INSERT INTO messages(conversation_id, role, content, turn_number, iteration, sequence, created_at)
        VALUES (?, 'user', ?, 1, 1, 1.0, ?)`, conversationID, "hello", createdAt)
	mustExec(t, db, `INSERT INTO messages(conversation_id, role, content, turn_number, iteration, sequence, created_at)
        VALUES (?, 'assistant', ?, 1, 1, 2.0, ?)`, conversationID, `[{"type":"text","text":"hi"}]`, createdAt)
	mustExec(t, db, `INSERT INTO messages(conversation_id, role, content, tool_use_id, tool_name, turn_number, iteration, sequence, created_at)
        VALUES (?, 'tool', ?, ?, ?, 1, 1, 3.0, ?)`, conversationID, "tool output", "toolu_1", "read_file", createdAt)

	mustExec(t, db, `INSERT INTO tool_executions(conversation_id, turn_number, iteration, tool_use_id, tool_name, input, output_size, error, success, duration_ms, created_at)
        VALUES (?, 1, 1, ?, ?, ?, ?, ?, ?, ?, ?)`, conversationID, "toolu_1", "read_file", `{"path":"README.md"}`, 42, nil, 1, 12, createdAt)

	mustExec(t, db, `INSERT INTO sub_calls(conversation_id, turn_number, iteration, provider, model, purpose, tokens_in, tokens_out, cache_read_tokens, cache_creation_tokens, latency_ms, success, error_message, created_at)
        VALUES (?, 1, 1, ?, ?, 'chat', 100, 25, 10, 5, 30, 1, NULL, ?)`, conversationID, "anthropic", "claude-sonnet-4-6", createdAt)

	mustExec(t, db, `INSERT INTO context_reports(conversation_id, turn_number, analysis_latency_ms, retrieval_latency_ms, total_latency_ms,
        needs_json, signals_json, rag_results_json, brain_results_json, graph_results_json, explicit_files_json,
        budget_total, budget_used, budget_breakdown_json, included_count, excluded_count,
        agent_used_search_tool, agent_read_files_json, context_hit_rate, created_at)
        VALUES (?, 1, 1, 2, 3, ?, ?, ?, ?, ?, ?, 1000, 750, ?, 3, 1, 1, ?, 0.5, ?)`,
		conversationID, `{}`, `[]`, `[]`, `[]`, `[]`, `[]`, `{"rag":500}`, `[]`, createdAt)

	mustExec(t, db, `INSERT INTO brain_documents(project_id, path, title, content_hash, tags, frontmatter, token_count, created_by, source_conversation_id, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, projectID, "notes.md", "Notes", "hash-1", `["tag"]`, `{}`, 10, "agent", conversationID, createdAt, createdAt)
	mustExec(t, db, `INSERT INTO brain_links(project_id, source_path, target_path, link_text)
        VALUES (?, ?, ?, ?)`, projectID, "notes.md", "other.md", "Other")
	mustExec(t, db, `INSERT INTO index_state(project_id, file_path, file_hash, chunk_count, last_indexed_at)
        VALUES (?, ?, ?, ?, ?)`, projectID, "main.go", "hash-2", 4, createdAt)

	queries := New(db)
	history, err := queries.ReconstructConversationHistory(ctx, conversationID)
	if err != nil {
		t.Fatalf("ReconstructConversationHistory returned error: %v", err)
	}
	if len(history) != 3 {
		t.Fatalf("history length = %d, want 3", len(history))
	}

	conversations, err := queries.ListConversations(ctx, ListConversationsParams{ProjectID: projectID, Limit: 10, Offset: 0})
	if err != nil {
		t.Fatalf("ListConversations returned error: %v", err)
	}
	if len(conversations) != 1 {
		t.Fatalf("conversation count = %d, want 1", len(conversations))
	}

	turns, err := queries.ListTurnMessages(ctx, conversationID)
	if err != nil {
		t.Fatalf("ListTurnMessages returned error: %v", err)
	}
	if len(turns) != 3 {
		t.Fatalf("turn message count = %d, want 3", len(turns))
	}

	conversationKey := sql.NullString{String: conversationID, Valid: true}

	usage, err := queries.GetConversationTokenUsage(ctx, conversationKey)
	if err != nil {
		t.Fatalf("GetConversationTokenUsage returned error: %v", err)
	}
	if usage.TotalIn != 100 || usage.TotalOut != 25 || usage.TotalCacheHits != 10 || usage.TotalCalls != 1 || usage.TotalLatencyMs != 30 {
		t.Fatalf("unexpected usage totals: %+v", usage)
	}

	cacheHit, err := queries.GetConversationCacheHitRate(ctx, conversationKey)
	if err != nil {
		t.Fatalf("GetConversationCacheHitRate returned error: %v", err)
	}
	if cacheHit != 10 {
		t.Fatalf("unexpected cache hit rate: %v", cacheHit)
	}

	toolUsage, err := queries.GetConversationToolUsage(ctx, conversationID)
	if err != nil {
		t.Fatalf("GetConversationToolUsage returned error: %v", err)
	}
	if len(toolUsage) != 1 || toolUsage[0].ToolName != "read_file" {
		t.Fatalf("unexpected tool usage rows: %+v", toolUsage)
	}

	quality, err := queries.GetConversationContextQuality(ctx, conversationID)
	if err != nil {
		t.Fatalf("GetConversationContextQuality returned error: %v", err)
	}
	if quality.TotalTurns != 1 || !quality.AvgHitRate.Valid || quality.AvgHitRate.Float64 != 0.5 {
		t.Fatalf("unexpected context quality: %+v", quality)
	}
}

func TestFTSAndCascadeBehavior(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	queries := New(db)

	projectID := sid.New()
	conversationID := sid.New()
	createdAt := time.Now().UTC().Format(time.RFC3339)

	mustExec(t, db, `INSERT INTO projects(id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, projectID, "proj", "/tmp/proj", createdAt, createdAt)
	mustExec(t, db, `INSERT INTO conversations(id, project_id, title, model, provider, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, conversationID, projectID, "Searchable", "claude", "anthropic", createdAt, createdAt)
	mustExec(t, db, `INSERT INTO messages(conversation_id, role, content, turn_number, iteration, sequence, created_at) VALUES (?, 'user', ?, 1, 1, 1.0, ?)`, conversationID, "auth middleware", createdAt)

	results, err := queries.SearchConversations(ctx, "auth")
	if err != nil {
		t.Fatalf("SearchConversations returned error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected FTS search results, got none")
	}

	mustExec(t, db, `DELETE FROM messages WHERE conversation_id = ?`, conversationID)
	results, err = queries.SearchConversations(ctx, "auth")
	if err != nil {
		t.Fatalf("SearchConversations after delete returned error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected FTS search results to be removed after delete, got %d rows", len(results))
	}

	mustExec(t, db, `INSERT INTO messages(conversation_id, role, content, turn_number, iteration, sequence, created_at) VALUES (?, 'user', ?, 1, 1, 1.0, ?)`, conversationID, "auth middleware", createdAt)
	mustExec(t, db, `INSERT INTO tool_executions(conversation_id, turn_number, iteration, tool_use_id, tool_name, success, duration_ms, created_at) VALUES (?, 1, 1, 'toolu_1', 'read_file', 1, 10, ?)`, conversationID, createdAt)
	mustExec(t, db, `DELETE FROM conversations WHERE id = ?`, conversationID)
	assertTableCountWhere(t, db, "messages", "conversation_id = ?", conversationID, 0)
	assertTableCountWhere(t, db, "tool_executions", "conversation_id = ?", conversationID, 0)

	secondConversationID := sid.New()
	mustExec(t, db, `INSERT INTO conversations(id, project_id, title, model, provider, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, secondConversationID, projectID, "Second", "claude", "anthropic", createdAt, createdAt)
	mustExec(t, db, `INSERT INTO messages(conversation_id, role, content, turn_number, iteration, sequence, created_at) VALUES (?, 'user', ?, 1, 1, 1.0, ?)`, secondConversationID, "hello", createdAt)
	mustExec(t, db, `DELETE FROM projects WHERE id = ?`, projectID)
	assertTableCountWhere(t, db, "conversations", "project_id = ?", projectID, 0)
	assertTableCountWhere(t, db, "messages", "conversation_id = ?", secondConversationID, 0)
}

func TestSequenceSortingAndUUIDv7ForeignKeys(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	queries := New(db)

	projectID := sid.New()
	conversationID := sid.New()
	createdAt := time.Now().UTC().Format(time.RFC3339)

	mustExec(t, db, `INSERT INTO projects(id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, projectID, "proj", "/tmp/sequence", createdAt, createdAt)
	mustExec(t, db, `INSERT INTO conversations(id, project_id, title, model, provider, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, conversationID, projectID, "Sequence", "claude", "anthropic", createdAt, createdAt)
	mustExec(t, db, `INSERT INTO messages(conversation_id, role, content, turn_number, iteration, sequence, created_at) VALUES (?, 'user', 'one', 1, 1, 1.0, ?)`, conversationID, createdAt)
	mustExec(t, db, `INSERT INTO messages(conversation_id, role, content, turn_number, iteration, sequence, created_at) VALUES (?, 'assistant', '[{"type":"text","text":"two"}]', 2, 1, 2.0, ?)`, conversationID, createdAt)
	mustExec(t, db, `INSERT INTO messages(conversation_id, role, content, turn_number, iteration, sequence, is_compressed, is_summary, compressed_turn_start, compressed_turn_end, created_at) VALUES (?, 'assistant', '[{"type":"text","text":"summary"}]', 20, 1, 20.5, 1, 1, 1, 19, ?)`, conversationID, createdAt)
	mustExec(t, db, `INSERT INTO messages(conversation_id, role, content, turn_number, iteration, sequence, created_at) VALUES (?, 'tool', 'tool', 21, 1, 21.0, ?)`, conversationID, createdAt)

	rows, err := queries.ListTurnMessages(ctx, conversationID)
	if err != nil {
		t.Fatalf("ListTurnMessages returned error: %v", err)
	}
	if len(rows) != 4 {
		t.Fatalf("turn row count = %d, want 4", len(rows))
	}
	if rows[2].Sequence != 20.5 {
		t.Fatalf("expected midpoint summary sequence 20.5 at index 2, got %v", rows[2].Sequence)
	}

	listed, err := queries.ListConversations(ctx, ListConversationsParams{ProjectID: projectID, Limit: 10, Offset: 0})
	if err != nil {
		t.Fatalf("ListConversations returned error: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != conversationID {
		t.Fatalf("unexpected conversation listing rows: %+v", listed)
	}
}

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "layer0.db")
	db, err := OpenDB(ctx, dbPath)
	if err != nil {
		t.Fatalf("OpenDB returned error: %v", err)
	}
	if err := Init(ctx, db); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec failed for %q: %v", query, err)
	}
}

func assertTableCount(t *testing.T, db *sql.DB, table string, want int) {
	t.Helper()
	assertTableCountWhere(t, db, table, "1=1", nil, want)
}

func assertTableCountWhere(t *testing.T, db *sql.DB, table string, where string, arg any, want int) {
	t.Helper()
	query := `SELECT COUNT(*) FROM ` + table + ` WHERE ` + where
	var got int
	var err error
	if arg == nil {
		err = db.QueryRow(query).Scan(&got)
	} else {
		err = db.QueryRow(query, arg).Scan(&got)
	}
	if err != nil {
		t.Fatalf("count query failed for %s: %v", table, err)
	}
	if got != want {
		t.Fatalf("row count for %s = %d, want %d", table, got, want)
	}
}
