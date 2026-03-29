# Task 05: Unit Tests — WAL Concurrent Read/Write

**Epic:** 04 — SQLite Connection Manager
**Status:** ⬚ Not started
**Dependencies:** Task 03

---

## Description

Write unit tests verifying that concurrent reads and writes work correctly under WAL mode. This validates the core reason WAL was chosen.

## Acceptance Criteria

- [ ] Test performs a write in one goroutine while reading in another
- [ ] Both operations complete without `database is locked` errors
- [ ] Written data is eventually visible to the reader
- [ ] Test uses a temporary database (cleaned up after test)
