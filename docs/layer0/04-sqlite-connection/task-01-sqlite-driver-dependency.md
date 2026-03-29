# Task 01: Finalize SQLite Driver and Pin Dependency

**Epic:** 04 — SQLite Connection Manager
**Status:** ⬚ Not started
**Dependencies:** Epic 01

---

## Description

Finalize the SQLite driver choice (`mattn/go-sqlite3` vs `modernc.org/sqlite`) and add it to `go.mod`. Doc 02 leans toward `mattn` since CGo is already accepted for tree-sitter.

## Acceptance Criteria

- [ ] Driver decision documented (comment in code or doc note)
- [ ] Dependency added to `go.mod` and `go.sum`
- [ ] A trivial import compiles successfully with `CGO_ENABLED=1`
