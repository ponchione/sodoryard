# Task 03: Delete Conversation Endpoint

**Epic:** 02 — REST API Conversations
**Status:** ⬚ Not started
**Dependencies:** Task 01

---

## Description

Implement the `DELETE /api/conversations/:id` endpoint. Deleting a conversation cascade-deletes all associated data: messages, tool_executions, sub_calls, and context_reports. The endpoint delegates to the conversation manager's `Delete` method which handles the cascade. Returns a success response on deletion and appropriate errors for missing or invalid IDs.

## Acceptance Criteria

- [ ] `DELETE /api/conversations/:id` deletes the conversation and all associated records (messages, tool_executions, sub_calls, context_reports)
- [ ] Returns HTTP 200 with `{"status": "deleted"}` on successful deletion
- [ ] Returns HTTP 404 with `{"error": "conversation not found"}` when the ID does not exist
- [ ] Invalid UUID format in the path returns HTTP 400 with `{"error": "invalid conversation ID"}`
- [ ] Delegates to conversation manager's `Delete` method — cascade deletion is handled at the data layer, not in the HTTP handler
- [ ] If the conversation is currently active (a turn is in progress), the delete still succeeds — the in-flight turn will fail gracefully when it tries to write to a deleted conversation
- [ ] Endpoint is idempotent-safe: deleting an already-deleted conversation returns 404 (not an error 500)
