# Task 07: Integration Tests

**Epic:** 06 — Agent Loop Core
**Status:** ⬚ Not started
**Dependencies:** Tasks 01-06

---

## Description

Write integration tests for the agent loop that verify the full turn lifecycle with mock dependencies. Each test constructs an `AgentLoop` with mock implementations of the provider, context assembler, prompt builder, conversation manager, compression engine, tool registry, and tool executor. Tests verify the state machine flow, event emission, persistence calls, error recovery, and cancellation behavior. A `ChannelSink` from L5-E01 captures emitted events for assertion.

## Acceptance Criteria

- [ ] **Single-iteration turn test:** mock provider returns text-only response (no tool calls). Verify: `RunTurn` returns nil, exactly one iteration, `TurnCompleteEvent` emitted with `IterationCount=1`, the user message is persisted via `PersistUserMessage`, the assistant message is persisted via `PersistIteration`, and `StatusEvent` transitions are `AssemblingContext → WaitingForLLM → Idle`
- [ ] **Multi-iteration turn test:** mock provider returns tool_use on iteration 1, then tool_use on iteration 2, then text-only on iteration 3. Mock executor returns success for all tool calls. Verify: 3 iterations, `PersistIteration` called 3 times (once per iteration), tool call events emitted for iterations 1 and 2 (start + end for each), `TurnCompleteEvent` emitted with `IterationCount=3`
- [ ] **Iteration limit test:** mock provider always returns tool_use (never text-only). Set `MaxIterations=5`. Verify: loop runs 5 iterations, the 5th iteration's prompt is built with `DisableTools=true`, the 5th provider call returns text-only (because tools are disabled). Verify `TurnCompleteEvent.IterationCount=5`
- [ ] **Loop detection test:** mock provider returns the same tool call with identical arguments 3 times in a row. Verify: a nudge message is injected before the 4th LLM call. Verify the nudge does not stop the turn — it continues to iterate
- [ ] **Cancellation during LLM call test:** start `RunTurn` in a goroutine. While the mock provider is streaming (blocks on a channel), cancel the context. Verify: `RunTurn` returns a cancellation error, `TurnCancelledEvent` emitted, any completed iterations are preserved (mock `PersistIteration` was called for earlier iterations), the in-flight iteration is not persisted
- [ ] **LLM error with retry test:** mock provider returns 429 on first call, then succeeds on second call. Verify: `RunTurn` completes successfully, `ErrorEvent(Recoverable=true)` emitted for the 429, retry delay is applied (can use a clock mock or just verify the second call happens)
- [ ] **Context overflow test:** mock provider returns 400 context_length_exceeded. Mock compression engine's `Compress()` returns nil (success). On retry, mock provider succeeds. Verify: compression was called, `StatusEvent(Compressing)` emitted, turn completes successfully
- [ ] **Event ordering test:** capture all events via `ChannelSink`. Verify chronological ordering and that status transitions are consistent (no `WaitingForLLM` after `Idle`, no `ExecutingTools` before `WaitingForLLM`, etc.)
- [ ] **Post-turn quality metrics test:** after a successful turn with tool calls that include `search_semantic` and `file_read`, verify `UpdateQuality` is called with the correct `usedSearch` flag and `readFiles` list
- [ ] All tests pass with `go test ./internal/agent/...`
