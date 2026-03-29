# Task 04: Heartbeat, Disconnect Handling, and One-Turn-at-a-Time Enforcement

**Epic:** 04 — WebSocket Handler
**Status:** ⬚ Not started
**Dependencies:** Task 02, Task 03

---

## Description

Implement the reliability and safety mechanisms for the WebSocket connection: periodic heartbeat ping/pong to detect stale connections, clean handling of client disconnection during an in-flight turn, and enforcement that only one turn runs at a time per connection. These mechanisms ensure the system remains robust under real-world conditions (network drops, browser tab closes, impatient users double-sending messages).

## Acceptance Criteria

- [ ] **Heartbeat:** Server sends WebSocket ping frames at a regular interval (e.g., every 30 seconds)
- [ ] If the client does not respond with a pong within a configurable timeout (e.g., 10 seconds), the connection is closed as stale
- [ ] Heartbeat runs in the write goroutine or a dedicated ticker goroutine and does not interfere with event forwarding
- [ ] **Disconnect during turn:** If the WebSocket disconnects while `RunTurn` is in progress, the turn continues to completion (data is persisted to the database) but events are dropped silently
- [ ] No goroutine leak on disconnect — the write loop detects the closed connection and exits, the RunTurn goroutine finishes naturally
- [ ] The client can reconnect after disconnect and load the completed turn via the REST `GET /api/conversations/:id/messages` endpoint
- [ ] **One turn at a time:** If the client sends a `message` while a turn is already running, the handler responds with an error event: `{"type": "error", "data": {"message": "a turn is already in progress — cancel or wait for completion"}}`
- [ ] The turn-in-progress flag is set when `RunTurn` starts and cleared when `turn_complete`, `turn_cancelled`, or `error` (non-recoverable) is received
- [ ] The flag is safely accessed from both the read and write goroutines (uses a mutex or atomic operation)
- [ ] After a `turn_cancelled` event, the client can immediately send a new `message` — the turn-in-progress flag is cleared
