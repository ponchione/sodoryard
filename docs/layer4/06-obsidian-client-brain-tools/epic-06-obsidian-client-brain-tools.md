# Layer 4 — Epic 06: Brain Tools and Legacy Obsidian REST Plan

Note: this epic doc is historical. The implemented runtime uses an MCP-backed vault backend for brain tools and already supports proactive keyword-backed brain retrieval in context assembly. Keep this page only as a record of the older REST-era plan.

**Layer:** 4 (Tool System)
**Package:** `internal/tool/`, `internal/brain/`
**Status:** ⬚ Not Started
**Dependencies:**
- Layer 4 Epic 01: Tool Interface, Registry & Executor
- Layer 0 Epic 03: Configuration (`internal/config/` — brain config section: vault_path, obsidian_api_url, obsidian_api_key)
- Layer 0 Epic 06: Schema & sqlc (`brain_documents`, `brain_links` tables — available but only populated by this epic and future brain indexer)

**Architecture Refs:**
- [[09-project-brain]] §Tools — brain_read, brain_write, brain_update, brain_search parameter specs and behavior
- [[09-project-brain]] §Build Phases — historical v0.1 REST-era plan versus current runtime truth
- [[09-project-brain]] §Architecture — current MCP/vault runtime contract
- [[09-project-brain]] §Vault Structure — document format, frontmatter, wikilinks, tags

---

## What This Epic Covers

Two parts:

**Part 1: historical Obsidian REST API client plan (`internal/brain/`).** This section describes the older Local REST API plugin plan. It is no longer the supported runtime path for brain tools or proactive retrieval.

**Part 2: Four brain tools** that implement the `Tool` interface:

- **`brain_search` (Pure):** Historical plan: keyword search via the Obsidian REST API. Current runtime uses the MCP/vault backend for keyword-backed note search.
- **`brain_read` (Pure):** Read a specific brain document by path. Returns full markdown content, extracted frontmatter metadata, and outgoing wikilinks parsed from the content.
- **`brain_write` (Mutating):** Historical plan: create or overwrite a note via the Obsidian REST API. Current runtime note mutation flows through the MCP/vault backend.
- **`brain_update` (Mutating):** Append to, prepend to, or replace a section of an existing document. Reads current content, applies the operation, writes back. For `replace_section`, locates the target heading and replaces content up to the next heading of equal or higher level.

Historical note: this page still documents the old v0.1 boundary where brain tools were reactive-only. Current runtime truth is different: proactive MCP/vault-backed keyword retrieval is already live in context assembly, and this epic should not be read as the active backend contract.

---

## Definition of Done

### Obsidian REST API Client
- [ ] `ObsidianClient` struct with constructor taking `baseURL string`, `apiKey string`
- [ ] Methods:
  - `ReadDocument(ctx, path) (content string, err error)` — GET document content by vault-relative path
  - `WriteDocument(ctx, path, content string) error` — PUT document content (creates or overwrites)
  - `SearchKeyword(ctx, query string) ([]SearchHit, error)` — POST search query, returns hits with path, snippet, score
  - `ListDocuments(ctx, directory string) ([]string, error)` — List document paths in a directory (or vault root)
- [ ] API key sent as `Authorization: Bearer <key>` header on all requests
- [ ] Connection failure produces clear error: `"Cannot connect to Obsidian REST API at <url>. Is Obsidian running with the Local REST API plugin enabled?"`
- [ ] HTTP error responses (401, 404, 500) mapped to descriptive Go errors
- [ ] Request timeout: 10 seconds per call (Obsidian is local, should be fast)
- [ ] Unit tests with httptest mock server: successful read, write, search, connection failure, auth failure, not found

### brain_search
- [ ] Implements `Tool` interface with purity `Pure`
- [ ] Parameters: `query` (required), `mode` (optional — "keyword" default; "semantic" and "auto" return guidance message that semantic search is v0.2), `tags` (optional []string — tag filter, currently passed as part of the keyword query), `max_results` (optional, default 10)
- [ ] Calls `ObsidianClient.SearchKeyword()` with the query
- [ ] Returns formatted results: document path, title (extracted from first heading or filename), relevant snippet, score
- [ ] No results returns "No brain documents found for query: ..." (success=true)
- [ ] If brain is disabled in config (`brain.enabled: false`), returns "Project brain is not configured. See sirtopham.yaml brain section."
- [ ] JSON Schema accurately describes parameters
- [ ] Unit tests with mock client

