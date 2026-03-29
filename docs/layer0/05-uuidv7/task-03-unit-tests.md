# Task 03: UUIDv7 Unit Tests

**Epic:** 05 — UUIDv7 Generation
**Status:** ⬚ Not started
**Dependencies:** Task 02

---

## Description

Write unit tests verifying format correctness, uniqueness, and time-ordering of generated UUIDv7 IDs.

## Acceptance Criteria

- [ ] Test verifies UUID format (8-4-4-4-12 hex pattern)
- [ ] Test verifies version nibble is `7`
- [ ] Test generates 10,000 IDs and verifies all are unique
- [ ] Test generates IDs in rapid sequence (no artificial delay) and verifies lexicographic ordering holds — this exercises both cross-millisecond and within-same-millisecond ordering
- [ ] All tests use Go's standard `testing` package (no external test framework)
- [ ] All tests pass via `go test ./internal/id/...`
