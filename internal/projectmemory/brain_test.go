package projectmemory

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestBrainBackendWriteReadListSearchAndRestart(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	backend, err := OpenBrainBackend(ctx, Config{DataDir: dataDir, DurableAck: true})
	if err != nil {
		t.Fatalf("OpenBrainBackend: %v", err)
	}

	const path = "notes/design.md"
	const content = "# Design\n\nPipeline memory notes."
	if err := backend.WriteDocument(ctx, path, content); err != nil {
		t.Fatalf("WriteDocument: %v", err)
	}
	got, err := backend.ReadDocument(ctx, path)
	if err != nil {
		t.Fatalf("ReadDocument: %v", err)
	}
	if got != content {
		t.Fatalf("ReadDocument = %q, want %q", got, content)
	}
	paths, err := backend.ListDocuments(ctx, "notes")
	if err != nil {
		t.Fatalf("ListDocuments: %v", err)
	}
	if len(paths) != 1 || paths[0] != path {
		t.Fatalf("ListDocuments = %#v, want [%s]", paths, path)
	}
	hits, err := backend.SearchKeywordLimit(ctx, "pipeline", 5)
	if err != nil {
		t.Fatalf("SearchKeywordLimit: %v", err)
	}
	if len(hits) != 1 || hits[0].Path != path {
		t.Fatalf("SearchKeywordLimit = %#v, want hit for %s", hits, path)
	}
	if err := backend.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := OpenBrainBackend(ctx, Config{DataDir: dataDir, DurableAck: true})
	if err != nil {
		t.Fatalf("reopen OpenBrainBackend: %v", err)
	}
	defer reopened.Close()
	got, err = reopened.ReadDocument(ctx, path)
	if err != nil {
		t.Fatalf("ReadDocument after restart: %v", err)
	}
	if got != content {
		t.Fatalf("ReadDocument after restart = %q, want %q", got, content)
	}
}

func TestBrainBackendPatchConflictUsesExpectedHash(t *testing.T) {
	ctx := context.Background()
	backend, err := OpenBrainBackend(ctx, Config{DataDir: t.TempDir(), DurableAck: true})
	if err != nil {
		t.Fatalf("OpenBrainBackend: %v", err)
	}
	defer backend.Close()

	const path = "notes/design.md"
	if err := backend.WriteDocument(ctx, path, "# Design\n\nInitial."); err != nil {
		t.Fatalf("WriteDocument: %v", err)
	}
	doc, _, err := backend.runtime.ReadDocument(ctx, path)
	if err != nil {
		t.Fatalf("ReadDocument metadata: %v", err)
	}
	if err := backend.PatchDocument(ctx, path, "append", "Concurrent update."); err != nil {
		t.Fatalf("PatchDocument concurrent update: %v", err)
	}
	err = backend.PatchDocumentWithExpectedHash(ctx, path, "append", doc.ContentHash, "# Design\n\nStale update.")
	if err == nil {
		t.Fatal("PatchDocumentWithExpectedHash succeeded, want conflict")
	}
	if !strings.Contains(err.Error(), "patch conflict") {
		t.Fatalf("PatchDocumentWithExpectedHash error = %v, want patch conflict", err)
	}
}

func TestBrainIndexStateTracksDirtyAndClean(t *testing.T) {
	ctx := context.Background()
	backend, err := OpenBrainBackend(ctx, Config{DataDir: t.TempDir(), DurableAck: true})
	if err != nil {
		t.Fatalf("OpenBrainBackend: %v", err)
	}
	defer backend.Close()

	state, found, err := backend.ReadBrainIndexState(ctx)
	if err != nil {
		t.Fatalf("ReadBrainIndexState before write: %v", err)
	}
	if found {
		t.Fatalf("ReadBrainIndexState before write found %+v, want missing", state)
	}
	if err := backend.WriteDocument(ctx, "notes/index.md", "# Index\n\nDirty me."); err != nil {
		t.Fatalf("WriteDocument: %v", err)
	}
	state, found, err = backend.ReadBrainIndexState(ctx)
	if err != nil {
		t.Fatalf("ReadBrainIndexState after write: %v", err)
	}
	if !found || !state.Dirty || state.DirtyReason != "write_document" || state.DirtySinceUS == 0 {
		t.Fatalf("state after write = %+v, want dirty write_document", state)
	}

	indexedAt := time.Date(2026, 5, 5, 12, 0, 0, 123000, time.UTC)
	if err := backend.MarkBrainIndexClean(ctx, indexedAt, `{"test":true}`); err != nil {
		t.Fatalf("MarkBrainIndexClean: %v", err)
	}
	state, found, err = backend.ReadBrainIndexState(ctx)
	if err != nil {
		t.Fatalf("ReadBrainIndexState after clean: %v", err)
	}
	if !found || state.Dirty || state.LastIndexedAtUS != uint64(indexedAt.UnixMicro()) || state.MetadataJSON != `{"test":true}` {
		t.Fatalf("state after clean = %+v, want clean indexed metadata", state)
	}

	if err := backend.PatchDocument(ctx, "notes/index.md", "append", "Dirty again."); err != nil {
		t.Fatalf("PatchDocument: %v", err)
	}
	state, found, err = backend.ReadBrainIndexState(ctx)
	if err != nil {
		t.Fatalf("ReadBrainIndexState after patch: %v", err)
	}
	if !found || !state.Dirty || state.DirtyReason != "patch_document" || state.LastIndexedAtUS != uint64(indexedAt.UnixMicro()) {
		t.Fatalf("state after patch = %+v, want dirty with preserved last indexed time", state)
	}
}

