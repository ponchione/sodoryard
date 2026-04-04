package agent

import (
	stdctx "context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	contextpkg "github.com/ponchione/sirtopham/internal/context"
	"github.com/ponchione/sirtopham/internal/db"
	"github.com/ponchione/sirtopham/internal/provider"
)

// --- Event ordering ---

// TestRunTurnEventOrderingSingleIteration verifies the complete event sequence
// for a text-only turn: AssemblingContext → WaitingForLLM → WaitingForLLM →
// TokenEvent(s) → TurnComplete → Idle.
func TestRunTurnEventOrderingSingleIteration(t *testing.T) {
	sink := NewChannelSink(64)
	assembler := &loopContextAssemblerStub{
		pkg: &contextpkg.FullContextPackage{Content: "context", Frozen: true},
	}
	conversations := &loopConversationManagerStub{
		history: []db.Message{},
		seen:    loopSeenFilesStub{},
	}

	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    assembler,
		ConversationManager: conversations,
		ProviderRouter: &providerRouterStub{
			streamEvents: [][]provider.StreamEvent{
				textOnlyStream("answer", 100, 20),
			},
		},
		ToolExecutor:  &toolExecutorStub{},
		PromptBuilder: NewPromptBuilder(nil),
		EventSink:     sink,
	})
	loop.now = func() time.Time { return time.Unix(1700000600, 0).UTC() }

	_, err := loop.RunTurn(stdctx.Background(), RunTurnRequest{
		ConversationID:    "conv-order-1",
		TurnNumber:        1,
		Message:           "hello",
		ModelContextLimit: 200000,
	})
	if err != nil {
		t.Fatalf("RunTurn error: %v", err)
	}

	events := drainEvents(sink, 64)
	types := eventTypes(events)

	// Expected order for a text-only turn:
	// StatusEvent(assembling_context)
	// StatusEvent(waiting_for_llm)   -- from PrepareTurnContext
	// StatusEvent(waiting_for_llm)   -- from iteration loop before stream
	// TokenEvent                     -- "answer"
	// TurnCompleteEvent
	// StatusEvent(idle)

	assertEventBefore(t, types, "status:assembling_context", "status:waiting_for_llm")
	assertEventBefore(t, types, "status:waiting_for_llm", "token")
	assertEventBefore(t, types, "token", "turn_complete")
	assertEventBefore(t, types, "turn_complete", "status:idle")

	// Verify no ExecutingTools status (text-only turn has no tool dispatch).
	for _, et := range types {
		if et == "status:executing_tools" {
			t.Fatal("status:executing_tools should not appear in text-only turn")
		}
	}

	// Verify the last event is StateIdle.
	if last := types[len(types)-1]; last != "status:idle" {
		t.Fatalf("last event = %q, want status:idle", last)
	}
}

