# Task 11: Database Initialization Function

**Epic:** 06 — Schema & sqlc Code Generation
**Status:** ⬚ Not started
**Dependencies:** Task 06, Epic 04

---

## Description

Implement an initialization function that creates all tables from `schema.sql` on a fresh database. Re-running init on an existing database should drop and recreate all tables (nuke-and-rebuild strategy per doc 08).

## Acceptance Criteria

- [ ] Exported init function reads and executes the full schema on a given `*sql.DB`
- [ ] Running on a fresh database creates all tables, indexes, FTS, and triggers
- [ ] Running on an existing database drops everything and recreates (nuke-and-rebuild)
- [ ] Returns a clear error if schema execution fails