func TestCodeIndexStateTracksFilesAndRestart(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	backend, err := OpenBrainBackend(ctx, Config{DataDir: dataDir, DurableAck: true})
	if err != nil {
		t.Fatalf("OpenBrainBackend: %v", err)
	}
	indexedAt := time.Date(2026, 5, 5, 14, 0, 0, 0, time.UTC)
	files := []CodeFileIndexArg{
		{FilePath: "main.go", FileHash: "hash-main", ChunkCount: 2},
		{FilePath: "internal/app.go", FileHash: "hash-app", ChunkCount: 1},
	}
	if err := backend.MarkCodeIndexClean(ctx, "abc123", indexedAt, files, nil, `{"test":true}`); err != nil {
		t.Fatalf("MarkCodeIndexClean: %v", err)
	}
	state, found, err := backend.ReadCodeIndexState(ctx)
	if err != nil {
		t.Fatalf("ReadCodeIndexState: %v", err)
	}
	if !found || state.LastIndexedCommit != "abc123" || state.LastIndexedAtUS != uint64(indexedAt.UnixMicro()) || state.Dirty {
		t.Fatalf("code index state = %+v found=%t, want clean abc123", state, found)
	}
	fileStates, err := backend.ListCodeFileIndexStates(ctx)
	if err != nil {
		t.Fatalf("ListCodeFileIndexStates: %v", err)
	}
	if len(fileStates) != 2 || fileStates[0].FilePath != "internal/app.go" || fileStates[1].FilePath != "main.go" {
		t.Fatalf("file states = %+v, want sorted app/main", fileStates)
	}
	if err := backend.MarkCodeIndexClean(ctx, "def456", indexedAt.Add(time.Minute), []CodeFileIndexArg{{FilePath: "main.go", FileHash: "hash-main-2", ChunkCount: 3}}, []string{"internal/app.go"}, ""); err != nil {
		t.Fatalf("MarkCodeIndexClean update: %v", err)
	}
	fileStates, err = backend.ListCodeFileIndexStates(ctx)
	if err != nil {
		t.Fatalf("ListCodeFileIndexStates after update: %v", err)
	}
	if len(fileStates) != 1 || fileStates[0].FilePath != "main.go" || fileStates[0].FileHash != "hash-main-2" || fileStates[0].ChunkCount != 3 {
		t.Fatalf("file states after update = %+v, want updated main only", fileStates)
	}
	if err := backend.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := OpenBrainBackend(ctx, Config{DataDir: dataDir, DurableAck: true})
	if err != nil {
		t.Fatalf("reopen OpenBrainBackend: %v", err)
	}
	defer reopened.Close()
	state, found, err = reopened.ReadCodeIndexState(ctx)
	if err != nil {
		t.Fatalf("ReadCodeIndexState after restart: %v", err)
	}
	if !found || state.LastIndexedCommit != "def456" {
		t.Fatalf("state after restart = %+v found=%t, want def456", state, found)
	}
}

func TestConversationHistorySearchAndRestart(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	backend, err := OpenBrainBackend(ctx, Config{DataDir: dataDir, DurableAck: true})
	if err != nil {
		t.Fatalf("OpenBrainBackend: %v", err)
	}

	createdAt := time.Date(2026, 5, 5, 16, 0, 0, 0, time.UTC)
	if err := backend.CreateConversation(ctx, CreateConversationArgs{
		ID:          "conv-1",
		ProjectID:   "project-1",
		Title:       "Memory Slice",
		Provider:    "codex",
		Model:       "gpt-5",
		CreatedAtUS: uint64(createdAt.UnixMicro()),
	}); err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	if err := backend.AppendUserMessage(ctx, AppendUserMessageArgs{
		ConversationID: "conv-1",
		TurnNumber:     1,
		Content:        "please wire Shunter conversation memory",
		CreatedAtUS:    uint64(createdAt.Add(time.Second).UnixMicro()),
	}); err != nil {
		t.Fatalf("AppendUserMessage: %v", err)
	}
	if err := backend.PersistIteration(ctx, PersistIterationArgs{
		ConversationID: "conv-1",
		TurnNumber:     1,
		Iteration:      1,
		CreatedAtUS:    uint64(createdAt.Add(2 * time.Second).UnixMicro()),
		Messages: []PersistIterationMessage{
			{Role: "assistant", Content: "Shunter conversation memory is persisted."},
			{Role: "tool", ToolUseID: "tool-1", ToolName: "read_file", Content: "tool output"},
		},
	}); err != nil {
		t.Fatalf("PersistIteration: %v", err)
	}
	next, err := backend.NextTurnNumber(ctx, "conv-1")
	if err != nil {
		t.Fatalf("NextTurnNumber: %v", err)
	}
	if next != 2 {
		t.Fatalf("NextTurnNumber = %d, want 2", next)
	}
	messages, err := backend.ListMessages(ctx, "conv-1", false)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(messages) != 3 || messages[0].Role != "user" || messages[1].Role != "assistant" || messages[2].ToolName != "read_file" {
		t.Fatalf("messages = %+v, want user/assistant/tool", messages)
	}
	hits, err := backend.SearchConversations(ctx, "project-1", "conversation memory", 20)
	if err != nil {
		t.Fatalf("SearchConversations: %v", err)
	}
	if len(hits) != 1 || hits[0].ID != "conv-1" {
		t.Fatalf("SearchConversations = %+v, want conv-1", hits)
	}
	if err := backend.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := OpenBrainBackend(ctx, Config{DataDir: dataDir, DurableAck: true})
	if err != nil {
		t.Fatalf("reopen OpenBrainBackend: %v", err)
	}
	defer reopened.Close()
	conversation, found, err := reopened.ReadConversation(ctx, "conv-1")
	if err != nil {
		t.Fatalf("ReadConversation after restart: %v", err)
	}
	if !found || conversation.Title != "Memory Slice" || conversation.Provider != "codex" {
		t.Fatalf("conversation after restart = %+v found=%t, want Memory Slice", conversation, found)
	}
	messages, err = reopened.ListMessages(ctx, "conv-1", false)
	if err != nil {
		t.Fatalf("ListMessages after restart: %v", err)
	}
	if len(messages) != 3 {
		t.Fatalf("messages after restart = %d, want 3", len(messages))
	}
	if err := reopened.CancelIteration(ctx, CancelIterationArgs{ConversationID: "conv-1", TurnNumber: 1, Iteration: 1}); err != nil {
		t.Fatalf("CancelIteration: %v", err)
	}
	messages, err = reopened.ListMessages(ctx, "conv-1", false)
	if err != nil {
		t.Fatalf("ListMessages after cancel: %v", err)
	}
	if len(messages) != 1 || messages[0].Role != "user" {
		t.Fatalf("messages after cancel = %+v, want preserved user message only", messages)
	}
	if err := reopened.DiscardTurn(ctx, DiscardTurnArgs{ConversationID: "conv-1", TurnNumber: 1}); err != nil {
		t.Fatalf("DiscardTurn: %v", err)
	}
	messages, err = reopened.ListMessages(ctx, "conv-1", false)
	if err != nil {
		t.Fatalf("ListMessages after discard: %v", err)
	}
	if len(messages) != 0 {
		t.Fatalf("messages after discard = %+v, want none", messages)
	}
}

