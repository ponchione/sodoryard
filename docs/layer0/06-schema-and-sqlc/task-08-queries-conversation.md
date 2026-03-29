# Task 08: Query Files — Conversation Queries

**Epic:** 06 — Schema & sqlc Code Generation
**Status:** ⬚ Not started
**Dependencies:** Task 07, Task 02

---

## Description

Write SQL query files for the conversation-related query patterns from doc 08: history reconstruction, conversation listing, turn messages for the web UI, and FTS5 conversation search.

## Acceptance Criteria

- [ ] Conversation history reconstruction query (agent loop hot path): `SELECT role, content, tool_use_id, tool_name FROM messages WHERE conversation_id = ? AND is_compressed = 0 ORDER BY sequence`
- [ ] Conversation listing query (web UI sidebar): `SELECT id, title, updated_at FROM conversations WHERE project_id = ? ORDER BY updated_at DESC LIMIT ? OFFSET ?`
- [ ] Turn messages query (web UI message thread, includes compressed): `SELECT id, role, content, tool_use_id, tool_name, turn_number, iteration, sequence FROM messages WHERE conversation_id = ? ORDER BY sequence`
- [ ] FTS5 conversation search query: `SELECT c.id, c.title, c.updated_at, snippet(messages_fts, 0, '<b>', '</b>', '...', 32) FROM messages_fts JOIN messages m ON m.id = messages_fts.rowid JOIN conversations c ON c.id = m.conversation_id WHERE messages_fts MATCH ? ORDER BY rank LIMIT 20`
- [ ] Queries are in `.sql` files with sqlc annotations (`:one`, `:many`, etc.)
- [ ] `sqlc generate` accepts the query files without errors
