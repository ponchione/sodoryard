# Task 03: brain_read Tool Implementation

Note: this task doc is historical. Current runtime uses the MCP/vault backend for `brain_read`; use this page only for the older REST-era design.

**Epic:** 06 — Obsidian Client & Brain Tools
**Status:** ⬚ Not started
**Dependencies:** Task 01, Layer 4 Epic 01

---

## Description

Historical plan: implement `brain_read` as a `Pure` tool around the Obsidian REST API. Current runtime keeps the same high-level tool role but serves it through the MCP/vault backend.

## Acceptance Criteria

- [ ] Implements the `Tool` interface with `Purity() = Pure`
- [ ] `Name()` returns `"brain_read"`, `Description()` returns a concise one-liner for the LLM
- [ ] Accepts JSON input with parameters: `path` (required string), `include_backlinks` (optional bool, default false)
- [ ] Constructor accepts an `*ObsidianClient` and brain config as dependencies
- [ ] Checks `brain.enabled` config before execution. If disabled, returns guidance message.
- [ ] Calls `ObsidianClient.ReadDocument()` to fetch the document content
- [ ] Parses YAML frontmatter (content between opening and closing `---` delimiters) and formats it as key-value pairs in the output header
- [ ] Extracts outgoing `[[wikilinks]]` from the markdown content using regex (`\[\[([^\]]+)\]\]`) and lists them in the output
- [ ] Returns formatted output:
  ```
  Path: decisions/error-handling.md

  Frontmatter:
    created: 2026-03-15
    tags: [architecture, error-handling]
    status: active

  Outgoing links: [[05-agent-loop]], [[tool-interface]]

  Content:
  # Error Handling Strategy
  ...
  ```
- [ ] If `include_backlinks=true`: calls `ObsidianClient.SearchKeyword()` with the document filename (e.g., `"error-handling"`) to find documents that reference it, and includes a "Referenced by:" section listing those document paths
- [ ] Document not found: returns `Success=false` with enriched error listing available documents in the parent directory via `ObsidianClient.ListDocuments()`
- [ ] `Schema()` returns valid JSON Schema with parameter types and descriptions
- [ ] Unit tests with mock client: successful read with frontmatter and wikilinks, read with backlinks, document not found with directory listing, document without frontmatter
