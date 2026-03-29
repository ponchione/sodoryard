# Task 02: file_write Implementation

**Epic:** 02 — File Tools
**Status:** ⬚ Not started
**Dependencies:** Layer 4 Epic 01

---

## Description

Implement the `file_write` tool as a `Mutating` tool in `internal/tool/`. This tool writes or overwrites a file with provided content. It creates parent directories if they don't exist, generates a unified diff preview on overwrite, and enforces project root sandboxing. The diff preview helps the LLM and UI confirm what changed.

## Acceptance Criteria

- [ ] Implements the `Tool` interface with `Purity() = Mutating`
- [ ] `Name()` returns `"file_write"`, `Description()` returns a concise one-liner for the LLM
- [ ] Accepts JSON input with parameters: `path` (required string), `content` (required string)
- [ ] Path resolved relative to `projectRoot`. Absolute paths rejected. Path traversal outside project root rejected with clear error.
- [ ] Creates parent directories if they don't exist (equivalent to `mkdir -p` on the directory portion of the path)
- [ ] On new file creation: returns `Success=true` with content `"[new file created] <path> (<N> bytes)"`
- [ ] On overwrite of existing file: generates a unified diff between old content and new content, returns the first 50 lines of the diff. If the diff exceeds 50 lines, appends `"[diff truncated — showing 50 of N lines]"`
- [ ] Diff format is human-readable unified diff with `---`/`+++` headers and `@@` hunk markers
- [ ] Writes the file atomically where possible (write to temp file, rename) to avoid partial writes on crash
- [ ] `Schema()` returns valid JSON Schema with parameter types, descriptions, and required fields
