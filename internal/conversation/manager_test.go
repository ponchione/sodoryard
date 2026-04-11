//go:build sqlite_fts5
// +build sqlite_fts5

package conversation

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/ponchione/sodoryard/internal/db"
	sid "github.com/ponchione/sodoryard/internal/id"
	"github.com/ponchione/sodoryard/internal/provider"
)

// --- Test helpers ---

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	// Reuse the existing helper.
	return newHistoryTestDB(t)
}

func seedProject(t *testing.T, database *sql.DB) string {
	t.Helper()
	projectID := sid.New()
	createdAt := time.Unix(1700000000, 0).UTC().Format(time.RFC3339)
	mustExec(t, database, `INSERT INTO projects(id, name, root_path, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)`, projectID, "test-project", "/tmp/test", createdAt, createdAt)
	return projectID
}

func mustExec(t *testing.T, database *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := database.Exec(query, args...); err != nil {
		t.Fatalf("exec failed for %q: %v", query, err)
	}
}

func newTestManager(t *testing.T, database *sql.DB) *Manager {
	t.Helper()
	m := NewManager(database, nil, nil)
	m.newID = func() string { return sid.New() }
	return m
}

// --- CRUD Tests ---

func TestManagerCreateAndGet(t *testing.T) {
	ctx := context.Background()
	database := newTestDB(t)
	projectID := seedProject(t, database)
	mgr := newTestManager(t, database)
	mgr.HistoryManager.now = func() time.Time { return time.Unix(1700001000, 0).UTC() }

	conv, err := mgr.Create(ctx, projectID, WithTitle("Test Conversation"), WithModel("claude-3"), WithProvider("anthropic"))
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if conv.ID == "" {
		t.Fatal("Create returned empty ID")
	}
	if conv.ProjectID != projectID {
		t.Fatalf("ProjectID = %q, want %q", conv.ProjectID, projectID)
	}
	if conv.Title == nil || *conv.Title != "Test Conversation" {
		t.Fatalf("Title = %v, want 'Test Conversation'", conv.Title)
	}
	if conv.Model == nil || *conv.Model != "claude-3" {
		t.Fatalf("Model = %v, want 'claude-3'", conv.Model)
	}
	if conv.Provider == nil || *conv.Provider != "anthropic" {
		t.Fatalf("Provider = %v, want 'anthropic'", conv.Provider)
	}

	// Get the conversation back.
	got, err := mgr.Get(ctx, conv.ID)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got.ID != conv.ID {
		t.Fatalf("Get.ID = %q, want %q", got.ID, conv.ID)
	}
	if got.Title == nil || *got.Title != "Test Conversation" {
		t.Fatalf("Get.Title = %v, want 'Test Conversation'", got.Title)
	}
}

func TestManagerCreateNoOptions(t *testing.T) {
	ctx := context.Background()
	database := newTestDB(t)
	projectID := seedProject(t, database)
	mgr := newTestManager(t, database)

	conv, err := mgr.Create(ctx, projectID)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if conv.Title != nil {
		t.Fatalf("Title should be nil, got %v", conv.Title)
	}
	if conv.Model != nil {
		t.Fatalf("Model should be nil, got %v", conv.Model)
	}
}

func TestManagerGetNotFound(t *testing.T) {
	ctx := context.Background()
	database := newTestDB(t)
	mgr := newTestManager(t, database)

	_, err := mgr.Get(ctx, "nonexistent-id")
	if err == nil {
		t.Fatal("Get should return error for nonexistent ID")
	}
	if !strings.Contains(err.Error(), "no rows") {
		t.Fatalf("error should mention no rows, got: %v", err)
	}
}

func TestManagerList(t *testing.T) {
	ctx := context.Background()
	database := newTestDB(t)
	projectID := seedProject(t, database)
	mgr := newTestManager(t, database)

	// Create 3 conversations with different timestamps.
	counter := 0
	mgr.newID = func() string {
		counter++
		return sid.New()
	}

	times := []time.Time{
		time.Unix(1700001000, 0).UTC(),
		time.Unix(1700002000, 0).UTC(),
		time.Unix(1700003000, 0).UTC(),
	}

	var ids []string
	for i, ts := range times {
		mgr.HistoryManager.now = func() time.Time { return ts }
		conv, err := mgr.Create(ctx, projectID, WithTitle("Conv "+string(rune('A'+i))))
		if err != nil {
			t.Fatalf("Create %d returned error: %v", i, err)
		}
		ids = append(ids, conv.ID)
	}

	summaries, err := mgr.List(ctx, projectID, 50, 0)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(summaries) != 3 {
		t.Fatalf("List returned %d conversations, want 3", len(summaries))
	}

	// Should be ordered by updated_at DESC (newest first).
	if summaries[0].ID != ids[2] {
		t.Fatalf("First in list should be newest, got %q want %q", summaries[0].ID, ids[2])
	}
	if summaries[2].ID != ids[0] {
		t.Fatalf("Last in list should be oldest, got %q want %q", summaries[2].ID, ids[0])
	}
}

