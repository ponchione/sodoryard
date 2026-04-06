package agent

import (
	stdctx "context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ponchione/sirtopham/internal/conversation"
	"github.com/ponchione/sirtopham/internal/provider"
)

func TestBuildCleanupPlanSkipsCompletedIteration(t *testing.T) {
	plan := buildCleanupPlan(inflightTurn{
		ConversationID:           "conv-1",
		TurnNumber:               2,
		Iteration:                1,
		CompletedIterations:      1,
		AssistantResponseStarted: true,
	}, cleanupReasonCancel)
	if len(plan.Actions) != 0 {
		t.Fatalf("cleanup actions = %#v, want none", plan.Actions)
	}
}

func TestBuildCleanupPlanSkipsUnmaterializedIterationSetupCancellation(t *testing.T) {
	plan := buildCleanupPlan(inflightTurn{
		ConversationID:      "conv-1",
		TurnNumber:          2,
		Iteration:           1,
		CompletedIterations: 0,
	}, cleanupReasonCancel)
	if len(plan.Actions) != 0 {
		t.Fatalf("cleanup actions = %#v, want none for cancellation before any assistant/tool state existed", plan.Actions)
	}
}

func TestBuildCleanupPlanPersistsInterruptedAssistantMessage(t *testing.T) {
	plan := buildCleanupPlan(inflightTurn{
		ConversationID:           "conv-1",
		TurnNumber:               2,
		Iteration:                3,
		CompletedIterations:      2,
		AssistantResponseStarted: true,
		AssistantMessageContent:  `[{"type":"text","text":"partial"}]`,
	}, cleanupReasonInterrupt)
	if len(plan.Actions) != 1 || plan.Actions[0].Kind != cleanupActionPersistIteration {
		t.Fatalf("cleanup actions = %#v, want one persist_iteration", plan.Actions)
	}
	if len(plan.Actions[0].Messages) != 1 || plan.Actions[0].Messages[0].Role != "assistant" {
		t.Fatalf("persisted messages = %#v, want one assistant message", plan.Actions[0].Messages)
	}
	if !strings.Contains(plan.Actions[0].Messages[0].Content, "[interrupted_assistant]") {
		t.Fatalf("assistant tombstone content = %q, want interrupted assistant marker", plan.Actions[0].Messages[0].Content)
	}
}

func TestBuildCleanupPlanPersistsFailedAssistantMessageForStreamFailure(t *testing.T) {
	plan := buildCleanupPlan(inflightTurn{
		ConversationID:           "conv-1",
		TurnNumber:               2,
		Iteration:                3,
		CompletedIterations:      2,
		AssistantResponseStarted: true,
		AssistantMessageContent:  `[{"type":"text","text":"partial"}]`,
	}, cleanupReasonStreamFailure)
	if len(plan.Actions) != 1 || plan.Actions[0].Kind != cleanupActionPersistIteration {
		t.Fatalf("cleanup actions = %#v, want one persist_iteration", plan.Actions)
	}
	if !strings.Contains(plan.Actions[0].Messages[0].Content, "[failed_assistant]") {
		t.Fatalf("assistant tombstone content = %q, want failed assistant marker", plan.Actions[0].Messages[0].Content)
	}
	if !strings.Contains(plan.Actions[0].Messages[0].Content, "reason=stream_failure") {
		t.Fatalf("assistant tombstone content = %q, want stream_failure reason", plan.Actions[0].Messages[0].Content)
	}
}