func TestSubCallsRecordLinkCancelAndRestart(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	backend, err := OpenBrainBackend(ctx, Config{DataDir: dataDir, DurableAck: true})
	if err != nil {
		t.Fatalf("OpenBrainBackend: %v", err)
	}

	createdAt := time.Date(2026, 5, 5, 18, 0, 0, 0, time.UTC)
	if err := backend.CreateConversation(ctx, CreateConversationArgs{
		ID:          "conv-sub",
		ProjectID:   "project-1",
		Title:       "Subcall Slice",
		CreatedAtUS: uint64(createdAt.UnixMicro()),
	}); err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	if err := backend.AppendUserMessage(ctx, AppendUserMessageArgs{
		ConversationID: "conv-sub",
		TurnNumber:     1,
		Content:        "track provider subcalls",
		CreatedAtUS:    uint64(createdAt.Add(time.Second).UnixMicro()),
	}); err != nil {
		t.Fatalf("AppendUserMessage: %v", err)
	}
	if err := backend.RecordSubCall(ctx, RecordSubCallArgs{
		ID:                  "sub-1",
		ConversationID:      "conv-sub",
		TurnNumber:          1,
		Iteration:           1,
		Provider:            "codex",
		Model:               "gpt-5.5",
		Purpose:             "chat",
		Status:              "success",
		StartedAtUS:         uint64(createdAt.Add(2 * time.Second).UnixMicro()),
		CompletedAtUS:       uint64(createdAt.Add(3 * time.Second).UnixMicro()),
		TokensIn:            123,
		TokensOut:           45,
		CacheReadTokens:     7,
		CacheCreationTokens: 8,
		LatencyMs:           1000,
	}); err != nil {
		t.Fatalf("RecordSubCall: %v", err)
	}
	subCalls, err := backend.ListSubCalls(ctx, "conv-sub")
	if err != nil {
		t.Fatalf("ListSubCalls before link: %v", err)
	}
	if len(subCalls) != 1 || subCalls[0].MessageID != "" || subCalls[0].TokensIn != 123 {
		t.Fatalf("subcalls before link = %+v, want one unlinked subcall", subCalls)
	}
	if err := backend.PersistIteration(ctx, PersistIterationArgs{
		ConversationID: "conv-sub",
		TurnNumber:     1,
		Iteration:      1,
		CreatedAtUS:    uint64(createdAt.Add(4 * time.Second).UnixMicro()),
		Messages: []PersistIterationMessage{
			{Role: "assistant", Content: "subcalls now live in Shunter"},
		},
	}); err != nil {
		t.Fatalf("PersistIteration: %v", err)
	}
	messages, err := backend.ListMessages(ctx, "conv-sub", false)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(messages) != 2 || messages[1].Role != "assistant" {
		t.Fatalf("messages = %+v, want user/assistant", messages)
	}
	subCalls, err = backend.ListTurnSubCalls(ctx, "conv-sub", 1)
	if err != nil {
		t.Fatalf("ListTurnSubCalls after link: %v", err)
	}
	if len(subCalls) != 1 || subCalls[0].MessageID != messages[1].ID || subCalls[0].Status != "success" {
		t.Fatalf("subcalls after link = %+v, want linked to %s", subCalls, messages[1].ID)
	}
	if err := backend.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := OpenBrainBackend(ctx, Config{DataDir: dataDir, DurableAck: true})
	if err != nil {
		t.Fatalf("reopen OpenBrainBackend: %v", err)
	}
	defer reopened.Close()
	subCalls, err = reopened.ListSubCalls(ctx, "conv-sub")
	if err != nil {
		t.Fatalf("ListSubCalls after restart: %v", err)
	}
	if len(subCalls) != 1 || subCalls[0].ID != "sub-1" || subCalls[0].MessageID != messages[1].ID {
		t.Fatalf("subcalls after restart = %+v, want linked sub-1", subCalls)
	}
	if err := reopened.CancelIteration(ctx, CancelIterationArgs{ConversationID: "conv-sub", TurnNumber: 1, Iteration: 1}); err != nil {
		t.Fatalf("CancelIteration: %v", err)
	}
	subCalls, err = reopened.ListSubCalls(ctx, "conv-sub")
	if err != nil {
		t.Fatalf("ListSubCalls after cancel: %v", err)
	}
	if len(subCalls) != 0 {
		t.Fatalf("subcalls after cancel = %+v, want none", subCalls)
	}
	if err := reopened.RecordSubCall(ctx, RecordSubCallArgs{
		ID:             "sub-2",
		ConversationID: "conv-sub",
		TurnNumber:     1,
		Iteration:      1,
		Provider:       "codex",
		Model:          "gpt-5.5",
		Purpose:        "chat",
		Status:         "error",
		Error:          "cancelled",
	}); err != nil {
		t.Fatalf("RecordSubCall second: %v", err)
	}
	if err := reopened.DiscardTurn(ctx, DiscardTurnArgs{ConversationID: "conv-sub", TurnNumber: 1}); err != nil {
		t.Fatalf("DiscardTurn: %v", err)
	}
	subCalls, err = reopened.ListSubCalls(ctx, "conv-sub")
	if err != nil {
		t.Fatalf("ListSubCalls after discard: %v", err)
	}
	if len(subCalls) != 0 {
		t.Fatalf("subcalls after discard = %+v, want none", subCalls)
	}
}