func TestManagerListPagination(t *testing.T) {
	ctx := context.Background()
	database := newTestDB(t)
	projectID := seedProject(t, database)
	mgr := newTestManager(t, database)

	for i := 0; i < 5; i++ {
		mgr.HistoryManager.now = func() time.Time { return time.Unix(int64(1700001000+i*1000), 0).UTC() }
		if _, err := mgr.Create(ctx, projectID); err != nil {
			t.Fatalf("Create %d returned error: %v", i, err)
		}
	}

	// Fetch page 1 (limit 2).
	page1, err := mgr.List(ctx, projectID, 2, 0)
	if err != nil {
		t.Fatalf("List page 1 error: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("page 1 has %d items, want 2", len(page1))
	}

	// Fetch page 2 (limit 2, offset 2).
	page2, err := mgr.List(ctx, projectID, 2, 2)
	if err != nil {
		t.Fatalf("List page 2 error: %v", err)
	}
	if len(page2) != 2 {
		t.Fatalf("page 2 has %d items, want 2", len(page2))
	}

	// Pages should have different IDs.
	if page1[0].ID == page2[0].ID {
		t.Fatal("page 1 and page 2 overlap")
	}
}

func TestManagerDelete(t *testing.T) {
	ctx := context.Background()
	database := newTestDB(t)
	projectID := seedProject(t, database)
	mgr := newTestManager(t, database)

	conv, err := mgr.Create(ctx, projectID, WithTitle("To Delete"))
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	// Persist a user message (creates a related row in messages).
	if err := mgr.PersistUserMessage(ctx, conv.ID, 1, "hello"); err != nil {
		t.Fatalf("PersistUserMessage error: %v", err)
	}

	// Delete the conversation.
	if err := mgr.Delete(ctx, conv.ID); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}

	// Get should now fail.
	_, err = mgr.Get(ctx, conv.ID)
	if err == nil {
		t.Fatal("Get should fail after Delete")
	}

	// Messages should be cascade-deleted.
	var msgCount int
	database.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages WHERE conversation_id = ?`, conv.ID).Scan(&msgCount)
	if msgCount != 0 {
		t.Fatalf("messages count = %d after cascade delete, want 0", msgCount)
	}
}

func TestManagerSetTitle(t *testing.T) {
	ctx := context.Background()
	database := newTestDB(t)
	projectID := seedProject(t, database)
	mgr := newTestManager(t, database)
	mgr.HistoryManager.now = func() time.Time { return time.Unix(1700001000, 0).UTC() }

	conv, err := mgr.Create(ctx, projectID)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if conv.Title != nil {
		t.Fatal("Title should be nil before SetTitle")
	}

	// Update the clock and set title.
	mgr.HistoryManager.now = func() time.Time { return time.Unix(1700002000, 0).UTC() }
	if err := mgr.SetTitle(ctx, conv.ID, "New Title"); err != nil {
		t.Fatalf("SetTitle returned error: %v", err)
	}

	got, err := mgr.Get(ctx, conv.ID)
	if err != nil {
		t.Fatalf("Get after SetTitle returned error: %v", err)
	}
	if got.Title == nil || *got.Title != "New Title" {
		t.Fatalf("Title = %v, want 'New Title'", got.Title)
	}
	// Updated_at should reflect the SetTitle time.
	wantUpdated := time.Unix(1700002000, 0).UTC()
	if !got.UpdatedAt.Equal(wantUpdated) {
		t.Fatalf("UpdatedAt = %v, want %v", got.UpdatedAt, wantUpdated)
	}
}

func TestManagerCount(t *testing.T) {
	ctx := context.Background()
	database := newTestDB(t)
	projectID := seedProject(t, database)
	mgr := newTestManager(t, database)

	count, err := mgr.Count(ctx, projectID)
	if err != nil {
		t.Fatalf("Count error: %v", err)
	}
	if count != 0 {
		t.Fatalf("initial count = %d, want 0", count)
	}

	for i := 0; i < 3; i++ {
		if _, err := mgr.Create(ctx, projectID); err != nil {
			t.Fatalf("Create %d error: %v", i, err)
		}
	}

	count, err = mgr.Count(ctx, projectID)
	if err != nil {
		t.Fatalf("Count error: %v", err)
	}
	if count != 3 {
		t.Fatalf("count = %d, want 3", count)
	}
}

// --- Multi-Iteration Integration Test ---

func TestManagerFullTurnLifecycle(t *testing.T) {
	ctx := context.Background()
	database := newTestDB(t)
	projectID := seedProject(t, database)
	mgr := newTestManager(t, database)
	mgr.HistoryManager.now = func() time.Time { return time.Unix(1700001000, 0).UTC() }

	conv, err := mgr.Create(ctx, projectID)
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}

	// Turn 1: user message.
	if err := mgr.PersistUserMessage(ctx, conv.ID, 1, "fix auth"); err != nil {
		t.Fatalf("PersistUserMessage error: %v", err)
	}

	// Iteration 1: assistant + tool call + tool result.
	if err := mgr.PersistIteration(ctx, conv.ID, 1, 1, []IterationMessage{
		{Role: "assistant", Content: `[{"type":"text","text":"checking"},{"type":"tool_use","id":"tc1","name":"file_read","input":{"path":"auth.go"}}]`},
		{Role: "tool", Content: "package auth\n", ToolUseID: "tc1", ToolName: "file_read"},
	}); err != nil {
		t.Fatalf("PersistIteration 1 error: %v", err)
	}

	// Iteration 2: text-only response.
	if err := mgr.PersistIteration(ctx, conv.ID, 1, 2, []IterationMessage{
		{Role: "assistant", Content: `[{"type":"text","text":"I fixed the auth bug."}]`},
	}); err != nil {
		t.Fatalf("PersistIteration 2 error: %v", err)
	}

	// Reconstruct and verify.
	history, err := mgr.ReconstructHistory(ctx, conv.ID)
	if err != nil {
		t.Fatalf("ReconstructHistory error: %v", err)
	}

	// Should have: user(0) + assistant(1) + tool(2) + assistant(3) = 4 messages.
	if len(history) != 4 {
		t.Fatalf("history length = %d, want 4", len(history))
	}
	if history[0].Role != "user" {
		t.Fatalf("history[0].Role = %q, want user", history[0].Role)
	}
	if history[1].Role != "assistant" {
		t.Fatalf("history[1].Role = %q, want assistant", history[1].Role)
	}
	if history[2].Role != "tool" {
		t.Fatalf("history[2].Role = %q, want tool", history[2].Role)
	}
	if history[3].Role != "assistant" {
		t.Fatalf("history[3].Role = %q, want assistant", history[3].Role)
	}

	// Verify monotonic sequences.
	for i := 1; i < len(history); i++ {
		if history[i].Sequence <= history[i-1].Sequence {
			t.Fatalf("sequence[%d]=%f <= sequence[%d]=%f, want monotonically increasing",
				i, history[i].Sequence, i-1, history[i-1].Sequence)
		}
	}
}

// --- Cancellation Integration Test ---

func TestManagerCancellationPreservesCompletedIterations(t *testing.T) {
	ctx := context.Background()
	database := newTestDB(t)
	projectID := seedProject(t, database)
	mgr := newTestManager(t, database)
	mgr.HistoryManager.now = func() time.Time { return time.Unix(1700001000, 0).UTC() }

	conv, err := mgr.Create(ctx, projectID)
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}

	// User message + 2 completed iterations.
	if err := mgr.PersistUserMessage(ctx, conv.ID, 1, "fix auth"); err != nil {
		t.Fatalf("PersistUserMessage error: %v", err)
	}
	if err := mgr.PersistIteration(ctx, conv.ID, 1, 1, []IterationMessage{
		{Role: "assistant", Content: `[{"type":"text","text":"checking"}]`},
	}); err != nil {
		t.Fatalf("PersistIteration 1 error: %v", err)
	}
	if err := mgr.PersistIteration(ctx, conv.ID, 1, 2, []IterationMessage{
		{Role: "assistant", Content: `[{"type":"text","text":"still working..."}]`},
	}); err != nil {
		t.Fatalf("PersistIteration 2 error: %v", err)
	}

	// Partial 3rd iteration.
	if err := mgr.PersistIteration(ctx, conv.ID, 1, 3, []IterationMessage{
		{Role: "assistant", Content: `[{"type":"text","text":"partial"}]`},
	}); err != nil {
		t.Fatalf("PersistIteration 3 error: %v", err)
	}

	// Cancel iteration 3.
	if err := mgr.CancelIteration(ctx, conv.ID, 1, 3); err != nil {
		t.Fatalf("CancelIteration error: %v", err)
	}

	// Reconstruct — should have user + iter1 + iter2 = 3 messages.
	history, err := mgr.ReconstructHistory(ctx, conv.ID)
	if err != nil {
		t.Fatalf("ReconstructHistory error: %v", err)
	}
	if len(history) != 3 {
		t.Fatalf("history after cancel = %d, want 3", len(history))
	}
	if history[0].Role != "user" || history[1].Role != "assistant" || history[2].Role != "assistant" {
		t.Fatalf("unexpected roles: %s, %s, %s", history[0].Role, history[1].Role, history[2].Role)
	}
}

// --- Title Generation Test ---

type mockTitleProvider struct {
	response *provider.Response
	err      error
	calls    int
}

func (m *mockTitleProvider) Complete(_ context.Context, _ *provider.Request) (*provider.Response, error) {
	m.calls++
	return m.response, m.err
}

func TestTitleGenSuccess(t *testing.T) {
	ctx := context.Background()
	database := newTestDB(t)
	projectID := seedProject(t, database)
	mgr := newTestManager(t, database)
	mgr.HistoryManager.now = func() time.Time { return time.Unix(1700001000, 0).UTC() }

	conv, err := mgr.Create(ctx, projectID)
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}

	// Persist a user message so title gen can find it.
	if err := mgr.PersistUserMessage(ctx, conv.ID, 1, "help me debug the auth middleware"); err != nil {
		t.Fatalf("PersistUserMessage error: %v", err)
	}

	mockProvider := &mockTitleProvider{
		response: &provider.Response{
			Content: []provider.ContentBlock{
				provider.NewTextBlock(`"Debugging Auth Middleware Issues"`),
			},
		},
	}

	gen := NewTitleGen(mgr, mockProvider, "fast-model", nil)
	gen.GenerateTitle(ctx, conv.ID)

	if mockProvider.calls != 1 {
		t.Fatalf("provider.Complete called %d times, want 1", mockProvider.calls)
	}

	// Verify the title was persisted (with quotes stripped).
	got, err := mgr.Get(ctx, conv.ID)
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if got.Title == nil {
		t.Fatal("Title should not be nil after generation")
	}
	if *got.Title != "Debugging Auth Middleware Issues" {
		t.Fatalf("Title = %q, want 'Debugging Auth Middleware Issues'", *got.Title)
	}
}

func TestTitleGenProviderError(t *testing.T) {
	ctx := context.Background()
	database := newTestDB(t)
	projectID := seedProject(t, database)
	mgr := newTestManager(t, database)
	mgr.HistoryManager.now = func() time.Time { return time.Unix(1700001000, 0).UTC() }

	conv, err := mgr.Create(ctx, projectID)
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}
	if err := mgr.PersistUserMessage(ctx, conv.ID, 1, "hello"); err != nil {
		t.Fatalf("PersistUserMessage error: %v", err)
	}

	mockProvider := &mockTitleProvider{
		err: sql.ErrConnDone, // some error
	}

	gen := NewTitleGen(mgr, mockProvider, "fast-model", nil)
	gen.GenerateTitle(ctx, conv.ID) // should not panic

	// Title should remain nil.
	got, err := mgr.Get(ctx, conv.ID)
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if got.Title != nil {
		t.Fatalf("Title should be nil after provider error, got %v", got.Title)
	}
}

func TestTitleGenSkipsTombstoneTitle(t *testing.T) {
	ctx := context.Background()
	database := newTestDB(t)
	projectID := seedProject(t, database)
	mgr := newTestManager(t, database)
	mgr.HistoryManager.now = func() time.Time { return time.Unix(1700001000, 0).UTC() }

	conv, err := mgr.Create(ctx, projectID)
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}
	if err := mgr.PersistUserMessage(ctx, conv.ID, 1, "help me debug the auth middleware"); err != nil {
		t.Fatalf("PersistUserMessage error: %v", err)
	}

	mockProvider := &mockTitleProvider{
		response: &provider.Response{
			Content: []provider.ContentBlock{
				provider.NewTextBlock("[interrupted_assistant]\nreason=interrupt\nmessage=Assistant output was interrupted before turn completion."),
			},
		},
	}

	gen := NewTitleGen(mgr, mockProvider, "fast-model", nil)
	gen.GenerateTitle(ctx, conv.ID)

	got, err := mgr.Get(ctx, conv.ID)
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if got.Title != nil {
		t.Fatalf("Title should be nil after tombstone title, got %v", *got.Title)
	}
}

func TestTitleGenFallsBackToAssistantTextForMisleadingAccessTitle(t *testing.T) {
	ctx := context.Background()
	database := newTestDB(t)
	projectID := seedProject(t, database)
	mgr := newTestManager(t, database)
	mgr.HistoryManager.now = func() time.Time { return time.Unix(1700001000, 0).UTC() }

	conv, err := mgr.Create(ctx, projectID)
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}
	if err := mgr.PersistUserMessage(ctx, conv.ID, 1, "Use file_read on NEXT_SESSION_HANDOFF.md and reply with exactly the first line of that file and nothing else."); err != nil {
		t.Fatalf("PersistUserMessage error: %v", err)
	}
	if err := mgr.PersistIteration(ctx, conv.ID, 1, 1, []IterationMessage{
		{Role: "assistant", Content: `[{"type":"text","text":"# Next session handoff"}]`},
	}); err != nil {
		t.Fatalf("PersistIteration error: %v", err)
	}

	mockProvider := &mockTitleProvider{
		response: &provider.Response{
			Content: []provider.ContentBlock{
				provider.NewTextBlock("Unable to Access NEXT_SESSION_HANDOFF"),
			},
		},
	}

	gen := NewTitleGen(mgr, mockProvider, "fast-model", nil)
	gen.GenerateTitle(ctx, conv.ID)

	got, err := mgr.Get(ctx, conv.ID)
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if got.Title == nil {
		t.Fatal("Title should not be nil after fallback generation")
	}
	if *got.Title != "Next session handoff" {
		t.Fatalf("Title = %q, want fallback assistant-derived title", *got.Title)
	}
}

func TestTitleGenNoUserMessage(t *testing.T) {
	ctx := context.Background()
	database := newTestDB(t)
	projectID := seedProject(t, database)
	mgr := newTestManager(t, database)

	conv, err := mgr.Create(ctx, projectID)
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}

	mockProvider := &mockTitleProvider{
		response: &provider.Response{
			Content: []provider.ContentBlock{provider.NewTextBlock("Title")},
		},
	}

	gen := NewTitleGen(mgr, mockProvider, "fast-model", nil)
	gen.GenerateTitle(ctx, conv.ID) // should not panic, should not call provider

	if mockProvider.calls != 0 {
		t.Fatalf("provider should not be called when no user message, got %d calls", mockProvider.calls)
	}
}

// --- cleanTitle Tests ---

func TestCleanTitle(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`"Debugging Auth Issues"`, "Debugging Auth Issues"},
		{`'Fix Login Bug'`, "Fix Login Bug"},
		{"  Hello World  ", "Hello World"},
		{"`Some Title`", "Some Title"},
		{"[interrupted_assistant]\nreason=interrupt\nmessage=Assistant output was interrupted before turn completion.", ""},
		{"[failed_assistant]\nreason=stream_failure\nmessage=Assistant output ended due to a stream failure before turn completion.", ""},
		{"[interrupted_tool_result]\nreason=interrupt\ntool=shell", ""},
		{"", ""},
		{"A", "A"},
	}
	for _, tt := range tests {
		got := cleanTitle(tt.input)
		if got != tt.want {
			t.Errorf("cleanTitle(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSearchSnippetSummaryForAssistantTombstone(t *testing.T) {
	input := `[{"type":"text","text":"[interrupted_assistant]\nreason=interrupt\nmessage=Assistant output was interrupted before turn completion.\npartial_text=<b>working</b> on it"}]`
	got := sanitizeSearchSnippet(input)
	if got != "[assistant interrupted tombstone]" {
		t.Fatalf("sanitizeSearchSnippet() = %q, want compact assistant tombstone summary", got)
	}
}

func TestSearchSnippetSummaryForFailedAssistantTombstone(t *testing.T) {
	input := `[{"type":"text","text":"[failed_assistant]\nreason=stream_failure\nmessage=Assistant output ended due to a stream failure before turn completion.\npartial_text=<b>working</b> on it"}]`
	got := sanitizeSearchSnippet(input)
	if got != "[assistant stream failure tombstone]" {
		t.Fatalf("sanitizeSearchSnippet() = %q, want compact failed assistant tombstone summary", got)
	}
}

func TestSearchSnippetSummaryForInterruptedToolResult(t *testing.T) {
	input := `[interrupted_tool_result]\nreason=interrupt\ntool=shell\ntool_use_id=tool-1\nstatus=interrupted_during_execution\nmessage=Tool execution did not complete before the turn ended.\n<b>working</b>`
	got := sanitizeSearchSnippet(input)
	if got != "[interrupted tool result]" {
		t.Fatalf("sanitizeSearchSnippet() = %q, want compact interrupted tool summary", got)
	}
}

func TestSearchSnippetExtractsAssistantTextFromJSONBlocks(t *testing.T) {
	input := `[{"type":"text","text":"I'll check <b>search_text</b>."},{"type":"tool_use","id":"tc1","name":"file_read","input":{"path":"internal/tool/search_text.go"}}]`
	got := sanitizeSearchSnippet(input)
	if got != "I'll check search_text." {
		t.Fatalf("sanitizeSearchSnippet() = %q, want assistant text only without FTS highlight tags", got)
	}
}

func TestSearchSnippetSummarizesToolOnlyAssistantJSON(t *testing.T) {
	input := `[{"type":"tool_use","id":"tc1","name":"search_text","input":{"pattern":"cleanup"}}]`
	got := sanitizeSearchSnippet(input)
	if got != "[assistant tool call: search_text]" {
		t.Fatalf("sanitizeSearchSnippet() = %q, want compact tool-call summary", got)
	}
}

func TestSearchSnippetSanitizesTruncatedToolJSON(t *testing.T) {
	input := `[{"type":"tool_use","id":"call_1","name":"shell","input":{"command":"git log -n 5 -- internal/tool/search_text.go...`
	got := sanitizeSearchSnippet(input)
	if got != "[assistant tool call: shell]" {
		t.Fatalf("sanitizeSearchSnippet() = %q, want compact tool-call summary for truncated JSON", got)
	}
}

func TestSearchSnippetSanitizesTruncatedTextJSON(t *testing.T) {
	input := `[{"type":"text","text":"internal/tool/<b>search_text</b>.go implements the search tool...`
	got := sanitizeSearchSnippet(input)
	if got != `internal/tool/search_text.go implements the search tool...` {
		t.Fatalf("sanitizeSearchSnippet() = %q, want extracted text from truncated JSON without FTS highlight tags", got)
	}
}

func TestManagerSearchSanitizesNormalAssistantToolJSONSnippets(t *testing.T) {
	ctx := context.Background()
	database := newTestDB(t)
	projectID := seedProject(t, database)
	mgr := newTestManager(t, database)
	mgr.HistoryManager.now = func() time.Time { return time.Unix(1700001000, 0).UTC() }

	conv, err := mgr.Create(ctx, projectID, WithTitle("Tool JSON search"))
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}
	if err := mgr.PersistUserMessage(ctx, conv.ID, 1, "please inspect this tool run"); err != nil {
		t.Fatalf("PersistUserMessage error: %v", err)
	}
	if err := mgr.PersistIteration(ctx, conv.ID, 1, 1, []IterationMessage{{
		Role:    "assistant",
		Content: `[{"type":"text","text":"I'll inspect search_text."},{"type":"tool_use","id":"tc1","name":"file_read","input":{"path":"internal/tool/search_text.go"}}]`,
	}}); err != nil {
		t.Fatalf("PersistIteration error: %v", err)
	}

	results, err := mgr.Search(ctx, "search_text")
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("Search returned no results, want assistant snippet-backed match")
	}
	if strings.Contains(results[0].Snippet, `"type":"tool_use"`) || strings.Contains(results[0].Snippet, `"input":`) {
		t.Fatalf("Search snippet = %q, want tool JSON stripped", results[0].Snippet)
	}
	if strings.Contains(results[0].Snippet, "<b>") || strings.Contains(results[0].Snippet, "</b>") {
		t.Fatalf("Search snippet = %q, want FTS highlight tags stripped", results[0].Snippet)
	}
	if results[0].Snippet != "I'll inspect search_text." {
		t.Fatalf("Search snippet = %q, want assistant text only without FTS highlight tags", results[0].Snippet)
	}
}

func TestManagerSearchSanitizesTombstoneSnippets(t *testing.T) {
	ctx := context.Background()
	database := newTestDB(t)
	projectID := seedProject(t, database)
	mgr := newTestManager(t, database)
	mgr.HistoryManager.now = func() time.Time { return time.Unix(1700001000, 0).UTC() }

	conv, err := mgr.Create(ctx, projectID, WithTitle("Tombstone search"))
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}
	if err := mgr.PersistUserMessage(ctx, conv.ID, 1, "please inspect the failed run"); err != nil {
		t.Fatalf("PersistUserMessage error: %v", err)
	}
	if err := mgr.PersistIteration(ctx, conv.ID, 1, 1, []IterationMessage{{
		Role:    "assistant",
		Content: `[{"type":"text","text":"[interrupted_assistant]\nreason=interrupt\nmessage=Assistant output was interrupted before turn completion.\npartial_text=working on it"}]`,
	}}); err != nil {
		t.Fatalf("PersistIteration error: %v", err)
	}

	results, err := mgr.Search(ctx, "working")
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("Search returned no results, want tombstone-backed match")
	}
	if strings.Contains(results[0].Snippet, "partial_text") || strings.Contains(results[0].Snippet, "[interrupted_assistant]") {
		t.Fatalf("Search snippet = %q, want sanitized tombstone summary", results[0].Snippet)
	}
	if results[0].Snippet != "[assistant interrupted tombstone]" {
		t.Fatalf("Search snippet = %q, want compact tombstone summary", results[0].Snippet)
	}
}

