# Task 05: Quality Metrics and Latency Display

**Epic:** 09 — Context Inspector
**Status:** ⬚ Not started
**Dependencies:** Task 01

---

## Description

Display the per-turn quality metrics and latency measurements that help the developer assess whether context assembly is working well. The three key quality signals are: whether the agent used a search tool (indicating the proactive context was insufficient), the context hit rate (what percentage of assembled context the agent actually used), and a cross-reference of files the agent read via tool calls against what was in the assembled context. Latency metrics show how long context assembly took, broken into analysis and retrieval phases.

## Acceptance Criteria

- [ ] **Quality metrics section:** Collapsible section labeled "Quality" in the context inspector panel
- [ ] **`agent_used_search_tool`:** Displayed as a boolean indicator. Highlighted prominently (e.g., yellow/orange warning badge) if true — this means the proactive context was insufficient and the agent had to search reactively
- [ ] If false, displayed as a green "No reactive search needed" indicator
- [ ] **`context_hit_rate`:** Displayed as a percentage with color coding: green (>70% — context was well-targeted), yellow (40-70% — some waste), red (<40% — most context was unused)
- [ ] Hit rate includes a brief explanation: "Percentage of assembled context chunks that the agent referenced in its response"
- [ ] **Agent-read files cross-reference:** Displays files from `agent_read_files_json` that were NOT in the assembled context — these are files the agent had to fetch manually, indicating gaps in context assembly
- [ ] Cross-reference is presented as a list: "Files read by agent but not in context: [file1.go, file2.go]". If empty (all reads were in context), display "All agent file reads were covered by context assembly"
- [ ] **Latency section:** Displayed below or alongside quality metrics
- [ ] Shows three latency values: `analysis_latency_ms` (turn analyzer), `retrieval_latency_ms` (RAG + brain + graph queries), `total_latency_ms` (end-to-end context assembly)
- [ ] Latency values formatted as human-readable: e.g., "42ms", "1.2s"
- [ ] Latency values color-coded: green (<200ms), yellow (200-500ms), red (>500ms) — thresholds may be adjusted based on real-world data
- [ ] All metrics are displayed for both real-time (from `context_debug` WebSocket event) and historical turns (from REST endpoint)
