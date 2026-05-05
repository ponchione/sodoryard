//go:build sqlite_fts5
// +build sqlite_fts5

package context

import (
	stdctx "context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ponchione/sodoryard/internal/config"
	dbpkg "github.com/ponchione/sodoryard/internal/db"
	"github.com/ponchione/sodoryard/internal/projectmemory"
	"github.com/ponchione/sodoryard/internal/provider"
)

type compressionProviderStub struct {
	responseText string
	err          error
	requests     []*provider.Request
}

func (s *compressionProviderStub) Complete(_ stdctx.Context, req *provider.Request) (*provider.Response, error) {
	s.requests = append(s.requests, req)
	if s.err != nil {
		return nil, s.err
	}
	return &provider.Response{
		Content: []provider.ContentBlock{provider.NewTextBlock(s.responseText)},
	}, nil
}

func (s *compressionProviderStub) Stream(stdctx.Context, *provider.Request) (<-chan provider.StreamEvent, error) {
	return nil, errors.New("not implemented")
}

func (s *compressionProviderStub) Models(stdctx.Context) ([]provider.Model, error) {
	return nil, nil
}

func (s *compressionProviderStub) Name() string {
	return "stub"
}

func TestCompressionTriggerChecks(t *testing.T) {
	cfg := config.ContextConfig{CompressionThreshold: 0.5}

	if !NeedsCompressionPreflight(500000, 200000, cfg) {
		t.Fatal("NeedsCompressionPreflight = false, want true")
	}
	if NeedsCompressionPreflight(1000, 200000, cfg) {
		t.Fatal("NeedsCompressionPreflight = true, want false")
	}
	if !NeedsCompressionPostResponse(100001, 200000, cfg) {
		t.Fatal("NeedsCompressionPostResponse = false, want true")
	}
	if NeedsCompressionPostResponse(99999, 200000, cfg) {
		t.Fatal("NeedsCompressionPostResponse = true, want false")
	}
	if !NeedsCompressionAfterProviderError(413, nil) {
		t.Fatal("NeedsCompressionAfterProviderError(413) = false, want true")
	}
	if !NeedsCompressionAfterProviderError(400, errors.New("provider failed: context_length_exceeded")) {
		t.Fatal("NeedsCompressionAfterProviderError(400 context_length_exceeded) = false, want true")
	}
	if NeedsCompressionAfterProviderError(400, errors.New("other bad request")) {
		t.Fatal("NeedsCompressionAfterProviderError(other 400) = true, want false")
	}
}

