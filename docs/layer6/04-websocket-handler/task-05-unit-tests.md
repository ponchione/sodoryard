# Task 05: Unit Tests

**Epic:** 04 — WebSocket Handler
**Status:** ⬚ Not started
**Dependencies:** Tasks 01-04

---

## Description

Write unit tests for the WebSocket handler covering the key behaviors: connection lifecycle, message handling, event forwarding, heartbeat, disconnect handling, and one-turn-at-a-time enforcement. Tests should use mock implementations of the agent loop and conversation manager to isolate the WebSocket handler logic. Use `httptest` and the WebSocket library's test helpers to establish in-process WebSocket connections without a real network.

## Acceptance Criteria

- [ ] **Connection test:** Verify that an HTTP request to `/api/ws` successfully upgrades to a WebSocket connection and that the connection can send and receive JSON messages
- [ ] **Message handling test:** Verify that sending a `{type: "message", data: {text: "hello"}}` frame triggers the agent loop's `RunTurn` method with the correct parameters
- [ ] **New conversation test:** Verify that sending a message without `conversation_id` creates a new conversation via the conversation manager and returns a `conversation_id` event
- [ ] **Cancel test:** Verify that sending `{type: "cancel"}` calls `AgentLoop.Cancel()`. Verify cancel is a no-op when no turn is running
- [ ] **Event forwarding test:** Verify that events emitted by the mock agent loop (token, thinking, tool_call, turn_complete, etc.) are serialized as JSON and received by the WebSocket client
- [ ] **Concurrent tool calls test:** Verify that events from parallel tool executions are forwarded with correct tool call IDs and can be matched by the client
- [ ] **One-turn-at-a-time test:** Verify that sending a second `message` while a turn is in progress returns an error event rather than starting a second turn
- [ ] **Disconnect test:** Verify that closing the WebSocket connection while a turn is in progress does not panic or leak goroutines. Verify the turn continues to completion (mock verifies `RunTurn` was not cancelled)
- [ ] **Disconnect cleanup test:** Verify disconnect cleanup calls `AgentLoop.Unsubscribe(sink)` exactly once for the per-connection sink before that sink is closed, so dead subscribers are removed from the loop
- [ ] **Malformed message test:** Verify that sending invalid JSON or an unknown message type results in an error event but does not close the connection
- [ ] Tests use mock/stub implementations of the agent loop and conversation manager interfaces — no real database or LLM calls
- [ ] All tests pass with `go test ./internal/server/... -v`
- [ ] No race conditions detected with `go test -race ./internal/server/...`
