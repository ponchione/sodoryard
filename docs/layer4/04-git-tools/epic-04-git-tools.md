# Layer 4 — Epic 04: Git Tools

**Layer:** 4 (Tool System)
**Package:** `internal/tool/`
**Status:** ⬚ Not Started
**Dependencies:**
- Layer 4 Epic 01: Tool Interface, Registry & Executor

**Architecture Refs:**
- [[05-agent-loop]] §Tool Set — git_status, git_diff definitions
- [[02-tech-stack-decisions]] — shell git execution (not go-git), carried forward from topham

---

## What This Epic Covers

Two git tools that implement the `Tool` interface. Both shell out to the `git` binary — per [[02-tech-stack-decisions]], sirtopham uses shell git, not go-git, due to index desync issues with the Go library.

**`git_status` (Pure):** Returns the current branch name, list of dirty files (staged, unstaged, untracked) with their status codes, and the N most recent commit summaries (one-line format). Structured output provides a clean snapshot of the repository state.

**`git_diff` (Pure):** Returns the diff output. Supports three modes: working tree diff (unstaged changes), staged diff (`--cached`), and diff between two refs (commits, branches, tags). Returns the raw unified diff output.

---

## Definition of Done

### git_status
- [ ] Implements `Tool` interface with purity `Pure`
- [ ] Parameters: `recent_commits` (optional, default 5 — number of recent commits to show)
- [ ] Returns structured output with three sections:
  - **Branch:** current branch name (from `git rev-parse --abbrev-ref HEAD`)
  - **Status:** dirty files with status codes (from `git status --porcelain`), formatted as `M  internal/auth/middleware.go` etc.
  - **Recent commits:** one-line commit summaries (from `git log --oneline -N`)
- [ ] Runs from `projectRoot` as working directory
- [ ] If not a git repository, returns clear error: "Not a git repository"
- [ ] If `git` is not installed, returns clear error: "git is required but not found in PATH"
- [ ] Clean repo (no dirty files) explicitly states "Working tree clean"
- [ ] JSON Schema accurately describes all parameters
- [ ] Unit tests: normal status, clean repo, not a git repo

### git_diff
- [ ] Implements `Tool` interface with purity `Pure`
- [ ] Parameters: `ref1` (optional), `ref2` (optional), `staged` (optional bool, default false), `path` (optional — restrict diff to a specific file or directory)
  - No refs + `staged=false`: working tree diff (unstaged changes) — `git diff`
  - No refs + `staged=true`: staged changes — `git diff --cached`
  - `ref1` only: diff between ref1 and working tree — `git diff ref1`
  - `ref1` + `ref2`: diff between two refs — `git diff ref1 ref2`
  - `path` appended as `-- path` to restrict scope
- [ ] Returns raw unified diff output
- [ ] If no diff (no changes), returns "No differences found" (success=true)
- [ ] If ref not found, returns git's error message (enriched: "Ref 'xyz' not found. Use git_status to see recent commits.")
- [ ] JSON Schema accurately describes all parameters with clear usage examples in description
- [ ] Unit tests: working tree diff, staged diff, ref-to-ref diff, path-scoped diff, no changes, invalid ref

### Both tools
- [ ] Registered in the tool registry
- [ ] All git commands run with `projectRoot` as working directory
- [ ] Git command execution uses `exec.CommandContext` with the tool's context for cancellation support

---

## Key Design Notes

**Shell git, not go-git.** This is a firm decision from [[02-tech-stack-decisions]], carried forward from topham. The `git` binary must be installed. Both tools should validate this at registration or first use and produce clear errors if missing.

**Output size.** `git_diff` output can be large (thousands of lines for big changes). The executor's output truncation (from Epic 01) handles this — the tool returns the full diff, and the executor truncates if it exceeds `tool_output_max_tokens`. The truncation notice guides the agent to use `git_diff` with a `path` filter for specific files.

**No mutating git operations.** Neither tool modifies the repository. `git add`, `git commit`, `git push` etc. are done via the `shell` tool if the agent needs them. This keeps the git tools pure and safe for parallel execution.

---

## Consumed By

- [[layer4-epic01-tool-interface]] — registered in the tool registry
- Layer 5 (Agent Loop) — dispatched via the executor
