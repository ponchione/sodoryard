# Task 06: Fallback and Cache Invalidation

**Epic:** 07 — Compression Engine
**Status:** ⬚ Not started
**Dependencies:** Task 02, Task 03

---

## Description

Implement the fallback path when auxiliary model summarization fails and the cache invalidation signal after compression completes. If the summarization model is unavailable, times out, or returns an error, the engine falls back to truncation: middle messages are marked as compressed WITHOUT inserting a summary. The conversation continues with less context but does not crash. After any compression (successful or fallback), return a signal indicating the conversation history shape has changed so the agent loop can invalidate cached system prompt state.

## Acceptance Criteria

- [ ] **Fallback:** If summarization model fails (unavailable, API error, timeout), middle messages are marked `is_compressed = 1` WITHOUT inserting a summary message
- [ ] The conversation continues with reduced context but does not crash or return an error to the user
- [ ] Summarization failure logged with structured logging: provider name, model name, error message, number of messages that would have been summarized
- [ ] **Cache invalidation:** After compression completes (success or fallback), returns a signal indicating conversation history shape has changed
- [ ] The agent loop uses this signal to invalidate cached system prompt state (set `_cached_system_prompt = nil`)
- [ ] Cache block 1 (base prompt) and block 2 (assembled context) are unaffected — only block 3 (history prefix) is invalidated
- [ ] Package compiles with no errors
