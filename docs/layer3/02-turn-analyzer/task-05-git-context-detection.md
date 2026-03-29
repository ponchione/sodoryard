# Task 05: Git Context Detection

**Epic:** 02 — Turn Analyzer
**Status:** ⬚ Not started
**Dependencies:** Task 01

---

## Description

Implement git context detection as the fifth signal rule. When the user message contains keywords related to version control (commit, diff, PR, pull request, merge, branch, recent changes, what changed, last push), set `ContextNeeds.IncludeGitContext = true` with `GitContextDepth` based on keyword specificity. More specific keywords (e.g., "last 3 commits") should set a matching depth, while general keywords use a default depth.

## Acceptance Criteria

- [ ] Git keyword set defined: "commit", "diff", "PR", "pull request", "merge", "branch", "recent changes", "what changed", "last push"
- [ ] When any git keyword is detected, `ContextNeeds.IncludeGitContext` is set to `true`
- [ ] `GitContextDepth` set based on keyword specificity: numeric references (e.g., "last 3 commits") use the specified number; general keywords use a default depth (e.g., 5)
- [ ] Each detection produces a `Signal{Type: "git_context", Source: <matched keyword>, Value: <depth>}`
- [ ] Package compiles with no errors
