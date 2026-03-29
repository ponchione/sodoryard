# Task 12: Integration Tests — Table Creation and CRUD

**Epic:** 06 — Schema & sqlc Code Generation
**Status:** ⬚ Not started
**Dependencies:** Task 10, Task 11

---

## Description

Write integration tests that verify table creation succeeds and insert/query round-trips work for each table.

## Acceptance Criteria

- [ ] Test that init function creates all tables on a fresh database
- [ ] Test insert/query round-trip for `projects`
- [ ] Test insert/query round-trip for `conversations`
- [ ] Test insert/query round-trip for `messages` (all three roles: user, assistant with JSON content blocks, tool with tool_use_id)
- [ ] Test insert/query round-trip for `tool_executions`
- [ ] Test insert/query round-trip for `sub_calls`
- [ ] Test insert/query round-trip for `context_reports` (verify both scalar columns and JSON blob columns round-trip correctly)
- [ ] Test insert/query round-trip for `brain_documents`
- [ ] Test insert/query round-trip for `brain_links`
- [ ] Test insert/query round-trip for `index_state`
- [ ] All tests use temporary databases (cleaned up after test)
