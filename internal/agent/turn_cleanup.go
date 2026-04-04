package agent

import (
	stdctx "context"
	"fmt"
	"strings"

	"github.com/ponchione/sirtopham/internal/conversation"
)

type turnCleanupReason string

const (
	cleanupReasonCancel           turnCleanupReason = "cancel"
	cleanupReasonInterrupt        turnCleanupReason = "interrupt"
	cleanupReasonDeadlineExceeded turnCleanupReason = "context_deadline_exceeded"
	cleanupReasonStreamFailure    turnCleanupReason = "stream_failure"
)

const (
	cleanupActionCancelIteration = "cancel_iteration"
	cleanupActionPersistIteration = "persist_iteration"
)

type inflightToolCall struct {
	ToolCallID   string
	ToolName     string
	Started      bool
	Completed    bool
	ResultStored bool
}

type inflightTurn struct {
	ConversationID           string
	TurnNumber               int
	Iteration                int
	CompletedIterations      int
	AssistantResponseStarted bool
	AssistantResponseStored  bool
	AssistantMessageContent  string
	ToolCalls                []inflightToolCall
	ToolMessages             []conversation.IterationMessage
}

type cleanupAction struct {
	Kind      string
	Iteration int
	Messages  []conversation.IterationMessage
}

type cleanupPlan struct {
	Reason  turnCleanupReason
	Actions []cleanupAction
}

// buildCleanupPlan preserves already-materialized assistant/tool state when it
// would be useful to future turns, and only falls back to CancelIteration when
// nothing durable should remain from the interrupted iteration.
func buildCleanupPlan(turn inflightTurn, reason turnCleanupReason) cleanupPlan {
	plan := cleanupPlan{Reason: reason}
	if turn.Iteration <= 0 || turn.Iteration <= turn.CompletedIterations {
		return plan
	}
	if !turn.AssistantResponseStarted && len(turn.ToolCalls) == 0 {
		return plan
	}

	if messages := buildInterruptedIterationMessages(turn, reason); len(messages) > 0 {
		plan.Actions = append(plan.Actions, cleanupAction{
			Kind:      cleanupActionPersistIteration,
			Iteration: turn.Iteration,
			Messages:  messages,
		})
		return plan
	}

	plan.Actions = append(plan.Actions, cleanupAction{
		Kind:      cleanupActionCancelIteration,
		Iteration: turn.Iteration,
	})
	return plan
}

func buildInterruptedIterationMessages(turn inflightTurn, reason turnCleanupReason) []conversation.IterationMessage {
	if turn.AssistantMessageContent == "" {
		return nil
	}

	assistantContent := turn.AssistantMessageContent
	if len(turn.ToolCalls) == 0 && len(turn.ToolMessages) == 0 {
		assistantContent = interruptedAssistantMessageContent(turn.AssistantMessageContent, reason)
	}

	messages := []conversation.IterationMessage{{
		Role:    "assistant",
		Content: assistantContent,
	}}
	messages = append(messages, append([]conversation.IterationMessage(nil), turn.ToolMessages...)...)

	for _, tc := range turn.ToolCalls {
		if tc.ResultStored {
			continue
		}
		messages = append(messages, conversation.IterationMessage{
			Role:      "tool",
			Content:   interruptedToolResultContent(tc, reason),
			ToolUseID: tc.ToolCallID,
			ToolName:  tc.ToolName,
		})
	}

	if len(turn.ToolCalls) == 0 && len(turn.ToolMessages) == 0 {
		return messages
	}
	return messages
}

func interruptedAssistantMessageContent(raw string, reason turnCleanupReason) string {
	kind := "[interrupted_assistant]"
	messageText := "Assistant output was interrupted before turn completion."
	if reason == cleanupReasonStreamFailure {
		kind = "[failed_assistant]"
		messageText = "Assistant output ended due to a stream failure before turn completion."
	}
	message := kind + "\nreason=" + string(reason) + "\nmessage=" + messageText
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return raw
	}
	return strings.Replace(raw, `"text":"`, `"text":"`+message+`\npartial_text=`, 1)
}

func interruptedToolResultContent(tc inflightToolCall, reason turnCleanupReason) string {
	status := "cancelled_before_execution"
	if tc.Started {
		status = "interrupted_during_execution"
	}
	return strings.Join([]string{
		"[interrupted_tool_result]",
		"reason=" + string(reason),
		"tool=" + tc.ToolName,
		"tool_use_id=" + tc.ToolCallID,
		"status=" + status,
		"message=Tool execution did not complete before the turn ended.",
	}, "\n")
}

func (l *AgentLoop) applyCleanupPlan(ctx stdctx.Context, turn inflightTurn, plan cleanupPlan) error {
	for _, action := range plan.Actions {
		switch action.Kind {
		case cleanupActionCancelIteration:
			if err := l.conversationManager.CancelIteration(ctx, turn.ConversationID, turn.TurnNumber, action.Iteration); err != nil {
				return fmt.Errorf("cancel iteration %d: %w", action.Iteration, err)
			}
		case cleanupActionPersistIteration:
			if err := l.conversationManager.PersistIteration(ctx, turn.ConversationID, turn.TurnNumber, action.Iteration, action.Messages); err != nil {
				return fmt.Errorf("persist interrupted iteration %d: %w", action.Iteration, err)
			}
		default:
			return fmt.Errorf("unknown cleanup action kind %q", action.Kind)
		}
	}
	return nil
}

func cleanupReasonEventValue(reason turnCleanupReason) string {
	switch reason {
	case cleanupReasonInterrupt:
		return "user_interrupted"
	case cleanupReasonDeadlineExceeded:
		return "context_deadline_exceeded"
	case cleanupReasonStreamFailure:
		return "stream_failure"
	default:
		return "user_cancelled"
	}
}
