# Task 04: Registration and Unit Tests

**Epic:** 02 — File Tools
**Status:** ⬚ Not started
**Dependencies:** Task 01, Task 02, Task 03

---

## Description

Register all three file tools (`file_read`, `file_write`, `file_edit`) in the tool registry and write comprehensive unit tests for each tool. Tests use a temporary directory as the project root and verify all happy paths, error paths, and edge cases. The registration function is the central wiring point that Layer 5 calls at startup.

## Acceptance Criteria

- [ ] A `RegisterFileTools(registry *Registry)` function (or equivalent) that registers all three file tools
- [ ] All tests in `internal/tool/` package (e.g., `file_read_test.go`, `file_write_test.go`, `file_edit_test.go`)
- [ ] All tests pass via `go test ./internal/tool/...`
- [ ] Tests use `t.TempDir()` as the project root to avoid filesystem side effects
- [ ] **file_read tests:**
  - Normal file read with line numbers verified
  - Line range read (`line_start=5, line_end=10`) returns exactly those lines with correct line numbers
  - File not found returns directory listing
  - Path traversal (`../../../etc/passwd`) rejected
  - Binary file (containing null bytes) detected and reported
  - Empty file returns appropriate message
- [ ] **file_write tests:**
  - New file creation in a nested directory that doesn't exist yet — verify directories created and file written
  - Overwrite existing file — verify unified diff returned, verify file content updated
  - Diff truncation — overwrite with a change that produces > 50 lines of diff, verify truncation notice
  - Path traversal rejected
- [ ] **file_edit tests:**
  - Successful edit — verify file content changed and unified diff returned
  - Zero matches — verify error message about typos/whitespace
  - Multiple matches — verify error message with count
  - File not found — verify enriched error with directory listing
  - Path traversal rejected
- [ ] Schema validation: each tool's `Schema()` output is valid JSON and contains the expected parameter names