func TestToolExecutionsRecordCancelDiscardAndRestart(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	backend, err := OpenBrainBackend(ctx, Config{DataDir: dataDir, DurableAck: true})
	if err != nil {
		t.Fatalf("OpenBrainBackend: %v", err)
	}

	createdAt := time.Date(2026, 5, 5, 20, 0, 0, 0, time.UTC)
	if err := backend.CreateConversation(ctx, CreateConversationArgs{
		ID:          "conv-tool",
		ProjectID:   "project-1",
		Title:       "Tool Slice",
		CreatedAtUS: uint64(createdAt.UnixMicro()),
	}); err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	if err := backend.AppendUserMessage(ctx, AppendUserMessageArgs{
		ConversationID: "conv-tool",
		TurnNumber:     1,
		Content:        "track tool executions",
		CreatedAtUS:    uint64(createdAt.Add(time.Second).UnixMicro()),
	}); err != nil {
		t.Fatalf("AppendUserMessage: %v", err)
	}
	if err := backend.RecordToolExecution(ctx, RecordToolExecutionArgs{
		ConversationID: "conv-tool",
		TurnNumber:     1,
		Iteration:      1,
		ToolUseID:      "toolu-1",
		ToolName:       "file_read",
		Status:         "success",
		StartedAtUS:    uint64(createdAt.Add(2 * time.Second).UnixMicro()),
		CompletedAtUS:  uint64(createdAt.Add(3 * time.Second).UnixMicro()),
		DurationMs:     1000,
		InputJSON:      `{"path":"main.go"}`,
		OutputSize:     80,
		NormalizedSize: 40,
	}); err != nil {
		t.Fatalf("RecordToolExecution: %v", err)
	}
	executions, err := backend.ListToolExecutions(ctx, "conv-tool")
	if err != nil {
		t.Fatalf("ListToolExecutions: %v", err)
	}
	if len(executions) != 1 || executions[0].ToolName != "file_read" || executions[0].OutputSize != 80 || executions[0].Status != "success" {
		t.Fatalf("executions = %+v, want recorded file_read", executions)
	}
	expectedID := ToolExecutionID("conv-tool", 1, 1, "toolu-1", "file_read")
	if executions[0].ID != expectedID {
		t.Fatalf("execution ID = %q, want deterministic %q", executions[0].ID, expectedID)
	}
	if err := backend.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := OpenBrainBackend(ctx, Config{DataDir: dataDir, DurableAck: true})
	if err != nil {
		t.Fatalf("reopen OpenBrainBackend: %v", err)
	}
	defer reopened.Close()
	executions, err = reopened.ListTurnToolExecutions(ctx, "conv-tool", 1)
	if err != nil {
		t.Fatalf("ListTurnToolExecutions after restart: %v", err)
	}
	if len(executions) != 1 || executions[0].ID != expectedID {
		t.Fatalf("executions after restart = %+v, want %s", executions, expectedID)
	}
	if err := reopened.CancelIteration(ctx, CancelIterationArgs{ConversationID: "conv-tool", TurnNumber: 1, Iteration: 1}); err != nil {
		t.Fatalf("CancelIteration: %v", err)
	}
	executions, err = reopened.ListToolExecutions(ctx, "conv-tool")
	if err != nil {
		t.Fatalf("ListToolExecutions after cancel: %v", err)
	}
	if len(executions) != 0 {
		t.Fatalf("executions after cancel = %+v, want none", executions)
	}
	if err := reopened.RecordToolExecution(ctx, RecordToolExecutionArgs{
		ConversationID: "conv-tool",
		TurnNumber:     1,
		Iteration:      1,
		ToolUseID:      "toolu-2",
		ToolName:       "shell",
		Status:         "error",
		Error:          "cancelled",
	}); err != nil {
		t.Fatalf("RecordToolExecution second: %v", err)
	}
	if err := reopened.DiscardTurn(ctx, DiscardTurnArgs{ConversationID: "conv-tool", TurnNumber: 1}); err != nil {
		t.Fatalf("DiscardTurn: %v", err)
	}
	executions, err = reopened.ListToolExecutions(ctx, "conv-tool")
	if err != nil {
		t.Fatalf("ListToolExecutions after discard: %v", err)
	}
	if len(executions) != 0 {
		t.Fatalf("executions after discard = %+v, want none", executions)
	}
}

func TestContextReportsStoreUpdateDiscardAndRestart(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	backend, err := OpenBrainBackend(ctx, Config{DataDir: dataDir, DurableAck: true})
	if err != nil {
		t.Fatalf("OpenBrainBackend: %v", err)
	}

	createdAt := time.Date(2026, 5, 5, 22, 0, 0, 0, time.UTC)
	if err := backend.CreateConversation(ctx, CreateConversationArgs{
		ID:          "conv-context",
		ProjectID:   "project-1",
		Title:       "Context Slice",
		CreatedAtUS: uint64(createdAt.UnixMicro()),
	}); err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	if err := backend.StoreContextReport(ctx, StoreContextReportArgs{
		ConversationID: "conv-context",
		TurnNumber:     1,
		CreatedAtUS:    uint64(createdAt.Add(time.Second).UnixMicro()),
		RequestJSON:    `{"conversation_id":"conv-context","turn_number":1}`,
		ReportJSON:     `{"turn_number":1,"included_chunks":["internal/auth/service.go"]}`,
		QualityJSON:    `{"agent_read_files":[]}`,
	}); err != nil {
		t.Fatalf("StoreContextReport: %v", err)
	}
	report, found, err := backend.ReadContextReport(ctx, "conv-context", 1)
	if err != nil {
		t.Fatalf("ReadContextReport: %v", err)
	}
	expectedID := ContextReportID("conv-context", 1)
	if !found || report.ID != expectedID || !strings.Contains(report.ReportJSON, "included_chunks") {
		t.Fatalf("report = %+v found=%t, want %s", report, found, expectedID)
	}
	if err := backend.StoreContextReport(ctx, StoreContextReportArgs{
		ConversationID: "conv-context",
		TurnNumber:     2,
		CreatedAtUS:    uint64(createdAt.Add(2 * time.Second).UnixMicro()),
		RequestJSON:    `{"conversation_id":"conv-context","turn_number":2}`,
		ReportJSON:     `{"turn_number":2,"included_chunks":["internal/runtime/engine.go"]}`,
		QualityJSON:    `{"agent_read_files":[]}`,
	}); err != nil {
		t.Fatalf("StoreContextReport second: %v", err)
	}
	reports, err := backend.ListContextReports(ctx, "conv-context")
	if err != nil {
		t.Fatalf("ListContextReports: %v", err)
	}
	if len(reports) != 2 || reports[0].TurnNumber != 1 || reports[1].TurnNumber != 2 {
		t.Fatalf("reports = %+v, want turn ordered reports 1,2", reports)
	}
	if err := backend.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := OpenBrainBackend(ctx, Config{DataDir: dataDir, DurableAck: true})
	if err != nil {
		t.Fatalf("reopen OpenBrainBackend: %v", err)
	}
	defer reopened.Close()
	report, found, err = reopened.ReadContextReport(ctx, "conv-context", 1)
	if err != nil {
		t.Fatalf("ReadContextReport after restart: %v", err)
	}
	if !found || report.ID != expectedID {
		t.Fatalf("report after restart = %+v found=%t, want %s", report, found, expectedID)
	}
	reports, err = reopened.ListContextReports(ctx, "conv-context")
	if err != nil {
		t.Fatalf("ListContextReports after restart: %v", err)
	}
	if len(reports) != 2 || reports[0].TurnNumber != 1 || reports[1].TurnNumber != 2 {
		t.Fatalf("reports after restart = %+v, want turn ordered reports 1,2", reports)
	}
	if err := reopened.UpdateContextReportQuality(ctx, UpdateContextReportQualityArgs{
		ConversationID: "conv-context",
		TurnNumber:     1,
		UpdatedAtUS:    uint64(createdAt.Add(2 * time.Second).UnixMicro()),
		QualityJSON:    `{"agent_used_search_tool":true,"agent_read_files":["internal/auth/service.go"],"context_hit_rate":1}`,
	}); err != nil {
		t.Fatalf("UpdateContextReportQuality: %v", err)
	}
	report, found, err = reopened.ReadContextReport(ctx, "conv-context", 1)
	if err != nil {
		t.Fatalf("ReadContextReport after quality: %v", err)
	}
	if !found || !strings.Contains(report.QualityJSON, "context_hit_rate") || report.UpdatedAtUS != uint64(createdAt.Add(2*time.Second).UnixMicro()) {
		t.Fatalf("report after quality = %+v found=%t, want quality update", report, found)
	}
	if err := reopened.DiscardTurn(ctx, DiscardTurnArgs{ConversationID: "conv-context", TurnNumber: 1}); err != nil {
		t.Fatalf("DiscardTurn: %v", err)
	}
	_, found, err = reopened.ReadContextReport(ctx, "conv-context", 1)
	if err != nil {
		t.Fatalf("ReadContextReport after discard: %v", err)
	}
	if found {
		t.Fatal("ReadContextReport after discard found report, want missing")
	}
	reports, err = reopened.ListContextReports(ctx, "conv-context")
	if err != nil {
		t.Fatalf("ListContextReports after discard: %v", err)
	}
	if len(reports) != 1 || reports[0].TurnNumber != 2 {
		t.Fatalf("reports after discard = %+v, want only turn 2", reports)
	}
}

