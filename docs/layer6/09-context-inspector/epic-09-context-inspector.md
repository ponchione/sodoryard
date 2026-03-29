# Layer 6, Epic 09: Context Inspector Debug Panel

**Layer:** 6 — Web Interface & Streaming
**Status:** ⬚ Not started
**Dependencies:** [[layer-6-epic-07-conversation-ui]] (lives within conversation view), [[layer-6-epic-03-rest-api-project-config-metrics]] (REST context report endpoint)

---

## Description

Build the context inspector debug panel — the primary mechanism for tuning sirtopham's context assembly system. Per [[06-context-assembly]]: "This panel is not a nice-to-have. It is the primary mechanism for tuning the relevance threshold, budget cap, and query extraction rules. For a system with no prior art to benchmark against, the debug panel IS the benchmark."

The panel displays the `ContextAssemblyReport` for each turn, showing what the turn analyzer detected, what RAG/brain/structural queries retrieved, what made it into the assembled context and what was excluded (with reasons), budget allocation breakdown, and quality metrics. It receives data from two sources: the `context_debug` WebSocket event (real-time, for the current turn) and the REST context report endpoint (historical, for past turns).

---

## Definition of Done

- [ ] **Panel toggle:** A button or tab in the conversation view toggles the context inspector panel. When open, it appears as a side panel or bottom panel alongside the message thread. When closed, it's fully hidden (no wasted space). Default: closed
- [ ] **Per-turn navigation:** The panel shows data for one turn at a time. Navigation (previous/next turn, or a turn selector) lets the user browse context reports across the conversation's turns
- [ ] **Turn analyzer signals:** Displays the `signals_json` array from the context report. Each signal shows: type (file_ref, symbol_ref, intent_verb, momentum), source text (what triggered it), and extracted value. This tells the developer "what did the analyzer detect in my message?"
- [ ] **Semantic queries:** Shows the queries derived from the user's message that were sent to the RAG pipeline. From the `needs_json` field
- [ ] **RAG results:** Displays `rag_results_json` — the code chunks retrieved by semantic search, pre-filtering. Each result shows: file path, chunk name, similarity score, and whether it was included or excluded in the final context. Color-code or badge included vs excluded. Excluded chunks show the reason (below_threshold, budget_exceeded)
- [ ] **Brain results:** Displays `brain_results_json` — brain documents retrieved. Each shows: document path, title, match score, match mode (keyword/semantic/graph). Included/excluded status
- [ ] **Structural graph results:** If present in `graph_results_json`, displays symbols found via blast radius analysis with relationship type (caller, callee, implements) and depth
- [ ] **Budget breakdown:** Visual display of `budget_breakdown_json` — a bar chart or stacked indicator showing token allocation by category (explicit files, brain, RAG, structural, conventions, git, remaining). Shows `budget_used` / `budget_total`
- [ ] **Included vs excluded summary:** Count of included and excluded chunks. Excluded chunks are browsable with their exclusion reasons. This is the most actionable section — "what did the system miss and why?"
- [ ] **Quality metrics:** Display the three key quality signals:
  - `agent_used_search_tool` — boolean flag, highlighted if true (indicates insufficient proactive context)
  - `context_hit_rate` — percentage, with color coding (green >70%, yellow 40-70%, red <40%)
  - Files the agent read via tool calls (`agent_read_files_json`) cross-referenced against assembled context
- [ ] **Latency:** Display `analysis_latency_ms`, `retrieval_latency_ms`, `total_latency_ms` — how long context assembly took
- [ ] **Real-time for current turn:** The `context_debug` WebSocket event populates the panel for the turn in progress, without waiting for the turn to complete. Past turns load from the REST endpoint `GET /api/metrics/conversation/:id/context/:turn`
- [ ] **Brain document links:** Brain results display their vault paths. v0.1: display path as text. (v0.3 will add `obsidian://` URI links to open in Obsidian, per [[09-project-brain]])

---

## Architecture References

- [[06-context-assembly]] — §Context Assembly Report: the `ContextAssemblyReport` struct with all fields. §Quality Metrics: AgentUsedSearchTool, ContextHitRate, ExcludedChunks analysis. §Tuning Guide: which parameters to adjust based on what the inspector shows
- [[06-context-assembly]] — §Context Serialization: what the assembled context actually looks like (helps the developer understand what the LLM received)
- [[08-data-model]] — §context_reports table: column definitions for all stored fields. JSON columns vs scalar columns
- [[07-web-interface-and-streaming]] — §UI Components: "Context inspector (debug): RAG chunks retrieved, conventions included, token budget allocation"
- [[01-project-vision-and-principles]] — §What Success Looks Like v0.1: "with RAG-assembled context visible in a debug panel"
- [[09-project-brain]] — §Integration with Context Assembly: brain results in assembled context. §Future Directions v0.3: "Open in Obsidian" links

---

## Notes for the Implementing Agent

This panel is dense with information. Prioritize clarity over compactness — it's a debugging tool, not a dashboard. Collapsible sections for each data category (signals, RAG results, brain results, budget, quality) let the developer focus on what they need.

The budget breakdown is the best candidate for a simple visual. A horizontal stacked bar showing the proportion of budget used by each category (RAG in blue, brain in green, explicit files in orange, etc.) communicates the allocation at a glance. Recharts or a simple CSS bar is sufficient.

For the RAG results table: sort by similarity score descending. Highlight the relevance threshold (0.35 default) as a visual line — results above are candidates, results below were filtered. Among candidates, distinguish included (made the budget) from excluded (cut for budget).

The `context_debug` WebSocket event carries the full report as a single JSON payload. When it arrives during a streaming turn, populate the panel immediately. The developer can watch context assembly results while the agent is still generating its response — this is the "debug panel visible during the conversation" experience from the v0.1 success criteria.

Performance note: the JSON payloads in `context_reports` can be large (dozens of RAG results with full scores). Parse and render progressively — don't block the UI on parsing. React's `useMemo` or lazy parsing can help.

For v0.1, the panel does not need to be beautiful. It needs to be correct, complete, and readable. Polish comes later.
