# Task 02: git_diff Implementation

**Epic:** 04 — Git Tools
**Status:** ⬚ Not started
**Dependencies:** Layer 4 Epic 01

---

## Description

Implement the `git_diff` tool as a `Pure` tool in `internal/tool/`. This tool returns unified diff output by shelling out to `git diff` with various modes: working tree (unstaged), staged, single ref comparison, and ref-to-ref comparison. An optional path parameter scopes the diff to a specific file or directory. The tool returns raw diff output and relies on the executor's output truncation for oversized diffs.

## Acceptance Criteria

- [ ] Implements the `Tool` interface with `Purity() = Pure`
- [ ] `Name()` returns `"git_diff"`, `Description()` returns a concise one-liner for the LLM
- [ ] Accepts JSON input with parameters: `ref1` (optional string), `ref2` (optional string), `staged` (optional bool, default false), `path` (optional string)
- [ ] Builds the `git diff` command based on parameter combination:
  - No refs, `staged=false`: `git diff` (unstaged working tree changes)
  - No refs, `staged=true`: `git diff --cached` (staged changes)
  - `ref1` only: `git diff <ref1>` (diff between ref and working tree)
  - `ref1` + `ref2`: `git diff <ref1> <ref2>` (diff between two refs)
  - `path` appended as `-- <path>` to any of the above combinations
- [ ] Runs from `projectRoot` as working directory via `exec.CommandContext`
- [ ] Returns the raw unified diff output from git
- [ ] No diff (no changes): returns `Success=true` with `"No differences found"`
- [ ] Invalid ref: returns `Success=false` with enriched error: `"Ref '<ref>' not found. Use git_status to see available branches and recent commits."`
- [ ] `git` binary not found in PATH: returns `Success=false` with clear error
- [ ] Not a git repository: returns `Success=false` with clear error
- [ ] `Schema()` returns valid JSON Schema with parameter types, descriptions, defaults, and usage examples in the description field showing common invocations
