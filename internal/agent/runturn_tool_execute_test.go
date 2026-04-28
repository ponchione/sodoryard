package agent

import (
	stdctx "context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ponchione/sodoryard/internal/provider"
)

type mismatchedBatchToolExecutorStub struct{}

func (m *mismatchedBatchToolExecutorStub) Execute(_ stdctx.Context, call provider.ToolCall) (*provider.ToolResult, error) {
	return &provider.ToolResult{ToolUseID: call.ID, Content: "unexpected serial fallback"}, nil
}

func (m *mismatchedBatchToolExecutorStub) ExecuteBatch(_ stdctx.Context, calls []provider.ToolCall) ([]provider.ToolResult, error) {
	return []provider.ToolResult{{ToolUseID: calls[0].ID, Content: "only one result"}}, nil
}

func TestExecuteToolCallsBatchCountMismatchProducesErrorResults(t *testing.T) {
	loop := NewAgentLoop(AgentLoopDeps{ToolExecutor: &mismatchedBatchToolExecutorStub{}})
	loop.now = func() time.Time { return time.Unix(1700000600, 0).UTC() }
	turnExec := &turnExecution{
		req: RunTurnRequest{
			ConversationID:    "conv-batch-mismatch",
			TurnNumber:        7,
			ModelContextLimit: 200000,
		},
		turnStart: time.Unix(1700000000, 0).UTC(),
		turnCtx:   &TurnStartResult{},
	}
	calls := []provider.ToolCall{{ID: "tool-1", Name: "read_file"}, {ID: "tool-2", Name: "search_text"}}

	executed, finalResult, err := loop.executeToolCalls(stdctx.Background(), turnExec, 3, &streamResult{}, calls)
	if err != nil {
		t.Fatalf("executeToolCalls error: %v", err)
	}
	if finalResult != nil {
		t.Fatalf("finalResult = %#v, want nil", finalResult)
	}
	if len(executed) != 2 {
		t.Fatalf("len(executed) = %d, want 2", len(executed))
	}
	for i, record := range executed {
		if !record.Result.IsError {
			t.Fatalf("executed[%d].Result.IsError = false, want true", i)
		}
		if !strings.Contains(record.Result.Content, "returned 1 batch results for 2 calls") {
			t.Fatalf("executed[%d].Result.Content = %q, want batch cardinality error", i, record.Result.Content)
		}
	}
}

