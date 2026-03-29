# Task 05: Seeded curl Verification

**Epic:** 02 — REST API Conversations
**Status:** ⬚ Not started
**Dependencies:** Tasks 01-04, Layer 6 Epic 05 (Serve Command)

---

## Description

Add an explicit verification task proving the conversation REST API works end-to-end against a running server with seeded test data. This task does not add new API behavior. Its purpose is to make the epic's existing "testable via curl against a running server with seeded test data" requirement concrete and repeatable.

## Acceptance Criteria

- [ ] A repeatable seeding procedure is documented for at least 3 conversations with distinct titles, timestamps, and message bodies
- [ ] Seeded data includes at least one search term that appears in exactly one conversation, so the search endpoint can be verified deterministically
- [ ] Verification command documented for `GET /api/conversations`, for example:
  - `curl http://localhost:3000/api/conversations`
- [ ] Verification command documented for `GET /api/conversations/search?q=<term>`, for example:
  - `curl "http://localhost:3000/api/conversations/search?q=auth"`
- [ ] `GET /api/conversations` verification checks:
  - response is valid JSON
  - seeded conversations are returned
  - ordering is `updated_at DESC`
  - fields `id`, `title`, `updated_at`, and `created_at` are present
- [ ] `GET /api/conversations/search` verification checks:
  - response is valid JSON
  - the expected seeded conversation is returned
  - non-matching seeded conversations are absent when the query is specific
  - the response includes a non-empty `snippet` field for the match
- [ ] The documented seeding and curl verification use only supported project interfaces for v0.1 (SQL seed script, fixture loader, or conversation manager helper — implementer's choice)
- [ ] The task is verification-only: no new endpoint semantics are introduced beyond Tasks 01-04
