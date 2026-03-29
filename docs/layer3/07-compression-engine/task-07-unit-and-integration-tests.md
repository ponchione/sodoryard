# Task 07: Unit and Integration Tests

**Epic:** 07 — Compression Engine
**Status:** ⬚ Not started
**Dependencies:** Task 01, Task 02, Task 03, Task 04, Task 05, Task 06

---

## Description

Comprehensive unit and integration tests for the compression engine covering trigger detection, head-tail preservation, summary persistence, orphaned tool call sanitization, cascading compression, fallback behavior, and the reconstruction query correctness after compression.

## Acceptance Criteria

- [ ] Test: preflight check with appropriate values — compression triggered when estimated tokens exceed threshold percentage of context window
- [ ] Test: head-tail preservation — 20 messages, protect first 3 and last 4, 13 messages identified as middle for compression
- [ ] Test: summary insertion — sequence value is the REAL midpoint between last head message sequence and first tail message sequence
- [ ] Test: cascading compression — two rounds of compression produce: old summary compressed, new summary covers full range, reconstruction query returns correct message order
- [ ] Test: orphaned tool call sanitization — assistant message with `tool_use` block whose result was compressed has the `tool_use` block removed from content JSON
- [ ] Test: fallback — summarization model fails, middle messages compressed without summary inserted, no crash
- [ ] Test: reconstruction query after compression — `WHERE is_compressed = 0 ORDER BY sequence` returns head + summary + tail in correct order
- [ ] Test: REAL sequence bisection — after compression, a new message appended to the conversation gets the next integer sequence with no collision with the bisected summary sequence
- [ ] Test: reactive trigger — HTTP 400 with `"context_length_exceeded"` correctly detected
- [ ] All tests pass: `go test ./internal/context/...`
