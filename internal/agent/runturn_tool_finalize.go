package agent

import (
	"github.com/ponchione/sodoryard/internal/conversation"
	"github.com/ponchione/sodoryard/internal/provider"
)

func (l *AgentLoop) finalizeExecutedToolResults(inflight *inflightTurn, validIndices []int, executed []toolExecutionRecord) []provider.ToolResult {
	toolResults := make([]provider.ToolResult, 0, len(executed))
	for i, record := range executed {
		toolResult := record.Result
		toolResult.ToolUseID = record.Call.ID
		toolResults = append(toolResults, toolResult)

		idx := validIndices[i]
		inflight.ToolCalls[idx].Completed = true
		inflight.ToolCalls[idx].ResultStored = true
		inflight.ToolMessages = append(inflight.ToolMessages, conversation.IterationMessage{
			Role:      "tool",
			Content:   toolResult.Content,
			ToolUseID: record.Call.ID,
			ToolName:  record.Call.Name,
		})

		if toolResult.IsError {
			l.emit(ErrorEvent{
				ErrorCode:   ErrorCodeToolExecution,
				Message:     toolResult.Content,
				Recoverable: true,
				Time:        l.now(),
			})
		}
		l.emit(ToolCallOutputEvent{ToolCallID: record.Call.ID, Output: toolResult.Content, Time: l.now()})
		l.emit(ToolCallEndEvent{ToolCallID: record.Call.ID, Result: toolResult.Content, Details: toolResult.Details, Duration: record.Duration, Success: !toolResult.IsError, Time: l.now()})
	}
	return toolResults
}
