# Task 04: Context Assembly Quality Metrics and Project Info

**Epic:** 10 — Settings & Metrics
**Status:** ⬚ Not started
**Dependencies:** Task 03

---

## Description

Display the aggregated context assembly quality metrics for the current conversation and integrate project info into the settings or metrics view. The quality metrics show the average context hit rate, the percentage of turns where the agent resorted to reactive search, and the average budget utilization across all turns in the conversation. These aggregate metrics complement the per-turn detail in the context inspector by providing a conversation-level summary of context assembly effectiveness.

## Acceptance Criteria

- [ ] **Context assembly quality section:** Displayed alongside or within the per-conversation metrics (from Task 03)
- [ ] **Average context hit rate:** Displayed as a percentage averaged across all turns in the conversation. Color coded: green (>70%), yellow (40-70%), red (<40%)
- [ ] Accompanied by a brief label: "Avg Context Hit Rate: 62% across 7 turns"
- [ ] **Reactive search percentage:** Percentage of turns where `agent_used_search_tool` was true. Displayed with color coding: green (<10% — context is sufficient), yellow (10-30%), red (>30% — context assembly needs tuning)
- [ ] Label: "Reactive Search: 14% of turns (1 of 7)"
- [ ] **Average budget utilization:** Average percentage of the token budget used across turns. Displayed as a percentage
- [ ] Label: "Avg Budget Used: 71% of 20,000 tokens"
- [ ] **Data source:** All quality metrics derived from the `GET /api/metrics/conversation/:id` response (context assembly quality fields)
- [ ] **Project info in settings:** The settings panel (from Task 01) includes a "Project" section showing: project name, root path, primary language, last indexed at (formatted as relative time or date), last indexed commit hash
- [ ] Project info is read-only — no editing from the UI
- [ ] **No cross-conversation metrics:** Global/aggregate metrics across all conversations are explicitly deferred to v0.5+. The UI does not attempt to show them
- [ ] Quality metrics section is collapsible and defaults to collapsed to avoid overwhelming new users
- [ ] All metrics handle the zero-data case gracefully (new conversation, no turns completed)
