package codex

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ponchione/sodoryard/internal/provider"
)

// responsesResponse is the top-level Responses API JSON response.
type responsesResponse struct {
	ID     string                `json:"id"`
	Object string                `json:"object"`
	Model  string                `json:"model"`
	Output []responsesOutputItem `json:"output"`
	Usage  responsesUsage        `json:"usage"`
}

// responsesOutputItem represents one item in the output array.
type responsesOutputItem struct {
	Type             string                   `json:"type"` // "message", "function_call", "reasoning"
	ID               string                   `json:"id"`
	Role             string                   `json:"role,omitempty"`              // "assistant" for type="message"
	Content          []responsesOutputContent `json:"content,omitempty"`           // for type="message"
	CallID           string                   `json:"call_id,omitempty"`           // for type="function_call"
	Name             string                   `json:"name,omitempty"`              // for type="function_call"
	Arguments        string                   `json:"arguments,omitempty"`         // for type="function_call" (JSON string)
	EncryptedContent string                   `json:"encrypted_content,omitempty"` // for type="reasoning"
}

// responsesOutputContent represents content within a message output item.
type responsesOutputContent struct {
	Type string `json:"type"` // "output_text"
	Text string `json:"text"`
}

// responsesUsage carries token usage from the Responses API.
type responsesUsage struct {
	InputTokens         int                    `json:"input_tokens"`
	OutputTokens        int                    `json:"output_tokens"`
	InputTokensDetails  responsesInputDetails  `json:"input_tokens_details"`
	OutputTokensDetails responsesOutputDetails `json:"output_tokens_details"`
}

// responsesInputDetails carries input token detail breakdowns.
type responsesInputDetails struct {
	CachedTokens int `json:"cached_tokens"`
}

// responsesOutputDetails carries output token detail breakdowns.
type responsesOutputDetails struct {
	ReasoningTokens int `json:"reasoning_tokens"`
}

// retryableStatuses are HTTP status codes that trigger retry logic.
var retryableStatuses = map[int]bool{
	429: true,
	500: true,
	502: true,
	503: true,
}

