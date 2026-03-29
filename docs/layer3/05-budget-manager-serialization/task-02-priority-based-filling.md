# Task 02: Priority-Based Filling Algorithm

**Epic:** 05 — Budget Manager & Context Serialization
**Status:** ⬚ Not started
**Dependencies:** Task 01

---

## Description

Implement the priority-based filling algorithm that iterates through retrieval result sources in priority order, adding content until the assembled context budget is exhausted. The priority order reflects signal strength: explicit files first (user mentioned them directly), then top RAG hits, structural graph results, conventions, git context, and finally lower-ranked RAG hits to fill remaining budget. The priority list should be easy to reorder in code for future tuning. Project-brain content is not part of v0.1 context assembly.

## Acceptance Criteria

- [ ] Priority-based iteration through sources in order:
  1. Explicit files (highest signal — user mentioned them directly)
  2. Top RAG code hits (above threshold, de-duped, re-ranked by hit count)
  3. Structural graph results (callers/callees of identified symbols)
  4. Conventions (derived from code analysis)
  5. Git context (recent commits)
  6. Lower-ranked RAG code hits (fill remaining budget)
- [ ] Each content piece's token count estimated and subtracted from remaining budget
- [ ] When budget is exhausted mid-category, remaining items in that category and all lower-priority categories are excluded
- [ ] Priority order is configurable or easily reorderable in code (not deeply hardcoded)
- [ ] Package compiles with no errors
