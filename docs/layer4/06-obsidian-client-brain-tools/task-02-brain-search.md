# Task 02: brain_search Tool Implementation

Note: this task doc is historical. Current runtime uses the MCP/vault backend for `brain_search`; use this page only to understand the older REST-era plan.

**Epic:** 06 — Obsidian Client & Brain Tools
**Status:** ⬚ Not started
**Dependencies:** Task 01, Layer 4 Epic 01

---

## Description

Historical plan: implement `brain_search` as a `Pure` tool backed by the Obsidian REST API. Current runtime truth is MCP/vault-backed keyword search, and proactive context assembly already consumes brain hits separately.

## Acceptance Criteria

- [ ] Implements the `Tool` interface with `Purity() = Pure`
- [ ] `Name()` returns `"brain_search"`, `Description()` returns a concise one-liner for the LLM
- [ ] Accepts JSON input with parameters: `query` (required string), `mode` (optional string, default `"keyword"`), `tags` (optional []string), `max_results` (optional int, default 10)
- [ ] Constructor accepts an `*ObsidianClient` and brain config as dependencies
- [ ] Checks `brain.enabled` config before execution. If disabled, returns `Success=false` with `"Project brain is not configured. See sirtopham.yaml brain section."`
- [ ] When `mode` is `"keyword"` (or omitted): calls `ObsidianClient.SearchKeyword()` with the query (tags appended to query string if provided)
- [ ] When `mode` is `"semantic"` or `"auto"`: returns `Success=true` with `"Semantic search is not yet available (coming in v0.2). Using keyword search instead."` and falls through to keyword search
- [ ] Formats results as readable blocks:
  ```
  1. decisions/error-handling.md
     Title: Error Handling Strategy
     Snippet: ...tool errors are not Go errors, they are ToolResult values...
     Score: 0.85
  ```
- [ ] Title extracted from the first `#` heading in the snippet, or from the filename if no heading is available
- [ ] Respects `max_results` — returns at most N results
- [ ] Zero results: returns `Success=true` with `"No brain documents found for query: '<query>'"`
- [ ] `Schema()` returns valid JSON Schema with parameter types, descriptions, and defaults
- [ ] Unit tests with mock `ObsidianClient`: successful search with multiple results, zero results, brain disabled, semantic mode fallback