func TestBuildCleanupPlanPersistsInterruptedAssistantMessageWhenFirstBlockIsThinking(t *testing.T) {
	raw, err := json.Marshal([]provider.ContentBlock{
		provider.NewThinkingBlock("reasoning"),
		provider.NewTextBlock("partial"),
	})
	if err != nil {
		t.Fatalf("marshal content blocks: %v", err)
	}

	plan := buildCleanupPlan(inflightTurn{
		ConversationID:           "conv-1",
		TurnNumber:               2,
		Iteration:                3,
		CompletedIterations:      2,
		AssistantResponseStarted: true,
		AssistantMessageContent:  string(raw),
	}, cleanupReasonInterrupt)
	if len(plan.Actions) != 1 || plan.Actions[0].Kind != cleanupActionPersistIteration {
		t.Fatalf("cleanup actions = %#v, want one persist_iteration", plan.Actions)
	}
	blocks, err := provider.ContentBlocksFromRaw(json.RawMessage(plan.Actions[0].Messages[0].Content))
	if err != nil {
		t.Fatalf("ContentBlocksFromRaw: %v", err)
	}
	if len(blocks) != 2 {
		t.Fatalf("persisted assistant blocks = %#v, want 2", blocks)
	}
	if blocks[0].Type != "thinking" || blocks[0].Thinking != "reasoning" {
		t.Fatalf("first block = %#v, want original thinking block preserved", blocks[0])
	}
	if blocks[1].Type != "text" || !strings.Contains(blocks[1].Text, "[interrupted_assistant]") || !strings.Contains(blocks[1].Text, "partial_text=partial") {
		t.Fatalf("second block = %#v, want interrupted tombstone text preserving partial text", blocks[1])
	}
}

func TestBuildCleanupPlanPersistsInterruptedAssistantMessageWhenOnlyThinkingBlockExists(t *testing.T) {
	raw, err := json.Marshal([]provider.ContentBlock{
		provider.NewThinkingBlock("reasoning"),
	})
	if err != nil {
		t.Fatalf("marshal content blocks: %v", err)
	}

	plan := buildCleanupPlan(inflightTurn{
		ConversationID:           "conv-1",
		TurnNumber:               2,
		Iteration:                3,
		CompletedIterations:      2,
		AssistantResponseStarted: true,
		AssistantMessageContent:  string(raw),
	}, cleanupReasonInterrupt)
	if len(plan.Actions) != 1 || plan.Actions[0].Kind != cleanupActionPersistIteration {
		t.Fatalf("cleanup actions = %#v, want one persist_iteration", plan.Actions)
	}
	blocks, err := provider.ContentBlocksFromRaw(json.RawMessage(plan.Actions[0].Messages[0].Content))
	if err != nil {
		t.Fatalf("ContentBlocksFromRaw: %v", err)
	}
	if len(blocks) != 2 {
		t.Fatalf("persisted assistant blocks = %#v, want original thinking plus appended tombstone text", blocks)
	}
	if blocks[0].Type != "thinking" || blocks[0].Thinking != "reasoning" {
		t.Fatalf("first block = %#v, want original thinking block preserved", blocks[0])
	}
	if blocks[1].Type != "text" || !strings.Contains(blocks[1].Text, "[interrupted_assistant]") || strings.Contains(blocks[1].Text, "partial_text=") {
		t.Fatalf("second block = %#v, want appended interrupted tombstone text without partial_text", blocks[1])
	}
}

func TestBuildCleanupPlanPersistsInterruptedToolResults(t *testing.T) {
	plan := buildCleanupPlan(inflightTurn{
		ConversationID:           "conv-1",
		TurnNumber:               2,
		Iteration:                3,
		CompletedIterations:      2,
		AssistantResponseStarted: true,
		AssistantMessageContent:  `[{"type":"tool_use","id":"tool-1","name":"shell","input":{}}]`,
		ToolCalls: []inflightToolCall{{
			ToolCallID: "tool-1",
			ToolName:   "shell",
			Started:    true,
		}},
	}, cleanupReasonInterrupt)
	if plan.Reason != cleanupReasonInterrupt {
		t.Fatalf("plan reason = %q, want %q", plan.Reason, cleanupReasonInterrupt)
	}
	if len(plan.Actions) != 1 {
		t.Fatalf("cleanup action count = %d, want 1", len(plan.Actions))
	}
	if plan.Actions[0].Kind != cleanupActionPersistIteration || plan.Actions[0].Iteration != 3 {
		t.Fatalf("cleanup action = %#v, want persist_iteration for iter 3", plan.Actions[0])
	}
	if len(plan.Actions[0].Messages) != 2 {
		t.Fatalf("persisted message count = %d, want 2", len(plan.Actions[0].Messages))
	}
	if plan.Actions[0].Messages[1].ToolUseID != "tool-1" {
		t.Fatalf("tool result = %#v, want tool_use_id tool-1", plan.Actions[0].Messages[1])
	}
}