func TestChainsStepsEventsStoreAndRestart(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	backend, err := OpenBrainBackend(ctx, Config{DataDir: dataDir, DurableAck: true})
	if err != nil {
		t.Fatalf("OpenBrainBackend: %v", err)
	}

	startedAt := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	if err := backend.StartChain(ctx, StartChainArgs{
		ID:               "chain-shunter",
		SourceSpecsJSON:  `["specs/one.md"]`,
		SourceTask:       "ship chain state",
		MaxSteps:         5,
		MaxResolverLoops: 2,
		MaxDurationSecs:  3600,
		TokenBudget:      1000,
		CreatedAtUS:      uint64(startedAt.UnixMicro()),
	}); err != nil {
		t.Fatalf("StartChain: %v", err)
	}
	if err := backend.StartStep(ctx, StartStepArgs{
		ID:          "step-1",
		ChainID:     "chain-shunter",
		Sequence:    1,
		Role:        "coder",
		Task:        "do work",
		TaskContext: "ctx-1",
		CreatedAtUS: uint64(startedAt.Add(time.Second).UnixMicro()),
	}); err != nil {
		t.Fatalf("StartStep: %v", err)
	}
	if err := backend.StepRunning(ctx, StepRunningArgs{ID: "step-1", StartedAtUS: uint64(startedAt.Add(2 * time.Second).UnixMicro())}); err != nil {
		t.Fatalf("StepRunning: %v", err)
	}
	if err := backend.CompleteStep(ctx, CompleteStepArgs{
		ID:            "step-1",
		Status:        "completed",
		Verdict:       "completed",
		ReceiptPath:   "receipts/coder/chain-shunter-step-001.md",
		TokensUsed:    42,
		TurnsUsed:     3,
		DurationSecs:  9,
		ExitCode:      0,
		HasExitCode:   true,
		CompletedAtUS: uint64(startedAt.Add(10 * time.Second).UnixMicro()),
	}); err != nil {
		t.Fatalf("CompleteStep: %v", err)
	}
	if err := backend.UpdateChainMetrics(ctx, UpdateChainMetricsArgs{
		ID:                "chain-shunter",
		TotalSteps:        1,
		TotalTokens:       42,
		TotalDurationSecs: 9,
		ResolverLoops:     0,
		UpdatedAtUS:       uint64(startedAt.Add(11 * time.Second).UnixMicro()),
	}); err != nil {
		t.Fatalf("UpdateChainMetrics: %v", err)
	}
	if err := backend.LogChainEvent(ctx, LogChainEventArgs{
		ChainID:     "chain-shunter",
		EventType:   "chain_started",
		PayloadJSON: `{"execution_id":"exec-1"}`,
		CreatedAtUS: uint64(startedAt.Add(12 * time.Second).UnixMicro()),
	}); err != nil {
		t.Fatalf("LogChainEvent start: %v", err)
	}
	if err := backend.LogChainEvent(ctx, LogChainEventArgs{
		ChainID:     "chain-shunter",
		StepID:      "step-1",
		EventType:   "step_completed",
		PayloadJSON: `{"verdict":"completed"}`,
		CreatedAtUS: uint64(startedAt.Add(13 * time.Second).UnixMicro()),
	}); err != nil {
		t.Fatalf("LogChainEvent step: %v", err)
	}
	if err := backend.CompleteChain(ctx, CompleteChainArgs{
		ID:            "chain-shunter",
		Status:        "completed",
		Summary:       "done",
		CompletedAtUS: uint64(startedAt.Add(14 * time.Second).UnixMicro()),
	}); err != nil {
		t.Fatalf("CompleteChain: %v", err)
	}
	if err := backend.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := OpenBrainBackend(ctx, Config{DataDir: dataDir, DurableAck: true})
	if err != nil {
		t.Fatalf("reopen OpenBrainBackend: %v", err)
	}
	defer reopened.Close()
	chain, found, err := reopened.ReadChain(ctx, "chain-shunter")
	if err != nil {
		t.Fatalf("ReadChain: %v", err)
	}
	if !found || chain.Status != "completed" || !strings.Contains(chain.MetricsJSON, `"total_tokens":42`) || !strings.Contains(chain.LimitsJSON, `"max_steps":5`) {
		t.Fatalf("chain = %+v found=%t, want completed metrics/limits", chain, found)
	}
	steps, err := reopened.ListChainSteps(ctx, "chain-shunter")
	if err != nil {
		t.Fatalf("ListChainSteps: %v", err)
	}
	if len(steps) != 1 || steps[0].ID != "step-1" || steps[0].TokensUsed != 42 || !steps[0].HasExitCode {
		t.Fatalf("steps = %+v, want completed step-1", steps)
	}
	events, err := reopened.ListChainEventsSince(ctx, "chain-shunter", 1)
	if err != nil {
		t.Fatalf("ListChainEventsSince: %v", err)
	}
	if len(events) != 1 || events[0].Sequence != 2 || events[0].EventType != "step_completed" {
		t.Fatalf("events since 1 = %+v, want step_completed sequence 2", events)
	}
}

