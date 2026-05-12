package agent

import (
	stdctx "context"
	"errors"
	"fmt"
	"time"

	"github.com/ponchione/sodoryard/internal/provider"
	toolpkg "github.com/ponchione/sodoryard/internal/tool"
	tracepkg "github.com/ponchione/sodoryard/internal/trace"
)

type toolExecutionRecord struct {
	Call     provider.ToolCall
	Result   provider.ToolResult
	Duration time.Duration
}

func buildToolExecutionContext(ctx stdctx.Context, req RunTurnRequest, iteration int) stdctx.Context {
	ctx = tracepkg.ContextWithScope(ctx, tracepkg.Scope{
		ConversationID: req.ConversationID,
		ChainID:        req.ChainID,
		StepID:         req.StepID,
		TurnNumber:     req.TurnNumber,
		Iteration:      iteration,
	})
	return toolpkg.ContextWithExecutionMeta(ctx, toolpkg.ExecutionMeta{
		ConversationID: req.ConversationID,
		TurnNumber:     req.TurnNumber,
		Iteration:      iteration,
	})
}

func (l *AgentLoop) executeToolCalls(ctx stdctx.Context, turnExec *turnExecution, iteration int, result *streamResult, calls []provider.ToolCall) ([]toolExecutionRecord, *TurnResult, error) {
	execCtx := buildToolExecutionContext(ctx, turnExec.req, iteration)
	if batchExecutor, ok := l.toolExecutor.(BatchToolExecutor); ok {
		return l.executeToolCallsBatch(execCtx, turnExec, iteration, result, calls, batchExecutor)
	}
	return l.executeToolCallsSerial(execCtx, turnExec, iteration, result, calls)
}

func (l *AgentLoop) executeToolCallsSerial(ctx stdctx.Context, turnExec *turnExecution, iteration int, result *streamResult, calls []provider.ToolCall) ([]toolExecutionRecord, *TurnResult, error) {
	records := make([]toolExecutionRecord, 0, len(calls))
	for _, call := range calls {
		toolStart := l.now()
		toolResult, toolErr := l.toolExecutor.Execute(ctx, call)
		duration := l.now().Sub(toolStart)
		if toolErr != nil {
			if errors.Is(toolErr, toolpkg.ErrChainComplete) {
				return nil, l.handleChainComplete(turnExec, result, iteration), nil
			}
			toolResult = &provider.ToolResult{
				ToolUseID: call.ID,
				Content:   enrichToolError(call.Name, toolErr),
				IsError:   true,
			}
		}
		records = append(records, toolExecutionRecord{
			Call:     call,
			Result:   *toolResult,
			Duration: duration,
		})
		if isCancelled(ctx) {
			return records, nil, nil
		}
	}
	return records, nil, nil
}

func (l *AgentLoop) executeToolCallsBatch(ctx stdctx.Context, turnExec *turnExecution, iteration int, result *streamResult, calls []provider.ToolCall, batchExecutor BatchToolExecutor) ([]toolExecutionRecord, *TurnResult, error) {
	batchStart := l.now()
	batchResults, batchErr := batchExecutor.ExecuteBatch(ctx, calls)
	batchDuration := l.now().Sub(batchStart)
	if batchErr != nil {
		if errors.Is(batchErr, toolpkg.ErrChainComplete) {
			return nil, l.handleChainComplete(turnExec, result, iteration), nil
		}
		return normalizeToolExecutionError(calls, batchErr, batchDuration), nil, nil
	}
	if len(batchResults) != len(calls) {
		batchErr = fmt.Errorf("tool executor returned %d batch results for %d calls", len(batchResults), len(calls))
		return normalizeToolExecutionError(calls, batchErr, batchDuration), nil, nil
	}

	records := make([]toolExecutionRecord, 0, len(calls))
	for i, call := range calls {
		toolResult := batchResults[i]
		toolResult.ToolUseID = call.ID
		records = append(records, toolExecutionRecord{
			Call:     call,
			Result:   toolResult,
			Duration: batchDuration,
		})
	}
	return records, nil, nil
}

func normalizeToolExecutionError(calls []provider.ToolCall, err error, duration time.Duration) []toolExecutionRecord {
	records := make([]toolExecutionRecord, 0, len(calls))
	for _, call := range calls {
		records = append(records, toolExecutionRecord{
			Call: call,
			Result: provider.ToolResult{
				ToolUseID: call.ID,
				Content:   enrichToolError(call.Name, err),
				IsError:   true,
			},
			Duration: duration,
		})
	}
	return records
}

func (l *AgentLoop) handleChainComplete(turnExec *turnExecution, result *streamResult, iteration int) *TurnResult {
	finalText := ""
	if result != nil {
		finalText = result.TextContent
	}
	return finalTurnResult(turnExec.turnCtx, finalText, iteration, turnExec.totalUsage, l.now().Sub(turnExec.turnStart))
}
