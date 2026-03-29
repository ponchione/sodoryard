# Task 01: Schema — Entity Tables (projects, conversations)

**Epic:** 06 — Schema & sqlc Code Generation
**Status:** ⬚ Not started
**Dependencies:** Epic 04, Epic 05

---

## Description

Begin `schema.sql` with the `projects` and `conversations` tables. Both use UUIDv7 TEXT primary keys. `conversations` has a foreign key to `projects` with cascading delete.

## Acceptance Criteria

- [ ] `schema.sql` created at the appropriate location
- [ ] `projects` table with UUIDv7 TEXT PK and fields per doc 08
- [ ] `conversations` table with UUIDv7 TEXT PK, FK to `projects`, `ON DELETE CASCADE`
- [ ] Schema is valid SQL that can be executed on a fresh SQLite database