func TestCompleteStepWithReceiptIsAtomicChainAndBrainWrite(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	backend, err := OpenBrainBackend(ctx, Config{DataDir: dataDir, DurableAck: true})
	if err != nil {
		t.Fatalf("OpenBrainBackend: %v", err)
	}

	startedAt := time.Date(2026, 5, 6, 13, 0, 0, 0, time.UTC)
	if err := backend.StartChain(ctx, StartChainArgs{
		ID:               "chain-receipt-atomic",
		SourceTask:       "atomic receipt",
		MaxSteps:         3,
		MaxResolverLoops: 1,
		MaxDurationSecs:  300,
		TokenBudget:      1000,
		CreatedAtUS:      uint64(startedAt.UnixMicro()),
	}); err != nil {
		t.Fatalf("StartChain: %v", err)
	}
	if err := backend.StartStep(ctx, StartStepArgs{
		ID:          "step-receipt-atomic",
		ChainID:     "chain-receipt-atomic",
		Sequence:    1,
		Role:        "coder",
		Task:        "write receipt",
		CreatedAtUS: uint64(startedAt.Add(time.Second).UnixMicro()),
	}); err != nil {
		t.Fatalf("StartStep: %v", err)
	}
	receiptContent := `---
agent: coder
chain_id: chain-receipt-atomic
step: 1
verdict: completed
timestamp: 2026-05-06T13:00:10Z
turns_used: 2
tokens_used: 55
duration_seconds: 7
---

Done atomically.
`
	if err := backend.CompleteStepWithReceipt(ctx, CompleteStepWithReceiptArgs{
		StepID:            "step-receipt-atomic",
		ChainID:           "chain-receipt-atomic",
		Status:            "completed",
		Verdict:           "completed",
		ReceiptPath:       "receipts/coder/chain-receipt-atomic-step-001.md",
		ReceiptContent:    receiptContent,
		TokensUsed:        55,
		TurnsUsed:         2,
		DurationSecs:      7,
		ExitCode:          0,
		HasExitCode:       true,
		CompletedAtUS:     uint64(startedAt.Add(10 * time.Second).UnixMicro()),
		TotalSteps:        1,
		TotalTokens:       55,
		TotalDurationSecs: 7,
		Events: []CompleteStepWithReceiptEvent{
			{
				StepID:      "step-receipt-atomic",
				EventType:   "step_completed",
				PayloadJSON: `{"verdict":"completed","tokens_used":55}`,
				CreatedAtUS: uint64(startedAt.Add(11 * time.Second).UnixMicro()),
			},
		},
	}); err != nil {
		t.Fatalf("CompleteStepWithReceipt: %v", err)
	}
	if err := backend.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := OpenBrainBackend(ctx, Config{DataDir: dataDir, DurableAck: true})
	if err != nil {
		t.Fatalf("reopen OpenBrainBackend: %v", err)
	}
	defer reopened.Close()
	content, err := reopened.ReadDocument(ctx, "receipts/coder/chain-receipt-atomic-step-001.md")
	if err != nil {
		t.Fatalf("ReadDocument receipt: %v", err)
	}
	if content != receiptContent {
		t.Fatalf("receipt content = %q, want atomic receipt content", content)
	}
	step, found, err := reopened.ReadStep(ctx, "step-receipt-atomic")
	if err != nil {
		t.Fatalf("ReadStep: %v", err)
	}
	if !found || step.Status != "completed" || step.ReceiptPath != "receipts/coder/chain-receipt-atomic-step-001.md" || step.TokensUsed != 55 {
		t.Fatalf("step = %+v found=%t, want completed atomic receipt step", step, found)
	}
	chain, found, err := reopened.ReadChain(ctx, "chain-receipt-atomic")
	if err != nil {
		t.Fatalf("ReadChain: %v", err)
	}
	if !found || !strings.Contains(chain.MetricsJSON, `"total_tokens":55`) {
		t.Fatalf("chain = %+v found=%t, want updated metrics", chain, found)
	}
	events, err := reopened.ListChainEvents(ctx, "chain-receipt-atomic")
	if err != nil {
		t.Fatalf("ListChainEvents: %v", err)
	}
	if len(events) != 1 || events[0].EventType != "step_completed" || !strings.Contains(events[0].PayloadJSON, `"tokens_used":55`) {
		t.Fatalf("events = %+v, want atomic step_completed", events)
	}
	state, found, err := reopened.ReadBrainIndexState(ctx)
	if err != nil {
		t.Fatalf("ReadBrainIndexState: %v", err)
	}
	if !found || !state.Dirty || state.DirtyReason != "complete_step_with_receipt" {
		t.Fatalf("brain index state = %+v found=%t, want dirty complete_step_with_receipt", state, found)
	}
}