// Complete sends a non-streaming request to the Responses API and returns
// the unified response.
func (p *CodexProvider) Complete(ctx context.Context, req *provider.Request) (*provider.Response, error) {
	if ctx.Err() != nil {
		return nil, &provider.ProviderError{
			Provider:   "codex",
			StatusCode: 0,
			Message:    "request cancelled",
			Retriable:  false,
		}
	}

	token, err := p.getAccessToken(ctx)
	if err != nil {
		return nil, err
	}

	model := req.Model
	if model == "" {
		model = "o3"
	}

	streamResponse := p.usesChatGPTCodexEndpoint()
	apiReq := buildResponsesRequest(model, req, streamResponse)
	body, err := json.Marshal(apiReq)
	if err != nil {
		return nil, &provider.ProviderError{
			Provider:   "codex",
			StatusCode: 0,
			Message:    fmt.Sprintf("failed to marshal request: %v", err),
			Retriable:  false,
		}
	}

	var lastStatusCode int
	var lastBody []byte
	maxAttempts := 3
	baseDelay := 1 * time.Second

	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			if ctx.Err() != nil {
				return nil, &provider.ProviderError{
					Provider:   "codex",
					StatusCode: 0,
					Message:    "request cancelled",
					Retriable:  false,
				}
			}

			delay := baseDelay * (1 << (attempt - 1))
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, &provider.ProviderError{
					Provider:   "codex",
					StatusCode: 0,
					Message:    "request cancelled",
					Retriable:  false,
				}
			case <-timer.C:
			}
		}

		httpReq, err := http.NewRequestWithContext(ctx, "POST", p.responsesEndpointURL(), bytes.NewReader(body))
		if err != nil {
			return nil, &provider.ProviderError{
				Provider:   "codex",
				StatusCode: 0,
				Message:    fmt.Sprintf("failed to create request: %v", err),
				Retriable:  false,
			}
		}
		httpReq.Header.Set("Authorization", "Bearer "+token)
		httpReq.Header.Set("Content-Type", "application/json")

		start := time.Now()
		resp, err := p.httpClient.Do(httpReq)
		if err != nil {
			if ctx.Err() != nil {
				return nil, &provider.ProviderError{
					Provider:   "codex",
					StatusCode: 0,
					Message:    "request cancelled",
					Retriable:  false,
				}
			}
			return nil, &provider.ProviderError{
				Provider:   "codex",
				StatusCode: 0,
				Message:    fmt.Sprintf("request failed: %v", err),
				Retriable:  true,
				Err:        err,
			}
		}

		var respBody []byte
		if p.usesChatGPTCodexEndpoint() && resp.StatusCode == 200 {
			contentBlocks, usage, stopReason, err := readStreamedResponse(resp.Body)
			resp.Body.Close()
			if err != nil {
				return nil, &provider.ProviderError{
					Provider:   "codex",
					StatusCode: 0,
					Message:    fmt.Sprintf("failed to read streamed response body: %v", err),
					Retriable:  false,
				}
			}
			latencyMs := time.Since(start).Milliseconds()
			return &provider.Response{
				Content:    contentBlocks,
				Usage:      usage,
				Model:      model,
				StopReason: stopReason,
				LatencyMs:  latencyMs,
			}, nil
		} else {
			var readErr error
			respBody, readErr = io.ReadAll(resp.Body)
			resp.Body.Close()
			if readErr != nil {
				return nil, &provider.ProviderError{
					Provider:   "codex",
					StatusCode: 0,
					Message:    fmt.Sprintf("failed to read response body: %v", readErr),
					Retriable:  false,
				}
			}
		}
		latencyMs := time.Since(start).Milliseconds()

		lastStatusCode = resp.StatusCode
		lastBody = respBody

		// Auth errors: no retry
		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			return nil, provider.NewAuthProviderError("codex", provider.AuthInvalidCredentials, resp.StatusCode, "Codex authentication failed.", codexAuthRemediation(), nil)
		}

		// Retryable errors
		if retryableStatuses[resp.StatusCode] {
			continue
		}

		// Other non-200 errors: no retry
		if resp.StatusCode != 200 {
			bodyStr := string(respBody)
			if len(bodyStr) > 1024 {
				bodyStr = bodyStr[:1024]
			}
			return nil, &provider.ProviderError{
				Provider:   "codex",
				StatusCode: resp.StatusCode,
				Message:    bodyStr,
				Retriable:  false,
			}
		}

		// Success: parse response
		var apiResp responsesResponse
		if err := json.Unmarshal(respBody, &apiResp); err != nil {
			return nil, &provider.ProviderError{
				Provider:   "codex",
				StatusCode: 0,
				Message:    fmt.Sprintf("failed to parse response: %v", err),
				Retriable:  false,
			}
		}

		contentBlocks, stopReason := parseOutputItems(apiResp.Output)

		usage := provider.Usage{
			InputTokens:         apiResp.Usage.InputTokens,
			OutputTokens:        apiResp.Usage.OutputTokens,
			CacheReadTokens:     apiResp.Usage.InputTokensDetails.CachedTokens,
			CacheCreationTokens: 0,
		}

		return &provider.Response{
			Content:    contentBlocks,
			Usage:      usage,
			Model:      apiResp.Model,
			StopReason: stopReason,
			LatencyMs:  latencyMs,
		}, nil
	}

	// All retries exhausted
	bodyStr := string(lastBody)
	if len(bodyStr) > 512 {
		bodyStr = bodyStr[:512]
	}

	msg := "server error after 3 attempts: " + bodyStr
	if lastStatusCode == 429 {
		msg = "rate limited after 3 attempts: " + bodyStr
	}

	return nil, &provider.ProviderError{
		Provider:   "codex",
		StatusCode: lastStatusCode,
		Message:    msg,
		Retriable:  true, // preserve retriable so router can attempt fallback
	}
}

