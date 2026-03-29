# Task 04: Git-Aware Incremental Indexing

**Epic:** 07 — Indexing Pipeline
**Status:** ⬚ Not started
**Dependencies:** Task 01, Task 03, L0-E06 (schema/sqlc — projects table)

---

## Description

Implement the git-diff-based incremental indexing that identifies changed files since the last indexed commit. This is net-new for sirtopham (topham uses only file hash comparison). The implementation shells out to `git diff --name-only` — no go-git library, consistent with the project's "shell git execution" principle from spec 01. Git-aware indexing narrows the file set before file hash comparison, making incremental re-indexing faster on large projects.

## Function Signatures

```go
// gitChangedFiles returns file paths changed between the last indexed commit
// and HEAD. Returns nil, nil if git is unavailable or no last_indexed_commit
// is stored (indicating a full index is needed).
func (idx *Indexer) gitChangedFiles(ctx context.Context) ([]string, error)

// currentGitCommit returns the current HEAD commit SHA.
func (idx *Indexer) currentGitCommit(ctx context.Context) (string, error)
```

## Acceptance Criteria

- [ ] `currentGitCommit` runs `git -C <projectRoot> rev-parse HEAD` and returns the full SHA. Returns an error if the project is not a git repo or git is not installed
- [ ] `gitChangedFiles` reads `last_indexed_commit` from the `projects` table via sqlc query
- [ ] If `last_indexed_commit` is NULL or empty (first index), returns `nil, nil` — signaling that a full index is needed, falling through to file-hash-based change detection
- [ ] If `last_indexed_commit` is set, runs: `git -C <projectRoot> diff --name-only <lastIndexedCommit>..HEAD`
- [ ] Parses the output: one relative file path per line, trimmed of whitespace
- [ ] Also runs `git -C <projectRoot> diff --name-only` (no commit range) to capture unstaged working directory changes
- [ ] Merges both lists (committed changes + working directory changes), deduplicates
- [ ] Returns the merged, deduplicated list of changed relative paths
- [ ] If `git diff` fails (e.g., the last_indexed_commit SHA no longer exists after a force push or rebase), logs a warning and returns `nil, nil` — triggering a full index via file hash comparison
- [ ] The git commands use `exec.CommandContext` for context cancellation support
- [ ] Git command timeout: 30 seconds (configurable or hardcoded — git diff should be fast)
- [ ] The changed file list from git is used to pre-filter the walker output in Pass 1 — only files appearing in the git diff list are candidates for hash comparison. Files not in the git diff list are skipped without even computing their hash
- [ ] After a successful index run, `last_indexed_commit` is updated in the `projects` table (this write happens in Task 08)
