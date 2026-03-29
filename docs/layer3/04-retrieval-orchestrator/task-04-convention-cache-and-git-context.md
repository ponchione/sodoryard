# Task 04: Convention Cache and Git Context Paths

**Epic:** 04 — Retrieval Orchestrator
**Status:** ⬚ Not started
**Dependencies:** Task 01

---

## Description

Implement the convention cache and git context retrieval paths. The convention cache path loads cached conventions from the Layer 1 convention extractor when `ContextNeeds.IncludeConventions` is true. The git context path executes `git log --oneline -N` (where N is `GitContextDepth`) using `exec.CommandContext` with the project root as working directory when `ContextNeeds.IncludeGitContext` is true. Both paths return string content that feeds into the serializer.

## Acceptance Criteria

- [ ] **Convention cache path:** If `ContextNeeds.IncludeConventions` is true, loads cached conventions from the Layer 1 convention extractor
- [ ] Returns convention text as a string in `RetrievalResults.ConventionText`
- [ ] If conventions are not cached or loading fails, returns empty string with a logged warning
- [ ] **Git context path:** If `ContextNeeds.IncludeGitContext` is true, executes `git log --oneline -N` where N is `ContextNeeds.GitContextDepth`
- [ ] Uses `exec.CommandContext` with the project root as working directory
- [ ] Returns git log output as a string in `RetrievalResults.GitContext`
- [ ] If `git log` fails (not a git repo, git not installed), returns empty string with a logged warning
- [ ] Both paths are conditional: skipped entirely when their respective `ContextNeeds` flags are false
- [ ] Package compiles with no errors
