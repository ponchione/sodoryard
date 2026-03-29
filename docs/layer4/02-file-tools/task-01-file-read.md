# Task 01: file_read Implementation

**Epic:** 02 — File Tools
**Status:** ⬚ Not started
**Dependencies:** Layer 4 Epic 01

---

## Description

Implement the `file_read` tool as a `Pure` tool in `internal/tool/`. This tool reads file contents with optional line range selection and returns the content with line numbers prepended. It provides enriched error messages (directory listing on file not found), rejects path traversal outside the project root, and detects binary files. This is the most frequently called tool in the system — every time the agent needs to see code, it uses `file_read`.

## Acceptance Criteria

- [ ] Implements the `Tool` interface with `Purity() = Pure`
- [ ] `Name()` returns `"file_read"`, `Description()` returns a concise one-liner for the LLM
- [ ] Accepts JSON input with parameters: `path` (required string), `line_start` (optional int), `line_end` (optional int)
- [ ] Path resolved relative to `projectRoot`. Absolute paths (starting with `/`) are rejected with a clear error.
- [ ] Path traversal outside project root (e.g., `../../../etc/passwd`) detected after resolution and rejected: `"Path escapes project root. All file operations are restricted to the project directory."`
- [ ] Returns file content with line numbers in right-aligned format with tab separator (e.g., `   15\tfunc ValidateToken(...)`)
- [ ] `line_start` and `line_end` are 1-indexed, inclusive. Omitting `line_start` reads from line 1. Omitting `line_end` reads to end of file.
- [ ] Out-of-range line numbers are clamped (not errored) — `line_start=500` on a 100-line file returns empty content with a note
- [ ] File not found: returns `Success=false` with error listing files in the same directory (e.g., `"File not found: pkg/auth/handler.go. Files in pkg/auth/: middleware.go, token.go, types.go"`)
- [ ] Binary file detection: reads first 8KB, checks for null bytes. If detected, returns `"Binary file detected, cannot display content"` instead of raw binary
- [ ] Empty file: returns success with a note `"(empty file)"` rather than blank content
- [ ] `Schema()` returns valid JSON Schema with parameter types, descriptions, and required fields
