# Task 04: AgentState Enum

**Epic:** 01 — Event System & Session Types
**Status:** ⬚ Not started
**Dependencies:** Layer 0 Epic 01 (project scaffolding)

---

## Description

Define the `AgentState` enum type in `internal/agent/state.go`. This type represents the current phase of the agent loop's execution and is included in `StatusEvent` emissions so the UI can display what the agent is doing (e.g., "Assembling context...", "Waiting for response...", "Running tools..."). The enum values correspond to the distinct phases of the turn state machine.

## Acceptance Criteria

- [ ] `AgentState` type defined as a string-based enum (using `type AgentState string` with constants)
- [ ] Constants defined: `StateIdle`, `StateAssemblingContext`, `StateWaitingForLLM`, `StateExecutingTools`, `StateCompressing`
- [ ] Each constant has a human-readable string value (e.g., `"idle"`, `"assembling_context"`, `"waiting_for_llm"`, `"executing_tools"`, `"compressing"`)
- [ ] `AgentState` implements `fmt.Stringer` if not already satisfied by the string type
- [ ] Godoc comment on the type explains the state machine transitions: Idle → AssemblingContext → WaitingForLLM → ExecutingTools → (back to WaitingForLLM or Compressing) → Idle
- [ ] Package compiles with `go build ./internal/agent/...`
