# Task 02: Event Emission Throughout the Turn

**Epic:** 06 — Agent Loop Core
**Status:** ⬚ Not started
**Dependencies:** Task 01, L5-E01 (EventSink, all event types)

---

## Description

Wire event emission into every phase of the turn state machine. The agent loop must emit the correct events at the correct times so the WebSocket handler (Layer 6/7) can provide real-time UI updates: status transitions, token streaming, thinking visualization, tool call lifecycle, turn completion, and errors. Events are emitted via the `EventSink` interface, which handles fan-out to multiple subscribers (WebSocket, logger, tests).

## Acceptance Criteria

- [ ] **Status transitions:** `StatusEvent(StateAssemblingContext)` emitted at turn start before context assembly. `StatusEvent(StateWaitingForLLM)` emitted before each LLM streaming call. `StatusEvent(StateExecutingTools)` emitted before tool dispatch. `StatusEvent(StateIdle)` emitted when turn completes
- [ ] **Context debug:** `ContextDebugEvent` emitted after context assembly completes, containing the assembly report (or summary fields). Emission is conditional on a configuration flag (debug mode)
- [ ] **Token streaming:** `TokenEvent` emitted for each text delta received from the provider's stream. Events are emitted as they arrive — no buffering or batching
- [ ] **Thinking lifecycle:** `ThinkingStartEvent` emitted when the first thinking delta is received. `ThinkingDeltaEvent` emitted for each thinking text chunk. `ThinkingEndEvent` emitted when thinking block completes
- [ ] **Tool call lifecycle:** `ToolCallStartEvent` emitted when tool dispatch begins for each tool call, including `ToolCallID`, `ToolName`, and `Arguments`. `ToolCallOutputEvent` emitted for incremental tool output (for v0.1, a single event with the full result is acceptable). `ToolCallEndEvent` emitted when tool execution completes, including result, duration, and success/failure status
- [ ] **Turn completion:** `TurnCompleteEvent` emitted with turn summary: turn number, iteration count, total input/output tokens, total duration
- [ ] **Turn cancellation:** `TurnCancelledEvent` emitted with turn number, completed iterations count, and cancellation reason
- [ ] **Errors:** `ErrorEvent` emitted for recoverable errors (tool failures, retried LLM errors) and non-recoverable errors (auth failure, exhausted retries). Includes error code, message, and `Recoverable` flag
- [ ] Tool call events include the `ToolCallID` so the UI can match start events to end events for the same tool call
- [ ] Event emission does not block the main turn execution — `EventSink.Emit()` is non-blocking by contract
- [ ] Package compiles with `go build ./internal/agent/...`
