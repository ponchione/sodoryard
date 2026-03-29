# Task 04: Query Extraction Unit Tests

**Epic:** 03 — Query Extraction & Momentum
**Status:** ⬚ Not started
**Dependencies:** Task 01, Task 02

---

## Description

Unit tests for the query extraction logic covering all three query sources, filler word stripping, sentence splitting, technical keyword extraction, momentum-enhanced queries, and the query cap. Tests should verify both the number and content of produced queries.

## Acceptance Criteria

- [ ] Test: "Fix the auth middleware validation" produces a cleaned query without filler words
- [ ] Test: long multi-sentence message splits into up to 2 queries
- [ ] Test: message with technical terms (camelCase identifiers, underscore identifiers) produces a source 2 supplementary query
- [ ] Test: momentum-enhanced query — "fix the tests" with `MomentumModule = "internal/auth"` produces query "internal/auth fix the tests"
- [ ] Test: query cap enforced — at most 3 queries returned regardless of input complexity
- [ ] Test: explicit entities (file paths, symbols in `ContextNeeds`) are excluded from queries
- [ ] Test: message with only filler words produces a minimal or empty query
- [ ] All tests pass: `go test ./internal/context/...`
