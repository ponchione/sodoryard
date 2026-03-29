# L5-E06 — Agent Loop Core

**Layer:** 5 — Agent Loop
**Epic:** 06
**Status:** ⬚ Not started
**Dependencies:** L5-E01 (event types, EventSink, session types), L5-E02 (conversation manager), L5-E05 (system prompt builder), Layer 3 Epic 06 (context assembly pipeline), Layer 3 Epic 07 (compression engine), Layer 2 Epic 01 (Provider interface, StreamEvent types), Layer 2 Epics 06-07 (sub-call tracking, provider router), Layer 4 Epic 01 (tool Registry, Executor)

---

## Description

Implement the core agent loop — the turn state machine that is sirtopham's orchestration engine. This is the capstone epic that wires together every preceding epic and every preceding layer. It receives a user message, assembles context (via Layer 3 Epic 06), constructs the system prompt (via Epic 05), streams LLM responses (via Layer 2), dispatches tools (via Layer 4), iterates until the turn is complete, persists all data (via Epic 02), handles cancellation and errors, checks compression triggers (via Layer 3 Epic 07), and emits events throughout (via Epic 01's EventSink).

This is entirely net-new — topham's pipeline model has no equivalent. The agent loop is the fundamental architectural shift from pipeline to conversation.

---

## Definition of Done

### Turn State Machine

- [ ] `internal/agent/loop.go` implements the `AgentLoop` with the step-by-step flow from [[05-agent-loop]] §The Loop Step by Step:
  1. User message received
  2. Context assembly
  3. System prompt construction
  4. LLM request (streaming with extended thinking)
  5. Response handling (text → TokenEvents, thinking → ThinkingEvents, tool_use → collect)
  6. Tool dispatch
  7. Iteration check (loop detection, iteration limit)
  8. Turn complete (persist, log, emit, return to idle)
- [ ] Multi-iteration turns handled correctly
- [ ] Text-only response completes the turn

### Event Emission

- [ ] Events emitted throughout the turn via EventSink
- [ ] Tool call events include tool call ID for UI matching

### Iteration Management

- [ ] Loop detection: same tool+args 3 times → nudge
- [ ] Iteration limit: configurable max (default 50), final iteration disables tools
- [ ] Iteration count tracked and included in TurnCompleteEvent

### Error Recovery

- [ ] Tool execution errors fed back to LLM
- [ ] LLM API errors: rate limiting (retry), server errors (retry), auth failure (no retry), context overflow (compress + retry), malformed tool calls (feed back)
- [ ] All errors emit ErrorEvents

### Cancellation

- [ ] During LLM call: cancel HTTP request, discard partial iteration
- [ ] During tool execution: cancel context (SIGTERM → wait → SIGKILL handled by Layer 4)
- [ ] Post-cancellation: completed iterations preserved, in-flight discarded

### Persistence

- [ ] User message persisted at turn start before context assembly begins
- [ ] Per-iteration persistence via ConversationManager.PersistIteration()
- [ ] Context report quality metrics updated post-turn

### Compression Integration

- [ ] Preflight check before each LLM call
- [ ] Post-response check after each LLM call
- [ ] Emergency compression on context overflow errors

### Public API

- [ ] `RunTurn(ctx, conversationID, message) error`
- [ ] `Cancel()`
- [ ] `Subscribe(sink EventSink)`
- [ ] `Unsubscribe(sink EventSink)`
- [ ] `NewAgentLoop(deps AgentLoopDeps) *AgentLoop`

### Tests

- [ ] Integration tests for: single-iteration turn, multi-iteration turn, iteration limit, cancellation, LLM error retry, context overflow compression

---

## Architecture References

- [[05-agent-loop]] — The entire document. This epic implements the document.
- [[05-agent-loop]] §The Loop Step by Step — steps 1-8
- [[05-agent-loop]] §Cancellation — all three paths
- [[05-agent-loop]] §Error Recovery — three layers + LLM API errors
- [[05-agent-loop]] §Streaming to the Web UI — event types and emission points
- [[05-agent-loop]] §Persistence — per-iteration atomic transactions
- [[08-data-model]] §Persistence Transaction Model — atomic commit per iteration

---

## Notes

- This is the largest epic in the phase. Approach step by step: basic turn first, then tool dispatch, then iteration management, then error recovery, then cancellation.
- The loop does NOT implement: WebSocket server, tool execution logic, context assembly logic, or compression logic. It calls into those components.
- The division of responsibilities is: the loop is the state machine that sequences everything; each component does its own work when called.
- Dynamic event subscriber lifecycle is part of the public contract. The WebSocket layer subscribes one sink per connection and must be able to unsubscribe it on disconnect without affecting other sinks.
