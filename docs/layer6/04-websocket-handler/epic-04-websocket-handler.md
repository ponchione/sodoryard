# Layer 6, Epic 04: WebSocket Handler

**Layer:** 6 — Web Interface & Streaming
**Status:** ⬚ Not started
**Dependencies:** [[layer-6-epic-01-http-server-foundation]], Layer 5 Epic 01 (event system & session types), Layer 5 Epic 02 (conversation manager), Layer 5 Epic 06 (agent loop)

---

## Description

Implement the WebSocket endpoint that bridges the agent loop to the browser. This is the most critical real-time component in Layer 6. When a client connects, the handler creates one EventSink for that WebSocket connection and subscribes it during setup. When the client sends a `message` event, the handler creates or resumes a conversation, calls `AgentLoop.RunTurn()`, and forwards all agent events received through that connection-scoped sink (token deltas, thinking blocks, tool call lifecycle, turn completion) to the client as JSON frames. It also handles `cancel` (aborts the in-flight turn) and `model_override` (changes the conversation's model).

The handler manages the WebSocket connection lifecycle: upgrade, concurrent read/write loops, heartbeat/keepalive, and clean disconnection. One WebSocket connection per browser tab, bound to one conversation at a time.

---

## Definition of Done

- [ ] `WS /api/ws` endpoint upgrades HTTP to WebSocket (using `gorilla/websocket` or `nhooyr.io/websocket` — agent's choice)
- [ ] **Client → Server: `message`** — Receives `{type: "message", data: {text: string, conversation_id?: string}}`. If `conversation_id` is provided, resumes that conversation. If absent, creates a new conversation via the conversation manager. Calls `AgentLoop.RunTurn(ctx, conversationID, text)`. Returns the conversation ID in an initial `conversation_id` event if newly created
- [ ] **Client → Server: `cancel`** — Calls `AgentLoop.Cancel()` for the in-flight turn. No-op if no turn is running
- [ ] **Client → Server: `model_override`** — Receives `{type: "model_override", data: {provider: string, model: string}}`. Updates the conversation's model setting for subsequent turns
- [ ] **Server → Client: all event types** from [[05-agent-loop]] §Streaming to the Web UI are forwarded:
  - `token` — text delta from LLM
  - `thinking_start`, `thinking_delta`, `thinking_end` — extended thinking lifecycle
  - `tool_call_start` — tool dispatch beginning (id, name, args)
  - `tool_call_output` — incremental tool output (streaming shell stdout)
  - `tool_call_end` — tool execution complete (id, result, duration, success/failure)
  - `turn_complete` — turn finished (usage summary, iteration count)
  - `turn_cancelled` — turn cancelled by user
  - `error` — recoverable or non-recoverable error
  - `status` — agent state change (idle, thinking, executing_tools)
  - `context_debug` — ContextAssemblyReport data (always sent; frontend decides whether to display)
- [ ] Events are serialized as JSON: `{"type": "<event_type>", "data": {...}}`
- [ ] **Concurrent tool calls** handled correctly: events from parallel tool executions are interleaved on the WebSocket, each tagged with the tool call ID so the frontend can match starts to ends
- [ ] **Connection lifecycle:** read loop and write loop run in separate goroutines. Write loop serializes events from the EventSink channel. Read loop processes client messages. One sink is subscribed per connection and unsubscribed during disconnect cleanup
- [ ] **Heartbeat:** periodic ping/pong to detect stale connections (e.g., every 30 seconds). Close connection if pong not received
- [ ] **Disconnect during turn:** if the WebSocket disconnects while a turn is running, the turn continues to completion (persisted to DB) but the disconnected sink is unsubscribed so future events are dropped cleanly. The client can reconnect and load the completed turn via the REST messages endpoint
- [ ] **One turn at a time:** if the client sends a `message` while a turn is already running, reject with an error event. The client must cancel or wait for `turn_complete` before sending another message
- [ ] Handler is registered on the server's router from [[layer-6-epic-01-http-server-foundation]]

---

## Architecture References

- [[07-web-interface-and-streaming]] — §WebSocket Streaming Protocol: all event types (proposed, now resolved). §Design Questions (resolved in [[layer-6-overview]])
- [[05-agent-loop]] — §Streaming to the Web UI: definitive event type list with fields. §The Loop Step by Step: RunTurn lifecycle. §Cancellation: cancel semantics, post-cancellation state
- [[05-agent-loop]] — §Tool Dispatch: pure vs mutating tool execution, parallel execution of pure tools — concurrent events need tool call IDs
- [[06-context-assembly]] — §Context Assembly Report: the data carried by the `context_debug` event

---

## Notes for the Implementing Agent

The agent loop exposes four methods relevant to the WebSocket layer: `RunTurn(ctx, conversationID, message)`, `Cancel()`, `Subscribe(sink EventSink)`, and `Unsubscribe(sink EventSink)`. The EventSink is a channel-based interface from Layer 5 Epic 01. The WebSocket handler creates one sink per connection during setup, subscribes it once, ranges over the channel in the write goroutine, then unsubscribes it on disconnect.

Key subtlety: `RunTurn` is a blocking call that returns when the turn completes. Run it in a goroutine — the write loop consumes events concurrently while RunTurn drives the agent loop. When RunTurn returns, the handler knows the turn is complete (the EventSink will have received `turn_complete` or `error`).

The `context_debug` event carries the full `ContextAssemblyReport` as JSON. It's emitted once per turn, right after context assembly completes (before the first LLM call). The frontend's context inspector panel ([[layer-6-epic-09-context-inspector]]) consumes this. Always emit it — the frontend decides whether to show it based on whether the debug panel is open.

For WebSocket library choice: `nhooyr.io/websocket` is more modern and has better context support. `gorilla/websocket` is more widely used. Either works — both support concurrent read/write, ping/pong, and graceful close.

The `conversation_id` response event on new conversations is not in the original Doc 07 event list — it's an addition needed so the frontend knows the ID of the newly created conversation (for URL routing and subsequent REST calls).