func TestLaunchDraftsAndPresetsStoreAndRestart(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	backend, err := OpenBrainBackend(ctx, Config{DataDir: dataDir, DurableAck: true})
	if err != nil {
		t.Fatalf("OpenBrainBackend: %v", err)
	}

	updatedAt := time.Date(2026, 5, 6, 15, 0, 0, 0, time.UTC)
	if err := backend.SaveLaunch(ctx, SaveLaunchArgs{
		ProjectID:        "project-launch",
		LaunchID:         "current",
		Status:           "draft",
		Mode:             "constrained_orchestration",
		Role:             "coder",
		AllowedRolesJSON: `["coder","planner"]`,
		SourceTask:       "persist launch draft",
		SourceSpecsJSON:  `["docs/specs/a.md"]`,
		UpdatedAtUS:      uint64(updatedAt.UnixMicro()),
	}); err != nil {
		t.Fatalf("SaveLaunch: %v", err)
	}
	if err := backend.SaveLaunchPreset(ctx, SaveLaunchPresetArgs{
		ProjectID:        "project-launch",
		Name:             "audit pair",
		Mode:             "manual_roster",
		Role:             "coder,orchestrator",
		RosterJSON:       `["coder","orchestrator"]`,
		AllowedRolesJSON: `[]`,
		UpdatedAtUS:      uint64(updatedAt.Add(time.Second).UnixMicro()),
	}); err != nil {
		t.Fatalf("SaveLaunchPreset: %v", err)
	}
	if err := backend.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := OpenBrainBackend(ctx, Config{DataDir: dataDir, DurableAck: true})
	if err != nil {
		t.Fatalf("reopen OpenBrainBackend: %v", err)
	}
	defer reopened.Close()
	launch, found, err := reopened.ReadLaunch(ctx, "project-launch", "current")
	if err != nil {
		t.Fatalf("ReadLaunch: %v", err)
	}
	if !found || launch.Status != "draft" || launch.Mode != "constrained_orchestration" || launch.SourceTask != "persist launch draft" || !strings.Contains(launch.AllowedRolesJSON, "planner") {
		t.Fatalf("launch = %+v found=%t, want saved draft", launch, found)
	}
	presets, err := reopened.ListLaunchPresets(ctx, "project-launch")
	if err != nil {
		t.Fatalf("ListLaunchPresets: %v", err)
	}
	if len(presets) != 1 || presets[0].PresetID != "custom:audit pair" || presets[0].Name != "audit pair" || !strings.Contains(presets[0].RosterJSON, "orchestrator") {
		t.Fatalf("presets = %+v, want saved audit pair", presets)
	}
}

