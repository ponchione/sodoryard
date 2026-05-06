package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ponchione/sodoryard/internal/provider"
	providersse "github.com/ponchione/sodoryard/internal/provider/sse"
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
	Type             string                           `json:"type"` // "message", "function_call", "reasoning"
	ID               string                           `json:"id"`
	Role             string                           `json:"role,omitempty"`              // "assistant" for type="message"
	Content          []responsesOutputContent         `json:"content,omitempty"`           // for type="message"
	CallID           string                           `json:"call_id,omitempty"`           // for type="function_call"
	Name             string                           `json:"name,omitempty"`              // for type="function_call"
	Arguments        string                           `json:"arguments,omitempty"`         // for type="function_call" (JSON string)
	EncryptedContent string                           `json:"encrypted_content,omitempty"` // for type="reasoning"
	Summary          []provider.ReasoningSummaryBlock `json:"summary,omitempty"`           // for type="reasoning"
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

	model := codexRequestModel(req.Model)

	streamResponse := p.usesChatGPTCodexEndpoint()
	apiReq := buildResponsesRequestWithReasoning(model, req, streamResponse, p.configuredReasoningEffort())
	body, err := json.Marshal(apiReq)
	if err != nil {
		return nil, codexMarshalError(err)
	}

	var lastStatusCode int
	var lastBody []byte
	maxAttempts := 3
	baseDelay := 1 * time.Second

	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			if ctx.Err() != nil {
				return nil, codexCancelledError()
			}

			delay := baseDelay * (1 << (attempt - 1))
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, codexCancelledError()
			case <-timer.C:
			}
		}

		httpReq, err := p.newResponsesHTTPRequest(ctx, body, token)
		if err != nil {
			return nil, err
		}

		start := time.Now()
		resp, err := p.httpClient.Do(httpReq)
		if err != nil {
			return nil, codexRequestFailure(ctx, err)
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
		if provider.IsRetryableHTTPStatus(resp.StatusCode) {
			continue
		}

		// Other non-200 errors: no retry
		if resp.StatusCode != 200 {
			return nil, &provider.ProviderError{
				Provider:   "codex",
				StatusCode: resp.StatusCode,
				Message:    truncateBody(string(respBody), 1024),
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

		usage := usageFromResponsesUsage(apiResp.Usage)

		return &provider.Response{
			Content:    contentBlocks,
			Usage:      usage,
			Model:      apiResp.Model,
			StopReason: stopReason,
			LatencyMs:  latencyMs,
		}, nil
	}

	// All retries exhausted
	bodyStr := truncateBody(string(lastBody), 512)

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
	reader := providersse.NewReader(body, maxSSEScannerTokenSize)
	accumulator := newResponsesSSEAccumulator()

	for {
		event, ok, err := reader.Next(context.Background())
		if err != nil {
			return nil, provider.Usage{}, "", err
		}
		if !ok {
			break
		}
		if _, err := accumulator.apply(event.Type, []byte(event.Data)); err != nil {
			return nil, provider.Usage{}, "", err
		}
	}
	blocks, usage, stopReason := accumulator.contentBlocks()
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
			if block, ok := codexReasoningBlockFromOutputItem(item); ok {
				blocks = append(blocks, block)
			}
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

func codexReasoningBlockFromOutputItem(item responsesOutputItem) (provider.ContentBlock, bool) {
	return codexReasoningBlock(item.Type, item.ID, item.EncryptedContent, item.Summary)
}

func usageFromResponsesUsage(usage responsesUsage) provider.Usage {
	return provider.Usage{
		InputTokens:         usage.InputTokens,
		OutputTokens:        usage.OutputTokens,
		CacheReadTokens:     usage.InputTokensDetails.CachedTokens,
		CacheCreationTokens: 0,
	}
}

func codexReasoningBlock(itemType, id, encryptedContent string, summary []provider.ReasoningSummaryBlock) (provider.ContentBlock, bool) {
	if itemType != "reasoning" || strings.TrimSpace(encryptedContent) == "" {
		return provider.ContentBlock{}, false
	}
	return provider.NewCodexReasoningBlock(id, encryptedContent, summary), true
}
