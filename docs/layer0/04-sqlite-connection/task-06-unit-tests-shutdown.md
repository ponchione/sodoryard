# Task 06: Unit Tests — Clean Shutdown

**Epic:** 04 — SQLite Connection Manager
**Status:** ⬚ Not started
**Dependencies:** Task 03

---

## Description

Write unit tests verifying that closing the database handle is clean — no data loss, no leaked file handles, WAL checkpoint completes.

## Acceptance Criteria

- [ ] Test writes data, closes the connection, reopens, and verifies data persists
- [ ] Test verifies no error is returned from `db.Close()`
- [ ] Test uses a temporary database (cleaned up after test)