func TestProjectMemoryCompressionEngineSummarizesAndReconstructs(t *testing.T) {
	ctx := stdctx.Background()
	dataDir := t.TempDir()
	backend, err := projectmemory.OpenBrainBackend(ctx, projectmemory.Config{DataDir: dataDir, DurableAck: true})
	if err != nil {
		t.Fatalf("OpenBrainBackend: %v", err)
	}

	createdAt := time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC)
	if err := backend.CreateConversation(ctx, projectmemory.CreateConversationArgs{
		ID:          "conv-pm-compress",
		ProjectID:   "project-1",
		Title:       "PM Compression",
		CreatedAtUS: uint64(createdAt.UnixMicro()),
	}); err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	for turn := uint32(1); turn <= 3; turn++ {
		if err := backend.AppendUserMessage(ctx, projectmemory.AppendUserMessageArgs{
			ConversationID: "conv-pm-compress",
			TurnNumber:     turn,
			Content:        "turn user message",
			CreatedAtUS:    uint64(createdAt.Add(time.Duration(turn) * time.Second).UnixMicro()),
		}); err != nil {
			t.Fatalf("AppendUserMessage turn %d: %v", turn, err)
		}
		if err := backend.PersistIteration(ctx, projectmemory.PersistIterationArgs{
			ConversationID: "conv-pm-compress",
			TurnNumber:     turn,
			Iteration:      1,
			CreatedAtUS:    uint64(createdAt.Add(time.Duration(turn+10) * time.Second).UnixMicro()),
			Messages: []projectmemory.PersistIterationMessage{{
				Role:    "assistant",
				Content: "turn assistant message",
			}},
		}); err != nil {
			t.Fatalf("PersistIteration turn %d: %v", turn, err)
		}
	}

	providerStub := &compressionProviderStub{responseText: "- turn two summarized"}
	engine := NewProjectMemoryCompressionEngine(backend, providerStub)
	result, err := engine.Compress(ctx, "conv-pm-compress", config.ContextConfig{
		CompressionHeadPreserve: 2,
		CompressionTailPreserve: 2,
		CompressionModel:        "local",
	})
	if err != nil {
		t.Fatalf("Compress: %v", err)
	}
	if !result.Compressed || !result.SummaryInserted || result.CompressedMessages != 2 {
		t.Fatalf("Compress result = %+v, want compressed summary with 2 messages", result)
	}
	if len(providerStub.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(providerStub.requests))
	}
	active, err := backend.ListMessages(ctx, "conv-pm-compress", false)
	if err != nil {
		t.Fatalf("ListMessages active: %v", err)
	}
	if len(active) != 5 || !active[2].IsSummary || !strings.Contains(active[2].Content, "turn two summarized") {
		t.Fatalf("active messages = %+v, want head summary tail", active)
	}
	all, err := backend.ListMessages(ctx, "conv-pm-compress", true)
	if err != nil {
		t.Fatalf("ListMessages all: %v", err)
	}
	compressed := 0
	for _, msg := range all {
		if msg.Compressed {
			compressed++
		}
	}
	if compressed != 2 {
		t.Fatalf("compressed messages = %d, want 2 in %+v", compressed, all)
	}
	if err := backend.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := projectmemory.OpenBrainBackend(ctx, projectmemory.Config{DataDir: dataDir, DurableAck: true})
	if err != nil {
		t.Fatalf("reopen OpenBrainBackend: %v", err)
	}
	defer reopened.Close()
	active, err = reopened.ListMessages(ctx, "conv-pm-compress", false)
	if err != nil {
		t.Fatalf("ListMessages active after restart: %v", err)
	}
	if len(active) != 5 || !active[2].IsSummary {
		t.Fatalf("active messages after restart = %+v, want persisted summary", active)
	}
}

