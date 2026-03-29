# Task 08: Event Subscription Lifecycle

**Epic:** 06 — Agent Loop Core
**Status:** ⬚ Not started
**Dependencies:** Task 01, L5-E01 (EventSink, MultiSink)

---

## Description

Implement the dynamic event subscription lifecycle for the agent loop. The loop emits events throughout turn execution, but the set of consumers is dynamic: a WebSocket connection subscribes when it connects and must unsubscribe when it disconnects, while logger/test sinks may stay attached for the lifetime of the process. This task makes that contract explicit by giving `AgentLoop` public `Subscribe` and `Unsubscribe` methods backed by its internal `MultiSink` fan-out.

## Acceptance Criteria

- [ ] `AgentLoop` owns an internal `MultiSink` used for all event fan-out during turn execution
- [ ] `NewAgentLoop(deps AgentLoopDeps)` registers `deps.EventSink` as an initial subscriber when one is provided, so process-wide sinks (logger, tests) still work without extra setup
- [ ] `Subscribe(sink EventSink)` method implemented on `AgentLoop`; delegates to the internal `MultiSink.Add(sink)`
- [ ] `Unsubscribe(sink EventSink)` method implemented on `AgentLoop`; delegates to the internal `MultiSink.Remove(sink)`
- [ ] `Subscribe` and `Unsubscribe` are safe to call concurrently with `RunTurn` and with ongoing event emission
- [ ] `Unsubscribe` is idempotent — removing a sink that is not currently subscribed is a no-op
- [ ] Event emission sites inside the agent loop continue to emit through a single fan-out sink abstraction; they do not need to know how many subscribers exist
- [ ] The WebSocket layer can subscribe one sink per connection during setup and unsubscribe that same sink on disconnect without affecting any other subscribers
- [ ] Ownership is explicit: callers that create a sink remain responsible for calling `sink.Close()` after unsubscribing if they need the sink's goroutines/channels to shut down
- [ ] Package compiles with `go build ./internal/agent/...`
