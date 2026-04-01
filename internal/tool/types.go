package tool

import (
	"context"
	"encoding/json"

	"github.com/ponchione/sirtopham/internal/provider"
)

// Purity classifies a tool's side-effect behavior for dispatch purposes.
type Purity int

const (
	// Pure tools are read-only and safe to execute concurrently.
	Pure Purity = iota
	// Mutating tools have side effects and must execute sequentially.
	Mutating
)

// String returns the human-readable purity classification.
func (p Purity) String() string {
	switch p {
	case Pure:
		return "pure"
	case Mutating:
		return "mutating"
	default:
		return "unknown"
	}
}

// OutputLimiter is optionally implemented by tools that need a different
// truncation limit than the global default.
type OutputLimiter interface {
	OutputLimit() int
}

// Tool is the interface every tool implementation must satisfy.
type Tool interface {
	// Name returns the tool identifier (e.g., "file_read").
	Name() string

	// Description returns a one-line description for the LLM.
	Description() string

	// ToolPurity declares whether the tool is Pure (read-only) or Mutating.
	ToolPurity() Purity

	// Schema returns the JSON Schema definition of the tool's parameters.
	// Format follows the Anthropic/OpenAI function calling convention:
	//   {"name": "...", "description": "...", "input_schema": {...}}
	Schema() json.RawMessage

	// Execute runs the tool. projectRoot restricts file operations to the
	// project directory. Returns a ToolResult on success or a Go error for
	// infrastructure failures only. Tool-level failures (file not found,
	// command failed) should be returned as ToolResult with Success=false.
	Execute(ctx context.Context, projectRoot string, input json.RawMessage) (*ToolResult, error)
}

// ToolCall represents an inbound tool invocation from the LLM.
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ToolCallFromProvider converts a provider.ToolCall to a tool.ToolCall.
func ToolCallFromProvider(pc provider.ToolCall) ToolCall {
	return ToolCall{
		ID:        pc.ID,
		Name:      pc.Name,
		Arguments: pc.Input,
	}
}

// ToolResult is the outcome of a tool execution.
type ToolResult struct {
	CallID     string `json:"call_id"`
	Content    string `json:"content"`
	Success    bool   `json:"success"`
	Error      string `json:"error,omitempty"`
	DurationMs int64  `json:"duration_ms"`
}

// ToProvider converts a tool.ToolResult to a provider.ToolResult for the
// agent loop's message assembly.
func (r ToolResult) ToProvider() provider.ToolResult {
	return provider.ToolResult{
		ToolUseID: r.CallID,
		Content:   r.Content,
		IsError:   !r.Success,
	}
}