func TestManagerSearchFindsInterruptedToolTombstones(t *testing.T) {
	ctx := context.Background()
	database := newTestDB(t)
	projectID := seedProject(t, database)
	mgr := newTestManager(t, database)
	mgr.HistoryManager.now = func() time.Time { return time.Unix(1700001000, 0).UTC() }

	conv, err := mgr.Create(ctx, projectID, WithTitle("Interrupted tool search"))
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}
	if err := mgr.PersistUserMessage(ctx, conv.ID, 1, "please inspect the cancelled tool run"); err != nil {
		t.Fatalf("PersistUserMessage error: %v", err)
	}
	if err := mgr.PersistIteration(ctx, conv.ID, 1, 1, []IterationMessage{
		{Role: "assistant", Content: `[{"type":"tool_use","id":"tool-1","name":"shell","input":{"command":"sleep 10"}}]`},
		{Role: "tool", Content: "[interrupted_tool_result]\nreason=interrupt\ntool=shell\ntool_use_id=tool-1\nstatus=interrupted_during_execution\nmessage=Tool execution did not complete before the turn ended.", ToolUseID: "tool-1", ToolName: "shell"},
	}); err != nil {
		t.Fatalf("PersistIteration error: %v", err)
	}

	results, err := mgr.Search(ctx, "interrupted")
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("Search returned no results, want interrupted tool tombstone-backed match")
	}
	if results[0].Snippet != "[interrupted tool result]" {
		t.Fatalf("Search snippet = %q, want compact interrupted tool summary", results[0].Snippet)
	}
}

