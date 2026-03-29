# Task 02: UUIDv7 Generator Function

**Epic:** 05 — UUIDv7 Generation
**Status:** ⬚ Not started
**Dependencies:** Task 01

---

## Description

Create `internal/id/` package with an exported function (e.g., `New() string`) that generates UUIDv7 strings. The generated IDs must have the correct version 7 bits and be time-ordered.

## Acceptance Criteria

- [ ] `internal/id/` package exists with exported generation function
- [ ] Generated IDs are valid UUIDv7 (version nibble is `7`, variant bits correct)
- [ ] IDs generated in sequence are lexicographically ordered
- [ ] Function signature is simple: returns a string (and optionally an error)
