package tool

import (
	"context"
	"fmt"
	"strings"

	"github.com/ponchione/sodoryard/internal/provider"
)

// AgentLoopAdapter wraps an Executor to satisfy the agent.ToolExecutor
// interface. It dispatches a single tool call at a time (the agent loop
// currently handles batching and event emission in its own iteration loop).
//
// This adapter exists as a bridge until the agent loop is refactored to
// use the executor's native batch dispatch.
type AgentLoopAdapter struct {
	executor *Executor
}

// NewAgentLoopAdapter creates an adapter for the given executor.
func NewAgentLoopAdapter(executor *Executor) *AgentLoopAdapter {
	return &AgentLoopAdapter{executor: executor}
}

// Execute satisfies the agent.ToolExecutor interface:
//
//	Execute(ctx context.Context, call provider.ToolCall) (*provider.ToolResult, error)
//
// It converts the provider types, dispatches a single-element batch through
// the executor, and converts the result back to provider types.
func (a *AgentLoopAdapter) Execute(ctx context.Context, call provider.ToolCall) (*provider.ToolResult, error) {
	results, err := a.ExecuteBatch(ctx, []provider.ToolCall{call})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("tool executor returned no results for %q", call.Name)
	}
	return &results[0], nil
}

// ExecuteBatch satisfies agent.BatchToolExecutor by dispatching the full batch
// through the underlying executor and converting results back to provider
// types in the same order.
func (a *AgentLoopAdapter) ExecuteBatch(ctx context.Context, calls []provider.ToolCall) ([]provider.ToolResult, error) {
	if len(calls) == 0 {
		return nil, nil
	}
	toolCalls := make([]ToolCall, 0, len(calls))
	for _, call := range calls {
		toolCalls = append(toolCalls, ToolCallFromProvider(call))
	}
	var results []ToolResult
	if meta, ok := ExecutionMetaFromContext(ctx); ok {
		results = a.executor.ExecuteWithMeta(ctx, toolCalls, meta)
	} else {
		results = a.executor.Execute(ctx, toolCalls)
	}
	if len(results) != len(toolCalls) {
		return nil, fmt.Errorf("tool executor returned %d results for %d calls", len(results), len(toolCalls))
	}
	providerResults := make([]provider.ToolResult, 0, len(results))
	for i, result := range results {
		pr := enrichToolResultForAgent(toolCalls[i].Name, result).ToProvider()
		providerResults = append(providerResults, pr)
	}
	return providerResults, nil
}

func enrichToolResultForAgent(toolName string, result ToolResult) ToolResult {
	if result.Success {
		return result
	}

	hint := ""
	switch toolName {
	case "file_edit":
		switch result.Error {
		case "not_read_first":
			hint = "Hint: Run file_read on the full file immediately before retrying file_edit. Partial reads do not satisfy the precondition."
		case "stale_write":
			hint = "Hint: The file changed after it was read. Re-run file_read on the full current file, then retry the edit against the updated contents."
		case "multiple_matches":
			hint = "Hint: Provide a longer old_str copied from the latest full file_read so it matches exactly one location. Use the candidate lines/snippets to disambiguate."
		case "zero_match":
			hint = "Hint: Re-run file_read and copy the exact current text, including whitespace and indentation, before retrying file_edit."
		case "invalid_create_via_edit":
			hint = "Hint: file_edit cannot create new content from an empty match. Use file_write to create/overwrite the file, or read the file first and replace an exact existing string."
		case "old_equals_new":
			hint = "Hint: Choose a different new_str if you want to change the file, or skip the edit entirely if no change is needed."
		}
	case "file_write":
		switch result.Error {
		case "not_read_first":
			hint = "Hint: file_write can create new files directly, but overwriting an existing non-empty file now requires a fresh full file_read first."
		case "stale_write":
			hint = "Hint: The file changed after it was read. Re-run file_read on the full current file, then retry file_write against the updated contents."
		}
	}
	if hint == "" {
		return result
	}
	if strings.Contains(result.Content, hint) {
		return result
	}
	result.Content = strings.TrimSpace(result.Content) + "\n" + hint
	return result
}
