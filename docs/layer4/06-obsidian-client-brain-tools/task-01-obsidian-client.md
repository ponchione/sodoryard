# Task 01: ObsidianClient HTTP Client

Note: this task is now historical. The supported runtime path is MCP-backed and no longer depends on `ObsidianClient`.

**Epic:** 06 — Obsidian Client & Brain Tools
**Status:** ⬚ Not started
**Dependencies:** Layer 0 Epic 03 (config — brain section)

---

## Description

Historical plan: implement `ObsidianClient` in `internal/brain/` as an HTTP client for the Obsidian Local REST API plugin. This page is retained only as legacy design context; the supported runtime path now uses the MCP/vault backend for brain tools and proactive retrieval.

## Acceptance Criteria

- [ ] `ObsidianClient` struct in `internal/brain/` with constructor `NewObsidianClient(baseURL string, apiKey string) *ObsidianClient`
- [ ] `ReadDocument(ctx context.Context, path string) (string, error)` — sends GET request to the vault-relative document path, returns the markdown content as a string
- [ ] `WriteDocument(ctx context.Context, path string, content string) error` — sends PUT request with content as the body, creates or overwrites the document
- [ ] `SearchKeyword(ctx context.Context, query string) ([]SearchHit, error)` — sends POST request with the search query, returns a slice of `SearchHit` structs containing `Path`, `Snippet`, and `Score` fields
- [ ] `ListDocuments(ctx context.Context, directory string) ([]string, error)` — sends GET request to list document paths in a vault directory (or vault root if directory is empty)
- [ ] All requests include `Authorization: Bearer <apiKey>` header
- [ ] HTTP client configured with a 10-second timeout per request (Obsidian is local, should respond quickly)
- [ ] Connection failure (Obsidian not running) returns a descriptive error: `"Cannot connect to Obsidian REST API at <url>. Is Obsidian running with the Local REST API plugin enabled?"`
- [ ] HTTP 401 (unauthorized) returns: `"Obsidian REST API authentication failed. Check the API key in sirtopham.yaml brain.obsidian_api_key."`
- [ ] HTTP 404 (not found) returns: `"Document not found: <path>"`
- [ ] HTTP 500 (server error) returns: `"Obsidian REST API error (500): <response body snippet>"`
- [ ] Unit tests with `net/http/httptest` mock server: successful read, successful write, successful search with multiple results, successful list, connection refused, 401 auth failure, 404 not found, 500 server error, request timeout
