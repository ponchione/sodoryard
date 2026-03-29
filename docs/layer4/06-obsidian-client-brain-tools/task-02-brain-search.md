# Task 02: brain_search Tool Implementation

**Epic:** 06 — Obsidian Client & Brain Tools
**Status:** ⬚ Not started
**Dependencies:** Task 01, Layer 4 Epic 01

---

## Description

Implement the `brain_search` tool as a `Pure` tool in `internal/tool/`. This tool provides keyword search against the project brain via the Obsidian REST API. In v0.1, only keyword mode is supported — the `mode` parameter accepts "keyword" (default), while "semantic" and "auto" return a guidance message that semantic search is coming in v0.2. Results are formatted with document paths, titles, and relevant snippets.

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
