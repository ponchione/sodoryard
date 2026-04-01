package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/ponchione/sirtopham/internal/provider"
)

// API request types for the Anthropic Messages API.

type apiRequest struct {
	Model       string           `json:"model"`
	MaxTokens   int              `json:"max_tokens"`
	Temperature *float64         `json:"temperature,omitempty"`
	System      []apiSystemBlock `json:"system,omitempty"`
	Messages    []apiMessage     `json:"messages"`
	Tools       []apiTool        `json:"tools,omitempty"`
	Stream      bool             `json:"stream"`
	Thinking    *apiThinking     `json:"thinking,omitempty"`
}

type apiSystemBlock struct {
	Type         string           `json:"type"`
	Text         string           `json:"text"`
	CacheControl *apiCacheControl `json:"cache_control,omitempty"`
}

type apiCacheControl struct {
	Type string `json:"type"` // always "ephemeral"
}

type apiMessage struct {
	Role         string           `json:"role"`
	Content      json.RawMessage  `json:"content"`
	CacheControl *apiCacheControl `json:"cache_control,omitempty"`
}

type apiTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type apiThinking struct {
	Type         string `json:"type"`          // always "enabled"
	BudgetTokens int    `json:"budget_tokens"`
}

// AnthropicOptions holds Anthropic-specific provider options parsed from
// Request.ProviderOptions.
type AnthropicOptions struct {
	ThinkingEnabled bool `json:"thinking_enabled"`
	ThinkingBudget  int  `json:"thinking_budget"`
}

// DefaultThinkingBudget is the default thinking budget when thinking is enabled
// but no budget is specified.
const DefaultThinkingBudget = 10000

// NewAnthropicOptions creates provider options JSON for the Anthropic provider.
// If thinkingBudget is 0, DefaultThinkingBudget is used.
func NewAnthropicOptions(thinkingEnabled bool, thinkingBudget int) json.RawMessage {
	if thinkingBudget == 0 {
		thinkingBudget = DefaultThinkingBudget
	}
	opts := AnthropicOptions{
		ThinkingEnabled: thinkingEnabled,
		ThinkingBudget:  thinkingBudget,
	}
	data, _ := json.Marshal(opts)
	return json.RawMessage(data)
}

// buildHTTPRequest constructs an HTTP request for the Anthropic Messages API
// from a unified provider.Request.
func (p *AnthropicProvider) buildHTTPRequest(ctx context.Context, req *provider.Request, stream bool) (*http.Request, error) {
	headerName, headerValue, err := p.creds.GetAuthHeader(ctx)
	if err != nil {
		return nil, &provider.ProviderError{
			Provider:  "anthropic",
			StatusCode: 0,
			Message:   fmt.Sprintf("failed to obtain credentials: %s", err),
			Retriable: false,
			Err:       err,
		}
	}

	body, err := p.buildRequestBody(req, stream)
	if err != nil {
		return nil, err
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, &provider.ProviderError{
			Provider:  "anthropic",
			StatusCode: 0,
			Message:   fmt.Sprintf("failed to marshal request body: %s", err),
			Retriable: false,
			Err:       err,
		}
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/v1/messages", bytes.NewReader(data))
	if err != nil {
		return nil, &provider.ProviderError{
			Provider:  "anthropic",
			StatusCode: 0,
			Message:   fmt.Sprintf("failed to create HTTP request: %s", err),
			Retriable: false,
			Err:       err,
		}
	}

	httpReq.Header.Set(headerName, headerValue)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	httpReq.Header.Set("anthropic-beta", "interleaved-thinking-2025-05-14,oauth-2025-04-20")
	httpReq.Header.Set("Content-Type", "application/json")

	return httpReq, nil
}

// buildRequestBody converts a unified provider.Request into the Anthropic API
// request body format.
func (p *AnthropicProvider) buildRequestBody(req *provider.Request, stream bool) (*apiRequest, error) {
	ar := &apiRequest{
		Model:       req.Model,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		Stream:      stream,
	}

	// Defaults.
	if ar.Model == "" {
		ar.Model = "claude-sonnet-4-6-20250514"
	}
	if ar.MaxTokens == 0 {
		ar.MaxTokens = 8192
	}

	// Convert system blocks.
	if len(req.SystemBlocks) > 0 {
		ar.System = make([]apiSystemBlock, len(req.SystemBlocks))
		for i, sb := range req.SystemBlocks {
			ar.System[i] = apiSystemBlock{
				Type: "text",
				Text: sb.Text,
			}
			if sb.CacheControl != nil {
				ar.System[i].CacheControl = &apiCacheControl{Type: sb.CacheControl.Type}
			}
		}
	}

	// Convert messages.
	ar.Messages = make([]apiMessage, len(req.Messages))
	for i, msg := range req.Messages {
		// Translate provider.CacheControl to apiCacheControl if present.
		var cc *apiCacheControl
		if msg.CacheControl != nil {
			cc = &apiCacheControl{Type: msg.CacheControl.Type}
		}

		if msg.Role == provider.RoleTool {
			// Tool result messages are wrapped as user messages with a tool_result
			// content array.
			var textContent string
			if err := json.Unmarshal(msg.Content, &textContent); err != nil {
				// If content is not a JSON string, use it raw.
				textContent = string(msg.Content)
			}
			toolResult := []map[string]interface{}{
				{
					"type":        "tool_result",
					"tool_use_id": msg.ToolUseID,
					"content":     textContent,
				},
			}
			content, _ := json.Marshal(toolResult)
			ar.Messages[i] = apiMessage{
				Role:         "user",
				Content:      json.RawMessage(content),
				CacheControl: cc,
			}
		} else {
			ar.Messages[i] = apiMessage{
				Role:         string(msg.Role),
				Content:      msg.Content,
				CacheControl: cc,
			}
		}
	}

	// Convert tools.
	if len(req.Tools) > 0 {
		ar.Tools = make([]apiTool, len(req.Tools))
		for i, td := range req.Tools {
			ar.Tools[i] = apiTool{
				Name:        td.Name,
				Description: td.Description,
				InputSchema: td.InputSchema,
			}
		}
	}

	// Parse provider options for thinking support.
	if len(req.ProviderOptions) > 0 {
		var opts AnthropicOptions
		if err := json.Unmarshal(req.ProviderOptions, &opts); err != nil {
			return nil, &provider.ProviderError{
				Provider:  "anthropic",
				StatusCode: 0,
				Message:   fmt.Sprintf("invalid provider options: %s", err),
				Retriable: false,
				Err:       err,
			}
		}
		if opts.ThinkingEnabled {
			budget := opts.ThinkingBudget
			if budget <= 0 {
				budget = DefaultThinkingBudget
			}
			ar.Thinking = &apiThinking{
				Type:         "enabled",
				BudgetTokens: budget,
			}
			// Anthropic requires temperature to be unset when thinking is enabled.
			if ar.Temperature != nil {
				slog.Warn("temperature is ignored when thinking is enabled")
				ar.Temperature = nil
			}
		}
	}

	return ar, nil
}
