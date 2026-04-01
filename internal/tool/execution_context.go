package tool

import "context"

type executionMetaContextKey struct{}

// ContextWithExecutionMeta attaches tool execution metadata to a context so the
// agent-loop adapter can route single-call execution through ExecuteWithMeta
// without changing the agent.ToolExecutor interface.
func ContextWithExecutionMeta(ctx context.Context, meta ExecutionMeta) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, executionMetaContextKey{}, meta)
}

// ExecutionMetaFromContext retrieves tool execution metadata from context.
func ExecutionMetaFromContext(ctx context.Context) (ExecutionMeta, bool) {
	if ctx == nil {
		return ExecutionMeta{}, false
	}
	meta, ok := ctx.Value(executionMetaContextKey{}).(ExecutionMeta)
	if !ok {
		return ExecutionMeta{}, false
	}
	if meta.ConversationID == "" || meta.TurnNumber == 0 || meta.Iteration == 0 {
		return ExecutionMeta{}, false
	}
	return meta, true
}
