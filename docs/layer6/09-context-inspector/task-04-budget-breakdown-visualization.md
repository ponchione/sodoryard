# Task 04: Budget Breakdown Visualization

**Epic:** 09 — Context Inspector
**Status:** ⬚ Not started
**Dependencies:** Task 01

---

## Description

Display the context assembly budget breakdown as a visual component showing token allocation by category. The budget breakdown answers "how was the context token budget spent?" and shows the proportion allocated to each source (explicit files, brain documents, RAG chunks, structural context, conventions, git context) versus the total budget. A stacked bar or similar visual makes the allocation immediately understandable at a glance.

## Acceptance Criteria

- [ ] **Budget breakdown section:** Collapsible section labeled "Token Budget" in the context inspector panel
- [ ] Rendered from `budget_breakdown_json` in the context report
- [ ] **Visual display:** A horizontal stacked bar chart (or equivalent) showing the proportion of budget used by each category
- [ ] Categories displayed with distinct colors: explicit files, brain documents, RAG chunks, structural context, conventions, git context, remaining/unused
- [ ] Each segment labeled with its category name and token count (on hover or always visible if space permits)
- [ ] **Budget totals:** Display `budget_used` / `budget_total` as a summary (e.g., "14,200 / 20,000 tokens used (71%)")
- [ ] Budget utilization percentage shown with color coding: green (<70% — headroom available), yellow (70-90% — getting tight), red (>90% — near capacity)
- [ ] **Category detail list:** Below or alongside the bar, a list of each category with: name, token count, percentage of total budget
- [ ] Categories are sorted by token usage (largest first) for easy identification of dominant context sources
- [ ] If a category has zero tokens (e.g., no brain documents retrieved), it is either omitted from the bar or shown as a zero-width segment with a note in the detail list
- [ ] The visualization uses simple CSS (stacked flexbox or CSS grid) or a lightweight chart library (e.g., a single Recharts `BarChart`). No heavy charting dependency for this single component
- [ ] When the budget is unset or zero (edge case), display "No budget data" rather than a broken visualization
