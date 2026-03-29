# Task 02: Streaming Event Types

**Epic:** 01 — Event System & Session Types
**Status:** ⬚ Not started
**Dependencies:** Task 01, Layer 3 Epic 01 (ContextAssemblyReport type)

---

## Description

Define the full set of server-to-client streaming event types in `internal/agent/events.go`. These events are emitted by the agent loop during turn execution and consumed by the WebSocket handler (Layer 6/7) for real-time UI updates. Each event type is a concrete struct implementing a common `Event` interface. The union covers all phases of turn execution: token streaming, extended thinking, tool call lifecycle, turn completion/cancellation, errors, status transitions, and context debug information.

## Acceptance Criteria

- [ ] `Event` interface defined with methods: `EventType() string` (returns the type discriminator string), `Timestamp() time.Time`
- [ ] `TokenEvent` struct: `Type string`, `Token string` (the text delta), `Time time.Time`
- [ ] `ThinkingStartEvent` struct: `Type string`, `Time time.Time`
- [ ] `ThinkingDeltaEvent` struct: `Type string`, `Delta string` (thinking text chunk), `Time time.Time`
- [ ] `ThinkingEndEvent` struct: `Type string`, `Time time.Time`
- [ ] `ToolCallStartEvent` struct: `Type string`, `ToolCallID string`, `ToolName string`, `Arguments json.RawMessage`, `Time time.Time`
- [ ] `ToolCallOutputEvent` struct: `Type string`, `ToolCallID string`, `Output string` (incremental output), `Time time.Time`
- [ ] `ToolCallEndEvent` struct: `Type string`, `ToolCallID string`, `Result string`, `Duration time.Duration`, `Success bool`, `Time time.Time`
- [ ] `TurnCompleteEvent` struct: `Type string`, `TurnNumber int`, `IterationCount int`, `TotalInputTokens int`, `TotalOutputTokens int`, `Duration time.Duration`, `Time time.Time`
- [ ] `TurnCancelledEvent` struct: `Type string`, `TurnNumber int`, `CompletedIterations int`, `Reason string`, `Time time.Time`
- [ ] `ErrorEvent` struct: `Type string`, `ErrorCode string`, `Message string`, `Recoverable bool`, `Time time.Time`
- [ ] `StatusEvent` struct: `Type string`, `State AgentState`, `Time time.Time`
- [ ] `ContextDebugEvent` struct: `Type string`, `Report *ContextAssemblyReport` (reusing the Layer 3 report type, or summary fields), `Time time.Time`
- [ ] All event structs implement the `Event` interface
- [ ] All structs use exported fields with JSON-compatible types (no unexported embedded structs, no channels, no func fields)
- [ ] Package compiles with `go build ./internal/agent/...`
