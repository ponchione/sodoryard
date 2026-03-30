package agent

import (
	stdctx "context"
	"encoding/json"
	"testing"

	"github.com/ponchione/sirtopham/internal/provider"
)

func makeStreamCh(events ...provider.StreamEvent) <-chan provider.StreamEvent {
	ch := make(chan provider.StreamEvent, len(events))
	for _, e := range events {
		ch <- e
	}
	close(ch)
	return ch
}

func noopEmit(Event) {}

func testNow() string { return "2024-01-01T00:00:00Z" }

func TestConsumeStreamTextOnly(t *testing.T) {
	ch := makeStreamCh(
		provider.TokenDelta{Text: "Hello"},
		provider.TokenDelta{Text: " world"},
		provider.StreamDone{
			StopReason: provider.StopReasonEndTurn,
			Usage:      provider.Usage{InputTokens: 10, OutputTokens: 5},
		},
	)

	result, err := consumeStream(stdctx.Background(), ch, noopEmit, testNow)
	if err != nil {
		t.Fatalf("consumeStream error: %v", err)
	}

	if result.TextContent != "Hello world" {
		t.Fatalf("TextContent = %q, want %q", result.TextContent, "Hello world")
	}
	if result.HasToolUse() {
		t.Fatal("HasToolUse = true, want false")
	}
	if result.StopReason != provider.StopReasonEndTurn {
		t.Fatalf("StopReason = %q, want %q", result.StopReason, provider.StopReasonEndTurn)
	}
	if result.Usage.InputTokens != 10 || result.Usage.OutputTokens != 5 {
		t.Fatalf("Usage = %+v, want 10/5", result.Usage)
	}
	if len(result.ContentBlocks) != 1 || result.ContentBlocks[0].Type != "text" {
		t.Fatalf("ContentBlocks = %+v, want 1 text block", result.ContentBlocks)
	}
}

func TestConsumeStreamWithThinking(t *testing.T) {
	ch := makeStreamCh(
		provider.ThinkingDelta{Thinking: "Let me think"},
		provider.ThinkingDelta{Thinking: " about this"},
		provider.TokenDelta{Text: "Answer"},
		provider.StreamDone{
			StopReason: provider.StopReasonEndTurn,
			Usage:      provider.Usage{InputTokens: 20, OutputTokens: 10},
		},
	)

	var events []string
	emit := func(e Event) { events = append(events, e.EventType()) }

	result, err := consumeStream(stdctx.Background(), ch, emit, testNow)
	if err != nil {
		t.Fatalf("consumeStream error: %v", err)
	}

	if result.ThinkingContent != "Let me think about this" {
		t.Fatalf("ThinkingContent = %q, want %q", result.ThinkingContent, "Let me think about this")
	}
	if result.TextContent != "Answer" {
		t.Fatalf("TextContent = %q, want %q", result.TextContent, "Answer")
	}
	// Should have: thinking_start, thinking_delta, thinking_delta, token, thinking_end
	if len(events) < 4 {
		t.Fatalf("events = %v, want at least 4 events", events)
	}
	// First event should be thinking_start.
	if events[0] != "thinking_start" {
		t.Fatalf("first event = %q, want thinking_start", events[0])
	}
	// ContentBlocks should be: thinking, text.
	if len(result.ContentBlocks) != 2 {
		t.Fatalf("ContentBlocks count = %d, want 2", len(result.ContentBlocks))
	}
	if result.ContentBlocks[0].Type != "thinking" {
		t.Fatalf("ContentBlocks[0].Type = %q, want thinking", result.ContentBlocks[0].Type)
	}
	if result.ContentBlocks[1].Type != "text" {
		t.Fatalf("ContentBlocks[1].Type = %q, want text", result.ContentBlocks[1].Type)
	}
}