func TestCompressionEngineSummarizesMiddleSanitizesOrphansAndInvalidatesCache(t *testing.T) {
	db := newCompressionTestDB(t)
	conversationID := seedCompressionConversation(t, db)
	insertCompressionMessage(t, db, compressionSeedMessage{sequence: 1, role: "user", content: "start", turn: 1, iteration: 1})
	insertCompressionMessage(t, db, compressionSeedMessage{
		sequence: 2,
		role:     "assistant",
		content: assistantJSON(t,
			provider.NewTextBlock("I'll inspect the auth file."),
			provider.NewToolUseBlock("toolu-old", "file_read", json.RawMessage(`{"path":"auth.go"}`)),
		),
		turn:      1,
		iteration: 1,
	})
	insertCompressionMessage(t, db, compressionSeedMessage{sequence: 3, role: "tool", content: "package auth", toolUseID: "toolu-old", toolName: "file_read", turn: 1, iteration: 1})
	insertCompressionMessage(t, db, compressionSeedMessage{sequence: 4, role: "assistant", content: assistantJSON(t, provider.NewTextBlock("Found the issue.")), turn: 1, iteration: 2})
	insertCompressionMessage(t, db, compressionSeedMessage{sequence: 5, role: "user", content: "continue", turn: 2, iteration: 1})
	insertCompressionMessage(t, db, compressionSeedMessage{sequence: 6, role: "assistant", content: assistantJSON(t, provider.NewTextBlock("working")), turn: 2, iteration: 1})
	insertCompressionMessage(t, db, compressionSeedMessage{sequence: 7, role: "user", content: "tail user", turn: 3, iteration: 1})
	insertCompressionMessage(t, db, compressionSeedMessage{sequence: 8, role: "assistant", content: assistantJSON(t, provider.NewTextBlock("tail assistant")), turn: 3, iteration: 1})

	providerStub := &compressionProviderStub{responseText: "- Kept auth work moving\n- Mentioned auth.go"}
	engine := NewCompressionEngine(db, providerStub)
	result, err := engine.Compress(stdctx.Background(), conversationID, config.ContextConfig{
		CompressionHeadPreserve: 2,
		CompressionTailPreserve: 4,
		CompressionModel:        "local",
	})
	if err != nil {
		t.Fatalf("Compress returned error: %v", err)
	}
	if !result.Compressed {
		t.Fatal("Compressed = false, want true")
	}
	if !result.SummaryInserted {
		t.Fatal("SummaryInserted = false, want true")
	}
	if result.FallbackUsed {
		t.Fatal("FallbackUsed = true, want false")
	}
	if !result.CacheInvalidated {
		t.Fatal("CacheInvalidated = false, want true")
	}
	if result.CompressedTurnStart != 1 || result.CompressedTurnEnd != 1 {
		t.Fatalf("compressed turn range = %d-%d, want 1-1", result.CompressedTurnStart, result.CompressedTurnEnd)
	}
	if len(providerStub.requests) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(providerStub.requests))
	}
	if providerStub.requests[0].Purpose != "compression" {
		t.Fatalf("provider purpose = %q, want compression", providerStub.requests[0].Purpose)
	}
	if providerStub.requests[0].Model != "local" {
		t.Fatalf("provider model = %q, want local", providerStub.requests[0].Model)
	}

	messages := listCompressionMessages(t, db, conversationID)
	active := activeCompressionMessages(messages)
	if len(active) != 7 {
		t.Fatalf("active message count = %d, want 7", len(active))
	}
	if active[2].Role != "user" || active[2].IsSummary != 1 {
		t.Fatalf("active[2] = %+v, want summary user message", active[2])
	}
	if active[2].Sequence != 3.5 {
		t.Fatalf("summary sequence = %v, want 3.5", active[2].Sequence)
	}
	if !strings.HasPrefix(active[2].Content, "[CONTEXT COMPACTION]\n") {
		t.Fatalf("summary content = %q, want [CONTEXT COMPACTION] prefix", active[2].Content)
	}

	assistantBlocks := assistantBlocks(t, active[1].Content)
	if len(assistantBlocks) != 1 || assistantBlocks[0].Type != "text" {
		t.Fatalf("assistant blocks after sanitization = %+v, want text-only", assistantBlocks)
	}
	if strings.Contains(active[1].Content, "tool_use") {
		t.Fatalf("assistant content still contains tool_use block: %s", active[1].Content)
	}
	if ftsCountForQuery(t, db, "file_read") != 0 {
		t.Fatal("messages_fts still matched removed tool_use content")
	}

	reconstructed := reconstructActiveHistory(t, db, conversationID)
	if len(reconstructed) != 7 {
		t.Fatalf("reconstructed history count = %d, want 7", len(reconstructed))
	}
}

