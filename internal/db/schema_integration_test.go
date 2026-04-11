//go:build sqlite_fts5
// +build sqlite_fts5

package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	sid "github.com/ponchione/sodoryard/internal/id"
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

func TestGetConversationContextQualityReturnsBudgetPercent(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	queries := New(db)

	projectID := sid.New()
	conversationID := sid.New()
	createdAt := time.Now().UTC().Format(time.RFC3339)

	mustExec(t, db, `INSERT INTO projects(id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, projectID, "proj", "/tmp/proj", createdAt, createdAt)
	mustExec(t, db, `INSERT INTO conversations(id, project_id, title, model, provider, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, conversationID, projectID, "Metrics", "claude", "anthropic", createdAt, createdAt)

	mustExec(t, db, `INSERT INTO context_reports(conversation_id, turn_number, analysis_latency_ms, retrieval_latency_ms, total_latency_ms,
		needs_json, signals_json, rag_results_json, brain_results_json, graph_results_json, explicit_files_json,
		budget_total, budget_used, budget_breakdown_json, included_count, excluded_count,
		agent_used_search_tool, agent_read_files_json, context_hit_rate, created_at)
		VALUES (?, 1, 1, 2, 3, ?, ?, ?, ?, ?, ?, 30000, 3400, ?, 3, 1, 0, ?, 0.5, ?)`,
		conversationID, `{}`, `[]`, `[]`, `[]`, `[]`, `[]`, `{"rag":3400}`, `[]`, createdAt)
	mustExec(t, db, `INSERT INTO context_reports(conversation_id, turn_number, analysis_latency_ms, retrieval_latency_ms, total_latency_ms,
		needs_json, signals_json, rag_results_json, brain_results_json, graph_results_json, explicit_files_json,
		budget_total, budget_used, budget_breakdown_json, included_count, excluded_count,
		agent_used_search_tool, agent_read_files_json, context_hit_rate, created_at)
		VALUES (?, 2, 1, 2, 3, ?, ?, ?, ?, ?, ?, 30000, 2262, ?, 3, 1, 1, ?, 0.7, ?)`,
		conversationID, `{}`, `[]`, `[]`, `[]`, `[]`, `[]`, `{"rag":2262}`, `[]`, createdAt)

	quality, err := queries.GetConversationContextQuality(ctx, conversationID)
	if err != nil {
		t.Fatalf("GetConversationContextQuality returned error: %v", err)
	}
	if !quality.AvgBudgetUsed.Valid {
		t.Fatalf("expected avg budget used to be valid: %+v", quality)
	}
	const want = 9.436666666666667
	if diff := quality.AvgBudgetUsed.Float64 - want; diff < -0.000001 || diff > 0.000001 {
		t.Fatalf("avg budget used = %v, want %v", quality.AvgBudgetUsed.Float64, want)
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

func TestEnsureMessageSearchIndexesIncludeToolsUpgradesOlderFTSTriggers(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	queries := New(db)
	projectID := sid.New()
	conversationID := sid.New()
	createdAt := time.Now().UTC().Format(time.RFC3339)

	mustExec(t, db, `DROP TRIGGER IF EXISTS messages_fts_insert`)
	mustExec(t, db, `DROP TRIGGER IF EXISTS messages_fts_delete`)
	mustExec(t, db, `DROP TRIGGER IF EXISTS messages_fts_update`)
	mustExec(t, db, `CREATE TRIGGER messages_fts_insert AFTER INSERT ON messages
WHEN NEW.role IN ('user', 'assistant')
BEGIN
    INSERT INTO messages_fts(rowid, content) VALUES (NEW.id, NEW.content);
END;`)
	mustExec(t, db, `CREATE TRIGGER messages_fts_delete AFTER DELETE ON messages
WHEN OLD.role IN ('user', 'assistant')
BEGIN
    INSERT INTO messages_fts(messages_fts, rowid, content)
    VALUES ('delete', OLD.id, OLD.content);
END;`)
	mustExec(t, db, `CREATE TRIGGER messages_fts_update AFTER UPDATE OF content ON messages
WHEN NEW.role IN ('user', 'assistant')
BEGIN
    INSERT INTO messages_fts(messages_fts, rowid, content)
    VALUES ('delete', OLD.id, OLD.content);
    INSERT INTO messages_fts(rowid, content) VALUES (NEW.id, NEW.content);
END;`)
	mustExec(t, db, `INSERT INTO messages_fts(messages_fts) VALUES ('rebuild')`)

	mustExec(t, db, `INSERT INTO projects(id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, projectID, "proj", "/tmp/proj", createdAt, createdAt)
	mustExec(t, db, `INSERT INTO conversations(id, project_id, title, model, provider, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, conversationID, projectID, "Interrupted", "claude", "anthropic", createdAt, createdAt)
	mustExec(t, db, `INSERT INTO messages(conversation_id, role, content, turn_number, iteration, sequence, created_at) VALUES (?, 'assistant', ?, 1, 1, 1.0, ?)`, conversationID, `[{"type":"tool_use","id":"tool-1","name":"shell","input":{"command":"sleep 10"}}]`, createdAt)
	mustExec(t, db, `INSERT INTO messages(conversation_id, role, content, tool_use_id, tool_name, turn_number, iteration, sequence, created_at) VALUES (?, 'tool', ?, ?, ?, 1, 1, 2.0, ?)`, conversationID, "[interrupted_tool_result]\nreason=interrupt\ntool=shell\ntool_use_id=tool-1\nstatus=interrupted_during_execution\nmessage=Tool execution did not complete before the turn ended.", "tool-1", "shell", createdAt)

	results, err := queries.SearchConversations(ctx, "interrupted")
	if err != nil {
		t.Fatalf("SearchConversations before upgrade returned error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("SearchConversations before upgrade = %d rows, want 0 with old triggers", len(results))
	}

	if err := EnsureMessageSearchIndexesIncludeTools(ctx, db); err != nil {
		t.Fatalf("EnsureMessageSearchIndexesIncludeTools returned error: %v", err)
	}

	results, err = queries.SearchConversations(ctx, "interrupted")
	if err != nil {
		t.Fatalf("SearchConversations after upgrade returned error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected interrupted tool tombstone search result after upgrade, got none")
	}
}

func TestEnsureContextReportsIncludeTokenBudgetUpgradesOlderSchema(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	mustExec(t, db, `ALTER TABLE context_reports RENAME TO context_reports_new`)
	mustExec(t, db, `CREATE TABLE context_reports (
	    id                     INTEGER PRIMARY KEY AUTOINCREMENT,
	    conversation_id        TEXT    NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
	    turn_number            INTEGER NOT NULL,
	    analysis_latency_ms    INTEGER,
	    retrieval_latency_ms   INTEGER,
	    total_latency_ms       INTEGER,
	    needs_json             TEXT,
	    signals_json           TEXT,
	    rag_results_json       TEXT,
	    brain_results_json     TEXT,
	    graph_results_json     TEXT,
	    explicit_files_json    TEXT,
	    budget_total           INTEGER,
	    budget_used            INTEGER,
	    budget_breakdown_json  TEXT,
	    included_count         INTEGER,
	    excluded_count         INTEGER,
	    agent_used_search_tool INTEGER,
	    agent_read_files_json  TEXT,
	    context_hit_rate       REAL,
	    created_at             TEXT    NOT NULL,
	    UNIQUE(conversation_id, turn_number)
	)`)
	mustExec(t, db, `DROP TABLE context_reports_new`)

	exists, err := tableHasColumn(ctx, db, "context_reports", "token_budget_json")
	if err != nil {
		t.Fatalf("tableHasColumn before upgrade returned error: %v", err)
	}
	if exists {
		t.Fatal("expected token_budget_json to be absent before upgrade")
	}
	if err := EnsureContextReportsIncludeTokenBudget(ctx, db); err != nil {
		t.Fatalf("EnsureContextReportsIncludeTokenBudget returned error: %v", err)
	}
	exists, err = tableHasColumn(ctx, db, "context_reports", "token_budget_json")
	if err != nil {
		t.Fatalf("tableHasColumn after upgrade returned error: %v", err)
	}
	if !exists {
		t.Fatal("expected token_budget_json to exist after upgrade")
	}
	if err := EnsureContextReportsIncludeTokenBudget(ctx, db); err != nil {
		t.Fatalf("EnsureContextReportsIncludeTokenBudget second call returned error: %v", err)
	}
}

func TestBrainDocumentQueriesUpsertListAndFetchByPath(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	queries := New(db)

	projectID := sid.New()
	otherProjectID := sid.New()
	createdAt := time.Now().UTC().Format(time.RFC3339)
	updatedAt := time.Now().UTC().Add(5 * time.Minute).Format(time.RFC3339)

	mustExec(t, db, `INSERT INTO projects(id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, projectID, "proj", "/tmp/proj", createdAt, createdAt)
	mustExec(t, db, `INSERT INTO projects(id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, otherProjectID, "other", "/tmp/other", createdAt, createdAt)

	initial := UpsertBrainDocumentParams{
		ProjectID:   projectID,
		Path:        "notes/architecture.md",
		Title:       sql.NullString{String: "Architecture", Valid: true},
		ContentHash: "hash-1",
		Tags:        sql.NullString{String: `["brain","arch"]`, Valid: true},
		Frontmatter: sql.NullString{String: `{"status":"draft"}`, Valid: true},
		TokenCount:  sql.NullInt64{Int64: 120, Valid: true},
		CreatedBy:   sql.NullString{String: "agent", Valid: true},
		CreatedAt:   createdAt,
		UpdatedAt:   createdAt,
	}
	if err := queries.UpsertBrainDocument(ctx, initial); err != nil {
		t.Fatalf("UpsertBrainDocument initial returned error: %v", err)
	}

	updated := UpsertBrainDocumentParams{
		ProjectID:            projectID,
		Path:                 "notes/architecture.md",
		Title:                sql.NullString{String: "Architecture v2", Valid: true},
		ContentHash:          "hash-2",
		Tags:                 sql.NullString{String: `["brain","updated"]`, Valid: true},
		Frontmatter:          sql.NullString{String: `{"status":"published"}`, Valid: true},
		TokenCount:           sql.NullInt64{Int64: 180, Valid: true},
		CreatedBy:            sql.NullString{String: "operator", Valid: true},
		SourceConversationID: sql.NullString{String: "conv-123", Valid: true},
		CreatedAt:            updatedAt,
		UpdatedAt:            updatedAt,
	}
	if err := queries.UpsertBrainDocument(ctx, updated); err != nil {
		t.Fatalf("UpsertBrainDocument update returned error: %v", err)
	}

	if err := queries.UpsertBrainDocument(ctx, UpsertBrainDocumentParams{
		ProjectID:   otherProjectID,
		Path:        "notes/other.md",
		Title:       sql.NullString{String: "Other", Valid: true},
		ContentHash: "hash-other",
		CreatedAt:   createdAt,
		UpdatedAt:   createdAt,
	}); err != nil {
		t.Fatalf("UpsertBrainDocument other project returned error: %v", err)
	}

	doc, err := queries.GetBrainDocumentByPath(ctx, GetBrainDocumentByPathParams{
		ProjectID: projectID,
		Path:      "notes/architecture.md",
	})
	if err != nil {
		t.Fatalf("GetBrainDocumentByPath returned error: %v", err)
	}
	if doc.Title.String != "Architecture v2" || doc.ContentHash != "hash-2" {
		t.Fatalf("unexpected fetched brain document: %+v", doc)
	}
	if !doc.SourceConversationID.Valid || doc.SourceConversationID.String != "conv-123" {
		t.Fatalf("expected source conversation id to be updated, got %+v", doc.SourceConversationID)
	}
	if doc.CreatedAt != createdAt {
		t.Fatalf("created_at changed on upsert: got %q want %q", doc.CreatedAt, createdAt)
	}
	if doc.UpdatedAt != updatedAt {
		t.Fatalf("updated_at = %q, want %q", doc.UpdatedAt, updatedAt)
	}

	docs, err := queries.ListBrainDocumentsByProject(ctx, projectID)
	if err != nil {
		t.Fatalf("ListBrainDocumentsByProject returned error: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("brain document count = %d, want 1", len(docs))
	}
	if docs[0].Path != "notes/architecture.md" || docs[0].ContentHash != "hash-2" {
		t.Fatalf("unexpected listed brain documents: %+v", docs)
	}
}

func TestBrainLinkQueriesDeleteAndRewriteForSourceDocument(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	queries := New(db)

	projectID := sid.New()
	createdAt := time.Now().UTC().Format(time.RFC3339)

	mustExec(t, db, `INSERT INTO projects(id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, projectID, "proj", "/tmp/proj", createdAt, createdAt)

	if err := queries.InsertBrainLink(ctx, InsertBrainLinkParams{
		ProjectID:  projectID,
		SourcePath: "notes/source.md",
		TargetPath: "notes/target-a.md",
		LinkText:   sql.NullString{String: "A", Valid: true},
	}); err != nil {
		t.Fatalf("InsertBrainLink first returned error: %v", err)
	}
	if err := queries.InsertBrainLink(ctx, InsertBrainLinkParams{
		ProjectID:  projectID,
		SourcePath: "notes/source.md",
		TargetPath: "notes/target-b.md",
		LinkText:   sql.NullString{String: "B", Valid: true},
	}); err != nil {
		t.Fatalf("InsertBrainLink second returned error: %v", err)
	}
	if err := queries.InsertBrainLink(ctx, InsertBrainLinkParams{
		ProjectID:  projectID,
		SourcePath: "notes/other-source.md",
		TargetPath: "notes/keep.md",
		LinkText:   sql.NullString{String: "Keep", Valid: true},
	}); err != nil {
		t.Fatalf("InsertBrainLink third returned error: %v", err)
	}

	assertTableCountWhere(t, db, "brain_links", "project_id = ?", projectID, 3)

	if err := queries.DeleteBrainLinksForSource(ctx, DeleteBrainLinksForSourceParams{
		ProjectID:  projectID,
		SourcePath: "notes/source.md",
	}); err != nil {
		t.Fatalf("DeleteBrainLinksForSource returned error: %v", err)
	}

	assertTableCountWhere(t, db, "brain_links", "project_id = ?", projectID, 1)
	assertTableCountWhere(t, db, "brain_links", "project_id = ? AND source_path = ?", []any{projectID, "notes/source.md"}, 0)

	if err := queries.InsertBrainLink(ctx, InsertBrainLinkParams{
		ProjectID:  projectID,
		SourcePath: "notes/source.md",
		TargetPath: "notes/target-c.md",
		LinkText:   sql.NullString{String: "C", Valid: true},
	}); err != nil {
		t.Fatalf("InsertBrainLink rewrite returned error: %v", err)
	}

	assertTableCountWhere(t, db, "brain_links", "project_id = ?", projectID, 2)
	assertTableCountWhere(t, db, "brain_links", "project_id = ? AND source_path = ? AND target_path = ?", []any{projectID, "notes/source.md", "notes/target-c.md"}, 1)
	assertTableCountWhere(t, db, "brain_links", "project_id = ? AND source_path = ? AND target_path = ?", []any{projectID, "notes/other-source.md", "notes/keep.md"}, 1)
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
	switch v := arg.(type) {
	case nil:
		err = db.QueryRow(query).Scan(&got)
	case []any:
		err = db.QueryRow(query, v...).Scan(&got)
	default:
		err = db.QueryRow(query, arg).Scan(&got)
	}
	if err != nil {
		t.Fatalf("count query failed for %s: %v", table, err)
	}
	if got != want {
		t.Fatalf("row count for %s = %d, want %d", table, got, want)
	}
}
