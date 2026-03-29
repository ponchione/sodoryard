# Task 05: Integration Tests

**Epic:** 02 — Conversation Manager
**Status:** ⬚ Not started
**Dependencies:** Tasks 01-04

---

## Description

Write integration tests for the conversation manager that exercise the full lifecycle against a real SQLite database. These tests verify that CRUD operations, history reconstruction, iteration persistence, cancellation cleanup, and title generation all work correctly with real database transactions and sqlc-generated code. Tests use an in-memory SQLite database with the schema applied.

## Acceptance Criteria

- [ ] Test setup: each test creates an in-memory SQLite database (`":memory:"`), applies the schema from Layer 0 Epic 06, and constructs a `ConversationManager` with sqlc queries
- [ ] **CRUD test:** Create a conversation, Get it (verify fields match), List conversations (verify ordering by `updated_at DESC`), SetTitle, Delete (verify cascade removes related records)
- [ ] **Multi-iteration persistence test:** Create a conversation, persist 3 iterations with mixed message types — iteration 1: user message + assistant with tool_calls + tool results; iteration 2: assistant with tool_calls + tool results; iteration 3: assistant text-only. Call `ReconstructHistory` and verify the returned message array matches what was persisted in the correct order. Verify sequence numbers are contiguous integers (0.0, 1.0, 2.0, ...)
- [ ] **Cancellation test:** Create a conversation, persist 2 completed iterations, then persist a partial 3rd iteration. Call `CancelIteration` for the 3rd iteration. Call `ReconstructHistory` and verify only messages from iterations 1 and 2 are returned. Verify the cancelled iteration's tool_executions and sub_calls are also removed
- [ ] **Title generation test:** Create a conversation with a mock provider that returns a title string. Call `GenerateTitle` and verify the conversation's title field is updated. Also test with a mock provider that returns an error — verify the conversation retains its null title and no error is propagated
- [ ] **Sequence numbering test:** Persist messages across multiple iterations, verify that `NextSequence` returns the correct next value each time. Verify that REAL sequence values support the midpoint insertion needed for compression summaries (e.g., inserting at 1.5 between 1.0 and 2.0)
- [ ] **SeenFiles test:** Add several file paths at different turn numbers, verify `Contains` returns correct turn numbers, verify `Paths` returns all tracked paths, verify thread safety by calling `Add` and `Contains` from multiple goroutines
- [ ] All tests clean up after themselves (in-memory SQLite handles this automatically)
- [ ] Tests pass with `go test ./internal/conversation/...`