// TestRunTurnEventOrderingWithToolUse verifies event ordering for a turn with
// one tool-use iteration followed by a text-only iteration.
func TestRunTurnEventOrderingWithToolUse(t *testing.T) {
	sink := NewChannelSink(128)
	assembler := &loopContextAssemblerStub{
		pkg: &contextpkg.FullContextPackage{Content: "context", Frozen: true},
	}
	conversations := &loopConversationManagerStub{
		history: []db.Message{},
		seen:    loopSeenFilesStub{},
	}

	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    assembler,
		ConversationManager: conversations,
		ProviderRouter: &providerRouterStub{
			streamEvents: [][]provider.StreamEvent{
				// Iteration 1: tool use.
				toolUseStream("tc-1", "file_read", `{"path":"main.go"}`, "reading", 100, 20),
				// Iteration 2: text only.
				textOnlyStream("done", 120, 30),
			},
		},
		ToolExecutor:  &toolExecutorStub{},
		PromptBuilder: NewPromptBuilder(nil),
		EventSink:     sink,
	})
	loop.now = func() time.Time { return time.Unix(1700000600, 0).UTC() }

	_, err := loop.RunTurn(stdctx.Background(), RunTurnRequest{
		ConversationID:    "conv-order-2",
		TurnNumber:        1,
		Message:           "read main.go",
		ModelContextLimit: 200000,
	})
	if err != nil {
		t.Fatalf("RunTurn error: %v", err)
	}

	events := drainEvents(sink, 128)
	types := eventTypes(events)

	// Key ordering constraints:
	// assembling_context comes first
	// waiting_for_llm comes before tool execution
	// executing_tools comes before tool_call_start
	// tool_call_start comes before tool_call_output
	// tool_call_output comes before tool_call_end
	// turn_complete comes before idle

	assertEventBefore(t, types, "status:assembling_context", "status:waiting_for_llm")
	assertEventBefore(t, types, "status:waiting_for_llm", "status:executing_tools")
	assertEventBefore(t, types, "status:executing_tools", "tool_call_start")
	assertEventBefore(t, types, "tool_call_start", "tool_call_output")
	assertEventBefore(t, types, "tool_call_output", "tool_call_end")
	assertEventBefore(t, types, "turn_complete", "status:idle")

	// Verify ToolCallOutputEvent carries the tool result.
	for _, e := range events {
		if out, ok := e.(ToolCallOutputEvent); ok {
			if out.ToolCallID != "tc-1" {
				t.Fatalf("ToolCallOutputEvent.ToolCallID = %q, want tc-1", out.ToolCallID)
			}
			if out.Output == "" {
				t.Fatal("ToolCallOutputEvent.Output is empty")
			}
			return
		}
	}
	t.Fatal("ToolCallOutputEvent not found in events")
}

// TestRunTurnEventOrderingCancellation verifies that cancellation emits the
// right events: TurnCancelledEvent followed by StatusEvent(Idle).
func TestRunTurnEventOrderingCancellation(t *testing.T) {
	sink := NewChannelSink(64)
	assembler := &loopContextAssemblerStub{
		pkg: &contextpkg.FullContextPackage{Content: "context", Frozen: true},
	}

	// Make the stream block so we can cancel mid-stream.
	blockCh := make(chan struct{})
	conversations := &loopConversationManagerStub{
		history: []db.Message{},
		seen:    loopSeenFilesStub{},
	}
	slowRouter := &blockingRouterStub{blockCh: blockCh}

	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    assembler,
		ConversationManager: conversations,
		ProviderRouter:      slowRouter,
		ToolExecutor:        &toolExecutorStub{},
		PromptBuilder:       NewPromptBuilder(nil),
		EventSink:           sink,
	})
	loop.now = func() time.Time { return time.Unix(1700000600, 0).UTC() }

	ctx, cancel := stdctx.WithCancel(stdctx.Background())

	doneCh := make(chan error, 1)
	go func() {
		_, err := loop.RunTurn(ctx, RunTurnRequest{
			ConversationID:    "conv-order-cancel",
			TurnNumber:        1,
			Message:           "hello",
			ModelContextLimit: 200000,
		})
		doneCh <- err
	}()

	// Wait a moment then cancel.
	time.Sleep(20 * time.Millisecond)
	cancel()
	close(blockCh)

	err := <-doneCh
	if err == nil || !strings.Contains(err.Error(), "turn cancelled") {
		t.Fatalf("expected ErrTurnCancelled, got: %v", err)
	}

	events := drainEvents(sink, 64)
	types := eventTypes(events)

	// Should have TurnCancelledEvent followed by StatusEvent(Idle).
	assertEventBefore(t, types, "turn_cancelled", "status:idle")

	// Last event should be idle.
	if last := types[len(types)-1]; last != "status:idle" {
		t.Fatalf("last event = %q, want status:idle", last)
	}
}

// blockingRouterStub blocks until blockCh is closed, then returns context error.
type blockingRouterStub struct {
	blockCh chan struct{}
}

func (s *blockingRouterStub) Stream(ctx stdctx.Context, req *provider.Request) (<-chan provider.StreamEvent, error) {
	ch := make(chan provider.StreamEvent)
	go func() {
		defer close(ch)
		select {
		case <-ctx.Done():
			return
		case <-s.blockCh:
			return
		}
	}()
	return ch, nil
}

