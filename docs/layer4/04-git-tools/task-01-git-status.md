# Task 01: git_status Implementation

**Epic:** 04 — Git Tools
**Status:** ⬚ Not started
**Dependencies:** Layer 4 Epic 01

---

## Description

Implement the `git_status` tool as a `Pure` tool in `internal/tool/`. This tool provides a structured snapshot of the git repository state by shelling out to the `git` binary. It returns the current branch name, a list of dirty files with their status codes, and the N most recent one-line commit summaries. This gives the agent a quick overview of the repository without needing to run multiple shell commands.

## Acceptance Criteria

- [ ] Implements the `Tool` interface with `Purity() = Pure`
- [ ] `Name()` returns `"git_status"`, `Description()` returns a concise one-liner for the LLM
- [ ] Accepts JSON input with parameters: `recent_commits` (optional int, default 5)
- [ ] Runs three git commands from `projectRoot` as working directory:
  - `git rev-parse --abbrev-ref HEAD` for the current branch name
  - `git status --porcelain` for dirty file list with status codes
  - `git log --oneline -N` for recent commit summaries (where N is `recent_commits`)
- [ ] All commands executed via `exec.CommandContext` for cancellation support
- [ ] Returns structured output with clear section headers:
  ```
  Branch: main

  Status:
  M  internal/auth/middleware.go
  ?? docs/new-feature.md

  Recent commits:
  a1b2c3d fix: resolve token validation edge case
  e4f5g6h feat: add embedding client batch support
  ```
- [ ] Clean repository (no dirty files): status section shows `"Working tree clean"`
- [ ] Not a git repository: returns `Success=false` with `"Not a git repository (or any parent up to filesystem root)"`
- [ ] `git` binary not found in PATH: returns `Success=false` with `"git is required but not found in PATH"`
- [ ] `Schema()` returns valid JSON Schema with parameter types, descriptions, and defaults
