# Task 03: Unit Tests

**Epic:** 04 — Git Tools
**Status:** ⬚ Not started
**Dependencies:** Task 01, Task 02

---

## Description

Write comprehensive unit tests for both git tools using temporary git repositories created in `t.TempDir()`. Each test initializes a fresh git repo with known commits and file states, then exercises the tool and verifies the structured output. Tests also cover error paths: non-git directories, missing git binary, and invalid refs.

## Acceptance Criteria

- [ ] All tests in `internal/tool/` package (e.g., `git_status_test.go`, `git_diff_test.go`)
- [ ] All tests pass via `go test ./internal/tool/...`
- [ ] Test helper: `setupGitRepo(t *testing.T) string` that creates a temp dir, runs `git init`, configures user.name/email, creates an initial commit, and returns the path — reused across tests
- [ ] **git_status — normal status:** Create a repo with commits, modify a file, add an untracked file. Verify output contains correct branch name, modified file with `M` status, untracked file with `??` status, and recent commit messages.
- [ ] **git_status — clean repo:** Create a repo with a commit, make no changes. Verify output contains `"Working tree clean"`.
- [ ] **git_status — recent_commits parameter:** Create a repo with 10 commits, request `recent_commits: 3`. Verify only 3 commit lines appear.
- [ ] **git_status — not a git repo:** Point the tool at a plain temp directory (no `.git`). Verify `Success=false` with "Not a git repository" message.
- [ ] **git_diff — working tree diff:** Modify a tracked file without staging. Verify the unified diff output contains the expected changes.
- [ ] **git_diff — staged diff:** Stage a change. Call with `staged=true`. Verify the diff shows the staged change.
- [ ] **git_diff — ref-to-ref diff:** Create two commits. Call with `ref1=commit1, ref2=commit2`. Verify the diff shows changes between those commits.
- [ ] **git_diff — path scoping:** Modify two files. Call with `path` set to one of them. Verify only that file appears in the diff.
- [ ] **git_diff — no changes:** Call on a clean repo with no unstaged changes. Verify `"No differences found"` message.
- [ ] **git_diff — invalid ref:** Call with `ref1="nonexistent"`. Verify enriched error message.
- [ ] Registration: verify both tools register without panic and appear in `registry.All()`
- [ ] Schema validation: each tool's `Schema()` output is valid JSON and contains expected parameter names
