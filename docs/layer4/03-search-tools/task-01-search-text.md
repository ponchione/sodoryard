# Task 01: search_text Implementation

**Epic:** 03 — Search Tools
**Status:** ⬚ Not started
**Dependencies:** Layer 4 Epic 01

---

## Description

Implement the `search_text` tool as a `Pure` tool in `internal/tool/`. This tool provides ripgrep-based text search across the project, parsing `rg --json` output into structured results with file paths, line numbers, matched lines, and surrounding context. It respects project exclude patterns and handles edge cases like missing ripgrep binary and zero results.

## Acceptance Criteria

- [ ] Implements the `Tool` interface with `Purity() = Pure`
- [ ] `Name()` returns `"search_text"`, `Description()` returns a concise one-liner for the LLM
- [ ] Accepts JSON input with parameters: `pattern` (required string), `file_glob` (optional string, e.g., `"*.go"`), `context_lines` (optional int, default 2), `max_results` (optional int, default 50)
- [ ] Shells out to `rg` with flags: `--json` for structured output, `-C <context_lines>` for surrounding context, `--max-count <max_results>` for result cap, `--glob <file_glob>` for file filtering
- [ ] Default exclude patterns applied as `--glob '!.git/'`, `--glob '!vendor/'`, `--glob '!node_modules/'` to skip common non-project directories
- [ ] Runs from `projectRoot` as the working directory via `exec.CommandContext`
- [ ] Parses ripgrep JSON output (line-delimited JSON with `type` field) into structured results: file path, line number, matched line content, context lines before, context lines after
- [ ] Formats results as human-readable text for the LLM with file:line headers, e.g.:
  ```
  internal/auth/middleware.go:42
     40  func ValidateToken(token string) error {
     41      if token == "" {
  >  42          return ErrEmptyToken
     43      }
     44      // ...
  ```
- [ ] Zero results: returns `Success=true` with `"No matches found for pattern: '<pattern>'"` — this is not an error
- [ ] If `rg` binary is not found in PATH: returns `Success=false` with `"ripgrep (rg) is required but not found in PATH. Install it: https://github.com/BurntSushi/ripgrep#installation"`
- [ ] Context cancellation propagated to the `rg` subprocess via `exec.CommandContext`
- [ ] `Schema()` returns valid JSON Schema with parameter types, descriptions, required fields, and defaults documented
