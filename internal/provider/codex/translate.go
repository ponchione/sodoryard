package codex

import (
	"encoding/json"
	"strings"

	"github.com/ponchione/sodoryard/internal/provider"
)

const (
	forcedCodexModel           = "gpt-5.5"
	forcedCodexReasoningEffort = "xhigh"
)

func codexRequestModel(_ string) string {
	return forcedCodexModel
}

// responsesRequest is the top-level JSON body for POST /v1/responses.
type responsesRequest struct {
	Model        string              `json:"model"`
	Instructions string              `json:"instructions"`
	Input        []responsesInput    `json:"input"`
	Tools        []responsesTool     `json:"tools,omitempty"`
	Stream       bool                `json:"stream"`
	Store        bool                `json:"store"`
	Reasoning    *responsesReasoning `json:"reasoning,omitempty"`
	Include      []string            `json:"include,omitempty"`
}

// responsesInput represents one item in the input array.
// ChatGPT Codex accepts both message items and top-level function call/result
// items in the same array.
type responsesInput struct {
	Type             string                           `json:"type,omitempty"`
	ID               string                           `json:"id,omitempty"`
	Role             string                           `json:"role,omitempty"`
	Content          interface{}                      `json:"content,omitempty"`
	CallID           string                           `json:"call_id,omitempty"`
	Name             string                           `json:"name,omitempty"`
	Arguments        string                           `json:"arguments,omitempty"`
	Output           string                           `json:"output,omitempty"`
	EncryptedContent string                           `json:"encrypted_content,omitempty"`
	Summary          []provider.ReasoningSummaryBlock `json:"summary,omitempty"`
}

// responsesTool represents a tool definition in the tools array.
type responsesTool struct {
	Type        string          `json:"type"` // always "function"
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"` // JSON Schema object
}

// responsesReasoning controls reasoning behavior.
type responsesReasoning struct {
	Effort  string `json:"effort"` // "high", "medium", "low"
	Summary string `json:"summary,omitempty"`
}

// buildResponsesRequest translates a unified Request into the Responses API
// request body. The model parameter comes from the provider config or request.
func buildResponsesRequest(model string, req *provider.Request, streamResponse bool) responsesRequest {
	model = codexRequestModel(model)
	rr := responsesRequest{
		Model:        model,
		Instructions: "You are a helpful assistant.",
		Stream:       streamResponse,
		Store:        false,
	}

	// System prompt handling: Codex expects top-level instructions rather than
	// a system-role input item.
	if len(req.SystemBlocks) > 0 {
		var parts []string
		for _, sb := range req.SystemBlocks {
			parts = append(parts, sb.Text)
		}
		rr.Instructions = strings.Join(parts, "\n\n")
	}

	// Message translation
	for _, msg := range req.Messages {
		switch msg.Role {
		case provider.RoleUser:
			var text string
			_ = json.Unmarshal(msg.Content, &text)
			rr.Input = append(rr.Input, responsesInput{
				Role:    "user",
				Content: text,
			})

		case provider.RoleAssistant:
			blocks, err := provider.ContentBlocksFromRaw(msg.Content)
			if err != nil {
				// If we can't parse content blocks, try as string
				var text string
				_ = json.Unmarshal(msg.Content, &text)
				rr.Input = append(rr.Input, responsesInput{
					Role:    "assistant",
					Content: text,
				})
				continue
			}

			var textParts []string
			var reasoningItems []responsesInput
			var toolCalls []responsesInput
			for _, block := range blocks {
				switch block.Type {
				case "text":
					if block.Text != "" {
						textParts = append(textParts, block.Text)
					}
				case "codex_reasoning":
					if block.EncryptedContent != "" {
						reasoningItems = append(reasoningItems, responsesInput{
							Type:             "reasoning",
							ID:               block.ReasoningID,
							EncryptedContent: block.EncryptedContent,
							Summary:          block.Summary,
						})
					}
				case "tool_use":
					toolCalls = append(toolCalls, responsesInput{
						Type:      "function_call",
						CallID:    block.ID,
						Name:      block.Name,
						Arguments: string(block.Input),
					})
				case "thinking":
					// Skip: Responses API uses encrypted reasoning, not plaintext thinking
				}
			}
			rr.Input = append(rr.Input, reasoningItems...)
			if len(textParts) > 0 {
				rr.Input = append(rr.Input, responsesInput{
					Role:    "assistant",
					Content: strings.Join(textParts, "\n"),
				})
			} else if len(reasoningItems) > 0 {
				rr.Input = append(rr.Input, responsesInput{
					Role:    "assistant",
					Content: "",
				})
			}
			rr.Input = append(rr.Input, toolCalls...)

		case provider.RoleTool:
			var text string
			_ = json.Unmarshal(msg.Content, &text)
			rr.Input = append(rr.Input, responsesInput{
				Type:   "function_call_output",
				CallID: msg.ToolUseID,
				Output: text,
			})
		}
	}

	// Tool definitions
	if len(req.Tools) > 0 {
		for _, tool := range req.Tools {
			rr.Tools = append(rr.Tools, responsesTool{
				Type:        "function",
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.InputSchema,
			})
		}
	}

	// Reasoning configuration is currently pinned for the forced Codex daily-driver model.
	rr.Reasoning = &responsesReasoning{
		Effort:  forcedCodexReasoningEffort,
		Summary: "auto",
	}
	rr.Include = []string{"reasoning.encrypted_content"}

	return rr
}