// TestRunTurnContextDebugOmittedWhenDisabled verifies that ContextDebugEvent is
// NOT emitted when EmitContextDebug is false.
func TestRunTurnContextDebugOmittedWhenDisabled(t *testing.T) {
	sink := NewChannelSink(64)
	report := &contextpkg.ContextAssemblyReport{TurnNumber: 1}
	assembler := &loopContextAssemblerStub{
		pkg: &contextpkg.FullContextPackage{Content: "context", Report: report, Frozen: true},
	}
	conversations := &loopConversationManagerStub{
		history: []db.Message{},
		seen:    loopSeenFilesStub{},
	}

	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    assembler,
		ConversationManager: conversations,
		ProviderRouter: &providerRouterStub{
			streamEvents: [][]provider.StreamEvent{
				textOnlyStream("done", 10, 5),
			},
		},
		ToolExecutor:  &toolExecutorStub{},
		PromptBuilder: NewPromptBuilder(nil),
		EventSink:     sink,
		// EmitContextDebug defaults to false.
	})
	loop.now = func() time.Time { return time.Unix(1700000600, 0).UTC() }

	_, err := loop.RunTurn(stdctx.Background(), RunTurnRequest{
		ConversationID:    "conv-no-debug",
		TurnNumber:        1,
		Message:           "hello",
		ModelContextLimit: 200000,
	})
	if err != nil {
		t.Fatalf("RunTurn error: %v", err)
	}

	events := drainEvents(sink, 64)
	for _, e := range events {
		if _, ok := e.(ContextDebugEvent); ok {
			t.Fatal("ContextDebugEvent should NOT be emitted when EmitContextDebug is false")
		}
	}
}

// TestRunTurnContextDebugEmittedWhenEnabled verifies that ContextDebugEvent IS
// emitted when EmitContextDebug is true.
func TestRunTurnContextDebugEmittedWhenEnabled(t *testing.T) {
	sink := NewChannelSink(64)
	report := &contextpkg.ContextAssemblyReport{TurnNumber: 1}
	assembler := &loopContextAssemblerStub{
		pkg: &contextpkg.FullContextPackage{Content: "context", Report: report, Frozen: true},
	}
	conversations := &loopConversationManagerStub{
		history: []db.Message{},
		seen:    loopSeenFilesStub{},
	}

	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    assembler,
		ConversationManager: conversations,
		ProviderRouter: &providerRouterStub{
			streamEvents: [][]provider.StreamEvent{
				textOnlyStream("done", 10, 5),
			},
		},
		ToolExecutor:  &toolExecutorStub{},
		PromptBuilder: NewPromptBuilder(nil),
		EventSink:     sink,
		Config:        AgentLoopConfig{EmitContextDebug: true},
	})
	loop.now = func() time.Time { return time.Unix(1700000600, 0).UTC() }

	_, err := loop.RunTurn(stdctx.Background(), RunTurnRequest{
		ConversationID:    "conv-debug-on",
		TurnNumber:        1,
		Message:           "hello",
		ModelContextLimit: 200000,
	})
	if err != nil {
		t.Fatalf("RunTurn error: %v", err)
	}

	events := drainEvents(sink, 64)
	found := false
	for _, e := range events {
		if cde, ok := e.(ContextDebugEvent); ok {
			found = true
			if cde.Report != report {
				t.Fatal("ContextDebugEvent.Report does not match expected report")
			}
		}
	}
	if !found {
		t.Fatal("ContextDebugEvent not emitted when EmitContextDebug is true")
	}
}

