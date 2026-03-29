# Task 01: Turn State Machine

**Epic:** 06 — Agent Loop Core
**Status:** ⬚ Not started
**Dependencies:** L5-E01 (types, EventSink), L5-E02 (conversation manager), Layer 3 Epic 06 (context assembler), L5-E05 (prompt builder), Layer 2 Epic 01 (Provider interface), Layer 4 Epic 01 (tool Registry, Executor)

---

## Description

Implement the `AgentLoop` struct and its `RunTurn` method in `internal/agent/loop.go`. This is the core turn state machine that sequences every step of a turn: receiving the user message, assembling context, constructing the system prompt, making the streaming LLM call, processing the response, dispatching tools when needed, and looping until the turn is complete (text-only response or iteration limit). This task implements the happy path — a turn that may involve one or more iterations of LLM call → tool dispatch → next LLM call. Error recovery and cancellation are handled in separate tasks.

## Acceptance Criteria

- [ ] `AgentLoopDeps` struct defined with all dependencies: `ProviderRouter` (Layer 2), `ContextAssembler` (Layer 3 Epic 06), `PromptBuilder` (L5-E05), `ConversationManager` (L5-E02), `CompressionEngine` (Layer 3 Epic 07), `ToolRegistry` (Layer 4), `ToolExecutor` (Layer 4), `EventSink` (L5-E01, optional initial sink registered into the loop's internal `MultiSink`), `Config AgentLoopConfig`, `Logger *slog.Logger`
- [ ] `AgentLoopConfig` struct with fields: `MaxIterations int` (default 50), `LoopDetectionThreshold int` (default 3), `ExtendedThinking bool` (default true)
- [ ] `NewAgentLoop(deps AgentLoopDeps) *AgentLoop` constructor validates non-nil dependencies
- [ ] `RunTurn(ctx context.Context, conversationID string, message string) error` method implements the step-by-step flow:
  1. **Persist user message** — call `ConversationManager.PersistUserMessage(conversationID, turnNumber, message)` before context assembly begins
  2. **Context assembly** — call `ConversationManager.ReconstructHistory()` after the user message is persisted, then pass that reconstructed history plus the session state / seen-files tracker to `ContextAssembler.Assemble(ctx, message, reconstructedHistory, sessionState)`. Fires once at turn start, result is frozen for the turn
  3. **Iteration loop** begins:
     a. **Reconstruct history** — call `ConversationManager.ReconstructHistory()` to get current message array (includes newly persisted messages from prior iterations)
     b. **Build prompt** — call `PromptBuilder.BuildPrompt()` with assembled context, history, current turn messages, and tool schemas
     c. **Stream LLM request** — call `Provider.Stream()` from the provider router. Process streamed response: accumulate text blocks, thinking blocks, and tool_use blocks
     d. **If text-only response** — turn is complete. Persist the assistant message, break iteration loop
     e. **If tool_use blocks present** — dispatch tools via `ToolExecutor.Execute()`. Convert results to tool result messages. Persist the iteration (assistant message + tool results). Increment iteration counter. Continue loop
  4. **Turn complete** — log completion, return nil
- [ ] Multi-iteration turns work: the loop continues calling the LLM with updated history (including tool results) until a text-only response is received
- [ ] `RunTurn` is blocking — it runs the full turn synchronously (events stream via EventSink throughout). Returns only when the turn is complete, cancelled, or fails
- [ ] The iteration counter starts at 1 and increments after each tool dispatch
- [ ] Package compiles with `go build ./internal/agent/...`