func TestApplyCleanupPlanCancelsInflightIteration(t *testing.T) {
	conversations := &loopConversationManagerStub{}
	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    &loopContextAssemblerStub{},
		ConversationManager: conversations,
		ProviderRouter:      &providerRouterStub{},
		ToolExecutor:        &toolExecutorStub{},
		PromptBuilder:       NewPromptBuilder(nil),
	})

	turn := inflightTurn{ConversationID: "conv-2", TurnNumber: 4, Iteration: 2}
	plan := cleanupPlan{
		Reason: cleanupReasonCancel,
		Actions: []cleanupAction{{
			Kind:      cleanupActionCancelIteration,
			Iteration: 2,
		}},
	}
	if err := loop.applyCleanupPlan(stdctx.Background(), turn, plan); err != nil {
		t.Fatalf("applyCleanupPlan returned error: %v", err)
	}
	if len(conversations.cancelIterCalls) != 1 {
		t.Fatalf("CancelIteration calls = %d, want 1", len(conversations.cancelIterCalls))
	}
	if got := conversations.cancelIterCalls[0]; got.conversationID != "conv-2" || got.turnNumber != 4 || got.iteration != 2 {
		t.Fatalf("CancelIteration call = %+v, want conv-2/4/2", got)
	}
}

func TestApplyCleanupPlanPersistsInterruptedIteration(t *testing.T) {
	conversations := &loopConversationManagerStub{}
	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    &loopContextAssemblerStub{},
		ConversationManager: conversations,
		ProviderRouter:      &providerRouterStub{},
		ToolExecutor:        &toolExecutorStub{},
		PromptBuilder:       NewPromptBuilder(nil),
	})

	turn := inflightTurn{ConversationID: "conv-3", TurnNumber: 5, Iteration: 2}
	plan := cleanupPlan{
		Reason: cleanupReasonInterrupt,
		Actions: []cleanupAction{{
			Kind:      cleanupActionPersistIteration,
			Iteration: 2,
			Messages: []conversation.IterationMessage{{Role: "assistant", Content: `[{"type":"tool_use"}]`}, {Role: "tool", Content: "[interrupted_tool_result]", ToolUseID: "tool-1", ToolName: "shell"}},
		}},
	}
	if err := loop.applyCleanupPlan(stdctx.Background(), turn, plan); err != nil {
		t.Fatalf("applyCleanupPlan returned error: %v", err)
	}
	if len(conversations.persistIterCalls) != 1 {
		t.Fatalf("PersistIteration calls = %d, want 1", len(conversations.persistIterCalls))
	}
	if got := conversations.persistIterCalls[0]; got.conversationID != "conv-3" || got.turnNumber != 5 || got.iteration != 2 || len(got.messages) != 2 {
		t.Fatalf("PersistIteration call = %+v, want conv-3/5/2 with 2 messages", got)
	}
}

func TestCancellationReasonUsesInterruptForLoopCancel(t *testing.T) {
	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    &loopContextAssemblerStub{},
		ConversationManager: &loopConversationManagerStub{},
		ProviderRouter:      &providerRouterStub{},
		ToolExecutor:        &toolExecutorStub{},
		PromptBuilder:       NewPromptBuilder(nil),
	})
	loop.interruptRequested = true

	if got := loop.cancellationReason(stdctx.Canceled); got != cleanupReasonInterrupt {
		t.Fatalf("cancellationReason = %q, want %q", got, cleanupReasonInterrupt)
	}
}
