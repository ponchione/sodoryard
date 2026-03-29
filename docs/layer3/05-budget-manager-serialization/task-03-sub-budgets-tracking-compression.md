# Task 03: Sub-Budgets, Tracking, and Compression Trigger

**Epic:** 05 — Budget Manager & Context Serialization
**Status:** ⬚ Not started
**Dependencies:** Task 02

---

## Description

Implement sub-budget enforcement, inclusion/exclusion tracking, budget breakdown reporting, and the history compression trigger signal. Sub-budgets are soft caps for specific content categories (conventions and git) — unused sub-budget capacity is available for other categories. Tracking records which chunks were included, which were excluded and why, feeding the `ContextAssemblyReport`. The compression trigger fires when conversation history exceeds the configured threshold of the total context window.

## Acceptance Criteria

- [ ] **Sub-budgets (soft caps):**
  - Conventions capped at `ConventionBudgetTokens` (default 3000)
  - Git context capped at `GitContextBudgetTokens` (default 2000)
  - Unused sub-budget capacity is available for other categories (not pre-reserved)
- [ ] **Tracking:** Records which chunks/documents were included and which were excluded
- [ ] Exclusion reasons recorded: `"below_threshold"` for relevance-filtered items, `"budget_exceeded"` for items that did not fit
- [ ] Tracking feeds `ContextAssemblyReport.IncludedChunks`, `ExcludedChunks`, `ExclusionReasons`
- [ ] **Budget breakdown:** Produces `map[string]int` showing tokens consumed per source category: `"explicit_files"`, `"rag"`, `"structural"`, `"conventions"`, `"git"`
- [ ] **History compression trigger:** If conversation history token count exceeds `CompressionThreshold` (default 50%) of the total context window, sets `CompressionNeeded = true` on `BudgetResult`
- [ ] Budget manager does not perform compression — it only signals that compression is needed
- [ ] Package compiles with no errors