// TestRunTurnToolCallOutputEventEmitted verifies that ToolCallOutputEvent is
// emitted for each successfully executed tool call.
func TestRunTurnToolCallOutputEventEmitted(t *testing.T) {
	sink := NewChannelSink(128)
	assembler := &loopContextAssemblerStub{
		pkg: &contextpkg.FullContextPackage{Content: "context", Frozen: true},
	}
	conversations := &loopConversationManagerStub{
		history: []db.Message{},
		seen:    loopSeenFilesStub{},
	}

	toolExec := &toolExecutorStub{
		results: map[string]*provider.ToolResult{
			"tc-1": {ToolUseID: "tc-1", Content: "file contents here"},
			"tc-2": {ToolUseID: "tc-2", Content: "search results"},
		},
	}

	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    assembler,
		ConversationManager: conversations,
		ProviderRouter: &providerRouterStub{
			streamEvents: [][]provider.StreamEvent{
				// Two tool calls in one iteration.
				{
					provider.ToolCallStart{ID: "tc-1", Name: "file_read"},
					provider.ToolCallEnd{ID: "tc-1", Input: json.RawMessage(`{"path":"main.go"}`)},
					provider.ToolCallStart{ID: "tc-2", Name: "search_semantic"},
					provider.ToolCallEnd{ID: "tc-2", Input: json.RawMessage(`{"query":"auth"}`)},
					provider.StreamDone{
						StopReason: provider.StopReasonToolUse,
						Usage:      provider.Usage{InputTokens: 50, OutputTokens: 20},
					},
				},
				// Final text response.
				textOnlyStream("done", 80, 10),
			},
		},
		ToolExecutor:  toolExec,
		PromptBuilder: NewPromptBuilder(nil),
		EventSink:     sink,
	})
	loop.now = func() time.Time { return time.Unix(1700000600, 0).UTC() }

	_, err := loop.RunTurn(stdctx.Background(), RunTurnRequest{
		ConversationID:    "conv-output-event",
		TurnNumber:        1,
		Message:           "do it",
		ModelContextLimit: 200000,
	})
	if err != nil {
		t.Fatalf("RunTurn error: %v", err)
	}

	events := drainEvents(sink, 128)

	// Collect all ToolCallOutputEvents.
	var outputs []ToolCallOutputEvent
	for _, e := range events {
		if out, ok := e.(ToolCallOutputEvent); ok {
			outputs = append(outputs, out)
		}
	}

	if len(outputs) != 2 {
		t.Fatalf("expected 2 ToolCallOutputEvents, got %d", len(outputs))
	}

	// Verify tc-1 output.
	if outputs[0].ToolCallID != "tc-1" || outputs[0].Output != "file contents here" {
		t.Fatalf("first ToolCallOutputEvent = %+v, want tc-1 with 'file contents here'", outputs[0])
	}
	// Verify tc-2 output.
	if outputs[1].ToolCallID != "tc-2" || outputs[1].Output != "search results" {
		t.Fatalf("second ToolCallOutputEvent = %+v, want tc-2 with 'search results'", outputs[1])
	}
}

// TestRunTurnToolCallOutputEventOnError verifies that ToolCallOutputEvent is
// emitted even when a tool call fails (with the enriched error message).
func TestRunTurnToolCallOutputEventOnError(t *testing.T) {
	sink := NewChannelSink(128)
	assembler := &loopContextAssemblerStub{
		pkg: &contextpkg.FullContextPackage{Content: "context", Frozen: true},
	}
	conversations := &loopConversationManagerStub{
		history: []db.Message{},
		seen:    loopSeenFilesStub{},
	}

	toolExec := &toolExecutorStub{
		err: fmt.Errorf("file not found: main.go"),
	}

	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    assembler,
		ConversationManager: conversations,
		ProviderRouter: &providerRouterStub{
			streamEvents: [][]provider.StreamEvent{
				toolUseStream("tc-1", "file_read", `{"path":"main.go"}`, "reading", 50, 20),
				textOnlyStream("couldn't find it", 80, 10),
			},
		},
		ToolExecutor:  toolExec,
		PromptBuilder: NewPromptBuilder(nil),
		EventSink:     sink,
	})
	loop.now = func() time.Time { return time.Unix(1700000600, 0).UTC() }

	_, err := loop.RunTurn(stdctx.Background(), RunTurnRequest{
		ConversationID:    "conv-output-error",
		TurnNumber:        1,
		Message:           "read main.go",
		ModelContextLimit: 200000,
	})
	if err != nil {
		t.Fatalf("RunTurn error: %v", err)
	}

	events := drainEvents(sink, 128)

	// Find ToolCallOutputEvent — should contain enriched error.
	found := false
	for _, e := range events {
		if out, ok := e.(ToolCallOutputEvent); ok {
			found = true
			if out.ToolCallID != "tc-1" {
				t.Fatalf("ToolCallOutputEvent.ToolCallID = %q, want tc-1", out.ToolCallID)
			}
			if !strings.Contains(out.Output, "file not found") {
				t.Fatalf("ToolCallOutputEvent.Output = %q, want to contain 'file not found'", out.Output)
			}
		}
	}
	if !found {
		t.Fatal("ToolCallOutputEvent not found for failed tool call")
	}
}

