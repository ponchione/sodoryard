# Task 01: Add UUIDv7 Dependency

**Epic:** 05 — UUIDv7 Generation
**Status:** ⬚ Not started
**Dependencies:** Epic 01

---

## Description

Decide on the implementation approach: use `github.com/google/uuid` (v1.6+ supports UUIDv7) or hand-roll per RFC 9562 (~30 lines). Add the dependency to `go.mod` if using a library.

## Acceptance Criteria

- [ ] Implementation approach decided and documented with rationale (comment or doc note explaining *why* — e.g., "chose google/uuid because it's well-maintained, already a transitive dependency, and ~30 lines isn't worth hand-rolling")
- [ ] If using a library, dependency is added to `go.mod` and `go.sum`
- [ ] A trivial import compiles successfully