func TestManagerSearchHandlesUnquotedHyphenatedQueries(t *testing.T) {
	ctx := context.Background()
	database := newTestDB(t)
	projectID := seedProject(t, database)
	mgr := newTestManager(t, database)
	mgr.HistoryManager.now = func() time.Time { return time.Unix(1700001000, 0).UTC() }

	conv, err := mgr.Create(ctx, projectID, WithTitle("Hyphenated search"))
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}
	if err := mgr.PersistUserMessage(ctx, conv.ID, 1, "runtime-token-1775380560910306008 should be searchable"); err != nil {
		t.Fatalf("PersistUserMessage error: %v", err)
	}

	results, err := mgr.Search(ctx, "runtime-token-1775380560910306008")
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("Search returned no results, want hyphenated exact token match")
	}
	if results[0].ID != conv.ID {
		t.Fatalf("Search result ID = %q, want %q", results[0].ID, conv.ID)
	}
}

func TestManagerSearchDeduplicatesConversationResults(t *testing.T) {
	ctx := context.Background()
	database := newTestDB(t)
	projectID := seedProject(t, database)
	mgr := newTestManager(t, database)
	mgr.HistoryManager.now = func() time.Time { return time.Unix(1700001000, 0).UTC() }

	conv, err := mgr.Create(ctx, projectID, WithTitle("Search dedupe"))
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}
	token := "runtime-token-search-dedupe"
	if err := mgr.PersistUserMessage(ctx, conv.ID, 1, token+" in user message"); err != nil {
		t.Fatalf("PersistUserMessage error: %v", err)
	}
	if err := mgr.PersistIteration(ctx, conv.ID, 1, 1, []IterationMessage{
		{Role: "assistant", Content: `[{"type":"text","text":"saw runtime-token-search-dedupe in assistant text"}]`},
		{Role: "tool", Content: "tool output mentions runtime-token-search-dedupe", ToolUseID: "tool-1", ToolName: "brain_search"},
	}); err != nil {
		t.Fatalf("PersistIteration error: %v", err)
	}

	results, err := mgr.Search(ctx, token)
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Search returned %d results, want 1 deduplicated conversation result", len(results))
	}
	if results[0].ID != conv.ID {
		t.Fatalf("Search result ID = %q, want %q", results[0].ID, conv.ID)
	}
}

