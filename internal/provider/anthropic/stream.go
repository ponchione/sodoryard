package anthropic

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/ponchione/sodoryard/internal/provider"
)

// SSE event deserialization types.

type sseMessageStart struct {
	Message struct {
		ID    string   `json:"id"`
		Usage apiUsage `json:"usage"`
	} `json:"message"`
}

type sseContentBlockStart struct {
	Index        int `json:"index"`
	ContentBlock struct {
		Type string `json:"type"`
		ID   string `json:"id,omitempty"`
		Name string `json:"name,omitempty"`
	} `json:"content_block"`
}

type sseContentBlockDelta struct {
	Index int `json:"index"`
	Delta struct {
		Type        string `json:"type"`
		Text        string `json:"text,omitempty"`
		Thinking    string `json:"thinking,omitempty"`
		PartialJSON string `json:"partial_json,omitempty"`
	} `json:"delta"`
}

type sseContentBlockStop struct {
	Index int `json:"index"`
}

type sseMessageDelta struct {
	Delta struct {
		StopReason string `json:"stop_reason"`
	} `json:"delta"`
	Usage struct {
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// streamState tracks active content blocks during SSE streaming.
type streamState struct {
	activeBlocks map[int]activeBlock
}

// activeBlock tracks a single content block being streamed.
type activeBlock struct {
	blockType string          // "text", "thinking", or "tool_use"
	toolID    string          // only set for tool_use blocks
	toolName  string          // only set for tool_use blocks
	jsonAccum strings.Builder // accumulates partial_json for tool_use blocks
}

// Stream executes a streaming LLM call to the Anthropic Messages API and
// returns a channel of StreamEvent values.
func (p *AnthropicProvider) Stream(ctx context.Context, req *provider.Request) (<-chan provider.StreamEvent, error) {
	resp, err := p.doWithRetry(ctx, func() (*http.Response, error) {
		httpReq, err := p.buildHTTPRequest(ctx, req, true)
		if err != nil {
			return nil, err
		}
		return p.httpClient.Do(httpReq)
	})
	if err != nil {
		return nil, err
	}

	ch := make(chan provider.StreamEvent, 32)

	go func() {
		defer close(ch)
		defer resp.Body.Close()

		p.processSSEStream(ctx, resp.Body, ch)
	}()

	return ch, nil
}

// send attempts to send an event on the channel, respecting context cancellation.
// Returns false if the context is done.
func send(ctx context.Context, ch chan<- provider.StreamEvent, event provider.StreamEvent) bool {
	select {
	case ch <- event:
		return true
	case <-ctx.Done():
		return false
	}
}

// processSSEStream reads SSE events from the reader and emits StreamEvents.
func (p *AnthropicProvider) processSSEStream(ctx context.Context, body io.Reader, ch chan<- provider.StreamEvent) {
	state := &streamState{
		activeBlocks: make(map[int]activeBlock),
	}

	var currentEventType string
	var stopReason string
	var finalUsage provider.Usage

	scanner := bufio.NewScanner(body)
	scanner.Split(bufio.ScanLines)

	for scanner.Scan() {
		// Check context before processing each line.
		if ctx.Err() != nil {
			send(ctx, ch, provider.StreamError{
				Err:     ctx.Err(),
				Fatal:   true,
				Message: "stream cancelled",
			})
			return
		}

		line := scanner.Text()

		// Ignore empty lines (SSE event separators) and comments.
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}

		if strings.HasPrefix(line, "event: ") {
			currentEventType = strings.TrimPrefix(line, "event: ")
			continue
		}

		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			p.handleSSEData(ctx, ch, state, currentEventType, []byte(data), &stopReason, &finalUsage)
			continue
		}
	}

	if err := scanner.Err(); err != nil {
		send(ctx, ch, provider.StreamError{
			Err:     err,
			Fatal:   true,
			Message: fmt.Sprintf("SSE stream read error: %s", err),
		})
	}
}

