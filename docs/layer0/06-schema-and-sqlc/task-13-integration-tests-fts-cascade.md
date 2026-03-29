# Task 13: Integration Tests — FTS5 Search and Cascading Deletes

**Epic:** 06 — Schema & sqlc Code Generation
**Status:** ⬚ Not started
**Dependencies:** Task 12

---

## Description

Write integration tests verifying FTS5 full-text search returns results for indexed messages, and that cascading deletes propagate correctly through the foreign key chain.

## Acceptance Criteria

- [ ] Test inserts messages and verifies FTS5 search returns matching results
- [ ] Test verifies FTS5 triggers keep the index in sync on insert and delete
- [ ] Test deletes a conversation and verifies all child messages are cascade-deleted
- [ ] Test deletes a project and verifies conversations and their children are cascade-deleted
