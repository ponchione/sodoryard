# Task 14: Integration Tests — REAL Sequence Sorting and UUIDv7 PKs

**Epic:** 06 — Schema & sqlc Code Generation
**Status:** ⬚ Not started
**Dependencies:** Task 12

---

## Description

Write integration tests verifying that the REAL sequence column sorts correctly after simulated compression (integer sequences plus midpoint summaries at e.g. 20.5), and that UUIDv7 TEXT primary keys work correctly for insert, query, and foreign key references.

## Acceptance Criteria

- [ ] Test inserts messages with integer sequences (1, 2, 3, ...) and a midpoint summary (e.g., 20.5), verifies ORDER BY sequence returns correct order
- [ ] Test verifies compressed/summary messages sort correctly alongside uncompressed messages
- [ ] Test inserts projects and conversations with UUIDv7 TEXT PKs and verifies they round-trip correctly
- [ ] Test verifies foreign key references between UUIDv7-keyed tables work
