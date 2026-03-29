# Task 03: Server-to-Client Event Forwarding

**Epic:** 04 — WebSocket Handler
**Status:** ⬚ Not started
**Dependencies:** Task 01, Layer 5 Epic 01 (event system & session types)

---

## Description

Implement the event forwarding pipeline that takes agent loop events from the EventSink channel and serializes them as JSON WebSocket frames to the client. All event types defined in the agent loop's streaming protocol must be forwarded: token deltas, thinking block lifecycle, tool call lifecycle, turn completion, cancellation, errors, status changes, and context debug reports. Concurrent tool call events must be interleaved correctly with tool call IDs preserved for frontend matching.

## Acceptance Criteria

- [ ] All agent loop event types are forwarded to the WebSocket client as JSON frames: `{"type": "<event_type>", "data": {...}}`
- [ ] **`token`** events forwarded with `{text: string}` — the text delta from the LLM
- [ ] **`thinking_start`** forwarded to signal the beginning of an extended thinking block
- [ ] **`thinking_delta`** forwarded with `{text: string}` — incremental thinking content
- [ ] **`thinking_end`** forwarded to signal the end of the thinking block
- [ ] **`tool_call_start`** forwarded with `{id, name, args}` — tool dispatch beginning
- [ ] **`tool_call_output`** forwarded with `{id, output}` — incremental tool output (e.g., streaming shell stdout)
- [ ] **`tool_call_end`** forwarded with `{id, result, duration_ms, success}` — tool execution complete
- [ ] **`turn_complete`** forwarded with usage summary (tokens_in, tokens_out, cache_read_tokens, iteration_count)
- [ ] **`turn_cancelled`** forwarded when the turn is cancelled by the user
- [ ] **`error`** forwarded with `{message, recoverable}` — distinguishes recoverable from non-recoverable errors
- [ ] **`status`** forwarded with `{state}` — agent state changes (idle, thinking, executing_tools)
- [ ] **`context_debug`** forwarded with the full `ContextAssemblyReport` as a JSON object — always sent, regardless of whether the frontend debug panel is open
- [ ] Concurrent tool call events are interleaved on the WebSocket — each event carries the tool call `id` so the frontend can match `tool_call_start` to `tool_call_output` and `tool_call_end`
- [ ] Events are sent in the order they are received from the EventSink channel — no reordering
- [ ] If the WebSocket connection is closed while events are queued, remaining events are dropped gracefully (no panic, no goroutine leak)
