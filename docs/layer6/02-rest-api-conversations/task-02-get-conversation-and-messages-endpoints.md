# Task 02: Get Conversation and Get Messages Endpoints

**Epic:** 02 — REST API Conversations
**Status:** ⬚ Not started
**Dependencies:** Task 01

---

## Description

Implement the `GET /api/conversations/:id` and `GET /api/conversations/:id/messages` endpoints. The conversation endpoint returns metadata for a single conversation. The messages endpoint returns all messages for a conversation ordered by sequence number, including compressed and summary messages with their flags. Assistant message content is returned as-is (JSON content block arrays) without server-side transformation — the frontend handles parsing.

## Acceptance Criteria

- [ ] `GET /api/conversations/:id` returns a single conversation object: `{id, title, model, provider, created_at, updated_at}`
- [ ] Returns HTTP 404 with `{"error": "conversation not found"}` when the ID does not exist
- [ ] Invalid UUID format in the path returns HTTP 400 with `{"error": "invalid conversation ID"}`
- [ ] `GET /api/conversations/:id/messages` returns a JSON array of message objects ordered by `sequence` ascending
- [ ] Each message object includes: `{id, role, content, sequence, tool_use_id, tool_name, is_compressed, is_summary, created_at}`
- [ ] Assistant message `content` field is returned as the raw JSON content block array (text, thinking, tool_use blocks) — no transformation
- [ ] User message `content` is returned as plain text string
- [ ] Tool result message `content` is returned as plain text with `tool_use_id` linking it to the corresponding tool_use block
- [ ] Compressed messages (`is_compressed=true`) and summary messages (`is_summary=true`) are included in the response with their flags set
- [ ] Returns HTTP 404 if the conversation ID does not exist
- [ ] Delegates to conversation manager's `Get` and `GetMessages` methods
