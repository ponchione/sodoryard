package provider

import (
	"context"
	"encoding/json"
)

// StreamEvent is the sealed interface for all streaming event types. The
// unexported marker method prevents types outside this package from satisfying
// the interface. Consumers use type-switch to handle each variant.
type StreamEvent interface {
	streamEvent()
}

// TokenDelta carries an incremental text fragment from the LLM response.
type TokenDelta struct {
	Text string
}

func (TokenDelta) streamEvent() {}

// ThinkingDelta carries an incremental thinking/reasoning text fragment.
type ThinkingDelta struct {
	Thinking string
}

func (ThinkingDelta) streamEvent() {}

// CodexReasoning carries a completed encrypted Codex reasoning block from the
// final Responses API output. It is persisted for replay, not displayed as
// visible thinking.
type CodexReasoning struct {
	Block ContentBlock
}

func (CodexReasoning) streamEvent() {}

// ToolCallStart signals the beginning of a tool call.
type ToolCallStart struct {
	ID   string
	Name string
}

func (ToolCallStart) streamEvent() {}

// ToolCallDelta carries an incremental JSON argument fragment for a tool call.
type ToolCallDelta struct {
	ID    string
	Delta string
}

func (ToolCallDelta) streamEvent() {}

// ToolCallEnd signals that tool call arguments are complete. Input contains the
// full, assembled JSON arguments.
type ToolCallEnd struct {
	ID    string
	Input json.RawMessage
}

func (ToolCallEnd) streamEvent() {}

// StreamUsage carries intermediate token usage data.
type StreamUsage struct {
	Usage Usage
}

func (StreamUsage) streamEvent() {}

// StreamError carries an error during streaming. Fatal is true for
// non-recoverable errors (stream should terminate); false for recoverable.
type StreamError struct {
	Err     error
	Fatal   bool
	Message string
}

func (StreamError) streamEvent() {}

// StreamDone signals stream completion with the final stop reason and usage.
type StreamDone struct {
	StopReason StopReason
	Usage      Usage
}

func (StreamDone) streamEvent() {}

func SendStreamEvent(ctx context.Context, ch chan<- StreamEvent, event StreamEvent) bool {
	select {
	case ch <- event:
		return true
	default:
	}

	select {
	case ch <- event:
		return true
	case <-ctx.Done():
		return false
	}
}
