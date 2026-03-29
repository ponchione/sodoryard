# Task 03: file_edit Implementation

**Epic:** 02 — File Tools
**Status:** ⬚ Not started
**Dependencies:** Layer 4 Epic 01

---

## Description

Implement the `file_edit` tool as a `Mutating` tool in `internal/tool/`. This tool performs search-and-replace within a file, validating that the search string appears exactly once to prevent ambiguous edits. It returns a unified diff of the change. This is dramatically more token-efficient than `file_write` for small edits to large files — the LLM sends only the old and new fragments rather than the entire file content.

## Acceptance Criteria

- [ ] Implements the `Tool` interface with `Purity() = Mutating`
- [ ] `Name()` returns `"file_edit"`, `Description()` returns a concise one-liner for the LLM
- [ ] Accepts JSON input with parameters: `path` (required string), `old_str` (required string), `new_str` (required string)
- [ ] Path resolved relative to `projectRoot`. Absolute paths and path traversal rejected.
- [ ] Reads the file, counts occurrences of `old_str` in the file content
- [ ] Exactly one match: performs the replacement, writes the file, returns unified diff of the change
- [ ] Zero matches: returns `Success=false` with `"String not found in file. Check for typos or whitespace differences."` — includes a few lines of context around where the string might have been expected if the file is small enough
- [ ] Multiple matches (N > 1): returns `Success=false` with `"String appears N times in the file. Provide a longer, more unique search string that includes surrounding context."`
- [ ] File not found: returns `Success=false` with enriched error listing files in the same directory
- [ ] Diff output uses unified diff format showing the changed hunk with surrounding context lines
- [ ] The match is exact (byte-for-byte), not regex — `old_str` is treated as a literal string
- [ ] `Schema()` returns valid JSON Schema with parameter types, descriptions, and required fields
