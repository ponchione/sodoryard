# Task 05: Logging Unit Tests

**Epic:** 02 — Structured Logging
**Status:** ⬚ Not started
**Dependencies:** Task 02, Task 03, Task 04

---

## Description

Write unit tests that verify log output structure, level filtering, and context propagation. Tests should capture log output to a buffer and assert on content.

## Acceptance Criteria

- [ ] Test that JSON format produces valid JSON with expected fields
- [ ] Test that text format produces readable output
- [ ] Test that level filtering suppresses messages below threshold
- [ ] Test that enriched child loggers include parent context fields in output
- [ ] All tests pass via `go test ./internal/logging/...`
