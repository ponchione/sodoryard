# Task 03: EventSink Interface

**Epic:** 01 — Event System & Session Types
**Status:** ⬚ Not started
**Dependencies:** Task 02

---

## Description

Define the `EventSink` interface in `internal/agent/eventsink.go`. This is the abstraction through which the agent loop emits events without coupling to any specific transport (WebSocket, HTTP SSE, logging, testing). The interface must support multiple subscribers so that the WebSocket handler, a structured logger, and test assertions can all receive the same events simultaneously. Include a `MultiSink` implementation that fans out events to multiple subscribers, and a `ChannelSink` implementation backed by a Go channel for asynchronous consumption.

## Acceptance Criteria

- [ ] `EventSink` interface defined with methods: `Emit(event Event)` and `Close()`
- [ ] `Emit` is non-blocking — implementations must not block the agent loop's goroutine. If a subscriber is slow, events are dropped or buffered, not blocked
- [ ] `Close()` signals that no more events will be emitted, allowing subscribers to clean up
- [ ] `MultiSink` struct that wraps multiple `EventSink` implementations and fans out each `Emit` call to all registered sinks
- [ ] `MultiSink` has `Add(sink EventSink)` and `Remove(sink EventSink)` methods for dynamic subscriber management. These are the internal primitives used by `AgentLoop.Subscribe()` and `AgentLoop.Unsubscribe()` when WebSocket connections come and go during a session
- [ ] `ChannelSink` struct backed by a buffered `chan Event` for asynchronous consumption. Buffer size is configurable (default 256). Events are dropped with a log warning if the channel is full
- [ ] `NewMultiSink() *MultiSink` and `NewChannelSink(bufferSize int) *ChannelSink` constructors
- [ ] `ChannelSink` exposes `Events() <-chan Event` for consumer read access
- [ ] Thread-safe: `MultiSink.Add`, `MultiSink.Remove`, and `MultiSink.Emit` can be called concurrently from different goroutines
- [ ] Package compiles with `go build ./internal/agent/...`