// TestRunTurnNoStateConflicts verifies that state transitions follow legal
// ordering: no Idle→ExecutingTools, no WaitingForLLM after Idle, etc.
func TestRunTurnNoStateConflicts(t *testing.T) {
	sink := NewChannelSink(128)
	assembler := &loopContextAssemblerStub{
		pkg: &contextpkg.FullContextPackage{Content: "context", Frozen: true},
	}
	conversations := &loopConversationManagerStub{
		history: []db.Message{},
		seen:    loopSeenFilesStub{},
	}

	// Multi-iteration: tool use → text.
	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    assembler,
		ConversationManager: conversations,
		ProviderRouter: &providerRouterStub{
			streamEvents: [][]provider.StreamEvent{
				toolUseStream("tc-1", "shell", `{"command":"ls"}`, "", 50, 20),
				textOnlyStream("done", 80, 10),
			},
		},
		ToolExecutor:  &toolExecutorStub{},
		PromptBuilder: NewPromptBuilder(nil),
		EventSink:     sink,
	})
	loop.now = func() time.Time { return time.Unix(1700000600, 0).UTC() }

	_, err := loop.RunTurn(stdctx.Background(), RunTurnRequest{
		ConversationID:    "conv-state-check",
		TurnNumber:        1,
		Message:           "do",
		ModelContextLimit: 200000,
	})
	if err != nil {
		t.Fatalf("RunTurn error: %v", err)
	}

	events := drainEvents(sink, 128)

	// Extract status transitions.
	var states []AgentState
	for _, e := range events {
		if se, ok := e.(StatusEvent); ok {
			states = append(states, se.State)
		}
	}

	// Verify: once idle appears, no more non-idle states follow.
	seenIdle := false
	for _, s := range states {
		if seenIdle && s != StateIdle {
			t.Fatalf("state %q appeared after Idle — illegal transition", s)
		}
		if s == StateIdle {
			seenIdle = true
		}
	}

	if !seenIdle {
		t.Fatal("StateIdle never emitted")
	}

	// Verify first state is AssemblingContext.
	if states[0] != StateAssemblingContext {
		t.Fatalf("first state = %q, want assembling_context", states[0])
	}
}

