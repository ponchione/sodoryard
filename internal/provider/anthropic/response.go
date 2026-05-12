package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/ponchione/sodoryard/internal/provider"
)

const maxResponseSize = 1 << 20

// API response types for the Anthropic Messages API.

type apiResponse struct {
	ID         string            `json:"id"`
	Type       string            `json:"type"`
	Role       string            `json:"role"`
	Content    []apiContentBlock `json:"content"`
	Model      string            `json:"model"`
	StopReason string            `json:"stop_reason"`
	Usage      apiUsage          `json:"usage"`
}

type apiContentBlock struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	Thinking string          `json:"thinking,omitempty"`
	ID       string          `json:"id,omitempty"`
	Name     string          `json:"name,omitempty"`
	Input    json.RawMessage `json:"input,omitempty"`
}

type apiUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
}

func (u apiUsage) toProviderUsage() provider.Usage {
	return provider.Usage{
		InputTokens:         u.InputTokens,
		OutputTokens:        u.OutputTokens,
		CacheReadTokens:     u.CacheReadInputTokens,
		CacheCreationTokens: u.CacheCreationInputTokens,
	}
}

// Complete executes a non-streaming LLM call to the Anthropic Messages API.
func (p *AnthropicProvider) Complete(ctx context.Context, req *provider.Request) (*provider.Response, error) {
	start := time.Now()

	resp, err := p.doWithRetry(ctx, func() (*http.Response, error) {
		httpReq, err := p.buildHTTPRequest(ctx, req, false)
		if err != nil {
			return nil, err
		}
		return p.httpClient.Do(httpReq)
	})
	if err != nil {
		// Check for context cancellation.
		if ctx.Err() != nil {
			return nil, &provider.ProviderError{
				Provider:   "anthropic",
				StatusCode: 0,
				Message:    "request cancelled",
				Retriable:  false,
				Err:        ctx.Err(),
			}
		}
		return nil, err
	}
	defer resp.Body.Close()

	latencyMs := time.Since(start).Milliseconds()

	body, err := readResponseBody(resp.Body)
	if err != nil {
		return nil, &provider.ProviderError{
			Provider:   "anthropic",
			StatusCode: 0,
			Message:    fmt.Sprintf("failed to read response body: %s", err),
			Retriable:  false,
			Err:        err,
		}
	}

	var apiResp apiResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, &provider.ProviderError{
			Provider:   "anthropic",
			StatusCode: 0,
			Message:    fmt.Sprintf("failed to parse response: %s", err),
			Retriable:  false,
			Err:        err,
		}
	}

	// Parse content blocks.
	var contentBlocks []provider.ContentBlock
	for _, block := range apiResp.Content {
		switch block.Type {
		case "text":
			contentBlocks = append(contentBlocks, provider.ContentBlock{
				Type: "text",
				Text: block.Text,
			})
		case "thinking":
			contentBlocks = append(contentBlocks, provider.ContentBlock{
				Type:     "thinking",
				Thinking: block.Thinking,
			})
		case "tool_use":
			contentBlocks = append(contentBlocks, provider.ContentBlock{
				Type:  "tool_use",
				ID:    block.ID,
				Name:  block.Name,
				Input: block.Input,
			})
		default:
			slog.Warn("unknown content block type", "type", block.Type)
		}
	}

	return &provider.Response{
		Content:    contentBlocks,
		Usage:      apiResp.Usage.toProviderUsage(),
		Model:      apiResp.Model,
		StopReason: mapStopReason(apiResp.StopReason),
		LatencyMs:  latencyMs,
	}, nil
}

func readResponseBody(body io.Reader) ([]byte, error) {
	return io.ReadAll(io.LimitReader(body, maxResponseSize))
}

// mapStopReason converts an Anthropic stop_reason string to a provider.StopReason.
func mapStopReason(reason string) provider.StopReason {
	switch reason {
	case "end_turn":
		return provider.StopReasonEndTurn
	case "tool_use":
		return provider.StopReasonToolUse
	case "max_tokens":
		return provider.StopReasonMaxTokens
	default:
		if reason != "" {
			slog.Warn("unknown stop_reason, defaulting to end_turn", "stop_reason", reason)
		}
		return provider.StopReasonEndTurn
	}
}