func TestManagerSearchPrefersNaturalLanguageSnippetOverToolOutput(t *testing.T) {
	ctx := context.Background()
	database := newTestDB(t)
	projectID := seedProject(t, database)
	mgr := newTestManager(t, database)
	mgr.HistoryManager.now = func() time.Time { return time.Unix(1700001000, 0).UTC() }

	conv, err := mgr.Create(ctx, projectID, WithTitle("Search snippet quality"))
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}
	token := "runtime-token-snippet-quality"
	if err := mgr.PersistUserMessage(ctx, conv.ID, 1, "Search for "+token); err != nil {
		t.Fatalf("PersistUserMessage error: %v", err)
	}
	if err := mgr.PersistIteration(ctx, conv.ID, 1, 1, []IterationMessage{
		{Role: "assistant", Content: `[{"type":"tool_use","id":"call1","name":"brain_search","input":{"query":"runtime-token-snippet-quality"}}]`},
		{Role: "tool", Content: "Found 1 brain document for \"runtime-token-snippet-quality\":\n- notes/runtime/runtime-token-snippet-quality.md — Runtime Token Snippet Quality", ToolUseID: "call1", ToolName: "brain_search"},
		{Role: "assistant", Content: `[{"type":"text","text":"Note path: runtime-token-snippet-quality.md. Search found the same note: yes."}]`},
	}); err != nil {
		t.Fatalf("PersistIteration error: %v", err)
	}

	results, err := mgr.Search(ctx, token)
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Search returned %d results, want 1 deduplicated conversation result", len(results))
	}
	if strings.Contains(results[0].Snippet, "<b>") || strings.Contains(results[0].Snippet, "</b>") {
		t.Fatalf("Search snippet = %q, want FTS highlight tags stripped", results[0].Snippet)
	}
	if got := results[0].Snippet; got != "Note path: runtime-token-snippet-quality.md. Search found the same note: yes." {
		t.Fatalf("Search snippet = %q, want natural-language assistant snippet without FTS highlight tags", got)
	}
}

