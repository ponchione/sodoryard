# Task 01: List and Create Conversation Endpoints

**Epic:** 02 — REST API Conversations
**Status:** ⬚ Not started
**Dependencies:** Epic 01 (HTTP Server Foundation), Layer 5 Epic 02 (conversation manager)

---

## Description

Implement the `GET /api/conversations` and `POST /api/conversations` endpoints. The list endpoint returns all conversations for the current project, paginated and sorted by most recently updated. The create endpoint accepts optional title, model, and provider fields and returns the newly created conversation with its generated ID. Both endpoints delegate to the conversation manager and return consistent JSON responses.

## Acceptance Criteria

- [ ] `GET /api/conversations` returns a JSON array of conversation objects: `[{id, title, updated_at, created_at}]`
- [ ] Conversations are sorted by `updated_at` descending (most recent first)
- [ ] Pagination supported via `limit` and `offset` query parameters (default limit: 50, max limit: 200)
- [ ] When there are no conversations, returns an empty array `[]` (not null)
- [ ] `POST /api/conversations` accepts a JSON body with optional fields: `{title?: string, model?: string, provider?: string}`
- [ ] If no body is provided or body is `{}`, the conversation is created with null title and default model/provider from config
- [ ] Returns the created conversation with HTTP 201 and body `{id, title, model, provider, created_at, updated_at}`
- [ ] Invalid JSON body returns HTTP 400 with `{"error": "invalid request body"}`
- [ ] Both endpoints delegate to the conversation manager's `List` and `Create` methods — no direct database access
- [ ] Both endpoints are registered on the server's router under `/api/conversations`
- [ ] `Content-Type: application/json` header set on all responses
