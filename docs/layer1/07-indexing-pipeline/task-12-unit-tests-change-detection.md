# Task 12: Unit Tests — Change Detection and Git Integration

**Epic:** 07 — Indexing Pipeline
**Status:** ⬚ Not started
**Dependencies:** Task 03, Task 04

---

## Description

Write unit tests for the file-hash-based change detection and the git-aware incremental indexing. Change detection tests use mock sqlc query interfaces to simulate stored index_state. Git integration tests use real temporary git repositories to verify `git diff --name-only` parsing.

## Acceptance Criteria

### File Hash Tests

- [ ] Test: `computeFileHash` returns consistent SHA-256 hex digest for known content. Input: `"hello world"` → expected hash: `b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9`
- [ ] Test: `computeFileHash` returns different hashes for different content
- [ ] Test: `computeFileHash` returns error for non-existent file

### Change Detection Tests (using mock queries)

- [ ] Test: file not in index_state (new file) → included in `changed` list
- [ ] Test: file in index_state with matching hash → included in `unchanged` list, not in `changed`
- [ ] Test: file in index_state with different hash (modified) → included in `changed` list
- [ ] Test: file in index_state but not in walker output (deleted) → included in `deleted` list
- [ ] Test: `IndexerConfig.Force = true` → all files in `changed` list regardless of stored hashes
- [ ] Test: empty index_state (first run) → all files in `changed` list

### Git Integration Tests (real temp git repos)

- [ ] Test: `currentGitCommit` returns HEAD SHA from a real temp git repo (create repo with `git init`, commit a file, verify SHA format: 40-char hex)
- [ ] Test: `currentGitCommit` returns error for non-git directory
- [ ] Test: `gitChangedFiles` with `last_indexed_commit` set to initial commit — create two commits, set last_indexed_commit to first. Assert only files changed in second commit are returned
- [ ] Test: `gitChangedFiles` with working directory changes — modify a tracked file without committing. Assert it appears in the output
- [ ] Test: `gitChangedFiles` with NULL `last_indexed_commit` (first index) — returns nil, nil
- [ ] Test: `gitChangedFiles` with invalid `last_indexed_commit` (SHA no longer exists) — returns nil, nil (graceful fallback)
- [ ] Test: deduplication — a file appears in both committed and working directory changes. Assert it appears only once in the output