### brain_read
- [ ] Implements `Tool` interface with purity `Pure`
- [ ] Parameters: `path` (required), `include_backlinks` (optional bool, default false)
- [ ] Calls `ObsidianClient.ReadDocument()` to fetch content
- [ ] Parses and returns: full markdown content, extracted YAML frontmatter (as formatted key-value pairs), outgoing `[[wikilinks]]` extracted via regex
- [ ] If `include_backlinks=true`: uses `ObsidianClient.SearchKeyword()` with the document filename to find documents that reference it (heuristic backlink detection — full graph traversal is v0.2)
- [ ] Document not found: error with listing of available documents in the parent directory
- [ ] JSON Schema accurately describes parameters
- [ ] Unit tests with mock client

### brain_write
- [ ] Implements `Tool` interface with purity `Mutating`
- [ ] Parameters: `path` (required), `content` (required — full markdown including frontmatter)
- [ ] Calls `ObsidianClient.WriteDocument()` to create/overwrite
- [ ] Returns confirmation: "Brain document written: <path>" with byte count
- [ ] If the content doesn't start with YAML frontmatter (`---`), the tool succeeds but logs a warning (frontmatter is encouraged but not enforced)
- [ ] JSON Schema accurately describes parameters
- [ ] Unit tests with mock client

### brain_update
- [ ] Implements `Tool` interface with purity `Mutating`
- [ ] Parameters: `path` (required), `operation` (required — "append", "prepend", "replace_section"), `content` (required), `section` (optional — heading text for replace_section, e.g., "## Workaround")
- [ ] **append:** Reads current document, appends `content` at the end, writes back
- [ ] **prepend:** Reads current document, inserts `content` after frontmatter (if present) or at the start, writes back
- [ ] **replace_section:** Finds the heading matching `section`, replaces content from that heading up to (but not including) the next heading of equal or higher level. If section not found, returns error with list of headings in the document
- [ ] Returns updated document content (or first 100 lines if very long)
- [ ] Document not found: error with directory listing
- [ ] JSON Schema accurately describes parameters with examples in descriptions
- [ ] Unit tests: append, prepend, replace_section (found and not found), missing document

### All four tools
- [ ] Registered in the tool registry
- [ ] All tools check `brain.enabled` config before execution; if disabled, return a clear guidance message
- [ ] Integration test: write a document, read it back, search for it, update a section, read again to verify

---

## Key Design Notes

**Graceful degradation note (historical plan).** This section describes the old REST-plugin failure mode. In current runtime, equivalent brain-tool availability depends on the MCP/vault backend rather than the Local REST plugin.

**No brain indexing in this epic.** The indexing caveat remains historical context, but the old sentence about the runtime operating entirely through the Obsidian REST API is no longer current. Today the operator-facing brain path is MCP/vault-backed keyword retrieval and tool access; semantic/index-backed brain retrieval remains future work unless separately landed.

**Wikilink extraction is lightweight.** `brain_read` extracts outgoing wikilinks via regex (`\[\[([^\]]+)\]\]`) from the markdown content. This is for display purposes, not for graph traversal. Full graph traversal with the `brain_links` SQLite table is v0.2.

**Section replacement parsing.** For `brain_update` with `replace_section`, the heading level matters. If the target is `## Workaround` (h2), the replacement extends until the next h2 or h1. Content under h3 subheadings within the section is included in the replacement. This follows standard markdown section semantics.

**v0.1 -> v0.2 upgrade path.** The tool interface is designed so that adding semantic search and context assembly integration in v0.2 requires no changes to the tool parameters or return format — only the internal implementation of `brain_search` changes (adding vector search alongside keyword search), and a brain indexer is added separately.

---

## Consumed By

- [[layer4-epic01-tool-interface]] — registered in the tool registry
- Layer 5 (Agent Loop) — dispatched via the executor
- Layer 3 (Context Assembly) — already a proactive brain-retrieval source in current runtime; this epic's REST design no longer defines that integration
