package tool

import (
	"context"
	"fmt"

	"github.com/ponchione/sirtopham/internal/provider"
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
	toolCall := ToolCallFromProvider(call)
	var results []ToolResult
	if meta, ok := ExecutionMetaFromContext(ctx); ok {
		results = a.executor.ExecuteWithMeta(ctx, []ToolCall{toolCall}, meta)
	} else {
		results = a.executor.Execute(ctx, []ToolCall{toolCall})
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("tool executor returned no results for %q", call.Name)
	}
	pr := results[0].ToProvider()
	return &pr, nil
}
