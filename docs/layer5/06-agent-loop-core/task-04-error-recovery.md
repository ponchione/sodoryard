# Task 04: Error Recovery

**Epic:** 06 — Agent Loop Core
**Status:** ⬚ Not started
**Dependencies:** Task 01, Task 02 (event emission for ErrorEvents)

---

## Description

Implement the multi-layered error recovery strategy within the turn state machine. Layer 1 handles tool execution errors by feeding them back to the LLM as tool result messages (the LLM learns from its mistakes). Layer 2 handles LLM API errors with provider-specific retry logic, fallback routing, and special handling for context overflow errors. All errors emit appropriate `ErrorEvent`s for the UI. The goal is maximum resilience — the agent should recover from transient failures and degrade gracefully on persistent ones.

## Acceptance Criteria

- [ ] **Layer 1 — Tool error recovery:** when `ToolExecutor.Execute()` returns an error for a tool call, the error message becomes the tool result message sent back to the LLM. The error message is enriched with helpful context where possible (e.g., if a file_read fails with "not found", include a suggestion to use `list_directory` to find the correct path). The turn continues normally — tool errors are not turn-ending
- [ ] **Rate limiting (429):** retry with exponential backoff. Base delay 1 second, factor 2x, up to 3 attempts. If a `Retry-After` header is present, respect it. If all retries exhausted, optionally fall back to configured fallback provider (if any). If no fallback or fallback also fails, emit `ErrorEvent(Recoverable=false)` and return error
- [ ] **Server error (500/502/503):** retry with exponential backoff, same strategy as 429. Up to 3 attempts. Fall back to alternate provider if exhausted
- [ ] **Auth failure (401/403):** do NOT retry. Emit `ErrorEvent(Recoverable=false)` with remediation message (e.g., "API key is invalid or expired. Please check your configuration."). Return error immediately
- [ ] **Context overflow (400 with context_length_exceeded):** trigger emergency compression via `CompressionEngine.Compress()` (from Layer 3 Epic 07). After compression, reconstruct history and retry the LLM call once. If the retry also fails with overflow, emit `ErrorEvent(Recoverable=false)` and return error
- [ ] **Malformed tool calls:** if the LLM produces tool calls that fail to parse (invalid JSON arguments, unknown tool name), create a synthetic tool result message with the parse error and correction guidance (e.g., "Invalid JSON in arguments. Ensure all strings are properly quoted."). Feed it back to the LLM for self-correction
- [ ] All error paths emit `ErrorEvent` via the EventSink with appropriate `Recoverable` flag and descriptive `Message`
- [ ] Retry attempts are logged with structured fields: attempt number, error type, delay before next attempt
- [ ] Error recovery does not break the iteration count — retried LLM calls do not increment the iteration counter (a retry is not a new iteration)
- [ ] Package compiles with `go build ./internal/agent/...`
