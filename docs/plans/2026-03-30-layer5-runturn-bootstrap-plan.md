# Layer 5 RunTurn Bootstrap Plan

> For Hermes: execute exactly one narrow slice, using strict TDD, one local commit, and no push.

Goal: add the first persistence-aware agent-loop entrypoint that records the user message at turn start and then delegates to the existing Layer 3-backed `PrepareTurnContext(...)` seam.

Architecture:
- Keep scope inside `internal/agent` only.
- Reuse the current `PrepareTurnContext(...)` seam instead of widening into prompt building, provider streaming, or tool execution.
- Extend the narrow `ConversationManager` boundary only as much as needed for turn-start persistence.

Tech stack: Go, `internal/agent`, existing Layer 3 `internal/context` contracts, TDD.

---

## Proposed slice

Implement a bootstrap `RunTurn(...)` entrypoint that:
1. validates input and required deps
2. calls `ConversationManager.PersistUserMessage(...)`
3. delegates to `PrepareTurnContext(...)`
4. returns the resulting `TurnStartResult`
5. emits `ErrorEvent` when the persistence step fails

This is intentionally not the full turn state machine.

## Files likely to change
- Modify: `internal/agent/loop.go`
- Modify: `internal/agent/loop_test.go`

## TDD steps
1. Add failing tests for `RunTurn(...)` and the new conversation-manager persistence boundary.
2. Run `go test ./internal/agent/...` and confirm RED on missing symbols/behavior.
3. Implement the minimal code to pass.
4. Re-run:
   - `go test ./internal/agent/...`
   - `go test -race ./internal/agent/...`
   - `go build ./internal/agent/...`
5. Commit only the `internal/agent/loop*` changes.

## Test cases to add
- successful `RunTurn(...)` persists the user message before calling history reconstruction / context assembly
- returned `TurnStartResult` from `PrepareTurnContext(...)` is preserved
- persistence error returns a wrapped error and emits `ErrorEvent`
- invalid request data returns a useful validation error

## Deliberate non-goals
- no prompt builder integration
- no provider calls / streaming
- no tool execution
- no compression engine invocation
- no final assistant persistence
- no post-turn `UpdateQuality(...)`

## Suggested commit message
- `feat(agent): persist user message at turn start`
