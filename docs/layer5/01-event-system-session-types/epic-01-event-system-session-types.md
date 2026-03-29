# L5-E01 — Event System & Session Types

**Layer:** 5 — Agent Loop
**Epic:** 01
**Status:** ⬚ Not started
**Dependencies:** Layer 0 Epic 01 (project scaffolding), Layer 3 Epic 01 (ContextAssemblyReport type reused by `ContextDebugEvent`)

---

## Description

Define the foundational agent-loop types that every Layer 5 component depends on. This includes the session/turn/iteration hierarchy (Session, Turn, Iteration structs), all streaming event types emitted by the agent loop (token, thinking, tool_call, turn_complete, error, status, etc.), and the EventSink interface that decouples the agent loop from the transport layer.

Context assembly types are not defined here. `ContextNeeds`, `FullContextPackage`, and `ContextAssemblyReport` are canonically owned by Layer 3 and reused by Layer 5 where needed.

---

## Definition of Done

- [ ] `internal/agent/types.go` defines `Session`, `Turn`, `Iteration` structs with the hierarchy from [[05-agent-loop]] §Core Concepts (session has turns, turns have iterations, iteration is one LLM roundtrip)
- [ ] `internal/agent/events.go` defines the full `Event` union type covering all server→client events from [[05-agent-loop]] §Streaming to the Web UI: `TokenEvent`, `ThinkingStartEvent`, `ThinkingDeltaEvent`, `ThinkingEndEvent`, `ToolCallStartEvent`, `ToolCallOutputEvent`, `ToolCallEndEvent`, `TurnCompleteEvent`, `TurnCancelledEvent`, `ErrorEvent`, `StatusEvent`, `ContextDebugEvent`
- [ ] Each event type includes the fields specified in [[05-agent-loop]] §Streaming to the Web UI (e.g., tool call events include tool call ID, status events include agent state enum)
- [ ] `ContextDebugEvent` reuses Layer 3's `ContextAssemblyReport` type rather than redefining context-assembly structs in Layer 5
- [ ] `internal/agent/eventsink.go` defines the `EventSink` interface — the abstraction through which the agent loop emits events without knowing about WebSocket, HTTP, or the frontend. At minimum: `Emit(event Event)`, `Close()`. The interface should support multiple subscribers (the WebSocket handler is one; a logger could be another)
- [ ] `internal/agent/state.go` defines the `AgentState` enum: `Idle`, `AssemblingContext`, `WaitingForLLM`, `ExecutingTools`, `Compressing`
- [ ] All types are exported and documented with godoc comments
- [ ] Package compiles with `go build ./internal/agent/...`

---

## Architecture References

- [[05-agent-loop]] §Core Concepts — Session/Turn/Iteration hierarchy
- [[05-agent-loop]] §Streaming to the Web UI — Event types and their fields
- [[06-context-assembly]] §Context Assembly Report — `ContextDebugEvent` payload type, owned by Layer 3
- [[07-web-interface-and-streaming]] §WebSocket Streaming Protocol — Event type definitions (TypeScript equivalents)

---

## Notes

- The EventSink interface is inspired by topham's `EventSink` pattern (Emit/ChannelSink) mentioned in [[05-agent-loop]] §What Ports from topham.
- Event types should be serializable to JSON for WebSocket transmission, but the serialization itself is Layer 6/7's concern. The types just need to be JSON-friendly (exported fields, no unexported embedded structs).
