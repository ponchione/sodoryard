# Task 04: brain_write Tool Implementation

Note: this task doc is historical. Current runtime uses the MCP/vault backend for note mutation rather than the older REST-plugin plan described here.

**Epic:** 06 — Obsidian Client & Brain Tools
**Status:** ⬚ Not started
**Dependencies:** Task 01, Layer 4 Epic 01

---

## Description

Historical plan: implement `brain_write` as a `Mutating` tool via the Obsidian REST API. Current runtime note mutation is MCP/vault-backed; this page is retained only as legacy implementation context.

## Acceptance Criteria

- [ ] Implements the `Tool` interface with `Purity() = Mutating`
- [ ] `Name()` returns `"brain_write"`, `Description()` returns a concise one-liner for the LLM
- [ ] Accepts JSON input with parameters: `path` (required string), `content` (required string — full markdown including frontmatter)
- [ ] Constructor accepts an `*ObsidianClient` and brain config as dependencies
- [ ] Checks `brain.enabled` config before execution. If disabled, returns guidance message.
- [ ] Calls `ObsidianClient.WriteDocument()` to create or overwrite the document
- [ ] Returns `Success=true` with confirmation: `"Brain document written: <path> (<N> bytes)"`
- [ ] If the content does not start with YAML frontmatter (`---` on the first line), the write still succeeds but a warning is logged via structured logging: `"brain_write: document written without YAML frontmatter"` with the path as a log field. Frontmatter is encouraged but not enforced.
- [ ] Connection failure to Obsidian API: returns `Success=false` with the ObsidianClient's descriptive error message
- [ ] `Schema()` returns valid JSON Schema with parameter types, descriptions, and a note in the description that content should include YAML frontmatter
- [ ] Unit tests with mock client: successful write of new document, successful overwrite, write without frontmatter (verify warning logged), Obsidian connection failure