func TestManagerSearchPrefersNaturalLanguageAssistantSnippetOverBrainToolDocumentBody(t *testing.T) {
	ctx := context.Background()
	database := newTestDB(t)
	projectID := seedProject(t, database)
	mgr := newTestManager(t, database)
	mgr.HistoryManager.now = func() time.Time { return time.Unix(1700001000, 0).UTC() }

	conv, err := mgr.Create(ctx, projectID, WithTitle("Brain tool snippet quality"))
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}
	token := "runtime-token-brain-tool-snippet"
	notePath := "notes/runtime/runtime-token-brain-tool-snippet.md"
	if err := mgr.PersistUserMessage(ctx, conv.ID, 1, "Read back "+notePath+" and search for "+token); err != nil {
		t.Fatalf("PersistUserMessage error: %v", err)
	}
	if err := mgr.PersistIteration(ctx, conv.ID, 1, 1, []IterationMessage{
		{Role: "assistant", Content: `[{"type":"tool_use","id":"call1","name":"brain_read","input":{"path":"notes/runtime/runtime-token-brain-tool-snippet.md"}},{"type":"tool_use","id":"call2","name":"brain_search","input":{"query":"runtime-token-brain-tool-snippet"}}]`},
		{Role: "tool", Content: "Brain document: notes/runtime/runtime-token-brain-tool-snippet.md\n\nContent:\n```md\n# Runtime Token Brain Tool Snippet\n\nExact token: `runtime-token-brain-tool-snippet`\n\nSummary: a long brain document body that should not win snippet selection over the assistant's short natural-language answer.\n```", ToolUseID: "call1", ToolName: "brain_read"},
		{Role: "tool", Content: "Found 1 brain document for \"runtime-token-brain-tool-snippet\":\n- notes/runtime/runtime-token-brain-tool-snippet.md — Runtime Token Brain Tool Snippet\n  Exact token: `runtime-token-brain-tool-snippet`", ToolUseID: "call2", ToolName: "brain_search"},
		{Role: "assistant", Content: "[{\"type\":\"text\",\"text\":\"Exact note path: `notes/runtime/runtime-token-brain-tool-snippet.md`\\n\\nDid search find that same note? Yes.\"}]"},
	}); err != nil {
		t.Fatalf("PersistIteration error: %v", err)
	}

	results, err := mgr.Search(ctx, token)
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Search returned %d results, want 1 deduplicated conversation result", len(results))
	}
	want := "Exact note path: `notes/runtime/runtime-token-brain-tool-snippet.md` Did search find that same note? Yes."
	if got := results[0].Snippet; got != want {
		t.Fatalf("Search snippet = %q, want %q", got, want)
	}
}

