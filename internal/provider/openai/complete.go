package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"time"

	"github.com/ponchione/sodoryard/internal/provider"
)

// chatResponse is the top-level JSON response from POST /chat/completions.
type chatResponse struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Model   string       `json:"model"`
	Choices []chatChoice `json:"choices"`
	Usage   chatUsage    `json:"usage"`
}

// chatChoice is one entry in the choices array.
type chatChoice struct {
	Index        int         `json:"index"`
	Message      chatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

// chatUsage holds token counts from the response.
type chatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func (u chatUsage) toProviderUsage() provider.Usage {
	return provider.Usage{
		InputTokens:  u.PromptTokens,
		OutputTokens: u.CompletionTokens,
	}
}

const maxRetryAttempts = 3

// Complete sends a non-streaming chat completion request and returns
// the unified response. It retries on transient server errors and rate limits.
func (p *OpenAIProvider) Complete(ctx context.Context, req *provider.Request) (*provider.Response, error) {
	chatReq := buildChatRequest(p.model, req, false)

	body, err := json.Marshal(chatReq)
	if err != nil {
		return nil, fmt.Errorf("OpenAI-compatible provider '%s': failed to marshal request: %w", p.name, err)
	}

	var lastErr error
	for attempt := 0; attempt < maxRetryAttempts; attempt++ {
		if attempt > 0 {
			if !p.backoff(ctx, attempt) {
				return nil, ctx.Err()
			}
		}

		httpReq, err := p.newChatCompletionRequest(ctx, body)
		if err != nil {
			return nil, err
		}

		resp, err := p.client.Do(httpReq)
		if err != nil {
			reqErr := p.requestFailure(ctx, err)
			if !reqErr.Retriable {
				return nil, reqErr
			}
			lastErr = reqErr
			continue
		}

		respBody, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = &provider.ProviderError{
				Provider:   p.name,
				StatusCode: 0,
				Message:    fmt.Sprintf("OpenAI-compatible provider '%s': failed to read response: %s", p.name, readErr),
				Retriable:  true,
				Err:        readErr,
			}
			continue
		}

		retryAfter := provider.ParseRetryAfter(resp.Header.Get("Retry-After"), time.Now())

		switch resp.StatusCode {
		case 200:
			var chatResp chatResponse
			if err := json.Unmarshal(respBody, &chatResp); err != nil {
				return nil, &provider.ProviderError{
					Provider:   p.name,
					StatusCode: resp.StatusCode,
					Message:    fmt.Sprintf("OpenAI-compatible provider '%s': failed to parse response JSON: %s", p.name, err),
					Retriable:  false,
				}
			}
			return translateResponse(p.name, &chatResp)

		case 401, 403:
			return nil, p.statusFailure(resp.StatusCode, retryAfter, false)

		case 429:
			lastErr = p.statusFailure(resp.StatusCode, retryAfter, true)
			continue

		case 500, 502, 503:
			lastErr = p.statusFailure(resp.StatusCode, retryAfter, true)
			continue

		default:
			return nil, p.statusFailure(resp.StatusCode, retryAfter, false)
		}
	}

	return nil, lastErr
}

// translateResponse converts an OpenAI chatResponse to a unified provider.Response.
func translateResponse(name string, resp *chatResponse) (*provider.Response, error) {
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("OpenAI-compatible provider '%s': response contained no choices", name)
	}

	choice := resp.Choices[0]
	var content []provider.ContentBlock

	// Text content.
	if choice.Message.Content != "" {
		content = append(content, provider.NewTextBlock(choice.Message.Content))
	}

	// Tool calls.
	for _, tc := range choice.Message.ToolCalls {
		if tc.Function.Arguments == "" && tc.ID == "" {
			continue // skip empty entries
		}
		content = append(content, provider.NewToolUseBlock(
			tc.ID,
			tc.Function.Name,
			json.RawMessage(tc.Function.Arguments),
		))
	}

	return &provider.Response{
		Content:    content,
		Usage:      resp.Usage.toProviderUsage(),
		Model:      resp.Model,
		StopReason: mapFinishReason(choice.FinishReason),
	}, nil
}

// mapFinishReason converts an OpenAI finish_reason string to a provider.StopReason.
func mapFinishReason(reason string) provider.StopReason {
	switch reason {
	case "stop":
		return provider.StopReasonEndTurn
	case "tool_calls":
		return provider.StopReasonToolUse
	case "length":
		return provider.StopReasonMaxTokens
	default:
		return provider.StopReasonEndTurn
	}
}

// backoff sleeps for an exponential backoff delay with jitter, respecting
// context cancellation. Returns true if the sleep completed, false if the
// context was cancelled.
func (p *OpenAIProvider) backoff(ctx context.Context, attempt int) bool {
	baseDelay := 1 * time.Second
	delay := baseDelay * time.Duration(1<<uint(attempt-1)) // attempt 1 -> 1s, attempt 2 -> 2s
	jitter := time.Duration(rand.Int63n(500_000_000))      // 0 to 500ms
	totalDelay := delay + jitter

	return provider.SleepWithContext(ctx, totalDelay)
}

// isConnectionError returns true if the error indicates a connection failure
// (e.g., connection refused, no route to host).
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}
	// Also check for generic dial errors.
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}
	return false
}
