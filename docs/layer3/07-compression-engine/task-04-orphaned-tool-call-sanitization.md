# Task 04: Orphaned Tool Call Sanitization

**Epic:** 07 — Compression Engine
**Status:** ⬚ Not started
**Dependencies:** Task 03

---

## Description

Implement robust orphaned tool call sanitization as a standalone, testable component. When middle messages are compressed, assistant messages in the remaining (uncompressed) set may contain `tool_use` blocks in their content JSON whose corresponding `role=tool` result messages no longer exist. These orphaned references cause Anthropic's API to reject the conversation. The sanitizer parses assistant message content JSON, checks each `tool_use` block against the set of uncompressed tool result messages by `tool_use_id`, and rewrites the content JSON to remove orphaned blocks.

## Acceptance Criteria

- [ ] Parses assistant message content JSON (the `[{"type":"tool_use",...}]` array format)
- [ ] For each `tool_use` block, checks whether a corresponding `role=tool` message with matching `tool_use_id` exists in the uncompressed message set
- [ ] Orphaned `tool_use` blocks (no matching result) are removed from the content JSON
- [ ] Rewritten content JSON is persisted back to the message row in SQLite
- [ ] If an assistant message has ALL tool_use blocks orphaned, the entire message content is rewritten appropriately (not left as an empty array)
- [ ] Handles edge cases: assistant messages with mixed tool_use and text blocks (only tool_use blocks are removed, text blocks preserved)
- [ ] Package compiles with no errors
