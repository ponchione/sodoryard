# Task 11: Unit Tests — File Walker and Glob Filtering

**Epic:** 07 — Indexing Pipeline
**Status:** ⬚ Not started
**Dependencies:** Task 02

---

## Description

Write unit tests for the file walker component, verifying include/exclude glob filtering, max file size enforcement, default excludes, and deterministic output ordering. Tests use temporary directories with controlled file structures — no real project trees.

## Acceptance Criteria

### Include Glob Tests

- [ ] Test: `IncludeGlobs = ["**/*.go"]` — only `.go` files are returned. Create temp dir with `main.go`, `README.md`, `style.css`. Assert only `main.go` is in output
- [ ] Test: `IncludeGlobs = ["**/*.go", "**/*.md"]` — both `.go` and `.md` files are returned
- [ ] Test: `IncludeGlobs = ["internal/**/*.go"]` — only Go files under `internal/` are returned. Create `internal/foo.go`, `cmd/main.go`. Assert only `internal/foo.go` is in output
- [ ] Test: `IncludeGlobs = []` (empty) — all files are included (no include filter applied)

### Exclude Glob Tests

- [ ] Test: `ExcludeGlobs = ["**/*_test.go"]` — test files are excluded. Create `handler.go`, `handler_test.go`. Assert only `handler.go` returned
- [ ] Test: `ExcludeGlobs = ["**/generated/**"]` — files under `generated/` directory are excluded
- [ ] Test: exclude takes priority over include. `IncludeGlobs = ["**/*.go"]`, `ExcludeGlobs = ["**/*_test.go"]`. Assert test files excluded even though they match include

### Default Exclude Tests

- [ ] Test: `.git/` directory is always skipped even if not in ExcludeGlobs. Create `.git/config`, `.git/HEAD`. Assert neither appears in output
- [ ] Test: `vendor/` directory is always skipped
- [ ] Test: `node_modules/` directory is always skipped
- [ ] Test: `.sirtopham/` directory is always skipped

### Max File Size Tests

- [ ] Test: file exceeding `MaxFileSizeBytes` is skipped. Create a file with 100KB content, set limit to 51200. Assert file is not in output
- [ ] Test: file exactly at `MaxFileSizeBytes` is included
- [ ] Test: file 1 byte over `MaxFileSizeBytes` is skipped

### Ordering and Edge Cases

- [ ] Test: output is sorted by relative path (deterministic ordering across runs)
- [ ] Test: empty directory returns empty slice, no error
- [ ] Test: nested directories are traversed correctly. Create `a/b/c/file.go` — assert it appears with correct relative path
- [ ] Test: symlinks are not followed — create a symlink `link.go` pointing to `real.go`. Verify that `walkFiles` does NOT follow the symlink — the output should NOT contain an entry for `link.go` (symlinks are skipped entirely, not resolved to their targets)
