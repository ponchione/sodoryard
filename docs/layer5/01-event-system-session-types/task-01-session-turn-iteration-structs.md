# Task 01: Session, Turn, Iteration Hierarchy Structs

**Epic:** 01 — Event System & Session Types
**Status:** ⬚ Not started
**Dependencies:** Layer 0 Epic 01 (project scaffolding)

---

## Description

Define the core session hierarchy types in `internal/agent/types.go`. These structs model the three-level nesting from the architecture: a Session contains Turns, a Turn contains Iterations. A Session maps to one conversation lifecycle (create → user messages → done). A Turn is one user message plus all LLM roundtrips until a text-only response. An Iteration is a single LLM request/response roundtrip within a turn. These types are referenced by the rest of the agent-loop runtime, conversation persistence code, and the Layer 3/Layer 6 boundaries that consume turn-level metadata.

## Acceptance Criteria

- [ ] `Session` struct defined with fields: `ID string` (UUIDv7), `ConversationID string`, `StartedAt time.Time`, `Turns []Turn`
- [ ] `Turn` struct defined with fields: `Number int`, `UserMessage string`, `Iterations []Iteration`, `StartedAt time.Time`, `CompletedAt *time.Time`, `TotalTokens int`, `Status TurnStatus` (enum: InProgress, Completed, Cancelled)
- [ ] `Iteration` struct defined with fields: `Number int`, `Request *provider.Request` (or a reference to the LLM call), `Response *provider.Response`, `ToolCalls []ToolCallRecord`, `StartedAt time.Time`, `CompletedAt *time.Time`
- [ ] `ToolCallRecord` struct defined with fields: `ID string`, `ToolName string`, `Arguments json.RawMessage`, `Result string`, `Duration time.Duration`, `Success bool`
- [ ] `TurnStatus` enum type with constants `TurnInProgress`, `TurnCompleted`, `TurnCancelled`
- [ ] All types are exported with godoc comments explaining the hierarchy relationship
- [ ] Package compiles with `go build ./internal/agent/...`
