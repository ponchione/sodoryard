# Task 05: Cancellation

**Epic:** 06 — Agent Loop Core
**Status:** ⬚ Not started
**Dependencies:** Task 01, Task 02 (TurnCancelledEvent emission), L5-E02 (CancelIteration)

---

## Description

Implement cancellation support for the agent loop covering all three scenarios: cancellation during an LLM streaming call, during tool execution, and the post-cancellation state cleanup. The user must be able to cancel a running turn at any point. Completed iterations are preserved (their data has already been persisted), while the in-flight iteration is cleanly discarded. The conversation remains in a consistent state after cancellation — ready for a new turn.

## Acceptance Criteria

- [ ] `Cancel()` method on `AgentLoop` that triggers cancellation of the in-progress turn
- [ ] Cancellation is also supported via the `ctx context.Context` passed to `RunTurn()` — if the context is cancelled (e.g., by the HTTP handler when the client disconnects), the turn is cancelled
- [ ] **During LLM call:** cancelling the context cancels the underlying HTTP request to the provider. Tokens already streamed and emitted via EventSink remain visible to the user (they saw them in real time). The partial assistant message from the in-flight streaming response is discarded — it is NOT persisted
- [ ] **During tool execution:** cancelling the context is propagated to the tool executor. For shell commands, the Layer 4 shell tool handles SIGTERM → 5s wait → SIGKILL. The agent loop does not need to manage the signal chain — it just cancels the context and waits for the executor to return
- [ ] **Post-cancellation state cleanup:** messages from completed iterations (already persisted via `PersistIteration`) are preserved. The in-flight iteration (not yet persisted) is discarded. If partial messages from the in-flight iteration were persisted (shouldn't happen with per-iteration atomicity, but as a safety measure), call `CancelIteration(conversationID, turnNumber, currentIteration)` to clean up
- [ ] After cancellation, `RunTurn` returns a cancellation error (e.g., `context.Canceled` or a typed `ErrTurnCancelled`). The conversation is in a consistent state — the next `RunTurn` call can proceed normally
- [ ] `TurnCancelledEvent` is emitted with turn number, completed iteration count, and reason ("user_cancelled" or "context_deadline_exceeded")
- [ ] `StatusEvent(StateIdle)` is emitted after cancellation cleanup completes
- [ ] Cancellation is idempotent — calling `Cancel()` multiple times or cancelling an already-idle loop is a no-op
- [ ] The `Cancel()` method is safe to call from any goroutine (thread-safe)
- [ ] Package compiles with `go build ./internal/agent/...`
