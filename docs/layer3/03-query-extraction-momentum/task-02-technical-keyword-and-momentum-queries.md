# Task 02: Technical Keyword Extraction (Source 2) and Momentum-Enhanced Query (Source 3)

**Epic:** 03 — Query Extraction & Momentum
**Status:** ⬚ Not started
**Dependencies:** Task 01

---

## Description

Implement the second and third query sources. Source 2 extracts technical keywords from the user message — identifiers with underscores, camelCase, PascalCase, dot notation, HTTP methods, status codes, and programming domain terms — and joins them into a supplementary query if they differ meaningfully from the cleaned message. Source 3 prepends the momentum module (if set) to the cleaned message query to scope semantic search to the active working area. Enforce the query cap of 3 maximum queries across all sources.

## Acceptance Criteria

- [ ] **Source 2 — Technical keyword extraction:**
  - Words with underscores extracted (`validate_token`)
  - camelCase words extracted (`validateToken`)
  - PascalCase words extracted (`ValidateToken`)
  - Dot notation extracted (`auth.Service`)
  - HTTP methods extracted (GET, POST, PUT, DELETE, PATCH)
  - Status codes extracted (200, 401, 404, 500)
  - Programming domain terms extracted: middleware, handler, router, schema, migration, query, endpoint, service, repository, controller, factory, adapter
  - Extracted terms joined into a supplementary query
- [ ] Source 2 query is skipped if it overlaps substantially with the source 1 cleaned message query
- [ ] **Source 3 — Momentum-enhanced query:**
  - If `ContextNeeds.MomentumModule` is set, prepend it to the cleaned message query
  - Example: "Fix the tests" with momentum module `internal/auth` produces "internal/auth fix the tests"
- [ ] **Query cap:** Maximum 3 queries returned. If all three sources produce distinct queries, return all three
- [ ] Package compiles with no errors
