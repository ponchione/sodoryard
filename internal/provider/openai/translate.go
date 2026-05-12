package openai

import (
	"encoding/json"
	"strings"

	"github.com/ponchione/sodoryard/internal/provider"
)

// chatRequest is the top-level JSON body for POST /chat/completions.
type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Tools       []chatTool    `json:"tools,omitempty"`
	Temperature *float64      `json:"temperature,omitempty"`
	MaxTokens   *int          `json:"max_tokens,omitempty"`
	Stream      bool          `json:"stream"`
}

// chatMessage represents one message in the messages array.
type chatMessage struct {
	Role       string         `json:"role"`                   // "system", "user", "assistant", "tool"
	Content    string         `json:"content,omitempty"`      // text content (empty string omitted)
	ToolCalls  []chatToolCall `json:"tool_calls,omitempty"`   // assistant tool invocations
	ToolCallID string         `json:"tool_call_id,omitempty"` // for role=tool, references the call
}

// chatToolCall represents one tool invocation from the assistant.
type chatToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"` // always "function"
	Function chatFunctionCall `json:"function"`
}

// chatFunctionCall holds the function name and JSON-encoded arguments.
type chatFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string
}

// chatTool represents a tool definition in the tools array.
type chatTool struct {
	Type     string       `json:"type"` // always "function"
	Function chatFunction `json:"function"`
}

// chatFunction describes a callable function.
type chatFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"` // JSON Schema object
}

// buildChatRequest translates a unified Request into the OpenAI chat
// completions request body. The model parameter comes from the provider config.
func buildChatRequest(model string, req *provider.Request, stream bool) chatRequest {
	cr := chatRequest{Model: model, Stream: stream}

	// Temperature.
	if req.Temperature != nil {
		cr.Temperature = req.Temperature
	}

	// MaxTokens: the unified Request uses int (0 means unset).
	if req.MaxTokens > 0 {
		mt := req.MaxTokens
		cr.MaxTokens = &mt
	}

	// System blocks: concatenate into a single system message.
	if len(req.SystemBlocks) > 0 {
		var parts []string
		for _, sb := range req.SystemBlocks {
			parts = append(parts, sb.Text)
		}
		cr.Messages = append(cr.Messages, chatMessage{
			Role:    "system",
			Content: strings.Join(parts, "\n\n"),
		})
	}

	// Translate messages.
	for _, msg := range req.Messages {
		switch msg.Role {
		case provider.RoleUser:
			cr.Messages = append(cr.Messages, chatMessage{
				Role:    "user",
				Content: rawContentText(msg.Content),
			})

		case provider.RoleAssistant:
			cm := chatMessage{Role: "assistant"}
			// Parse content blocks from the assistant message.
			blocks, err := provider.ContentBlocksFromRaw(msg.Content)
			if err != nil {
				cm.Content = rawContentText(msg.Content)
			} else {
				var textParts []string
				for _, block := range blocks {
					switch block.Type {
					case "text":
						textParts = append(textParts, block.Text)
					case "tool_use":
						cm.ToolCalls = append(cm.ToolCalls, chatToolCall{
							ID:   block.ID,
							Type: "function",
							Function: chatFunctionCall{
								Name:      block.Name,
								Arguments: string(block.Input),
							},
						})
					}
				}
				if len(textParts) > 0 {
					cm.Content = strings.Join(textParts, "\n")
				}
			}
			cr.Messages = append(cr.Messages, cm)

		case provider.RoleTool:
			cr.Messages = append(cr.Messages, chatMessage{
				Role:       "tool",
				ToolCallID: msg.ToolUseID,
				Content:    rawContentText(msg.Content),
			})
		}
	}

	// Tool definitions.
	if len(req.Tools) > 0 {
		cr.Tools = make([]chatTool, len(req.Tools))
		for i, td := range req.Tools {
			cr.Tools[i] = chatTool{
				Type: "function",
				Function: chatFunction{
					Name:        td.Name,
					Description: td.Description,
					Parameters:  td.InputSchema,
				},
			}
		}
	}

	return cr
}

func rawContentText(raw json.RawMessage) string {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	return string(raw)
}
