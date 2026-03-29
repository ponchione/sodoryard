# Task 04: FTS5 Search Endpoint

**Epic:** 02 — REST API Conversations
**Status:** ⬚ Not started
**Dependencies:** Task 01, Layer 0 Epic 06 (schema & sqlc — FTS5 virtual table)

---

## Description

Implement the `GET /api/conversations/search?q=<query>` endpoint that performs full-text search across conversation messages using SQLite FTS5. The endpoint returns matching conversations with content snippets highlighting the matched text. This powers the search functionality in the frontend sidebar. The FTS5 query uses the `snippet()` function to extract relevant context around matches.

## Acceptance Criteria

- [ ] `GET /api/conversations/search?q=<query>` performs full-text search via the FTS5 `messages_fts` virtual table
- [ ] Returns a JSON array of matching conversations: `[{id, title, updated_at, snippet}]` where `snippet` is the FTS5 `snippet()` output with match highlighting
- [ ] Results are ordered by FTS5 rank (most relevant first)
- [ ] Results are limited to 20 matches by default (configurable via `limit` query parameter)
- [ ] Empty query string (`q=` or missing `q`) returns HTTP 400 with `{"error": "search query required"}`
- [ ] FTS5 query syntax errors (malformed queries) return HTTP 400 with a descriptive error rather than HTTP 500
- [ ] Conversations are deduplicated — if multiple messages in the same conversation match, the conversation appears once with the best snippet
- [ ] Delegates to conversation manager's `Search` method which executes the FTS5 query
- [ ] Search results include conversations from the current project only
- [ ] Endpoint is registered at a path that does not conflict with `/api/conversations/:id` (the router must distinguish `/api/conversations/search` from `/api/conversations/<uuid>`)
