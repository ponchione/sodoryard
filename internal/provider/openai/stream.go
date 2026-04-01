package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/ponchione/sirtopham/internal/provider"
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

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("OpenAI-compatible provider '%s': failed to create request: %w", p.name, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		if isConnectionError(err) {
			return nil, &provider.ProviderError{
				Provider:   p.name,
				StatusCode: 0,
				Message:    fmt.Sprintf("OpenAI-compatible provider '%s' at %s is not reachable. Is the model server running?", p.name, p.baseURL),
				Retriable:  false,
				Err:        err,
			}
		}
		if ctx.Err() != nil {
			return nil, &provider.ProviderError{
				Provider:   p.name,
				StatusCode: 0,
				Message:    ctx.Err().Error(),
				Retriable:  false,
				Err:        ctx.Err(),
			}
		}
		return nil, &provider.ProviderError{
			Provider:   p.name,
			StatusCode: 0,
			Message:    fmt.Sprintf("OpenAI-compatible provider '%s': request failed: %s", p.name, err),
			Retriable:  true,
			Err:        err,
		}
	}

	// Handle non-200 responses before streaming.
	if resp.StatusCode != 200 {
		retryAfter := provider.ParseRetryAfter(resp.Header.Get("Retry-After"), time.Now())
		resp.Body.Close()
		switch resp.StatusCode {
		case 401, 403:
			return nil, &provider.ProviderError{
				Provider:   p.name,
				StatusCode: resp.StatusCode,
				Message:    fmt.Sprintf("OpenAI-compatible provider '%s' authentication failed. Check API key configuration.", p.name),
				Retriable:  false,
			}
		case 429:
			return nil, &provider.ProviderError{
				Provider:   p.name,
				StatusCode: resp.StatusCode,
				Message:    fmt.Sprintf("OpenAI-compatible provider '%s': rate limited", p.name),
				Retriable:  true,
				RetryAfter: retryAfter,
			}
		case 500, 502, 503:
			return nil, &provider.ProviderError{
				Provider:   p.name,
				StatusCode: resp.StatusCode,
				Message:    fmt.Sprintf("OpenAI-compatible provider '%s': server error (HTTP %d)", p.name, resp.StatusCode),
				Retriable:  true,
				RetryAfter: retryAfter,
			}
		default:
			return nil, &provider.ProviderError{
				Provider:   p.name,
				StatusCode: resp.StatusCode,
				Message:    fmt.Sprintf("OpenAI-compatible provider '%s': unexpected HTTP status %d", p.name, resp.StatusCode),
				Retriable:  false,
			}
		}
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
	scanner := bufio.NewScanner(resp.Body)
	var dataLines []string

	flushEvent := func() bool {
		if len(dataLines) == 0 {
			return true
		}
		payload := strings.Join(dataLines, "\n")
		dataLines = dataLines[:0]
		if payload == "[DONE]" {
			return false
		}
		p.handleStreamPayload(ctx, payload, accumulated, ch)
		return true
	}

	for scanner.Scan() {
		if ctx.Err() != nil {
			sendEvent(ctx, ch, provider.StreamError{
				Err:     ctx.Err(),
				Fatal:   true,
				Message: ctx.Err().Error(),
			})
			return
		}

		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			if !flushEvent() {
				return
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}

		field, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.TrimPrefix(value, " ")
		if field != "data" {
			continue
		}
		dataLines = append(dataLines, value)
	}

	if !flushEvent() {
		return
	}
	if err := scanner.Err(); err != nil {
		sendEvent(ctx, ch, provider.StreamError{
			Err:     err,
			Fatal:   true,
			Message: fmt.Sprintf("OpenAI-compatible provider '%s': stream read error: %s", p.name, err),
		})
	}
}

// handleStreamPayload parses one SSE event payload and emits StreamEvents.
func (p *OpenAIProvider) handleStreamPayload(ctx context.Context, payload string, accumulated map[int]*accumulatedToolCall, ch chan<- provider.StreamEvent) {
	var chunk streamChunk
	if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
		sendEvent(ctx, ch, provider.StreamError{
			Err:     err,
			Fatal:   false,
			Message: fmt.Sprintf("OpenAI-compatible provider '%s': failed to parse stream chunk: %s", p.name, err),
		})
		return
	}

	if len(chunk.Choices) == 0 {
		if chunk.Usage != nil {
			sendEvent(ctx, ch, provider.StreamUsage{
				Usage: provider.Usage{
					InputTokens:         chunk.Usage.PromptTokens,
					OutputTokens:        chunk.Usage.CompletionTokens,
					CacheReadTokens:     0,
					CacheCreationTokens: 0,
				},
			})
		}
		return
	}

	choice := chunk.Choices[0]
	if choice.Delta.Content != "" {
		sendEvent(ctx, ch, provider.TokenDelta{Text: choice.Delta.Content})
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
		sendEvent(ctx, ch, provider.StreamDone{StopReason: mapFinishReason(reason)})
	}

	if chunk.Usage != nil {
		sendEvent(ctx, ch, provider.StreamUsage{
			Usage: provider.Usage{
				InputTokens:         chunk.Usage.PromptTokens,
				OutputTokens:        chunk.Usage.CompletionTokens,
				CacheReadTokens:     0,
				CacheCreationTokens: 0,
			},
		})
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
		sendEvent(ctx, ch, provider.ToolCallStart{
			ID:   acc.ID,
			Name: acc.Name,
		})
		sendEvent(ctx, ch, provider.ToolCallEnd{
			ID:    acc.ID,
			Input: json.RawMessage(acc.Arguments.String()),
		})
	}
}

// sendEvent attempts to send an event on the channel, respecting context cancellation.
func sendEvent(ctx context.Context, ch chan<- provider.StreamEvent, event provider.StreamEvent) {
	select {
	case ch <- event:
	case <-ctx.Done():
	}
}
