# Task 06: Unit Tests for Budget and Serialization

**Epic:** 05 — Budget Manager & Context Serialization
**Status:** ⬚ Not started
**Dependencies:** Task 01, Task 02, Task 03, Task 04, Task 05

---

## Description

Comprehensive unit tests for both the budget manager and context serializer. Budget tests verify token accounting, priority-based filling, sub-budget enforcement, exclusion tracking, and the compression trigger. Serialization tests verify markdown format, section ordering, file grouping, previously-viewed annotations, and edge cases.

## Acceptance Criteria

- [ ] Test: model with 200k context, 50k history tokens — available budget computed correctly
- [ ] Test: explicit files fill first, then RAG, then structural / conventions / git, in correct priority order
- [ ] Test: budget exhausted mid-priority-2 — remaining lower-priority items land in `ExcludedChunks` with reason `"budget_exceeded"`
- [ ] Test: history exceeds 50% of context window — `CompressionNeeded` flag set to `true`
- [ ] Test: budget breakdown produces correct token counts per category
- [ ] Test: two chunks from same file grouped under one file header
- [ ] Test: chunk from a seen file gets `[previously viewed]` annotation
- [ ] Test: output is valid markdown (code fences properly closed, headers properly nested)
- [ ] Test: empty retrieval results produce empty or minimal markdown output (no crash)
- [ ] All tests pass: `go test ./internal/context/...`
