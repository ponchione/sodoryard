# Task 01: WebSocket Upgrade and Connection Lifecycle

**Epic:** 04 — WebSocket Handler
**Status:** ⬚ Not started
**Dependencies:** Epic 01 (HTTP Server Foundation), Layer 5 Epic 01 (ChannelSink), Layer 5 Epic 06 (Subscribe/Unsubscribe API)

---

## Description

Implement the WebSocket endpoint at `/api/ws` that upgrades HTTP connections to WebSocket and manages the connection lifecycle. This includes the HTTP-to-WebSocket upgrade, spawning concurrent read and write goroutines, and clean connection teardown. The read loop processes incoming client messages and dispatches them to the appropriate handler (message, cancel, model_override). The write loop consumes events from the EventSink channel and serializes them as JSON frames to the client. Both loops coordinate shutdown when the connection closes.

## Acceptance Criteria

- [ ] `WS /api/ws` endpoint registered on the server's router from Epic 01
- [ ] HTTP-to-WebSocket upgrade using `gorilla/websocket` or `nhooyr.io/websocket` (agent's choice)
- [ ] Upgrade handler validates the WebSocket handshake and returns HTTP 400 if the upgrade fails
- [ ] After upgrade, create one per-connection `ChannelSink`, subscribe it to the agent loop, and spawn two goroutines: a read loop (client -> server) and a write loop (server -> client)
- [ ] Read loop deserializes incoming JSON messages into `{type: string, data: any}` format and dispatches based on `type`
- [ ] Write loop reads events from an EventSink channel and serializes each as `{"type": "<event_type>", "data": {...}}` JSON frames
- [ ] Malformed JSON from the client is logged and an error event is sent back — the connection is NOT closed on a single bad message
- [ ] When the WebSocket connection closes (client disconnect, network drop), both goroutines clean up and exit
- [ ] Connection teardown unregisters the connection's `EventSink` via `AgentLoop.Unsubscribe(sink)` before the sink is closed, so future emits do not target a dead subscriber
- [ ] Connection cleanup cancels any context associated with the connection (used for in-flight operations)
- [ ] The handler tracks connection state (connected/disconnected) to avoid writing to a closed connection
- [ ] One WebSocket connection per handler invocation — no connection multiplexing
