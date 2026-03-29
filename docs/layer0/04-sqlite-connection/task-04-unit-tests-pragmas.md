# Task 04: Unit Tests — Pragma Verification

**Epic:** 04 — SQLite Connection Manager
**Status:** ⬚ Not started
**Dependencies:** Task 03

---

## Description

Write unit tests that open a fresh temporary database and verify all four pragmas are in the expected state.

## Acceptance Criteria

- [ ] Test queries `PRAGMA journal_mode` and asserts the result is the string `"wal"`
- [ ] Test queries `PRAGMA busy_timeout` and asserts the result is the integer `5000`
- [ ] Test queries `PRAGMA foreign_keys` and asserts the result is the integer `1` (ON)
- [ ] Test queries `PRAGMA synchronous` and asserts the result is the integer `1` (NORMAL)
- [ ] Tests use a temporary database file (cleaned up after test via `t.TempDir()`)
