package agent

import (
	stdctx "context"

	"github.com/ponchione/sodoryard/internal/provider"
)

type validatedToolCalls struct {
	toolResults    []provider.ToolResult
	validCalls     []provider.ToolCall
	validIndices   []int
	toolsCancelled bool
}

func newInflightToolTurn(req RunTurnRequest, iteration, completedIterations int, result *streamResult, assistantContentJSON string) inflightTurn {
	inflight := cleanupInflightTurn(req.ConversationID, req.TurnNumber, iteration, completedIterations)
	inflight.AssistantResponseStarted = true
	inflight.AssistantMessageContent = assistantContentJSON
	inflight.ToolCalls = make([]inflightToolCall, len(result.ToolCalls))
	for i, tc := range result.ToolCalls {
		inflight.ToolCalls[i] = inflightToolCall{ToolCallID: tc.ID, ToolName: tc.Name}
	}
	return inflight
}

func (l *AgentLoop) validateToolCalls(ctx stdctx.Context, turnExec *turnExecution, iteration int, result *streamResult, inflight *inflightTurn, toolDefinitions []provider.ToolDefinition) validatedToolCalls {
	validated := validatedToolCalls{
		toolResults:  make([]provider.ToolResult, 0, len(result.ToolCalls)),
		validCalls:   make([]provider.ToolCall, 0, len(result.ToolCalls)),
		validIndices: make([]int, 0, len(result.ToolCalls)),
	}
	for idx, tc := range result.ToolCalls {
		if isCancelled(ctx) {
			validated.toolsCancelled = true
			break
		}

		turnExec.allToolCalls = append(turnExec.allToolCalls, completedToolCall{ToolName: tc.Name, Arguments: tc.Input})
		validation := validateToolCallAgainstSchema(tc, toolDefinitions)
		if !validation.Valid {
			l.logger.Warn("malformed tool call",
				"conversation_id", turnExec.req.ConversationID,
				"turn", turnExec.req.TurnNumber,
				"iteration", iteration,
				"tool_name", tc.Name,
				"tool_call_id", tc.ID,
				"error", validation.ErrorMessage,
			)
			l.emit(ErrorEvent{
				ErrorCode:   ErrorCodeMalformedToolCall,
				Message:     validation.ErrorMessage,
				Recoverable: true,
				Time:        l.now(),
			})
			validated.toolResults = append(validated.toolResults, provider.ToolResult{
				ToolUseID: tc.ID,
				Content:   validation.ErrorMessage,
				IsError:   true,
			})
			l.emit(ToolCallEndEvent{
				ToolCallID: tc.ID,
				Result:     validation.ErrorMessage,
				Duration:   0,
				Success:    false,
				Time:       l.now(),
			})
			continue
		}

		l.emit(ToolCallStartEvent{
			ToolCallID: tc.ID,
			ToolName:   tc.Name,
			Arguments:  tc.Input,
			Time:       l.now(),
		})
		inflight.ToolCalls[idx].Started = true
		validated.validCalls = append(validated.validCalls, tc)
		validated.validIndices = append(validated.validIndices, idx)
	}
	return validated
}

func (l *AgentLoop) executeValidToolCalls(
	ctx stdctx.Context,
	turnExec *turnExecution,
	iteration int,
	result *streamResult,
	inflight *inflightTurn,
	validated validatedToolCalls,
) ([]provider.ToolResult, *TurnResult, error) {
	toolResults := append([]provider.ToolResult(nil), validated.toolResults...)
	if len(validated.validCalls) == 0 {
		return toolResults, nil, nil
	}

	executed, finalResult, err := l.executeToolCalls(ctx, turnExec, iteration, result, validated.validCalls)
	if err != nil {
		return nil, nil, err
	}
	if finalResult != nil {
		return nil, finalResult, nil
	}
	if isCancelled(ctx) {
		return nil, nil, l.handleTurnCancellation(*inflight, ctx.Err())
	}

	toolResults = append(toolResults, l.finalizeExecutedToolResults(inflight, validated.validIndices, executed)...)
	return toolResults, nil, nil
}