func TestCompressionEngineCompressesOrphanedToolResults(t *testing.T) {
	db := newCompressionTestDB(t)
	conversationID := seedCompressionConversation(t, db)

	// Messages in order:
	// 1: user           (head)
	// 2: assistant+tool_use:A  (head)
	// 3: tool_result:A  (middle - compressed)
	// 4: assistant+tool_use:B  (middle - compressed)
	// 5: tool_result:B  (tail - orphaned! its tool_use:B is in compressed middle)
	// 6: user           (tail)
	// 7: assistant      (tail)
	// 8: user           (tail)
	insertCompressionMessage(t, db, compressionSeedMessage{sequence: 1, role: "user", content: "start", turn: 1, iteration: 1})
	insertCompressionMessage(t, db, compressionSeedMessage{
		sequence: 2,
		role:     "assistant",
		content: assistantJSON(t,
			provider.NewTextBlock("I'll read file A."),
			provider.NewToolUseBlock("toolu-A", "file_read", json.RawMessage(`{"path":"a.go"}`)),
		),
		turn:      1,
		iteration: 1,
	})
	insertCompressionMessage(t, db, compressionSeedMessage{sequence: 3, role: "tool", content: "package a", toolUseID: "toolu-A", toolName: "file_read", turn: 1, iteration: 1})
	insertCompressionMessage(t, db, compressionSeedMessage{
		sequence: 4,
		role:     "assistant",
		content: assistantJSON(t,
			provider.NewTextBlock("Now I'll read file B."),
			provider.NewToolUseBlock("toolu-B", "file_read", json.RawMessage(`{"path":"b.go"}`)),
		),
		turn:      2,
		iteration: 1,
	})
	insertCompressionMessage(t, db, compressionSeedMessage{sequence: 5, role: "tool", content: "package b", toolUseID: "toolu-B", toolName: "file_read", turn: 2, iteration: 1})
	insertCompressionMessage(t, db, compressionSeedMessage{sequence: 6, role: "user", content: "continue", turn: 3, iteration: 1})
	insertCompressionMessage(t, db, compressionSeedMessage{sequence: 7, role: "assistant", content: assistantJSON(t, provider.NewTextBlock("all done")), turn: 3, iteration: 1})
	insertCompressionMessage(t, db, compressionSeedMessage{sequence: 8, role: "user", content: "thanks", turn: 4, iteration: 1})

	providerStub := &compressionProviderStub{responseText: "- Read files A and B"}
	engine := NewCompressionEngine(db, providerStub)
	result, err := engine.Compress(stdctx.Background(), conversationID, config.ContextConfig{
		CompressionHeadPreserve: 2,
		CompressionTailPreserve: 4,
		CompressionModel:        "local",
	})
	if err != nil {
		t.Fatalf("Compress returned error: %v", err)
	}
	if !result.Compressed {
		t.Fatal("Compressed = false, want true")
	}

	messages := listCompressionMessages(t, db, conversationID)
	active := activeCompressionMessages(messages)

	// Expected active messages after compression:
	// 1: user (head)
	// 2: assistant+tool_use:A (head, but tool_use:A stripped because tool_result:A was compressed)
	// summary (inserted between head and tail)
	// 6: user (tail)
	// 7: assistant (tail)
	// 8: user (tail)
	// NOTE: tool_result:B at seq 5 should be compressed because its tool_use:B (seq 4) was in the middle

	// Verify tool_result:B (seq 5) is compressed
	for _, msg := range messages {
		if msg.Sequence == 5 {
			if msg.IsCompressed != 1 {
				t.Fatalf("tool_result:B at seq 5 should be compressed (is_compressed=%d), orphaned tool result not handled", msg.IsCompressed)
			}
		}
	}

	// Verify tool_result:B is not in active messages
	for _, msg := range active {
		if msg.ToolUseID == "toolu-B" {
			t.Fatalf("orphaned tool_result:B (toolu-B) should not be in active messages, but found: %+v", msg)
		}
	}

	// Active should be: user(1), sanitized-assistant(2), summary, user(6), assistant(7), user(8) = 6
	if len(active) != 6 {
		t.Fatalf("active message count = %d, want 6 (orphaned tool result should be compressed)", len(active))
	}
}

func TestCompressionEngineFallsBackWithoutSummary(t *testing.T) {
	db := newCompressionTestDB(t)
	conversationID := seedCompressionConversation(t, db)
	for i := 1; i <= 8; i++ {
		role := "user"
		content := "message"
		if i%2 == 0 {
			role = "assistant"
			content = assistantJSON(t, provider.NewTextBlock("assistant"))
		}
		insertCompressionMessage(t, db, compressionSeedMessage{sequence: float64(i), role: role, content: content, turn: (i + 1) / 2, iteration: 1})
	}

	engine := NewCompressionEngine(db, &compressionProviderStub{err: errors.New("compression model unavailable")})
	result, err := engine.Compress(stdctx.Background(), conversationID, config.ContextConfig{
		CompressionHeadPreserve: 2,
		CompressionTailPreserve: 4,
		CompressionModel:        "local",
	})
	if err != nil {
		t.Fatalf("Compress returned error: %v", err)
	}
	if !result.Compressed {
		t.Fatal("Compressed = false, want true")
	}
	if result.SummaryInserted {
		t.Fatal("SummaryInserted = true, want false")
	}
	if !result.FallbackUsed {
		t.Fatal("FallbackUsed = false, want true")
	}
	if !result.CacheInvalidated {
		t.Fatal("CacheInvalidated = false, want true")
	}

	messages := listCompressionMessages(t, db, conversationID)
	active := activeCompressionMessages(messages)
	if len(active) != 6 {
		t.Fatalf("active message count = %d, want 6", len(active))
	}
	for _, msg := range active {
		if msg.IsSummary == 1 {
			t.Fatalf("unexpected summary row in fallback path: %+v", msg)
		}
	}
}

