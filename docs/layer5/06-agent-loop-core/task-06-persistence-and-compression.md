# Task 06: Persistence and Compression Integration

**Epic:** 06 — Agent Loop Core
**Status:** ⬚ Not started
**Dependencies:** Task 01, L5-E02 (ConversationManager.PersistUserMessage, PersistIteration), Layer 3 Epic 06 (UpdateQuality), Layer 3 Epic 07 (CompressionEngine)

---

## Description

Wire persistence and compression into the turn state machine. Persistence happens per-iteration — after each completed iteration (assistant message + tool results), the data is atomically committed to SQLite via the conversation manager. Compression checks happen before and after each LLM call, using both rough token estimates (preflight) and exact token counts from the API response (post-response). After turn completion, quality metrics from the context assembly report are updated with post-turn data (whether the agent used reactive search, which files it read).

## Acceptance Criteria

- [ ] **Per-iteration persistence:** after each completed iteration (assistant response + tool dispatch + tool results), call `ConversationManager.PersistIteration(conversationID, turnNumber, iteration, messages)`. This commits the assistant message and all tool result messages in an atomic transaction. Persistence happens immediately after tool dispatch, not batched to end of turn
- [ ] **User message persistence:** call `ConversationManager.PersistUserMessage(conversationID, turnNumber, message)` at the very start of `RunTurn` before context assembly begins. This ensures the user's message survives even if the turn fails mid-execution
- [ ] **Final iteration persistence:** the text-only assistant response that ends the turn is persisted as the final iteration
- [ ] **Preflight compression check:** before each LLM call, call `CompressionEngine.ShouldCompress(conversationID, contextLimit)` with a rough token estimate. If true, call `CompressionEngine.Compress()` before proceeding with the LLM call. After compression, the prompt builder uses the updated (compressed) history
- [ ] **Post-response compression check:** after each LLM response, call `CompressionEngine.ShouldCompressExact(response.Usage.PromptTokens, contextLimit)` with the exact token count from the API response. If true, compress before the next iteration
- [ ] **Emergency compression:** integrated with error recovery (Task 04) — when a 400 context_length_exceeded error is received, compression is triggered as part of the retry flow
- [ ] **StatusEvent(StateCompressing)** emitted before compression runs, `StatusEvent(StateWaitingForLLM)` emitted after compression completes and before the LLM call resumes
- [ ] **Post-turn quality metrics:** after the turn completes, analyze the completed turn's tool calls to determine: (a) whether `search_semantic` was called (AgentUsedSearchTool), (b) which files were read via `file_read` (AgentReadFiles). Call `ContextAssembler.UpdateQuality()` with these metrics
- [ ] **Title generation trigger:** after the first turn in a new conversation (conversation has no title and this is turn 1), fire `GenerateTitle()` in a goroutine. This is non-blocking and non-fatal
- [ ] Persistence errors are treated as non-recoverable — if `PersistIteration` fails, log the error, emit `ErrorEvent`, and return an error from `RunTurn`
- [ ] Package compiles with `go build ./internal/agent/...`
