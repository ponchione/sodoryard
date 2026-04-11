package agent

import (
	"encoding/json"
	"time"

	"github.com/ponchione/sodoryard/internal/provider"
)

// TurnStatus describes the lifecycle state of a single user turn.
type TurnStatus string

const (
	// TurnInProgress means the turn is still executing.
	TurnInProgress TurnStatus = "in_progress"
	// TurnCompleted means the turn ended with a final assistant response.
	TurnCompleted TurnStatus = "completed"
	// TurnCancelled means the turn was cancelled before completion.
	TurnCancelled TurnStatus = "cancelled"
)

// String returns the human-readable status value.
func (s TurnStatus) String() string {
	return string(s)
}

// ToolCallRecord captures one tool invocation within an iteration.
type ToolCallRecord struct {
	ID        string          `json:"id"`
	ToolName  string          `json:"tool_name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	Result    string          `json:"result,omitempty"`
	Duration  time.Duration   `json:"duration,omitempty"`
	Success   bool            `json:"success,omitempty"`
}

// Iteration is one LLM request/response roundtrip within a turn.
type Iteration struct {
	Number      int                `json:"number"`
	Request     *provider.Request  `json:"request,omitempty"`
	Response    *provider.Response `json:"response,omitempty"`
	ToolCalls   []ToolCallRecord   `json:"tool_calls,omitempty"`
	StartedAt   time.Time          `json:"started_at"`
	CompletedAt *time.Time         `json:"completed_at,omitempty"`
}

// Turn is one user message plus every iteration required to finish it.
type Turn struct {
	Number      int         `json:"number"`
	UserMessage string      `json:"user_message"`
	Iterations  []Iteration `json:"iterations,omitempty"`
	StartedAt   time.Time   `json:"started_at"`
	CompletedAt *time.Time  `json:"completed_at,omitempty"`
	TotalTokens int         `json:"total_tokens,omitempty"`
	Status      TurnStatus  `json:"status"`
}

// Session is the top-level conversation runtime: a session contains turns.
type Session struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversation_id"`
	StartedAt      time.Time `json:"started_at"`
	Turns          []Turn    `json:"turns,omitempty"`
}