// parseOutputItems converts Responses API output items to unified ContentBlock
// values and determines the stop reason.
func readStreamedResponse(body io.Reader) ([]provider.ContentBlock, provider.Usage, provider.StopReason, error) {
	scanner := bufio.NewScanner(body)
	var eventType string
	var text strings.Builder
	var usage provider.Usage
	stopReason := provider.StopReasonEndTurn
	var toolName string
	var toolCallID string
	var toolArgs strings.Builder
	var blocks []provider.ContentBlock

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		switch eventType {
		case "response.output_text.delta":
			var delta sseTextDelta
			if err := json.Unmarshal([]byte(data), &delta); err != nil {
				return nil, provider.Usage{}, "", err
			}
			text.WriteString(delta.Delta)
		case "response.output_item.added":
			var added sseOutputItemAdded
			if err := json.Unmarshal([]byte(data), &added); err != nil {
				return nil, provider.Usage{}, "", err
			}
			if added.Item.Type == "function_call" {
				toolCallID = added.Item.CallID
				toolName = added.Item.Name
				toolArgs.Reset()
				stopReason = provider.StopReasonToolUse
			}
		case "response.function_call_arguments.delta":
			var delta sseFuncArgDelta
			if err := json.Unmarshal([]byte(data), &delta); err != nil {
				return nil, provider.Usage{}, "", err
			}
			toolArgs.WriteString(delta.Delta)
		case "response.output_item.done":
			var done sseOutputItemDone
			if err := json.Unmarshal([]byte(data), &done); err != nil {
				return nil, provider.Usage{}, "", err
			}
			if done.Item.Type == "function_call" {
				blocks = append(blocks, provider.ContentBlock{
					Type:  "tool_use",
					ID:    toolCallID,
					Name:  toolName,
					Input: json.RawMessage(toolArgs.String()),
				})
				toolCallID = ""
				toolName = ""
				toolArgs.Reset()
			}
		case "response.completed":
			var completed sseCompleted
			if err := json.Unmarshal([]byte(data), &completed); err != nil {
				return nil, provider.Usage{}, "", err
			}
			usage = provider.Usage{
				InputTokens:         completed.Response.Usage.InputTokens,
				OutputTokens:        completed.Response.Usage.OutputTokens,
				CacheReadTokens:     completed.Response.Usage.InputTokensDetails.CachedTokens,
				CacheCreationTokens: 0,
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, provider.Usage{}, "", err
	}
	if text.Len() > 0 {
		blocks = append([]provider.ContentBlock{{Type: "text", Text: text.String()}}, blocks...)
	}
	return blocks, usage, stopReason, nil
}

func parseOutputItems(items []responsesOutputItem) ([]provider.ContentBlock, provider.StopReason) {
	var blocks []provider.ContentBlock
	hasToolCall := false

	for _, item := range items {
		switch item.Type {
		case "message":
			for _, content := range item.Content {
				if content.Type == "output_text" {
					blocks = append(blocks, provider.ContentBlock{
						Type: "text",
						Text: content.Text,
					})
				}
			}
		case "function_call":
			hasToolCall = true
			blocks = append(blocks, provider.ContentBlock{
				Type:  "tool_use",
				ID:    item.CallID,
				Name:  item.Name,
				Input: json.RawMessage(item.Arguments),
			})
		case "reasoning":
			blocks = append(blocks, provider.NewThinkingBlock(item.EncryptedContent))
		default:
			// Unknown output item type; skip with warning
			// (In production, this would be logged)
		}
	}

	stopReason := provider.StopReasonEndTurn
	if hasToolCall {
		stopReason = provider.StopReasonToolUse
	}

	return blocks, stopReason
}
