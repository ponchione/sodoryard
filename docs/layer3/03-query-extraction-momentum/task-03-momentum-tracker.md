# Task 03: Momentum Tracker Implementation

**Epic:** 03 — Query Extraction & Momentum
**Status:** ⬚ Not started
**Dependencies:** Epic 01

---

## Description

Implement the `MomentumTracker` that scans recent conversation history to determine the active working area. It extracts file paths from tool calls in the last N turns (configurable via `MomentumLookbackTurns`, default 2), computes the longest common directory prefix as `MomentumModule`, and produces a deduplicated list of touched files as `MomentumFiles`. Momentum is only applied when the turn analyzer detects weak signals or a continuation — strong-signal turns ignore momentum.

## Acceptance Criteria

- [ ] `MomentumTracker` function or struct that scans recent conversation history and populates `MomentumFiles` and `MomentumModule` on `ContextNeeds`
- [ ] Scans the last N turns (configurable via `MomentumLookbackTurns`, default 2)
- [ ] Extracts file paths from tool calls:
  - `file_read` calls: extracts the `path` argument from tool_use input JSON
  - `file_write` / `file_edit` calls: extracts the `path` argument
  - `search_text` / `search_semantic` results: extracts file paths from tool result content
- [ ] `MomentumFiles`: deduplicated list of all extracted file paths
- [ ] `MomentumModule`: longest common directory prefix among extracted paths (e.g., all paths in `internal/auth/` yields `internal/auth`)
- [ ] Edge cases handled: single file (prefix is its directory), files in project root (empty prefix), no files (empty prefix)
- [ ] Momentum only applied when turn analyzer detects continuation signal or weak signals (no explicit files/symbols)
- [ ] Package compiles with no errors
