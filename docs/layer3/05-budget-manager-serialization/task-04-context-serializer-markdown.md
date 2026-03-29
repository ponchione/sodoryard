# Task 04: Context Serializer - Markdown Format

**Epic:** 05 — Budget Manager & Context Serialization
**Status:** ⬚ Not started
**Dependencies:** Task 02

---

## Description

Implement the `ContextSerializer` that takes the budget-fitted content (selected chunks, conventions, git context) and produces a single markdown-formatted string ready for injection as system prompt cache block 2. The format follows the specification from the architecture docs: code chunks grouped by file, description before code body, language-tagged code fences, conventions section, and git context section. Project-brain content is not proactively serialized in v0.1.

## Acceptance Criteria

- [ ] `ContextSerializer` function or struct that takes `BudgetResult` (selected content) and a `seenFiles` set, and produces a markdown-formatted string
- [ ] **Code chunks grouped by file:** `## Relevant Code` section with chunks organized by file path; multiple chunks from the same file grouped under a single file-level header
- [ ] **Description before code:** Each code chunk has: header (`### file/path.go (lines X-Y)`), 1-2 sentence description, then the code fence with language tag
- [ ] **Language-tagged code fences:** Go code in `` ```go ``, TypeScript in `` ```typescript ``, Python in `` ```python ``, etc.
- [ ] **Conventions section:** `## Project Conventions` with 5-10 bullet points (if conventions content is present)
- [ ] **Git context section:** `## Recent Changes (last N commits)` with one-line commit summaries (if git context is present)
- [ ] Output is a single markdown string, deterministic: given the same input, produces the same output (no randomization, no timestamps, no non-deterministic ordering)
- [ ] Package compiles with no errors