func TestCompressionEngineBisectsSummarySequenceWhenRawMidpointCollides(t *testing.T) {
	db := newCompressionTestDB(t)
	conversationID := seedCompressionConversation(t, db)
	for i := 1; i <= 20; i++ {
		role := "user"
		content := "user"
		if i%2 == 0 {
			role = "assistant"
			content = assistantJSON(t, provider.NewTextBlock("assistant"))
		}
		insertCompressionMessage(t, db, compressionSeedMessage{sequence: float64(i), role: role, content: content, turn: (i + 1) / 2, iteration: 1})
	}

	engine := NewCompressionEngine(db, &compressionProviderStub{responseText: "- compacted"})
	result, err := engine.Compress(stdctx.Background(), conversationID, config.ContextConfig{
		CompressionHeadPreserve: 3,
		CompressionTailPreserve: 4,
		CompressionModel:        "local",
	})
	if err != nil {
		t.Fatalf("Compress returned error: %v", err)
	}
	if !result.SummaryInserted {
		t.Fatal("SummaryInserted = false, want true")
	}

	messages := listCompressionMessages(t, db, conversationID)
	active := activeCompressionMessages(messages)
	if len(active) != 8 {
		t.Fatalf("active message count = %d, want 8", len(active))
	}
	summary := active[3]
	if summary.IsSummary != 1 {
		t.Fatalf("summary row = %+v, want is_summary=1", summary)
	}
	if summary.Sequence == 10.0 {
		t.Fatalf("summary sequence = %v, want non-colliding bisected value", summary.Sequence)
	}
	if summary.Sequence <= 3.0 || summary.Sequence >= 17.0 {
		t.Fatalf("summary sequence = %v, want to fall between 3 and 17", summary.Sequence)
	}

	insertCompressionMessage(t, db, compressionSeedMessage{sequence: 21, role: "user", content: "after compression", turn: 11, iteration: 1})
}

type compressionSeedMessage struct {
	sequence  float64
	role      string
	content   string
	toolUseID string
	toolName  string
	turn      int
	iteration int
}

type compressionMessageRow struct {
	ID                  int64
	Role                string
	Content             string
	ToolUseID           string
	ToolName            string
	TurnNumber          int
	Iteration           int
	Sequence            float64
	IsCompressed        int
	IsSummary           int
	CompressedTurnStart sql.NullInt64
	CompressedTurnEnd   sql.NullInt64
}

func newCompressionTestDB(t *testing.T) *sql.DB {
	t.Helper()
	ctx := stdctx.Background()
	dbPath := filepath.Join(t.TempDir(), "compression.db")
	sqlDB, err := dbpkg.OpenDB(ctx, dbPath)
	if err != nil {
		t.Fatalf("OpenDB returned error: %v", err)
	}
	if err := dbpkg.Init(ctx, sqlDB); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return sqlDB
}

