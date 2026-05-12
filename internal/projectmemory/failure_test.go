package projectmemory

import (
	"context"
	"testing"
	"time"
)

func TestCommittedDocumentWriteSurvivesCloseReopen(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	backend, err := OpenBrainBackend(ctx, Config{DataDir: dataDir, DurableAck: true})
	if err != nil {
		t.Fatalf("OpenBrainBackend: %v", err)
	}
	if err := backend.WriteDocument(ctx, "notes/failure-mode.md", "# Failure Mode\n\nCommitted document."); err != nil {
		t.Fatalf("WriteDocument: %v", err)
	}
	if err := backend.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := OpenBrainBackend(ctx, Config{DataDir: dataDir, DurableAck: true})
	if err != nil {
		t.Fatalf("reopen OpenBrainBackend: %v", err)
	}
	defer reopened.Close()
	got, err := reopened.ReadDocument(ctx, "notes/failure-mode.md")
	if err != nil {
		t.Fatalf("ReadDocument after reopen: %v", err)
	}
	if got != "# Failure Mode\n\nCommitted document." {
		t.Fatalf("document after reopen = %q, want committed content", got)
	}
}

func TestCommittedConversationMessageSurvivesCloseReopen(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	backend, err := OpenBrainBackend(ctx, Config{DataDir: dataDir, DurableAck: true})
	if err != nil {
		t.Fatalf("OpenBrainBackend: %v", err)
	}
	createdAt := time.Date(2026, 5, 7, 10, 0, 0, 0, time.UTC)
	if err := backend.CreateConversation(ctx, CreateConversationArgs{
		ID:          "conv-durable",
		ProjectID:   "project-durable",
		Title:       "Durable Conversation",
		Provider:    "codex",
		Model:       "gpt-5.5",
		CreatedAtUS: uint64(createdAt.UnixMicro()),
	}); err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	if err := backend.AppendUserMessage(ctx, AppendUserMessageArgs{
		ConversationID: "conv-durable",
		TurnNumber:     1,
		Content:        "persist this message across restart",
		CreatedAtUS:    uint64(createdAt.Add(time.Second).UnixMicro()),
	}); err != nil {
		t.Fatalf("AppendUserMessage: %v", err)
	}
	if err := backend.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := OpenBrainBackend(ctx, Config{DataDir: dataDir, DurableAck: true})
	if err != nil {
		t.Fatalf("reopen OpenBrainBackend: %v", err)
	}
	defer reopened.Close()
	conversation, found, err := reopened.ReadConversation(ctx, "conv-durable")
	if err != nil {
		t.Fatalf("ReadConversation after reopen: %v", err)
	}
	if !found || conversation.Title != "Durable Conversation" || conversation.Provider != "codex" {
		t.Fatalf("conversation after reopen = %+v found=%t, want durable conversation", conversation, found)
	}
	messages, err := reopened.ListMessages(ctx, "conv-durable", false)
	if err != nil {
		t.Fatalf("ListMessages after reopen: %v", err)
	}
	if len(messages) != 1 || messages[0].Content != "persist this message across restart" {
		t.Fatalf("messages after reopen = %+v, want committed user message", messages)
	}
}

func TestCommittedChainWriteSurvivesCloseReopen(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	backend, err := OpenBrainBackend(ctx, Config{DataDir: dataDir, DurableAck: true})
	if err != nil {
		t.Fatalf("OpenBrainBackend: %v", err)
	}
	createdAt := time.Date(2026, 5, 7, 11, 0, 0, 0, time.UTC)
	if err := backend.StartChain(ctx, StartChainArgs{
		ID:          "chain-durable",
		SourceTask:  "recover chain state",
		MaxSteps:    3,
		CreatedAtUS: uint64(createdAt.UnixMicro()),
	}); err != nil {
		t.Fatalf("StartChain: %v", err)
	}
	if err := backend.StartStep(ctx, StartStepArgs{
		ID:          "step-durable",
		ChainID:     "chain-durable",
		Sequence:    1,
		Role:        "coder",
		Task:        "write durable step",
		CreatedAtUS: uint64(createdAt.Add(time.Second).UnixMicro()),
	}); err != nil {
		t.Fatalf("StartStep: %v", err)
	}
	if err := backend.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := OpenBrainBackend(ctx, Config{DataDir: dataDir, DurableAck: true})
	if err != nil {
		t.Fatalf("reopen OpenBrainBackend: %v", err)
	}
	defer reopened.Close()
	chain, found, err := reopened.ReadChain(ctx, "chain-durable")
	if err != nil {
		t.Fatalf("ReadChain after reopen: %v", err)
	}
	if !found || chain.SourceTask != "recover chain state" {
		t.Fatalf("chain after reopen = %+v found=%t, want durable chain", chain, found)
	}
	steps, err := reopened.ListChainSteps(ctx, "chain-durable")
	if err != nil {
		t.Fatalf("ListChainSteps after reopen: %v", err)
	}
	if len(steps) != 1 || steps[0].ID != "step-durable" || steps[0].Role != "coder" {
		t.Fatalf("steps after reopen = %+v, want durable step", steps)
	}
}

func TestDurableAckWriteReturnsAfterTxIsDurable(t *testing.T) {
	ctx := context.Background()
	backend, err := OpenBrainBackend(ctx, Config{DataDir: t.TempDir(), DurableAck: true})
	if err != nil {
		t.Fatalf("OpenBrainBackend: %v", err)
	}
	defer backend.Close()
	before := backend.runtime.rt.Health().Durability.DurableTxID
	if err := backend.WriteDocument(ctx, "notes/durable-ack.md", "# Durable Ack\n\nAck after disk."); err != nil {
		t.Fatalf("WriteDocument: %v", err)
	}
	after := backend.runtime.rt.Health().Durability.DurableTxID
	if after <= before {
		t.Fatalf("DurableTxID after write = %d, before = %d; durable_ack should wait for committed tx", after, before)
	}
}