// handleSSEData dispatches an SSE data payload based on the event type.
func (p *AnthropicProvider) handleSSEData(
	ctx context.Context,
	ch chan<- provider.StreamEvent,
	state *streamState,
	eventType string,
	data []byte,
	stopReason *string,
	finalUsage *provider.Usage,
) {
	switch eventType {
	case "message_start":
		var msg sseMessageStart
		if err := json.Unmarshal(data, &msg); err != nil {
			send(ctx, ch, provider.StreamError{
				Err:     err,
				Fatal:   false,
				Message: fmt.Sprintf("failed to parse SSE event: message_start: %s", err),
			})
			return
		}
		finalUsage.InputTokens = msg.Message.Usage.InputTokens
		finalUsage.CacheReadTokens = msg.Message.Usage.CacheReadInputTokens
		finalUsage.CacheCreationTokens = msg.Message.Usage.CacheCreationInputTokens
		send(ctx, ch, provider.StreamUsage{
			Usage: provider.Usage{
				InputTokens:         msg.Message.Usage.InputTokens,
				CacheReadTokens:     msg.Message.Usage.CacheReadInputTokens,
				CacheCreationTokens: msg.Message.Usage.CacheCreationInputTokens,
			},
		})

	case "content_block_start":
		var cbs sseContentBlockStart
		if err := json.Unmarshal(data, &cbs); err != nil {
			send(ctx, ch, provider.StreamError{
				Err:     err,
				Fatal:   false,
				Message: fmt.Sprintf("failed to parse SSE event: content_block_start: %s", err),
			})
			return
		}
		ab := activeBlock{
			blockType: cbs.ContentBlock.Type,
		}
		if cbs.ContentBlock.Type == "tool_use" {
			ab.toolID = cbs.ContentBlock.ID
			ab.toolName = cbs.ContentBlock.Name
			send(ctx, ch, provider.ToolCallStart{
				ID:   cbs.ContentBlock.ID,
				Name: cbs.ContentBlock.Name,
			})
		}
		state.activeBlocks[cbs.Index] = ab

	case "content_block_delta":
		var cbd sseContentBlockDelta
		if err := json.Unmarshal(data, &cbd); err != nil {
			send(ctx, ch, provider.StreamError{
				Err:     err,
				Fatal:   false,
				Message: fmt.Sprintf("failed to parse SSE event: content_block_delta: %s", err),
			})
			return
		}
		ab, ok := state.activeBlocks[cbd.Index]
		if !ok {
			slog.Warn("content_block_delta for unknown index", "index", cbd.Index)
			return
		}

		switch cbd.Delta.Type {
		case "text_delta":
			send(ctx, ch, provider.TokenDelta{Text: cbd.Delta.Text})
		case "thinking_delta":
			send(ctx, ch, provider.ThinkingDelta{Thinking: cbd.Delta.Thinking})
		case "input_json_delta":
			ab.jsonAccum.WriteString(cbd.Delta.PartialJSON)
			state.activeBlocks[cbd.Index] = ab
			send(ctx, ch, provider.ToolCallDelta{
				ID:    ab.toolID,
				Delta: cbd.Delta.PartialJSON,
			})
		default:
			slog.Warn("unknown content_block_delta type", "delta_type", cbd.Delta.Type, "raw_json", string(data))
		}

	case "content_block_stop":
		var cbs sseContentBlockStop
		if err := json.Unmarshal(data, &cbs); err != nil {
			send(ctx, ch, provider.StreamError{
				Err:     err,
				Fatal:   false,
				Message: fmt.Sprintf("failed to parse SSE event: content_block_stop: %s", err),
			})
			return
		}
		ab, ok := state.activeBlocks[cbs.Index]
		if ok && ab.blockType == "tool_use" {
			send(ctx, ch, provider.ToolCallEnd{
				ID:    ab.toolID,
				Input: json.RawMessage(ab.jsonAccum.String()),
			})
		}
		delete(state.activeBlocks, cbs.Index)

	case "message_delta":
		var md sseMessageDelta
		if err := json.Unmarshal(data, &md); err != nil {
			send(ctx, ch, provider.StreamError{
				Err:     err,
				Fatal:   false,
				Message: fmt.Sprintf("failed to parse SSE event: message_delta: %s", err),
			})
			return
		}
		*stopReason = md.Delta.StopReason
		finalUsage.OutputTokens = md.Usage.OutputTokens

	case "message_stop":
		send(ctx, ch, provider.StreamDone{
			StopReason: mapStopReason(*stopReason),
			Usage:      *finalUsage,
		})
	}
}
