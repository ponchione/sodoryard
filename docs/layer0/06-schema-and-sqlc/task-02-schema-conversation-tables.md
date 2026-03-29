# Task 02: Schema — Conversation Detail Tables (messages, tool_executions, sub_calls)

**Epic:** 06 — Schema & sqlc Code Generation
**Status:** ⬚ Not started
**Dependencies:** Task 01

---

## Description

Add the high-frequency conversation detail tables to `schema.sql`. All use INTEGER AUTOINCREMENT PKs. `messages` uses REAL for `sequence` to support compression midpoint insertion.

## Acceptance Criteria

- [ ] `messages` table with AUTOINCREMENT PK, FK to `conversations`, `role`, `content`, `sequence` (REAL), `is_compressed`, `is_summary` flags
- [ ] `tool_executions` table with AUTOINCREMENT PK, FK to `conversations`, tool dispatch fields
- [ ] `sub_calls` table with AUTOINCREMENT PK, FK to `conversations`, cache token tracking columns (`cache_read_tokens`, `cache_creation_tokens`)
- [ ] All FKs use `ON DELETE CASCADE` from the parent conversation
