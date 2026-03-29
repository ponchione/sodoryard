# Task 05: Schema — FTS5 Virtual Table and Triggers

**Epic:** 06 — Schema & sqlc Code Generation
**Status:** ⬚ Not started
**Dependencies:** Task 02

---

## Description

Add the `messages_fts` FTS5 virtual table and the insert/delete triggers that keep it in sync with the `messages` table. FTS indexes `user` and `assistant` message content.

## Acceptance Criteria

- [ ] `messages_fts` FTS5 virtual table defined in `schema.sql`
- [ ] INSERT trigger on `messages` populates `messages_fts`
- [ ] DELETE trigger on `messages` removes from `messages_fts`
- [ ] FTS indexes content for `user` and `assistant` roles
