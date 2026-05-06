package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/ponchione/sodoryard/internal/provider"
	providersse "github.com/ponchione/sodoryard/internal/provider/sse"
)

// streamChunk is one parsed SSE data payload from the streaming response.
type streamChunk struct {
	ID      string              `json:"id"`
	Object  string              `json:"object"` // "chat.completion.chunk"
	Choices []streamChunkChoice `json:"choices"`
	Usage   *chatUsage          `json:"usage,omitempty"`
}

// streamChunkChoice is one entry in a streaming chunk's choices array.
type streamChunkChoice struct {
	Index        int         `json:"index"`
	Delta        streamDelta `json:"delta"`
	FinishReason *string     `json:"finish_reason"` // null until final chunk
}

// streamDelta holds incremental content or tool call fragments.
type streamDelta struct {
	Role      string           `json:"role,omitempty"`
	Content   string           `json:"content,omitempty"`
	ToolCalls []streamToolCall `json:"tool_calls,omitempty"`
}

// streamToolCall is an incremental tool call fragment in a streaming delta.
type streamToolCall struct {
	Index    int                `json:"index"`
	ID       string             `json:"id,omitempty"`   // present only in the first chunk for this call
	Type     string             `json:"type,omitempty"` // "function", present only in first chunk
	Function streamFunctionCall `json:"function,omitempty"`
}

// streamFunctionCall holds incremental function name/arguments fragments.
type streamFunctionCall struct {
	Name      string `json:"name,omitempty"`      // present only in first chunk
	Arguments string `json:"arguments,omitempty"` // appended across chunks
}

// accumulatedToolCall collects incremental tool call fragments.
type accumulatedToolCall struct {
	ID        string
	Name      string
	Arguments strings.Builder
}

// Stream sends a streaming chat completion request and returns a channel
// of unified stream events. The channel is closed when the stream ends
// or an error occurs. The caller must drain the channel.
func (p *OpenAIProvider) Stream(ctx context.Context, req *provider.Request) (<-chan provider.StreamEvent, error) {
	chatReq := buildChatRequest(p.model, req, true)

	body, err := json.Marshal(chatReq)
	if err != nil {
		return nil, fmt.Errorf("OpenAI-compatible provider '%s': failed to marshal request: %w", p.name, err)
	}

	httpReq, err := p.newChatCompletionRequest(ctx, body)
	if err != nil {
		return nil, err
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, p.requestFailure(ctx, err)
	}

	// Handle non-200 responses before streaming.
	if resp.StatusCode != 200 {
		retryAfter := provider.ParseRetryAfter(resp.Header.Get("Retry-After"), time.Now())
		resp.Body.Close()
		return nil, p.statusFailure(resp.StatusCode, retryAfter, false)
	}

	ch := make(chan provider.StreamEvent, 32)

	go func() {
		defer close(ch)
		defer resp.Body.Close()

		p.processStream(ctx, resp, ch)
	}()

	return ch, nil
}

// processStream reads SSE lines from the response body and emits StreamEvents.
func (p *OpenAIProvider) processStream(ctx context.Context, resp *http.Response, ch chan<- provider.StreamEvent) {
	accumulated := make(map[int]*accumulatedToolCall)
	reader := providersse.NewReader(resp.Body, providersse.DefaultMaxEventBytes)

	for {
		event, ok, err := reader.Next(ctx)
		if err != nil {
			provider.SendStreamEvent(ctx, ch, provider.StreamError{
				Err:     err,
				Fatal:   true,
				Message: fmt.Sprintf("OpenAI-compatible provider '%s': stream read error: %s", p.name, err),
			})
			return
		}
		if !ok {
			return
		}
		if event.Data == "[DONE]" {
			return
		}
		p.handleStreamPayload(ctx, event.Data, accumulated, ch)
	}
}

// handleStreamPayload parses one SSE event payload and emits StreamEvents.
func (p *OpenAIProvider) handleStreamPayload(ctx context.Context, payload string, accumulated map[int]*accumulatedToolCall, ch chan<- provider.StreamEvent) {
	var chunk streamChunk
	if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
		provider.SendStreamEvent(ctx, ch, provider.StreamError{
			Err:     err,
			Fatal:   false,
			Message: fmt.Sprintf("OpenAI-compatible provider '%s': failed to parse stream chunk: %s", p.name, err),
		})
		return
	}

	if len(chunk.Choices) == 0 {
		emitStreamUsage(ctx, ch, chunk.Usage)
		return
	}

	choice := chunk.Choices[0]
	if choice.Delta.Content != "" {
		provider.SendStreamEvent(ctx, ch, provider.TokenDelta{Text: choice.Delta.Content})
	}

	for _, tc := range choice.Delta.ToolCalls {
		acc, exists := accumulated[tc.Index]
		if !exists {
			acc = &accumulatedToolCall{ID: tc.ID, Name: tc.Function.Name}
			accumulated[tc.Index] = acc
		} else {
			if tc.ID != "" {
				acc.ID = tc.ID
			}
			if tc.Function.Name != "" {
				acc.Name = tc.Function.Name
			}
		}
		acc.Arguments.WriteString(tc.Function.Arguments)
	}

	if choice.FinishReason != nil {
		reason := *choice.FinishReason
		if reason == "tool_calls" {
			emitToolCalls(ctx, ch, accumulated)
		}
		provider.SendStreamEvent(ctx, ch, provider.StreamDone{StopReason: mapFinishReason(reason)})
	}

	emitStreamUsage(ctx, ch, chunk.Usage)
}

func emitStreamUsage(ctx context.Context, ch chan<- provider.StreamEvent, usage *chatUsage) {
	if usage != nil {
		provider.SendStreamEvent(ctx, ch, provider.StreamUsage{Usage: usage.toProviderUsage()})
	}
}

// emitToolCalls emits accumulated tool calls as ToolCallStart + ToolCallEnd events
// in index order.
func emitToolCalls(ctx context.Context, ch chan<- provider.StreamEvent, accumulated map[int]*accumulatedToolCall) {
	// Collect and sort indices.
	indices := make([]int, 0, len(accumulated))
	for idx := range accumulated {
		indices = append(indices, idx)
	}
	sort.Ints(indices)

	for _, idx := range indices {
		acc := accumulated[idx]
		provider.SendStreamEvent(ctx, ch, provider.ToolCallStart{
			ID:   acc.ID,
			Name: acc.Name,
		})
		provider.SendStreamEvent(ctx, ch, provider.ToolCallEnd{
			ID:    acc.ID,
			Input: json.RawMessage(acc.Arguments.String()),
		})
	}
}
