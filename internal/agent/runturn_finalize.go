package agent

import (
	stdctx "context"
	"fmt"
	"time"

	"github.com/ponchione/sodoryard/internal/conversation"
	"github.com/ponchione/sodoryard/internal/provider"
)

func finalTurnResult(turnCtx *TurnStartResult, finalText string, iteration int, usage provider.Usage, duration time.Duration) *TurnResult {
	return &TurnResult{
		TurnStartResult: *turnCtx,
		FinalText:       finalText,
		IterationCount:  iteration,
		TotalUsage:      usage,
		Duration:        duration,
	}
}

func serializeAssistantResponse(result *streamResult, iteration int) (string, error) {
	assistantContentJSON, err := contentBlocksToJSON(sanitizeContentBlocks(result.ContentBlocks))
	if err != nil {
		return "", fmt.Errorf("agent loop: serialize assistant content for iteration %d: %w", iteration, err)
	}
	return assistantContentJSON, nil
}

func (l *AgentLoop) completeTextOnlyIteration(ctx stdctx.Context, turnExec *turnExecution, iteration int, result *streamResult, assistantContentJSON string) (*TurnResult, error) {
	persistMessages := []conversation.IterationMessage{{
		Role:    "assistant",
		Content: assistantContentJSON,
	}}
	if err := l.conversationManager.PersistIteration(ctx, turnExec.req.ConversationID, turnExec.req.TurnNumber, iteration, persistMessages); err != nil {
		if isCancelled(ctx) {
			cleanupTurn := cleanupInflightTurnBase(turnExec, iteration)
			cleanupTurn.AssistantResponseStarted = true
			cleanupTurn.AssistantMessageContent = assistantContentJSON
			return nil, l.handleTurnCancellation(cleanupTurn, ctx.Err())
		}
		return nil, fmt.Errorf("agent loop: persist final iteration %d: %w", iteration, err)
	}

	l.updatePostTurnQuality(ctx, turnExec.req.ConversationID, turnExec.req.TurnNumber, turnExec.allToolCalls)
	l.maybeGenerateTitle(turnExec.req.ConversationID, turnExec.req.TurnNumber)

	turnDuration := l.now().Sub(turnExec.turnStart)
	l.emit(TurnCompleteEvent{
		TurnNumber:        turnExec.req.TurnNumber,
		IterationCount:    iteration,
		TotalInputTokens:  turnExec.totalUsage.InputTokens,
		TotalOutputTokens: turnExec.totalUsage.OutputTokens,
		Duration:          turnDuration,
		Time:              l.now(),
	})
	l.emit(StatusEvent{State: StateIdle, Time: l.now()})

	return finalTurnResult(turnExec.turnCtx, result.TextContent, iteration, turnExec.totalUsage, turnDuration), nil
}

func (l *AgentLoop) completeFinalToolIteration(
	ctx stdctx.Context,
	turnExec *turnExecution,
	iteration int,
	result *streamResult,
	assistantContentJSON string,
	toolCalls []provider.ToolCall,
	toolResults []provider.ToolResult,
) (*TurnResult, error) {
	if err := l.completeToolIteration(ctx, turnExec, iteration, assistantContentJSON, toolCalls, toolResults); err != nil {
		return nil, err
	}

	l.updatePostTurnQuality(ctx, turnExec.req.ConversationID, turnExec.req.TurnNumber, turnExec.allToolCalls)
	l.maybeGenerateTitle(turnExec.req.ConversationID, turnExec.req.TurnNumber)

	turnDuration := l.now().Sub(turnExec.turnStart)
	l.emit(TurnCompleteEvent{
		TurnNumber:        turnExec.req.TurnNumber,
		IterationCount:    iteration,
		TotalInputTokens:  turnExec.totalUsage.InputTokens,
		TotalOutputTokens: turnExec.totalUsage.OutputTokens,
		Duration:          turnDuration,
		Time:              l.now(),
	})
	l.emit(StatusEvent{State: StateIdle, Time: l.now()})

	finalText := ""
	if result != nil {
		finalText = result.TextContent
	}
	return finalTurnResult(turnExec.turnCtx, finalText, iteration, turnExec.totalUsage, turnDuration), nil
}
