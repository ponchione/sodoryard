# Task 06: Unit Tests and Integration Test

Note: this task doc is historical. It describes the older REST-client test plan, not the current MCP/vault-backed runtime contract.

**Epic:** 06 — Obsidian Client & Brain Tools
**Status:** ⬚ Not started
**Dependencies:** Task 01, Task 02, Task 03, Task 04, Task 05

---

## Description

Historical plan: write the remaining unit tests for the ObsidianClient and a REST-era end-to-end integration test. Current runtime validation should prefer the maintained live package in `docs/v2-b4-brain-retrieval-validation.md` plus `scripts/validate_brain_retrieval.py`.

## Acceptance Criteria

- [ ] All tests in `internal/brain/` and `internal/tool/` packages
- [ ] All tests pass via `go test ./internal/brain/... ./internal/tool/...`
- [ ] **ObsidianClient — read document:** Mock server returns markdown content for a GET request, verify content returned correctly
- [ ] **ObsidianClient — write document:** Mock server accepts PUT request, verify the request body matches the content and the correct path is targeted
- [ ] **ObsidianClient — search keyword:** Mock server returns JSON search results for a POST request, verify `SearchHit` structs populated correctly with path, snippet, and score
- [ ] **ObsidianClient — list documents:** Mock server returns a list of document paths, verify slice returned correctly
- [ ] **ObsidianClient — connection refused:** Client pointed at a closed server, verify descriptive connection error message
- [ ] **ObsidianClient — 401 auth failure:** Mock server returns 401, verify error message mentions API key
- [ ] **ObsidianClient — 404 not found:** Mock server returns 404, verify "Document not found" error
- [ ] **ObsidianClient — request timeout:** Mock server sleeps longer than the client timeout, verify timeout error
- [ ] **Registration:** All four brain tools register in the registry without panics and appear in `registry.All()`
- [ ] **Integration test — full lifecycle:**
  1. Historical plan: set up `httptest` server simulating the Obsidian REST API (responding to write, read, search, and update requests)
  2. Register all four brain tools with the mock server's ObsidianClient
  3. Dispatch `brain_write` via executor to create a document with frontmatter and wikilinks
  4. Dispatch `brain_read` via executor to read the document back, verify content, frontmatter, and wikilinks are present
  5. Dispatch `brain_search` via executor with a keyword query, verify the written document appears in results
  6. Dispatch `brain_update` via executor with `replace_section` to modify a section of the document
  7. Dispatch `brain_read` again via executor, verify the section was updated
- [ ] **Brain disabled:** Configure `brain.enabled: false`, dispatch any brain tool, verify guidance message returned
- [ ] Schema validation: each brain tool's `Schema()` output is valid JSON and contains expected parameter names
