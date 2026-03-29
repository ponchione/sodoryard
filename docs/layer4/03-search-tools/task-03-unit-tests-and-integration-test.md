# Task 03: Unit Tests and Integration Test

**Epic:** 03 — Search Tools
**Status:** ⬚ Not started
**Dependencies:** Task 01, Task 02

---

## Description

Write comprehensive unit tests for both search tools and an integration test that registers them in the registry and dispatches calls through the executor. `search_text` tests use a temporary project directory with known files. `search_semantic` tests use a mock `Searcher` implementation to avoid requiring a running embedding container or LanceDB.

## Acceptance Criteria

- [ ] All tests pass via `go test ./internal/tool/...`
- [ ] **search_text — successful search:** Create temp files with known content, search for a pattern that matches, verify structured results contain correct file paths, line numbers, and matched content
- [ ] **search_text — no results:** Search for a pattern that doesn't match anything, verify `Success=true` with "No matches found" message
- [ ] **search_text — file glob filter:** Create `.go` and `.md` files, search with `file_glob: "*.go"`, verify only `.go` files appear in results
- [ ] **search_text — regex pattern:** Search with a regex pattern (e.g., `func\s+\w+`), verify matches are correct
- [ ] **search_text — ripgrep not found:** Override PATH to exclude `rg`, verify clear error message about missing binary
- [ ] **search_text — context lines:** Search with `context_lines: 3`, verify surrounding context lines are included in results
- [ ] **search_semantic — successful search:** Mock searcher returns 3 ranked results, verify formatted output contains all three with file paths, names, descriptions, and scores
- [ ] **search_semantic — empty results:** Mock searcher returns no results, verify `Success=true` with guidance message
- [ ] **search_semantic — index not initialized:** Mock searcher returns a "not initialized" error, verify `Success=false` with guidance to run `sirtopham index`
- [ ] **search_semantic — filters passed through:** Call with `language: "go"` and `chunk_type: "function"`, verify mock searcher receives those filters
- [ ] **Integration test:** Register both search tools in a registry, create an executor, dispatch calls for both tools, verify results are returned correctly. For `search_text`, use a temp directory with real files. For `search_semantic`, use a mock searcher.
- [ ] Schema validation: each tool's `Schema()` output is valid JSON and contains expected parameter names
