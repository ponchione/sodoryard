// Package provider defines the unified LLM provider interface and shared types
// that all provider backends (Anthropic, OpenAI-compatible, Codex) implement.
package provider

import (
	"context"
	"encoding/json"
)

// Provider is the central abstraction for LLM inference backends.
// Every provider (Anthropic, OpenAI-compatible, Codex) implements this interface.
type Provider interface {
	Complete(ctx context.Context, req *Request) (*Response, error)
	Stream(ctx context.Context, req *Request) (<-chan StreamEvent, error)
	Models(ctx context.Context) ([]Model, error)
	Name() string
}

// Pinger is an optional interface that providers may implement to provide a
// lightweight reachability check. The router calls Ping during Validate()
// instead of the heavier Models() call when a provider supports it.
//
// Implementations should be fast and targeted:
//   - Anthropic: auth check (GetAuthHeader) with ~5s timeout
//   - OpenAI-compatible/local: HTTP HEAD to baseURL with ~2s timeout
//   - Codex: lightweight authenticated probe against the configured endpoint
type Pinger interface {
	Ping(ctx context.Context) error
}

// Request carries every parameter needed to make an LLM call.
type Request struct {
	Messages        []Message        `json:"messages"`
	Tools           []ToolDefinition `json:"tools,omitempty"`
	Model           string           `json:"model"`
	Temperature     *float64         `json:"temperature,omitempty"`
	MaxTokens       int              `json:"max_tokens"`
	SystemBlocks    []SystemBlock    `json:"system,omitempty"`
	ProviderOptions json.RawMessage  `json:"provider_options,omitempty"`

	// Purpose classifies this call: "chat", "compression", or "title_generation".
	Purpose        string `json:"purpose,omitempty"`
	ConversationID string `json:"conversation_id,omitempty"`
	TurnNumber     int    `json:"turn_number,omitempty"`
	Iteration      int    `json:"iteration,omitempty"`
}

// CacheControl marks a system block as a prompt cache breakpoint.
type CacheControl struct {
	Type string `json:"type"` // "ephemeral"
}

// SystemBlock is a text block within the system prompt, optionally marked for
// prompt caching via CacheControl.
type SystemBlock struct {
	Text         string        `json:"text"`
	CacheControl *CacheControl `json:"cache_control,omitempty"`
}

// ToolDefinition describes a tool the model may invoke. InputSchema is a JSON
// Schema object passed through without deserialization.
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}
