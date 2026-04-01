package agent

import (
	stdctx "context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ponchione/sirtopham/internal/provider"
)

// streamResult holds the accumulated output from consuming a provider's stream.
type streamResult struct {
	// TextContent is the concatenated visible text from TokenDelta events.
	TextContent string

	// ThinkingContent is the concatenated thinking/reasoning text.
	ThinkingContent string

	// ToolCalls contains all complete tool_use blocks extracted from the stream.
	ToolCalls []provider.ToolCall

	// ContentBlocks is the ordered list of content blocks suitable for
	// serializing as the assistant message content.
	ContentBlocks []provider.ContentBlock

	// Usage is the final token usage from the stream.
	Usage provider.Usage

	// StopReason is the final stop reason from the stream.
	StopReason provider.StopReason
}

// HasToolUse returns true if the response contains tool_use blocks.
func (r *streamResult) HasToolUse() bool {
	return len(r.ToolCalls) > 0
}

// consumeStream reads all events from a provider stream channel, accumulates
// text, thinking, and tool_use blocks, and emits agent events via the provided
// emit function. It returns the accumulated result or an error if the stream
// produces a fatal error.
//
// The emit function is called synchronously for each streamed delta, enabling
// real-time token streaming to subscribers.
func consumeStream(
	ctx stdctx.Context,
	ch <-chan provider.StreamEvent,
	emit func(Event),
	now func() string,
) (*streamResult, error) {
	result := &streamResult{}

	var textBuilder strings.Builder
	var thinkingBuilder strings.Builder
	var inThinking bool

	// Track in-progress tool calls by ID for argument accumulation.
	pendingToolArgs := make(map[string]*strings.Builder)
	pendingToolNames := make(map[string]string)

	for {
		select {
		case <-ctx.Done():
			result.TextContent = textBuilder.String()
			result.ThinkingContent = thinkingBuilder.String()
			result.ContentBlocks = buildContentBlocks(result.ThinkingContent, result.TextContent, result.ContentBlocks)
			return result, ctx.Err()
		case event, ok := <-ch:
			if !ok {
				// Channel closed — finalize.
				result.TextContent = textBuilder.String()
				result.ThinkingContent = thinkingBuilder.String()
				return result, nil
			}

			switch e := event.(type) {
			case provider.TokenDelta:
				textBuilder.WriteString(e.Text)
				emit(TokenEvent{Token: e.Text})

			case provider.ThinkingDelta:
				if !inThinking {
					inThinking = true
					emit(ThinkingStartEvent{})
				}
				thinkingBuilder.WriteString(e.Thinking)
				emit(ThinkingDeltaEvent{Delta: e.Thinking})

			case provider.ToolCallStart:
				// End any open thinking block before tool dispatch.
				if inThinking {
					inThinking = false
					emit(ThinkingEndEvent{})
				}

				pendingToolArgs[e.ID] = &strings.Builder{}
				pendingToolNames[e.ID] = e.Name
				emit(ToolCallStartEvent{ToolCallID: e.ID, ToolName: e.Name})

			case provider.ToolCallDelta:
				if builder, ok := pendingToolArgs[e.ID]; ok {
					builder.WriteString(e.Delta)
				}

			case provider.ToolCallEnd:
				name := pendingToolNames[e.ID]
				input := e.Input

				// If ToolCallEnd.Input is empty, use accumulated delta args.
				if len(input) == 0 {
					if builder, ok := pendingToolArgs[e.ID]; ok {
						input = json.RawMessage(builder.String())
					}
				}

				tc := provider.ToolCall{
					ID:    e.ID,
					Name:  name,
					Input: input,
				}
				result.ToolCalls = append(result.ToolCalls, tc)

				// Build the content block for this tool_use.
				result.ContentBlocks = append(result.ContentBlocks, provider.NewToolUseBlock(e.ID, name, input))

				delete(pendingToolArgs, e.ID)
				delete(pendingToolNames, e.ID)

			case provider.StreamUsage:
				result.Usage = e.Usage

			case provider.StreamDone:
				// Close any open thinking block.
				if inThinking {
					inThinking = false
					emit(ThinkingEndEvent{})
				}

				result.StopReason = e.StopReason
				result.Usage = e.Usage

				// Finalize text content.
				result.TextContent = textBuilder.String()
				result.ThinkingContent = thinkingBuilder.String()

				result.ContentBlocks = buildContentBlocks(result.ThinkingContent, result.TextContent, result.ContentBlocks)

				return result, nil

			case provider.StreamError:
				if e.Fatal {
					return nil, fmt.Errorf("stream error: %s", e.Message)
				}
				// Non-fatal: log/emit and continue.
				emit(ErrorEvent{
					ErrorCode:   "stream_error",
					Message:     e.Message,
					Recoverable: true,
				})
			}
		}
	}
}

// contentBlocksToJSON serializes content blocks to a JSON array string suitable
// for storing as the assistant message content in the database.
func buildContentBlocks(thinkingContent, textContent string, existing []provider.ContentBlock) []provider.ContentBlock {
	finalBlocks := make([]provider.ContentBlock, 0, len(existing)+2)
	if thinkingContent != "" {
		finalBlocks = append(finalBlocks, provider.NewThinkingBlock(thinkingContent))
	}
	if textContent != "" {
		finalBlocks = append(finalBlocks, provider.NewTextBlock(textContent))
	}
	for _, cb := range existing {
		if cb.Type == "tool_use" {
			finalBlocks = append(finalBlocks, cb)
		}
	}
	return finalBlocks
}

func contentBlocksToJSON(blocks []provider.ContentBlock) (string, error) {
	data, err := json.Marshal(blocks)
	if err != nil {
		return "", fmt.Errorf("marshal content blocks: %w", err)
	}
	return string(data), nil
}

// sanitizeContentBlocks fixes any content blocks that contain invalid JSON in
// their Input fields (from malformed LLM tool calls). Invalid Input is replaced
// with a JSON-serialized error placeholder so that contentBlocksToJSON won't
// fail during serialization.
func sanitizeContentBlocks(blocks []provider.ContentBlock) []provider.ContentBlock {
	sanitized := make([]provider.ContentBlock, len(blocks))
	copy(sanitized, blocks)
	for i := range sanitized {
		if sanitized[i].Type == "tool_use" && len(sanitized[i].Input) > 0 {
			var parsed interface{}
			if err := json.Unmarshal(sanitized[i].Input, &parsed); err != nil {
				// Replace invalid Input with a JSON string describing the error.
				sanitized[i].Input = json.RawMessage(fmt.Sprintf(
					`{"_error":"malformed input","_raw":%q}`,
					string(sanitized[i].Input),
				))
			}
		}
	}
	return sanitized
}