func TestConsumeStreamWithToolUse(t *testing.T) {
	input := json.RawMessage(`{"path":"main.go"}`)
	ch := makeStreamCh(
		provider.TokenDelta{Text: "Let me read that file."},
		provider.ToolCallStart{ID: "tool_1", Name: "read_file"},
		provider.ToolCallDelta{ID: "tool_1", Delta: `{"path":`},
		provider.ToolCallDelta{ID: "tool_1", Delta: `"main.go"}`},
		provider.ToolCallEnd{ID: "tool_1", Input: input},
		provider.StreamDone{
			StopReason: provider.StopReasonToolUse,
			Usage:      provider.Usage{InputTokens: 30, OutputTokens: 15},
		},
	)

	result, err := consumeStream(stdctx.Background(), ch, noopEmit, testNow)
	if err != nil {
		t.Fatalf("consumeStream error: %v", err)
	}

	if !result.HasToolUse() {
		t.Fatal("HasToolUse = false, want true")
	}
	if len(result.ToolCalls) != 1 {
		t.Fatalf("ToolCalls count = %d, want 1", len(result.ToolCalls))
	}
	tc := result.ToolCalls[0]
	if tc.ID != "tool_1" || tc.Name != "read_file" {
		t.Fatalf("ToolCall = %+v, want tool_1/read_file", tc)
	}
	if result.StopReason != provider.StopReasonToolUse {
		t.Fatalf("StopReason = %q, want %q", result.StopReason, provider.StopReasonToolUse)
	}
	if result.TextContent != "Let me read that file." {
		t.Fatalf("TextContent = %q, want %q", result.TextContent, "Let me read that file.")
	}

	// ContentBlocks: text, tool_use.
	if len(result.ContentBlocks) != 2 {
		t.Fatalf("ContentBlocks count = %d, want 2", len(result.ContentBlocks))
	}
	if result.ContentBlocks[0].Type != "text" {
		t.Fatalf("ContentBlocks[0].Type = %q, want text", result.ContentBlocks[0].Type)
	}
	if result.ContentBlocks[1].Type != "tool_use" {
		t.Fatalf("ContentBlocks[1].Type = %q, want tool_use", result.ContentBlocks[1].Type)
	}
}

func TestConsumeStreamMultipleToolCalls(t *testing.T) {
	input1 := json.RawMessage(`{"path":"a.go"}`)
	input2 := json.RawMessage(`{"path":"b.go"}`)
	ch := makeStreamCh(
		provider.ToolCallStart{ID: "t1", Name: "read_file"},
		provider.ToolCallEnd{ID: "t1", Input: input1},
		provider.ToolCallStart{ID: "t2", Name: "read_file"},
		provider.ToolCallEnd{ID: "t2", Input: input2},
		provider.StreamDone{
			StopReason: provider.StopReasonToolUse,
			Usage:      provider.Usage{InputTokens: 50, OutputTokens: 20},
		},
	)

	result, err := consumeStream(stdctx.Background(), ch, noopEmit, testNow)
	if err != nil {
		t.Fatalf("consumeStream error: %v", err)
	}

	if len(result.ToolCalls) != 2 {
		t.Fatalf("ToolCalls count = %d, want 2", len(result.ToolCalls))
	}
	if result.ToolCalls[0].ID != "t1" || result.ToolCalls[1].ID != "t2" {
		t.Fatalf("ToolCall IDs = %q/%q, want t1/t2", result.ToolCalls[0].ID, result.ToolCalls[1].ID)
	}
}

func TestConsumeStreamToolCallDeltaFallback(t *testing.T) {
	// When ToolCallEnd has empty Input, it should use accumulated delta args.
	ch := makeStreamCh(
		provider.ToolCallStart{ID: "t1", Name: "write_file"},
		provider.ToolCallDelta{ID: "t1", Delta: `{"path":"x.go","content":"hello"}`},
		provider.ToolCallEnd{ID: "t1", Input: nil},
		provider.StreamDone{
			StopReason: provider.StopReasonToolUse,
			Usage:      provider.Usage{InputTokens: 5, OutputTokens: 5},
		},
	)

	result, err := consumeStream(stdctx.Background(), ch, noopEmit, testNow)
	if err != nil {
		t.Fatalf("consumeStream error: %v", err)
	}

	if len(result.ToolCalls) != 1 {
		t.Fatalf("ToolCalls count = %d, want 1", len(result.ToolCalls))
	}
	if string(result.ToolCalls[0].Input) != `{"path":"x.go","content":"hello"}` {
		t.Fatalf("ToolCall.Input = %s, want accumulated delta", string(result.ToolCalls[0].Input))
	}
}

