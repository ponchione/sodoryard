package provider

import (
	"encoding/json"
	"fmt"
)

// Role identifies the sender of a message in the conversation.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message represents a single message in the conversation history. Content is
// stored as json.RawMessage: user/tool messages contain a JSON string, assistant
// messages contain a JSON array of content blocks.
//
// ToolUseID and ToolName are only populated when Role is RoleTool.
type Message struct {
	Role      Role            `json:"role"`
	Content   json.RawMessage `json:"content"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	ToolName  string          `json:"tool_name,omitempty"`

	// CacheControl optionally marks this message as a prompt cache breakpoint
	// (Anthropic). Other providers ignore this field.
	CacheControl *CacheControl `json:"cache_control,omitempty"`
}

// ContentBlock represents a typed block within an assistant message.
// Type is one of "text", "thinking", or "tool_use".
//
// For "text": only Text is populated.
// For "thinking": only Thinking is populated.
// For "tool_use": ID, Name, and Input are populated.
type ContentBlock struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	Thinking string          `json:"thinking,omitempty"`
	ID       string          `json:"id,omitempty"`
	Name     string          `json:"name,omitempty"`
	Input    json.RawMessage `json:"input,omitempty"`
}

// NewTextBlock creates a ContentBlock of type "text".
func NewTextBlock(text string) ContentBlock {
	return ContentBlock{Type: "text", Text: text}
}

// NewThinkingBlock creates a ContentBlock of type "thinking".
func NewThinkingBlock(thinking string) ContentBlock {
	return ContentBlock{Type: "thinking", Thinking: thinking}
}

// NewToolUseBlock creates a ContentBlock of type "tool_use".
func NewToolUseBlock(id, name string, input json.RawMessage) ContentBlock {
	return ContentBlock{Type: "tool_use", ID: id, Name: name, Input: input}
}

// NewUserMessage creates a user message with the given text content.
func NewUserMessage(text string) Message {
	content, _ := json.Marshal(text)
	return Message{
		Role:    RoleUser,
		Content: content,
	}
}

// NewToolResultMessage creates a tool result message.
func NewToolResultMessage(toolUseID, toolName, content string) Message {
	raw, _ := json.Marshal(content)
	return Message{
		Role:      RoleTool,
		Content:   raw,
		ToolUseID: toolUseID,
		ToolName:  toolName,
	}
}

// ContentBlocksFromRaw unmarshals a json.RawMessage (a JSON array) into a
// slice of ContentBlock values.
func ContentBlocksFromRaw(raw json.RawMessage) ([]ContentBlock, error) {
	var blocks []ContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, fmt.Errorf("parsing content blocks: %w", err)
	}
	return blocks, nil
}
