package agent

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ponchione/sodoryard/internal/provider"
)

func TestSessionTurnIterationHierarchy(t *testing.T) {
	startedAt := time.Unix(1700000000, 0).UTC()
	completedAt := startedAt.Add(2 * time.Second)

	request := &provider.Request{Model: "test-model", MaxTokens: 256}
	response := &provider.Response{Model: "test-model", StopReason: provider.StopReasonEndTurn}
	toolArgs := json.RawMessage(`{"path":"internal/auth/service.go"}`)

	session := Session{
		ID:             "session-1",
		ConversationID: "conversation-1",
		StartedAt:      startedAt,
		Turns: []Turn{{
			Number:      1,
			UserMessage: "fix auth",
			Iterations: []Iteration{{
				Number:   1,
				Request:  request,
				Response: response,
				ToolCalls: []ToolCallRecord{{
					ID:        "toolu_1",
					ToolName:  "file_read",
					Arguments: toolArgs,
					Result:    "ok",
					Duration:  150 * time.Millisecond,
					Success:   true,
				}},
				StartedAt:   startedAt,
				CompletedAt: &completedAt,
			}},
			StartedAt:   startedAt,
			CompletedAt: &completedAt,
			TotalTokens: 42,
			Status:      TurnCompleted,
		}},
	}

	if got := session.Turns[0].Iterations[0].ToolCalls[0].ToolName; got != "file_read" {
		t.Fatalf("ToolName = %q, want file_read", got)
	}
	if got := session.Turns[0].Status.String(); got != "completed" {
		t.Fatalf("TurnStatus.String() = %q, want completed", got)
	}
	if got := StateExecutingTools.String(); got != "executing_tools" {
		t.Fatalf("AgentState.String() = %q, want executing_tools", got)
	}
}