func TestFinalizeExecutedToolResultsRecordsMessagesAndEventsInOrder(t *testing.T) {
	sink := NewChannelSink(16)
	loop := NewAgentLoop(AgentLoopDeps{EventSink: sink})
	loop.now = func() time.Time { return time.Unix(1700000700, 0).UTC() }
	inflight := &inflightTurn{
		ConversationID: "conv-finalize",
		TurnNumber:     4,
		Iteration:      2,
		ToolCalls: []inflightToolCall{
			{ToolCallID: "tool-1", ToolName: "read_file", Started: true},
			{ToolCallID: "tool-2", ToolName: "search_text", Started: true},
		},
	}
	executed := []toolExecutionRecord{
		{
			Call:     provider.ToolCall{ID: "tool-1", Name: "read_file"},
			Result:   provider.ToolResult{ToolUseID: "tool-1", Content: "file contents", Details: json.RawMessage(`{"version":1,"kind":"file_read"}`)},
			Duration: 20 * time.Millisecond,
		},
		{
			Call:     provider.ToolCall{ID: "tool-2", Name: "search_text"},
			Result:   provider.ToolResult{ToolUseID: "tool-2", Content: "search failed", IsError: true},
			Duration: 25 * time.Millisecond,
		},
	}

	toolResults := loop.finalizeExecutedToolResults(inflight, []int{0, 1}, executed)
	if len(toolResults) != 2 {
		t.Fatalf("len(toolResults) = %d, want 2", len(toolResults))
	}
	if string(toolResults[0].Details) != `{"version":1,"kind":"file_read"}` {
		t.Fatalf("toolResults[0].Details = %s, want copied details", toolResults[0].Details)
	}
	if len(inflight.ToolMessages) != 2 {
		t.Fatalf("len(inflight.ToolMessages) = %d, want 2", len(inflight.ToolMessages))
	}
	if inflight.ToolMessages[0].ToolUseID != "tool-1" || inflight.ToolMessages[1].ToolUseID != "tool-2" {
		t.Fatalf("tool message order = %q/%q, want tool-1/tool-2", inflight.ToolMessages[0].ToolUseID, inflight.ToolMessages[1].ToolUseID)
	}
	if !inflight.ToolCalls[0].Completed || !inflight.ToolCalls[1].Completed {
		t.Fatalf("inflight completion flags = %+v, want all completed", inflight.ToolCalls)
	}
	if !inflight.ToolCalls[0].ResultStored || !inflight.ToolCalls[1].ResultStored {
		t.Fatalf("inflight result stored flags = %+v, want all stored", inflight.ToolCalls)
	}

	events := drainEvents(sink, 16)
	if len(events) != 5 {
		t.Fatalf("len(events) = %d, want 5", len(events))
	}
	if out, ok := events[0].(ToolCallOutputEvent); !ok || out.ToolCallID != "tool-1" {
		t.Fatalf("events[0] = %#v, want ToolCallOutputEvent for tool-1", events[0])
	}
	if end, ok := events[1].(ToolCallEndEvent); !ok || end.ToolCallID != "tool-1" || !end.Success || string(end.Details) != `{"version":1,"kind":"file_read"}` {
		t.Fatalf("events[1] = %#v, want successful ToolCallEndEvent for tool-1", events[1])
	}
	if errEvt, ok := events[2].(ErrorEvent); !ok || errEvt.ErrorCode != ErrorCodeToolExecution {
		t.Fatalf("events[2] = %#v, want tool execution ErrorEvent", events[2])
	}
	if out, ok := events[3].(ToolCallOutputEvent); !ok || out.ToolCallID != "tool-2" {
		t.Fatalf("events[3] = %#v, want ToolCallOutputEvent for tool-2", events[3])
	}
	if end, ok := events[4].(ToolCallEndEvent); !ok || end.ToolCallID != "tool-2" || end.Success {
		t.Fatalf("events[4] = %#v, want failed ToolCallEndEvent for tool-2", events[4])
	}
}

func TestHandleChainCompleteReturnsFinalTurnResult(t *testing.T) {
	loop := NewAgentLoop(AgentLoopDeps{})
	loop.now = func() time.Time { return time.Unix(1700000800, 0).UTC() }
	turnExec := &turnExecution{
		req:        RunTurnRequest{ConversationID: "conv-chain-complete", TurnNumber: 9},
		turnStart:  time.Unix(1700000000, 0).UTC(),
		turnCtx:    &TurnStartResult{CompressionNeeded: true},
		totalUsage: provider.Usage{InputTokens: 11, OutputTokens: 5},
	}
	result := &streamResult{TextContent: "done"}

	finalResult := loop.handleChainComplete(turnExec, result, 4)
	if finalResult == nil {
		t.Fatal("finalResult = nil, want value")
	}
	if finalResult.FinalText != "done" {
		t.Fatalf("FinalText = %q, want done", finalResult.FinalText)
	}
	if finalResult.IterationCount != 4 {
		t.Fatalf("IterationCount = %d, want 4", finalResult.IterationCount)
	}
	if finalResult.TotalUsage.InputTokens != 11 || finalResult.TotalUsage.OutputTokens != 5 {
		t.Fatalf("TotalUsage = %+v, want 11/5", finalResult.TotalUsage)
	}
	if !finalResult.CompressionNeeded {
		t.Fatal("CompressionNeeded = false, want true from turn context")
	}
}
