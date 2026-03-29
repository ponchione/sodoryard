# Task 03: brain_read Tool Implementation

**Epic:** 06 — Obsidian Client & Brain Tools
**Status:** ⬚ Not started
**Dependencies:** Task 01, Layer 4 Epic 01

---

## Description

Implement the `brain_read` tool as a `Pure` tool in `internal/tool/`. This tool reads a specific brain document by its vault-relative path, returning the full markdown content along with extracted YAML frontmatter metadata and outgoing wikilinks. An optional `include_backlinks` parameter uses a heuristic keyword search to find documents that reference the requested document.

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