// --- SeenFiles Integration ---

func TestManagerSeenFilesIntegration(t *testing.T) {
	database := newTestDB(t)
	seen := NewSeenFiles()
	mgr := NewManager(database, seen, nil)

	seen.Add("internal/auth/service.go", 1)
	seen.Add("internal/db/schema.sql", 2)

	lookup := mgr.SeenFiles("any-conv-id")
	if lookup == nil {
		t.Fatal("SeenFiles returned nil")
	}

	found, turn := lookup.Contains("internal/auth/service.go")
	if !found || turn != 1 {
		t.Fatalf("Contains(auth/service.go) = (%v, %d), want (true, 1)", found, turn)
	}

	found, _ = lookup.Contains("nonexistent.go")
	if found {
		t.Fatal("Contains should return false for unseen file")
	}
}

// --- Sequence Numbering Test ---

func TestManagerSequenceNumbering(t *testing.T) {
	ctx := context.Background()
	database := newTestDB(t)
	projectID := seedProject(t, database)
	mgr := newTestManager(t, database)
	mgr.HistoryManager.now = func() time.Time { return time.Unix(1700001000, 0).UTC() }

	conv, err := mgr.Create(ctx, projectID)
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}

	// Persist messages across multiple interactions.
	if err := mgr.PersistUserMessage(ctx, conv.ID, 1, "msg1"); err != nil {
		t.Fatal(err)
	}
	if err := mgr.PersistIteration(ctx, conv.ID, 1, 1, []IterationMessage{
		{Role: "assistant", Content: `[{"type":"text","text":"response1"}]`},
		{Role: "tool", Content: "result1", ToolUseID: "tc1", ToolName: "file_read"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := mgr.PersistIteration(ctx, conv.ID, 1, 2, []IterationMessage{
		{Role: "assistant", Content: `[{"type":"text","text":"response2"}]`},
	}); err != nil {
		t.Fatal(err)
	}

	history, err := mgr.ReconstructHistory(ctx, conv.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Expected sequences: 0.0, 1.0, 2.0, 3.0
	expectedSeqs := []float64{0.0, 1.0, 2.0, 3.0}
	if len(history) != len(expectedSeqs) {
		t.Fatalf("history length = %d, want %d", len(history), len(expectedSeqs))
	}
	for i, want := range expectedSeqs {
		if history[i].Sequence != want {
			t.Fatalf("history[%d].Sequence = %f, want %f", i, history[i].Sequence, want)
		}
	}
}

// --- Delete Cascade Integration ---

func TestManagerDeleteCascadesAllRelated(t *testing.T) {
	ctx := context.Background()
	database := newTestDB(t)
	projectID := seedProject(t, database)
	queries := db.New(database)
	mgr := newTestManager(t, database)
	mgr.HistoryManager.now = func() time.Time { return time.Unix(1700001000, 0).UTC() }

	conv, err := mgr.Create(ctx, projectID)
	if err != nil {
		t.Fatal(err)
	}

	// Add messages.
	if err := mgr.PersistUserMessage(ctx, conv.ID, 1, "hello"); err != nil {
		t.Fatal(err)
	}
	if err := mgr.PersistIteration(ctx, conv.ID, 1, 1, []IterationMessage{
		{Role: "assistant", Content: `[{"type":"text","text":"hi"}]`},
	}); err != nil {
		t.Fatal(err)
	}

	// Verify messages exist.
	msgs, _ := queries.ListActiveMessages(ctx, conv.ID)
	if len(msgs) == 0 {
		t.Fatal("expected messages before delete")
	}

	// Delete.
	if err := mgr.Delete(ctx, conv.ID); err != nil {
		t.Fatal(err)
	}

	// Verify all messages are gone.
	msgs, _ = queries.ListActiveMessages(ctx, conv.ID)
	if len(msgs) != 0 {
		t.Fatalf("messages after delete = %d, want 0", len(msgs))
	}
}
