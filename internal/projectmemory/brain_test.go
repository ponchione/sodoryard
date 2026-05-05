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
}
