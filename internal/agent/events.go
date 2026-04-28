package agent

import (
	"encoding/json"
	"time"

	contextpkg "github.com/ponchione/sodoryard/internal/context"
)

const (
	eventTypeToken          = "token"
	eventTypeThinkingStart  = "thinking_start"
	eventTypeThinkingDelta  = "thinking_delta"
	eventTypeThinkingEnd    = "thinking_end"
	eventTypeToolCallStart  = "tool_call_start"
	eventTypeToolCallOutput = "tool_call_output"
	eventTypeToolCallEnd    = "tool_call_end"
	eventTypeTurnComplete   = "turn_complete"
	eventTypeTurnCancelled  = "turn_cancelled"
	eventTypeError          = "error"
	eventTypeStatus         = "status"
	eventTypeContextDebug   = "context_debug"
)

// Event is the common interface implemented by every server-to-client agent event.
type Event interface {
	EventType() string
	Timestamp() time.Time
}

// TokenEvent carries a streamed visible text delta from the LLM.
type TokenEvent struct {
	Type  string    `json:"type"`
	Token string    `json:"token"`
	Time  time.Time `json:"time"`
}

func (e TokenEvent) EventType() string    { return resolveEventType(e.Type, eventTypeToken) }
func (e TokenEvent) Timestamp() time.Time { return e.Time }

// ThinkingStartEvent marks the beginning of a streamed thinking block.
type ThinkingStartEvent struct {
	Type string    `json:"type"`
	Time time.Time `json:"time"`
}

func (e ThinkingStartEvent) EventType() string {
	return resolveEventType(e.Type, eventTypeThinkingStart)
}
func (e ThinkingStartEvent) Timestamp() time.Time { return e.Time }

// ThinkingDeltaEvent carries an incremental thinking delta.
type ThinkingDeltaEvent struct {
	Type  string    `json:"type"`
	Delta string    `json:"delta"`
	Time  time.Time `json:"time"`
}

func (e ThinkingDeltaEvent) EventType() string {
	return resolveEventType(e.Type, eventTypeThinkingDelta)
}
func (e ThinkingDeltaEvent) Timestamp() time.Time { return e.Time }

// ThinkingEndEvent marks the end of a streamed thinking block.
type ThinkingEndEvent struct {
	Type string    `json:"type"`
	Time time.Time `json:"time"`
}

func (e ThinkingEndEvent) EventType() string    { return resolveEventType(e.Type, eventTypeThinkingEnd) }
func (e ThinkingEndEvent) Timestamp() time.Time { return e.Time }

// ToolCallStartEvent announces that a tool call is beginning.
type ToolCallStartEvent struct {
	Type       string          `json:"type"`
	ToolCallID string          `json:"tool_call_id"`
	ToolName   string          `json:"tool_name"`
	Arguments  json.RawMessage `json:"arguments,omitempty"`
	Time       time.Time       `json:"time"`
}

func (e ToolCallStartEvent) EventType() string {
	return resolveEventType(e.Type, eventTypeToolCallStart)
}
func (e ToolCallStartEvent) Timestamp() time.Time { return e.Time }

// ToolCallOutputEvent carries incremental tool output.
type ToolCallOutputEvent struct {
	Type       string    `json:"type"`
	ToolCallID string    `json:"tool_call_id"`
	Output     string    `json:"output,omitempty"`
	Time       time.Time `json:"time"`
}

func (e ToolCallOutputEvent) EventType() string {
	return resolveEventType(e.Type, eventTypeToolCallOutput)
}
func (e ToolCallOutputEvent) Timestamp() time.Time { return e.Time }

// ToolCallEndEvent records the final result of a tool call.
type ToolCallEndEvent struct {
	Type       string          `json:"type"`
	ToolCallID string          `json:"tool_call_id"`
	Result     string          `json:"result,omitempty"`
	Details    json.RawMessage `json:"details,omitempty"`
	Duration   time.Duration   `json:"duration,omitempty"`
	Success    bool            `json:"success,omitempty"`
	Time       time.Time       `json:"time"`
}

func (e ToolCallEndEvent) EventType() string    { return resolveEventType(e.Type, eventTypeToolCallEnd) }
func (e ToolCallEndEvent) Timestamp() time.Time { return e.Time }

// TurnCompleteEvent reports a finished turn and its aggregate usage.
type TurnCompleteEvent struct {
	Type              string        `json:"type"`
	TurnNumber        int           `json:"turn_number"`
	IterationCount    int           `json:"iteration_count"`
	TotalInputTokens  int           `json:"total_input_tokens,omitempty"`
	TotalOutputTokens int           `json:"total_output_tokens,omitempty"`
	Duration          time.Duration `json:"duration,omitempty"`
	Time              time.Time     `json:"time"`
}

func (e TurnCompleteEvent) EventType() string    { return resolveEventType(e.Type, eventTypeTurnComplete) }
func (e TurnCompleteEvent) Timestamp() time.Time { return e.Time }

// TurnCancelledEvent reports a user-cancelled turn.
type TurnCancelledEvent struct {
	Type                string    `json:"type"`
	TurnNumber          int       `json:"turn_number"`
	CompletedIterations int       `json:"completed_iterations,omitempty"`
	Reason              string    `json:"reason,omitempty"`
	Time                time.Time `json:"time"`
}

func (e TurnCancelledEvent) EventType() string {
	return resolveEventType(e.Type, eventTypeTurnCancelled)
}
func (e TurnCancelledEvent) Timestamp() time.Time { return e.Time }

// ErrorEvent carries a recoverable or terminal agent-loop error.
type ErrorEvent struct {
	Type        string    `json:"type"`
	ErrorCode   string    `json:"error_code,omitempty"`
	Message     string    `json:"message,omitempty"`
	Recoverable bool      `json:"recoverable,omitempty"`
	Time        time.Time `json:"time"`
}

func (e ErrorEvent) EventType() string    { return resolveEventType(e.Type, eventTypeError) }
func (e ErrorEvent) Timestamp() time.Time { return e.Time }

// StatusEvent reports a state-machine transition for the running agent loop.
type StatusEvent struct {
	Type  string     `json:"type"`
	State AgentState `json:"state"`
	Time  time.Time  `json:"time"`
}

func (e StatusEvent) EventType() string    { return resolveEventType(e.Type, eventTypeStatus) }
func (e StatusEvent) Timestamp() time.Time { return e.Time }

// ContextDebugEvent emits the full Layer 3 context-assembly report for a turn.
type ContextDebugEvent struct {
	Type   string                            `json:"type"`
	Report *contextpkg.ContextAssemblyReport `json:"report,omitempty"`
	Time   time.Time                         `json:"time"`
}

func (e ContextDebugEvent) EventType() string    { return resolveEventType(e.Type, eventTypeContextDebug) }
func (e ContextDebugEvent) Timestamp() time.Time { return e.Time }

func resolveEventType(actual string, fallback string) string {
	if actual != "" {
		return actual
	}
	return fallback
}
