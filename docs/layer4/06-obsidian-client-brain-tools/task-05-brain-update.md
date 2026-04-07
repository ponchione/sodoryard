# Task 05: brain_update Tool Implementation

Note: this task doc is historical. Current runtime uses the MCP/vault backend for note mutation rather than the older REST-plugin plan described here.

**Epic:** 06 — Obsidian Client & Brain Tools
**Status:** ⬚ Not started
**Dependencies:** Task 01, Layer 4 Epic 01

---

## Description

Historical plan: implement `brain_update` as a `Mutating` tool via the Obsidian REST API. Current runtime note mutation is MCP/vault-backed; this page is retained only as legacy implementation context.

## Acceptance Criteria

- [ ] Implements the `Tool` interface with `Purity() = Mutating`
- [ ] `Name()` returns `"brain_update"`, `Description()` returns a concise one-liner for the LLM
- [ ] Accepts JSON input with parameters: `path` (required string), `operation` (required string — one of `"append"`, `"prepend"`, `"replace_section"`), `content` (required string), `section` (optional string — heading text for `replace_section`, e.g., `"## Workaround"`)
- [ ] Constructor accepts an `*ObsidianClient` and brain config as dependencies
- [ ] Checks `brain.enabled` config before execution. If disabled, returns guidance message.
- [ ] **append:** Reads current document via `ObsidianClient.ReadDocument()`, appends `content` at the end (with a blank line separator), writes back via `ObsidianClient.WriteDocument()`
- [ ] **prepend:** Reads current document, inserts `content` after YAML frontmatter (if present) or at the very start (if no frontmatter). Writes back. The frontmatter is preserved in its original position.
- [ ] **replace_section:** Parses the document to find the heading matching `section` (exact match including the `##` prefix). Replaces content from that heading up to (but not including) the next heading of equal or higher level. Subheadings (lower level) within the section are included in the replacement. Writes the updated document back.
- [ ] `replace_section` with `section` not found: returns `Success=false` with error listing all headings in the document (e.g., `"Section '## Workaround' not found. Available headings: ## Overview, ## Decision, ## Consequences"`)
- [ ] `replace_section` without `section` parameter: returns `Success=false` with `"The 'section' parameter is required for replace_section operation."`
- [ ] Invalid `operation` value: returns `Success=false` with `"Invalid operation: '<value>'. Must be one of: append, prepend, replace_section."`
- [ ] Returns the updated document content — first 100 lines if the document is longer, with a note `"[showing first 100 of N lines]"`
- [ ] Document not found: returns `Success=false` with enriched error listing available documents in the parent directory
- [ ] `Schema()` returns valid JSON Schema with parameter types, descriptions, enum values for `operation`, and usage examples
- [ ] Unit tests with mock client: append to existing document, prepend with and without frontmatter, replace_section (found heading, heading not found with listing), missing section parameter, invalid operation, document not found
