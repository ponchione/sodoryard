package conversation

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/ponchione/sodoryard/internal/projectmemory"
)

func TestProjectMemoryManagerPersistsHistorySearchesAndRestarts(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	backend, err := projectmemory.OpenBrainBackend(ctx, projectmemory.Config{DataDir: dataDir, DurableAck: true})
	if err != nil {
		t.Fatalf("OpenBrainBackend: %v", err)
	}

	manager := NewProjectMemoryManager(backend, nil, slog.Default())
	manager.newID = func() string { return "conv-1" }
	now := time.Date(2026, 5, 5, 17, 0, 0, 0, time.UTC)
	manager.SetNowForTest(func() time.Time { return now })

	conv, err := manager.Create(ctx, "project-1", WithTitle("Shunter Chat"), WithProvider("codex"), WithModel("gpt-5"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if conv.ID != "conv-1" || conv.Title == nil || *conv.Title != "Shunter Chat" {
		t.Fatalf("conversation = %+v, want conv-1 Shunter Chat", conv)
	}
	if err := manager.PersistUserMessage(ctx, "conv-1", 1, "please persist history in Shunter"); err != nil {
		t.Fatalf("PersistUserMessage: %v", err)
	}
	if err := manager.PersistIteration(ctx, "conv-1", 1, 1, []IterationMessage{
		{Role: "assistant", Content: "history is stored in Shunter now"},
		{Role: "tool", ToolUseID: "tool-1", ToolName: "read_file", Content: "read output"},
	}); err != nil {
		t.Fatalf("PersistIteration: %v", err)
	}
	history, err := manager.ReconstructHistory(ctx, "conv-1")
	if err != nil {
		t.Fatalf("ReconstructHistory: %v", err)
	}
	if len(history) != 3 || history[0].Role != "user" || history[1].Role != "assistant" || history[2].ToolName.String != "read_file" {
		t.Fatalf("history = %+v, want user/assistant/tool", history)
	}
	next, err := manager.NextTurnNumber(ctx, "conv-1")
	if err != nil {
		t.Fatalf("NextTurnNumber: %v", err)
	}
	if next != 2 {
		t.Fatalf("NextTurnNumber = %d, want 2", next)
	}
	page, err := manager.GetMessagePage(ctx, "conv-1", 2, 0)
	if err != nil {
		t.Fatalf("GetMessagePage: %v", err)
	}
	if len(page) != 2 || page[0].Role != "assistant" || page[1].Role != "tool" {
		t.Fatalf("page = %+v, want newest assistant/tool in chronological order", page)
	}
	results, err := manager.Search(ctx, "project-1", "stored in Shunter")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 || results[0].ID != "conv-1" || results[0].Snippet == "" {
		t.Fatalf("Search = %+v, want conv-1 with snippet", results)
	}
	if err := backend.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := projectmemory.OpenBrainBackend(ctx, projectmemory.Config{DataDir: dataDir, DurableAck: true})
	if err != nil {
		t.Fatalf("reopen OpenBrainBackend: %v", err)
	}
	defer reopened.Close()
	reopenedManager := NewProjectMemoryManager(reopened, nil, slog.Default())
	got, err := reopenedManager.Get(ctx, "conv-1")
	if err != nil {
		t.Fatalf("Get after restart: %v", err)
	}
	if got.Title == nil || *got.Title != "Shunter Chat" {
		t.Fatalf("conversation after restart = %+v, want Shunter Chat", got)
	}
	history, err = reopenedManager.ReconstructHistory(ctx, "conv-1")
	if err != nil {
		t.Fatalf("ReconstructHistory after restart: %v", err)
	}
	if len(history) != 3 {
		t.Fatalf("history after restart = %d messages, want 3", len(history))
	}
}
