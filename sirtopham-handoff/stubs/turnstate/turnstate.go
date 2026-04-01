package turnstate

import "context"

// CleanupReason distinguishes why the turn ended unexpectedly.
type CleanupReason string

const (
	CleanupReasonCancel       CleanupReason = "cancel"
	CleanupReasonInterrupt    CleanupReason = "interrupt"
	CleanupReasonStreamFail   CleanupReason = "stream_failure"
	CleanupReasonToolFail     CleanupReason = "tool_failure"
	CleanupReasonShutdown     CleanupReason = "shutdown"
)

// InflightToolCall tracks tool execution state that may need finalization.
type InflightToolCall struct {
	ToolCallID   string
	ToolName     string
	Started      bool
	Completed    bool
	ResultStored bool
}

// InflightTurn tracks partial assistant and tool state for the active iteration.
type InflightTurn struct {
	SessionID              string
	TurnID                 string
	IterationID            string
	AssistantMessageID     string
	AssistantStreamStarted bool
	AssistantStreamClosed  bool
	ToolCalls              []InflightToolCall
}

// CleanupAction describes one durable-state mutation.
type CleanupAction struct {
	Kind    string
	Target  string
	Payload string
}

// CleanupPlan is the deterministic set of actions needed to restore transcript invariants.
type CleanupPlan struct {
	Reason  CleanupReason
	Actions []CleanupAction
}

// Repository applies durable transcript mutations.
type Repository interface {
	ApplyCleanup(ctx context.Context, plan CleanupPlan) error
}

// Planner computes the cleanup plan from in-flight state.
type Planner interface {
	Plan(ctx context.Context, turn InflightTurn, reason CleanupReason) (CleanupPlan, error)
}

// Executor orchestrates cleanup application.
type Executor interface {
	Cleanup(ctx context.Context, turn InflightTurn, reason CleanupReason) error
}

// Suggested cleanup behaviors:
// - Completed tool calls remain durable.
// - Assistant output that never completed should not look complete in the transcript.
// - Started tool calls lacking terminal state may need a tombstone or synthesized terminal record.
// - Interrupt should remain distinguishable from generic cancel for later analytics and UX.