func TestRPCClientUsesParentBrainBackend(t *testing.T) {
	ctx := context.Background()
	backend, err := OpenBrainBackend(ctx, Config{DataDir: t.TempDir(), DurableAck: true})
	if err != nil {
		t.Fatalf("OpenBrainBackend: %v", err)
	}
	defer backend.Close()

	socketPath := t.TempDir() + "/memory.sock"
	server, err := StartRPCServer(ctx, RPCConfig{Transport: "unix", Path: socketPath}, backend)
	if err != nil {
		t.Fatalf("StartRPCServer: %v", err)
	}
	defer server.Close()

	client, err := DialBrainBackend("unix:" + socketPath)
	if err != nil {
		t.Fatalf("DialBrainBackend: %v", err)
	}
	defer client.Close()

	if err := client.WriteDocument(ctx, "notes/rpc.md", "# RPC\n\nRemote memory works."); err != nil {
		t.Fatalf("client WriteDocument: %v", err)
	}
	got, err := backend.ReadDocument(ctx, "notes/rpc.md")
	if err != nil {
		t.Fatalf("parent ReadDocument: %v", err)
	}
	if got != "# RPC\n\nRemote memory works." {
		t.Fatalf("parent content = %q, want RPC content", got)
	}
	hits, err := client.SearchKeywordLimit(ctx, "remote memory", 5)
	if err != nil {
		t.Fatalf("client SearchKeywordLimit: %v", err)
	}
	if len(hits) != 1 || hits[0].Path != "notes/rpc.md" {
		t.Fatalf("client SearchKeywordLimit = %#v, want notes/rpc.md", hits)
	}
	state, found, err := client.ReadBrainIndexState(ctx)
	if err != nil {
		t.Fatalf("client ReadBrainIndexState: %v", err)
	}
	if !found || !state.Dirty || state.DirtyReason != "write_document" {
		t.Fatalf("client ReadBrainIndexState = %+v found=%t, want dirty write_document", state, found)
	}
	indexedAt := time.Date(2026, 5, 5, 13, 0, 0, 0, time.UTC)
	if err := client.MarkBrainIndexClean(ctx, indexedAt, `{"rpc":true}`); err != nil {
		t.Fatalf("client MarkBrainIndexClean: %v", err)
	}
	parentState, found, err := backend.ReadBrainIndexState(ctx)
	if err != nil {
		t.Fatalf("parent ReadBrainIndexState: %v", err)
	}
	if !found || parentState.Dirty || parentState.LastIndexedAtUS != uint64(indexedAt.UnixMicro()) {
		t.Fatalf("parent state after RPC clean = %+v found=%t, want clean", parentState, found)
	}
	codeIndexedAt := time.Date(2026, 5, 5, 15, 0, 0, 0, time.UTC)
	if err := client.MarkCodeIndexClean(ctx, "rpc-commit", codeIndexedAt, []CodeFileIndexArg{{FilePath: "main.go", FileHash: "hash", ChunkCount: 1}}, nil, `{"rpc":true}`); err != nil {
		t.Fatalf("client MarkCodeIndexClean: %v", err)
	}
	codeState, found, err := backend.ReadCodeIndexState(ctx)
	if err != nil {
		t.Fatalf("parent ReadCodeIndexState: %v", err)
	}
	if !found || codeState.LastIndexedCommit != "rpc-commit" {
		t.Fatalf("parent code index state after RPC = %+v found=%t, want rpc-commit", codeState, found)
	}
	if err := client.CreateConversation(ctx, CreateConversationArgs{ID: "rpc-conv", ProjectID: "rpc-project", Title: "RPC Conversation", CreatedAtUS: uint64(time.Now().UTC().UnixMicro())}); err != nil {
		t.Fatalf("client CreateConversation: %v", err)
	}
	if err := client.AppendUserMessage(ctx, AppendUserMessageArgs{ConversationID: "rpc-conv", TurnNumber: 1, Content: "remote conversation write"}); err != nil {
		t.Fatalf("client AppendUserMessage: %v", err)
	}
	parentConversation, found, err := backend.ReadConversation(ctx, "rpc-conv")
	if err != nil {
		t.Fatalf("parent ReadConversation: %v", err)
	}
	if !found || parentConversation.Title != "RPC Conversation" {
		t.Fatalf("parent conversation after RPC = %+v found=%t, want RPC Conversation", parentConversation, found)
	}
	parentMessages, err := backend.ListMessages(ctx, "rpc-conv", false)
	if err != nil {
		t.Fatalf("parent ListMessages: %v", err)
	}
	if len(parentMessages) != 1 || parentMessages[0].Content != "remote conversation write" {
		t.Fatalf("parent messages after RPC = %+v, want remote conversation write", parentMessages)
	}
	if err := client.RecordSubCall(ctx, RecordSubCallArgs{
		ID:             "rpc-sub",
		ConversationID: "rpc-conv",
		TurnNumber:     1,
		Iteration:      1,
		Provider:       "codex",
		Model:          "gpt-5.5",
		Purpose:        "chat",
		Status:         "success",
		TokensIn:       10,
		TokensOut:      5,
	}); err != nil {
		t.Fatalf("client RecordSubCall: %v", err)
	}
	parentSubCalls, err := backend.ListSubCalls(ctx, "rpc-conv")
	if err != nil {
		t.Fatalf("parent ListSubCalls: %v", err)
	}
	if len(parentSubCalls) != 1 || parentSubCalls[0].ID != "rpc-sub" || parentSubCalls[0].TokensOut != 5 {
		t.Fatalf("parent subcalls after RPC = %+v, want rpc-sub", parentSubCalls)
	}
	if err := client.RecordToolExecution(ctx, RecordToolExecutionArgs{
		ConversationID: "rpc-conv",
		TurnNumber:     1,
		Iteration:      1,
		ToolUseID:      "toolu-rpc",
		ToolName:       "file_read",
		Status:         "success",
		OutputSize:     10,
		NormalizedSize: 8,
	}); err != nil {
		t.Fatalf("client RecordToolExecution: %v", err)
	}
	parentExecutions, err := backend.ListToolExecutions(ctx, "rpc-conv")
	if err != nil {
		t.Fatalf("parent ListToolExecutions: %v", err)
	}
	if len(parentExecutions) != 1 || parentExecutions[0].ToolUseID != "toolu-rpc" || parentExecutions[0].NormalizedSize != 8 {
		t.Fatalf("parent executions after RPC = %+v, want toolu-rpc", parentExecutions)
	}
	if err := client.StoreContextReport(ctx, StoreContextReportArgs{
		ConversationID: "rpc-conv",
		TurnNumber:     1,
		RequestJSON:    `{"conversation_id":"rpc-conv","turn_number":1}`,
		ReportJSON:     `{"turn_number":1,"included_chunks":["notes/rpc.md"]}`,
		QualityJSON:    `{"agent_read_files":[]}`,
	}); err != nil {
		t.Fatalf("client StoreContextReport: %v", err)
	}
	if err := client.UpdateContextReportQuality(ctx, UpdateContextReportQualityArgs{
		ConversationID: "rpc-conv",
		TurnNumber:     1,
		QualityJSON:    `{"agent_used_search_tool":true,"agent_read_files":["notes/rpc.md"],"context_hit_rate":1}`,
	}); err != nil {
		t.Fatalf("client UpdateContextReportQuality: %v", err)
	}
	parentReport, found, err := backend.ReadContextReport(ctx, "rpc-conv", 1)
	if err != nil {
		t.Fatalf("parent ReadContextReport: %v", err)
	}
	if !found || !strings.Contains(parentReport.QualityJSON, "notes/rpc.md") {
		t.Fatalf("parent report after RPC = %+v found=%t, want notes/rpc.md quality", parentReport, found)
	}
	clientReports, err := client.ListContextReports(ctx, "rpc-conv")
	if err != nil {
		t.Fatalf("client ListContextReports: %v", err)
	}
	if len(clientReports) != 1 || clientReports[0].TurnNumber != 1 || !strings.Contains(clientReports[0].QualityJSON, "notes/rpc.md") {
		t.Fatalf("client reports = %+v, want turn 1 report with quality", clientReports)
	}
	if err := client.StartChain(ctx, StartChainArgs{
		ID:               "rpc-chain",
		SourceSpecsJSON:  `["specs/rpc.md"]`,
		SourceTask:       "rpc chain",
		MaxSteps:         3,
		MaxResolverLoops: 1,
		MaxDurationSecs:  60,
		TokenBudget:      100,
	}); err != nil {
		t.Fatalf("client StartChain: %v", err)
	}
	if err := client.StartStep(ctx, StartStepArgs{ID: "rpc-step", ChainID: "rpc-chain", Sequence: 1, Role: "coder", Task: "rpc step"}); err != nil {
		t.Fatalf("client StartStep: %v", err)
	}
	if err := client.StepRunning(ctx, StepRunningArgs{ID: "rpc-step"}); err != nil {
		t.Fatalf("client StepRunning: %v", err)
	}
	if err := client.CompleteStep(ctx, CompleteStepArgs{ID: "rpc-step", Status: "completed", Verdict: "completed", TokensUsed: 7}); err != nil {
		t.Fatalf("client CompleteStep: %v", err)
	}
	if err := client.UpdateChainMetrics(ctx, UpdateChainMetricsArgs{ID: "rpc-chain", TotalSteps: 1, TotalTokens: 7}); err != nil {
		t.Fatalf("client UpdateChainMetrics: %v", err)
	}
	if err := client.LogChainEvent(ctx, LogChainEventArgs{ChainID: "rpc-chain", EventType: "chain_started", PayloadJSON: `{"rpc":true}`}); err != nil {
		t.Fatalf("client LogChainEvent: %v", err)
	}
	parentChain, found, err := backend.ReadChain(ctx, "rpc-chain")
	if err != nil {
		t.Fatalf("parent ReadChain: %v", err)
	}
	if !found || parentChain.SourceTask != "rpc chain" || !strings.Contains(parentChain.MetricsJSON, `"total_tokens":7`) {
		t.Fatalf("parent chain after RPC = %+v found=%t, want rpc chain metrics", parentChain, found)
	}
	parentSteps, err := backend.ListChainSteps(ctx, "rpc-chain")
	if err != nil {
		t.Fatalf("parent ListChainSteps: %v", err)
	}
	if len(parentSteps) != 1 || parentSteps[0].ID != "rpc-step" || parentSteps[0].TokensUsed != 7 {
		t.Fatalf("parent steps after RPC = %+v, want rpc-step", parentSteps)
	}
	parentEvents, err := backend.ListChainEvents(ctx, "rpc-chain")
	if err != nil {
		t.Fatalf("parent ListChainEvents: %v", err)
	}
	if len(parentEvents) != 1 || parentEvents[0].Sequence != 1 || parentEvents[0].EventType != "chain_started" {
		t.Fatalf("parent events after RPC = %+v, want chain_started", parentEvents)
	}
}