// TestRunTurnMultiIterationEventSequence verifies the complete event flow for
// a 3-iteration turn (tool → tool → text).
func TestRunTurnMultiIterationEventSequence(t *testing.T) {
	sink := NewChannelSink(256)
	assembler := &loopContextAssemblerStub{
		pkg: &contextpkg.FullContextPackage{Content: "context", Frozen: true},
	}
	conversations := &loopConversationManagerStub{
		history: []db.Message{},
		seen:    loopSeenFilesStub{},
	}

	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    assembler,
		ConversationManager: conversations,
		ProviderRouter: &providerRouterStub{
			streamEvents: [][]provider.StreamEvent{
				toolUseStream("tc-1", "file_read", `{"path":"a.go"}`, "", 50, 20),
				toolUseStream("tc-2", "file_read", `{"path":"b.go"}`, "", 60, 20),
				textOnlyStream("finished", 80, 15),
			},
		},
		ToolExecutor:  &toolExecutorStub{},
		PromptBuilder: NewPromptBuilder(nil),
		EventSink:     sink,
	})
	loop.now = func() time.Time { return time.Unix(1700000600, 0).UTC() }

	result, err := loop.RunTurn(stdctx.Background(), RunTurnRequest{
		ConversationID:    "conv-multi-iter",
		TurnNumber:        1,
		Message:           "analyze",
		ModelContextLimit: 200000,
	})
	if err != nil {
		t.Fatalf("RunTurn error: %v", err)
	}
	if result.IterationCount != 3 {
		t.Fatalf("IterationCount = %d, want 3", result.IterationCount)
	}

	events := drainEvents(sink, 256)
	types := eventTypes(events)

	// Count key event types.
	waitingCount := 0
	executingCount := 0
	toolStartCount := 0
	toolOutputCount := 0
	toolEndCount := 0
	for _, et := range types {
		switch et {
		case "status:waiting_for_llm":
			waitingCount++
		case "status:executing_tools":
			executingCount++
		case "tool_call_start":
			toolStartCount++
		case "tool_call_output":
			toolOutputCount++
		case "tool_call_end":
			toolEndCount++
		}
	}

	// Should have at least 3 WaitingForLLM (one from PrepareTurnContext + 3 iterations).
	if waitingCount < 3 {
		t.Fatalf("WaitingForLLM events = %d, want at least 3", waitingCount)
	}
	// Two tool-use iterations.
	if executingCount != 2 {
		t.Fatalf("ExecutingTools events = %d, want 2", executingCount)
	}
	// Two tool calls (one per tool-use iteration).
	if toolStartCount != 2 {
		t.Fatalf("ToolCallStart events = %d, want 2", toolStartCount)
	}
	// Two ToolCallOutputEvents (one per dispatched tool).
	if toolOutputCount != 2 {
		t.Fatalf("ToolCallOutput events = %d, want 2", toolOutputCount)
	}
	// Two ToolCallEndEvents from dispatch (stream may emit more from ToolCallStart).
	if toolEndCount < 2 {
		t.Fatalf("ToolCallEnd events = %d, want at least 2", toolEndCount)
	}
}

// --- Helpers ---

// eventTypes returns a typed label for each event for ordering assertions.
func eventTypes(events []Event) []string {
	types := make([]string, 0, len(events))
	for _, e := range events {
		switch ev := e.(type) {
		case StatusEvent:
			types = append(types, "status:"+string(ev.State))
		case TokenEvent:
			types = append(types, "token")
		case ThinkingStartEvent:
			types = append(types, "thinking_start")
		case ThinkingDeltaEvent:
			types = append(types, "thinking_delta")
		case ThinkingEndEvent:
			types = append(types, "thinking_end")
		case ToolCallStartEvent:
			types = append(types, "tool_call_start")
		case ToolCallOutputEvent:
			types = append(types, "tool_call_output")
		case ToolCallEndEvent:
			types = append(types, "tool_call_end")
		case TurnCompleteEvent:
			types = append(types, "turn_complete")
		case TurnCancelledEvent:
			types = append(types, "turn_cancelled")
		case ErrorEvent:
			types = append(types, "error:"+ev.ErrorCode)
		case ContextDebugEvent:
			types = append(types, "context_debug")
		default:
			types = append(types, "unknown")
		}
	}
	return types
}

// assertEventBefore asserts that at least one instance of eventA appears before
// at least one instance of eventB in the types slice.
func assertEventBefore(t *testing.T, types []string, eventA, eventB string) {
	t.Helper()
	indexA := -1
	indexB := -1
	for i, et := range types {
		if et == eventA && indexA < 0 {
			indexA = i
		}
		if et == eventB && indexB < 0 {
			indexB = i
		}
	}
	if indexA < 0 {
		t.Fatalf("event %q not found in events: %v", eventA, types)
	}
	if indexB < 0 {
		t.Fatalf("event %q not found in events: %v", eventB, types)
	}
	if indexA >= indexB {
		t.Fatalf("event %q (index %d) should appear before %q (index %d) in events: %v",
			eventA, indexA, eventB, indexB, types)
	}
}