func seedCompressionConversation(t *testing.T, sqlDB *sql.DB) string {
	t.Helper()
	createdAt := time.Now().UTC().Format(time.RFC3339)
	projectID := "project-1"
	conversationID := "conversation-1"
	mustExecCompression(t, sqlDB, `INSERT INTO projects(id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, projectID, "sirtopham", filepath.Join(t.TempDir(), "project"), createdAt, createdAt)
	mustExecCompression(t, sqlDB, `INSERT INTO conversations(id, project_id, title, model, provider, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, conversationID, projectID, "Compression", "claude", "anthropic", createdAt, createdAt)
	return conversationID
}

func insertCompressionMessage(t *testing.T, sqlDB *sql.DB, msg compressionSeedMessage) {
	t.Helper()
	createdAt := time.Now().UTC().Format(time.RFC3339)
	mustExecCompression(t, sqlDB, `
		INSERT INTO messages(
			conversation_id, role, content, tool_use_id, tool_name, turn_number, iteration, sequence, created_at
		) VALUES (?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?, ?)
	`, "conversation-1", msg.role, msg.content, msg.toolUseID, msg.toolName, msg.turn, msg.iteration, msg.sequence, createdAt)
}

func listCompressionMessages(t *testing.T, sqlDB *sql.DB, conversationID string) []compressionMessageRow {
	t.Helper()
	rows, err := sqlDB.Query(`
		SELECT id, role, content, COALESCE(tool_use_id, ''), COALESCE(tool_name, ''), turn_number, iteration, sequence,
		       is_compressed, is_summary, compressed_turn_start, compressed_turn_end
		FROM messages
		WHERE conversation_id = ?
		ORDER BY sequence
	`, conversationID)
	if err != nil {
		t.Fatalf("query messages: %v", err)
	}
	defer rows.Close()

	var messages []compressionMessageRow
	for rows.Next() {
		var msg compressionMessageRow
		if err := rows.Scan(
			&msg.ID,
			&msg.Role,
			&msg.Content,
			&msg.ToolUseID,
			&msg.ToolName,
			&msg.TurnNumber,
			&msg.Iteration,
			&msg.Sequence,
			&msg.IsCompressed,
			&msg.IsSummary,
			&msg.CompressedTurnStart,
			&msg.CompressedTurnEnd,
		); err != nil {
			t.Fatalf("scan message: %v", err)
		}
		messages = append(messages, msg)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate messages: %v", err)
	}
	return messages
}

func activeCompressionMessages(messages []compressionMessageRow) []compressionMessageRow {
	active := make([]compressionMessageRow, 0, len(messages))
	for _, msg := range messages {
		if msg.IsCompressed == 0 {
			active = append(active, msg)
		}
	}
	return active
}

func reconstructActiveHistory(t *testing.T, sqlDB *sql.DB, conversationID string) []dbpkg.ReconstructConversationHistoryRow {
	t.Helper()
	queries := dbpkg.New(sqlDB)
	rows, err := queries.ReconstructConversationHistory(stdctx.Background(), conversationID)
	if err != nil {
		t.Fatalf("ReconstructConversationHistory returned error: %v", err)
	}
	return rows
}

func assistantBlocks(t *testing.T, raw string) []provider.ContentBlock {
	t.Helper()
	blocks, err := provider.ContentBlocksFromRaw(json.RawMessage(raw))
	if err != nil {
		t.Fatalf("ContentBlocksFromRaw returned error: %v", err)
	}
	return blocks
}

func assistantJSON(t *testing.T, blocks ...provider.ContentBlock) string {
	t.Helper()
	raw, err := json.Marshal(blocks)
	if err != nil {
		t.Fatalf("marshal assistant blocks: %v", err)
	}
	return string(raw)
}

func TestRenderCompressionMessageSummarizesInterruptedAssistantTombstone(t *testing.T) {
	msg := dbpkg.Message{
		Role:       "assistant",
		TurnNumber: 4,
		Iteration:  1,
		Content:    sql.NullString{Valid: true, String: assistantJSON(t, provider.NewTextBlock("[interrupted_assistant]\nreason=interrupt\nmessage=Assistant output was interrupted before turn completion.\npartial_text=working on it"))},
	}
	got := renderCompressionMessage(msg)
	if strings.Contains(got, "partial_text=working on it") {
		t.Fatalf("renderCompressionMessage leaked partial tombstone text: %q", got)
	}
	if !strings.Contains(got, "[assistant interrupted tombstone]") {
		t.Fatalf("renderCompressionMessage = %q, want interrupted tombstone summary", got)
	}
}

func TestRenderCompressionMessageSummarizesFailedAssistantTombstone(t *testing.T) {
	msg := dbpkg.Message{
		Role:       "assistant",
		TurnNumber: 4,
		Iteration:  1,
		Content:    sql.NullString{Valid: true, String: assistantJSON(t, provider.NewTextBlock("[failed_assistant]\nreason=stream_failure\nmessage=Assistant output ended due to a stream failure before turn completion.\npartial_text=working on it"))},
	}
	got := renderCompressionMessage(msg)
	if strings.Contains(got, "partial_text=working on it") {
		t.Fatalf("renderCompressionMessage leaked failed tombstone partial text: %q", got)
	}
	if !strings.Contains(got, "[assistant stream failure tombstone]") {
		t.Fatalf("renderCompressionMessage = %q, want failed tombstone summary", got)
	}
}

func ftsCountForQuery(t *testing.T, sqlDB *sql.DB, query string) int {
	t.Helper()
	var count int
	if err := sqlDB.QueryRow(`SELECT COUNT(*) FROM messages_fts WHERE messages_fts.content MATCH ?`, query).Scan(&count); err != nil {
		t.Fatalf("fts count query failed: %v", err)
	}
	return count
}

func TestCompressionEngineCascadesTwoRoundsCompressingOldSummary(t *testing.T) {
	db := newCompressionTestDB(t)
	conversationID := seedCompressionConversation(t, db)

	// Insert 15 messages: alternating user/assistant pairs across 8 turns.
	// With head=3, tail=4 the layout is:
	//   head:   seq 1(turn1), 2(turn1), 3(turn2)
	//   middle: seq 4(turn2)..11(turn6)   — 8 messages
	//   tail:   seq 12(turn6), 13(turn7), 14(turn7), 15(turn8)
	for i := 1; i <= 15; i++ {
		role := "user"
		content := "user message"
		if i%2 == 0 {
			role = "assistant"
			content = assistantJSON(t, provider.NewTextBlock("assistant message"))
		}
		insertCompressionMessage(t, db, compressionSeedMessage{
			sequence:  float64(i),
			role:      role,
			content:   content,
			turn:      (i + 1) / 2,
			iteration: 1,
		})
	}

	// ---- Round 1 ----
	stubR1 := &compressionProviderStub{responseText: "- Round 1 summary of turns 2-6"}
	engine := NewCompressionEngine(db, stubR1)
	cfg := config.ContextConfig{
		CompressionHeadPreserve: 3,
		CompressionTailPreserve: 4,
		CompressionModel:        "local",
	}
	r1, err := engine.Compress(stdctx.Background(), conversationID, cfg)
	if err != nil {
		t.Fatalf("Round 1 Compress returned error: %v", err)
	}
	if !r1.Compressed {
		t.Fatal("Round 1: Compressed = false, want true")
	}
	if !r1.SummaryInserted {
		t.Fatal("Round 1: SummaryInserted = false, want true")
	}
	if r1.CompressedMessages != 8 {
		t.Fatalf("Round 1: CompressedMessages = %d, want 8", r1.CompressedMessages)
	}
	// Middle turns 2-6
	if r1.CompressedTurnStart != 2 || r1.CompressedTurnEnd != 6 {
		t.Fatalf("Round 1: turn range = %d-%d, want 2-6", r1.CompressedTurnStart, r1.CompressedTurnEnd)
	}

	// After round 1: active = head(3) + summary(1) + tail(4) = 8
	r1Messages := listCompressionMessages(t, db, conversationID)
	r1Active := activeCompressionMessages(r1Messages)
	if len(r1Active) != 8 {
		t.Fatalf("Round 1: active message count = %d, want 8", len(r1Active))
	}

	// Find the round-1 summary and record its ID.
	var r1SummaryID int64
	var r1SummarySeq float64
	for _, msg := range r1Active {
		if msg.IsSummary == 1 {
			r1SummaryID = msg.ID
			r1SummarySeq = msg.Sequence
			if !msg.CompressedTurnStart.Valid || int(msg.CompressedTurnStart.Int64) != 2 {
				t.Fatalf("Round 1 summary compressed_turn_start = %v, want 2", msg.CompressedTurnStart)
			}
			if !msg.CompressedTurnEnd.Valid || int(msg.CompressedTurnEnd.Int64) != 6 {
				t.Fatalf("Round 1 summary compressed_turn_end = %v, want 6", msg.CompressedTurnEnd)
			}
			break
		}
	}
	if r1SummaryID == 0 {
		t.Fatal("Round 1: no summary message found")
	}

	// ---- Add 8 more messages (seq 16-23, turns 8-12) ----
	// After these the active set is 16 messages:
	//   head:   seq 1,2,3
	//   middle: summary, seq 12..19  (9 messages)
	//   tail:   seq 20,21,22,23
	for i := 16; i <= 23; i++ {
		role := "user"
		content := "user message round2"
		if i%2 == 0 {
			role = "assistant"
			content = assistantJSON(t, provider.NewTextBlock("assistant message round2"))
		}
		insertCompressionMessage(t, db, compressionSeedMessage{
			sequence:  float64(i),
			role:      role,
			content:   content,
			turn:      (i + 1) / 2,
			iteration: 1,
		})
	}

	// ---- Round 2 ----
	stubR2 := &compressionProviderStub{responseText: "- Round 2 cascading summary of turns 2-10"}
	engine2 := NewCompressionEngine(db, stubR2)
	r2, err := engine2.Compress(stdctx.Background(), conversationID, cfg)
	if err != nil {
		t.Fatalf("Round 2 Compress returned error: %v", err)
	}
	if !r2.Compressed {
		t.Fatal("Round 2: Compressed = false, want true")
	}
	if !r2.SummaryInserted {
		t.Fatal("Round 2: SummaryInserted = false, want true")
	}
	// Round 2 middle includes old summary (turns 2-6) plus seq 12-19 (turns 6-10).
	// compressionTurnRange should merge them: start=2, end=10.
	if r2.CompressedTurnStart != 2 || r2.CompressedTurnEnd != 10 {
		t.Fatalf("Round 2: turn range = %d-%d, want 2-10", r2.CompressedTurnStart, r2.CompressedTurnEnd)
	}

	// Verify old summary is now compressed.
	r2Messages := listCompressionMessages(t, db, conversationID)
	for _, msg := range r2Messages {
		if msg.ID == r1SummaryID {
			if msg.IsCompressed != 1 {
				t.Fatalf("Old summary (id=%d, seq=%v) is_compressed = %d, want 1", r1SummaryID, r1SummarySeq, msg.IsCompressed)
			}
			break
		}
	}

	// Active after round 2 should be: head(3) + new summary(1) + tail(4) = 8
	r2Active := activeCompressionMessages(r2Messages)
	if len(r2Active) != 8 {
		t.Fatalf("Round 2: active message count = %d, want 8", len(r2Active))
	}

	// Find new summary and verify its turn range covers the full cascade.
	var r2SummaryFound bool
	for _, msg := range r2Active {
		if msg.IsSummary == 1 {
			r2SummaryFound = true
			if msg.ID == r1SummaryID {
				t.Fatal("Round 2: active summary is still the old one; expected a new summary")
			}
			if !msg.CompressedTurnStart.Valid || int(msg.CompressedTurnStart.Int64) != 2 {
				t.Fatalf("Round 2 summary compressed_turn_start = %v, want 2", msg.CompressedTurnStart)
			}
			if !msg.CompressedTurnEnd.Valid || int(msg.CompressedTurnEnd.Int64) != 10 {
				t.Fatalf("Round 2 summary compressed_turn_end = %v, want 10", msg.CompressedTurnEnd)
			}
			if !strings.HasPrefix(msg.Content, "[CONTEXT COMPACTION]\n") {
				t.Fatalf("Round 2 summary content = %q, want [CONTEXT COMPACTION] prefix", msg.Content)
			}
			break
		}
	}
	if !r2SummaryFound {
		t.Fatal("Round 2: no summary message found in active messages")
	}

	// Verify head is preserved (seq 1,2,3).
	for i, wantSeq := range []float64{1, 2, 3} {
		if r2Active[i].Sequence != wantSeq {
			t.Fatalf("Round 2: head[%d].Sequence = %v, want %v", i, r2Active[i].Sequence, wantSeq)
		}
	}

	// Verify tail is the last 4 active messages (seq 20,21,22,23).
	for i, wantSeq := range []float64{20, 21, 22, 23} {
		if r2Active[4+i].Sequence != wantSeq {
			t.Fatalf("Round 2: tail[%d].Sequence = %v, want %v", i, r2Active[4+i].Sequence, wantSeq)
		}
	}

	// Verify reconstructed history via the query returns messages in correct order.
	reconstructed := reconstructActiveHistory(t, db, conversationID)
	if len(reconstructed) != 8 {
		t.Fatalf("Round 2: reconstructed history count = %d, want 8", len(reconstructed))
	}
}

func mustExecCompression(t *testing.T, sqlDB *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := sqlDB.Exec(query, args...); err != nil {
		t.Fatalf("exec failed for %q: %v", strings.TrimSpace(query), err)
	}
}
