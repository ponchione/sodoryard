# Layer 6, Epic 02: REST API — Conversations

**Layer:** 6 — Web Interface & Streaming
**Status:** ⬚ Not started
**Dependencies:** [[layer-6-epic-01-http-server-foundation]], Layer 5 Epic 02 (conversation manager), Layer 0 Epic 06 (schema & sqlc)

---

## Description

Implement the REST API endpoints for conversation CRUD and search. These endpoints are consumed by the frontend's conversation sidebar (list, create, delete, search) and conversation view (get with messages). All data access goes through the conversation manager from Layer 5 Epic 02 — this epic is a thin HTTP layer that translates REST requests into conversation manager calls and serializes responses to JSON.

---

## Definition of Done

- [ ] `GET /api/conversations` — List conversations for the current project, paginated, sorted by `updated_at DESC`. Returns: `[{id, title, updated_at, created_at}]`
- [ ] `POST /api/conversations` — Create a new conversation. Accepts optional `{title, model, provider}`. Returns the created conversation with its ID
- [ ] `GET /api/conversations/:id` — Get a single conversation with metadata (title, model, provider, created_at, updated_at)
- [ ] `GET /api/conversations/:id/messages` — Get all messages for a conversation, ordered by `sequence`. Returns ALL messages including compressed ones (with `is_compressed` and `is_summary` flags). Per [[08-data-model]]: "The web UI shows all messages including compressed ones (greyed out or collapsed)"
- [ ] `DELETE /api/conversations/:id` — Delete a conversation and cascade-delete all messages, tool_executions, sub_calls, and context_reports
- [ ] `GET /api/conversations/search?q=<query>` — Full-text search across conversation messages via FTS5. Returns matching conversations with snippets. Uses the FTS5 query pattern from [[08-data-model]] §Full-Text Search
- [ ] All endpoints return proper HTTP status codes: 200 (success), 201 (created), 404 (not found), 400 (bad request), 500 (server error)
- [ ] JSON response format is consistent: success responses have the data directly, error responses have `{"error": "message"}`
- [ ] Endpoints are registered on the server's router from [[layer-6-epic-01-http-server-foundation]]
- [ ] Conversation list and search are testable via curl against a running server with seeded test data

---

## Architecture References

- [[07-web-interface-and-streaming]] — §REST API Endpoints: conversation CRUD endpoints
- [[08-data-model]] — §Key Query Patterns: conversation list, turn messages, FTS5 search. §Message Storage Model: role, content, tool_use_id, tool_name columns. §Compression Model: is_compressed, is_summary flags visible to UI
- [[08-data-model]] — §Full-Text Search: FTS5 virtual table, triggers, and search query pattern with `snippet()`
- [[05-agent-loop]] — The messages endpoint returns messages in the format that matches the API-faithful storage model: user messages as plain text, assistant messages as JSON content block arrays, tool results as plain text with tool_use_id

---

## Notes for the Implementing Agent

The conversation manager from Layer 5 Epic 02 already handles the database queries — `Create`, `Get`, `List`, `Delete`, `SetTitle`, `Search`, `GetMessages`. This epic is the HTTP adapter layer. Keep it thin.

The messages endpoint is the most complex. Assistant message `content` is stored as a JSON array of content blocks (text, thinking, tool_use). Return it as-is — the frontend will parse it. Don't transform the content on the server side.

For pagination on the conversation list, use cursor-based pagination (pass `updated_at` of the last item) or simple limit/offset. Limit/offset is fine for v0.1 — a personal tool won't have thousands of conversations initially.

The search endpoint uses SQLite FTS5. The query from [[08-data-model]]:
```sql
SELECT c.id, c.title, c.updated_at, snippet(messages_fts, 0, '<b>', '</b>', '...', 32)
FROM messages_fts
JOIN messages m ON m.id = messages_fts.rowid
JOIN conversations c ON c.id = m.conversation_id
WHERE messages_fts MATCH ?
ORDER BY rank LIMIT 20;
```