func TestConsumeStreamFatalError(t *testing.T) {
	ch := makeStreamCh(
		provider.TokenDelta{Text: "partial"},
		provider.StreamError{Fatal: true, Message: "connection reset"},
	)

	_, err := consumeStream(stdctx.Background(), ch, noopEmit, testNow)
	if err == nil {
		t.Fatal("consumeStream error = nil, want fatal stream error")
	}
	if got := err.Error(); got != "stream error: connection reset" {
		t.Fatalf("error = %q, want stream error: connection reset", got)
	}
}

func TestConsumeStreamNonFatalError(t *testing.T) {
	ch := makeStreamCh(
		provider.StreamError{Fatal: false, Message: "rate limited"},
		provider.TokenDelta{Text: "recovered"},
		provider.StreamDone{
			StopReason: provider.StopReasonEndTurn,
			Usage:      provider.Usage{InputTokens: 1, OutputTokens: 1},
		},
	)

	var errorEvents int
	emit := func(e Event) {
		if e.EventType() == "error" {
			errorEvents++
		}
	}

	result, err := consumeStream(stdctx.Background(), ch, emit, testNow)
	if err != nil {
		t.Fatalf("consumeStream error: %v", err)
	}
	if result.TextContent != "recovered" {
		t.Fatalf("TextContent = %q, want recovered", result.TextContent)
	}
	if errorEvents != 1 {
		t.Fatalf("error events = %d, want 1", errorEvents)
	}
}

func TestConsumeStreamContextCancelled(t *testing.T) {
	ctx, cancel := stdctx.WithCancel(stdctx.Background())
	cancel()

	// Channel that never sends — should hit ctx.Done.
	ch := make(chan provider.StreamEvent)

	_, err := consumeStream(ctx, ch, noopEmit, testNow)
	if err == nil {
		t.Fatal("consumeStream error = nil, want context cancelled")
	}
}

func TestConsumeStreamChannelClose(t *testing.T) {
	// Channel closed without StreamDone — should still finalize.
	ch := makeStreamCh(
		provider.TokenDelta{Text: "hello"},
	)

	result, err := consumeStream(stdctx.Background(), ch, noopEmit, testNow)
	if err != nil {
		t.Fatalf("consumeStream error: %v", err)
	}
	if result.TextContent != "hello" {
		t.Fatalf("TextContent = %q, want hello", result.TextContent)
	}
}

func TestContentBlocksToJSON(t *testing.T) {
	blocks := []provider.ContentBlock{
		provider.NewTextBlock("hello"),
		provider.NewThinkingBlock("thinking"),
	}

	got, err := contentBlocksToJSON(blocks)
	if err != nil {
		t.Fatalf("contentBlocksToJSON error: %v", err)
	}
	// Should be valid JSON.
	var parsed []provider.ContentBlock
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(parsed) != 2 {
		t.Fatalf("parsed count = %d, want 2", len(parsed))
	}
}

func TestConsumeStreamEmitsTokenEvents(t *testing.T) {
	ch := makeStreamCh(
		provider.TokenDelta{Text: "a"},
		provider.TokenDelta{Text: "b"},
		provider.StreamDone{
			StopReason: provider.StopReasonEndTurn,
			Usage:      provider.Usage{},
		},
	)

	var tokenEvents []string
	emit := func(e Event) {
		if te, ok := e.(TokenEvent); ok {
			tokenEvents = append(tokenEvents, te.Token)
		}
	}

	_, err := consumeStream(stdctx.Background(), ch, emit, testNow)
	if err != nil {
		t.Fatalf("consumeStream error: %v", err)
	}
	if len(tokenEvents) != 2 || tokenEvents[0] != "a" || tokenEvents[1] != "b" {
		t.Fatalf("tokenEvents = %v, want [a b]", tokenEvents)
	}
}
