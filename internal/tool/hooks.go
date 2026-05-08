package tool

import (
	"context"
	"errors"
	"fmt"
)

// Hook observes or gates individual tool executions. Before hooks run in
// registration order; after hooks run in reverse order around the tool call.
type Hook interface {
	BeforeTool(context.Context, ToolCall, Tool) (context.Context, error)
	AfterTool(context.Context, ToolCall, ToolResult) (ToolResult, error)
}

func runBeforeHooks(ctx context.Context, hooks []Hook, call ToolCall, def Tool) (context.Context, int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	for i, hook := range hooks {
		if hook == nil {
			continue
		}
		next, err := hook.BeforeTool(ctx, call, def)
		if err != nil {
			return ctx, i, err
		}
		if next != nil {
			ctx = next
		}
	}
	return ctx, len(hooks), nil
}

func runAfterHooks(ctx context.Context, hooks []Hook, call ToolCall, result ToolResult) (ToolResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var errs []error
	for i := len(hooks) - 1; i >= 0; i-- {
		hook := hooks[i]
		if hook == nil {
			continue
		}
		next, err := hook.AfterTool(ctx, call, result)
		result = next
		if err != nil {
			errs = append(errs, err)
			result = hookFailureResult(call, result, errors.Join(errs...))
		}
	}
	return result, errors.Join(errs...)
}

func hookFailureResult(call ToolCall, result ToolResult, err error) ToolResult {
	if err == nil {
		return result
	}
	if result.Success {
		result.Success = false
		result.Content = fmt.Sprintf("Tool %q hook failed: %v", call.Name, err)
	}
	if result.Error == "" {
		result.Error = err.Error()
	}
	return result
}
