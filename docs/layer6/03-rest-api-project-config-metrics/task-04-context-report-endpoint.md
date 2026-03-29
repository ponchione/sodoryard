# Task 04: Context Report Endpoint

**Epic:** 03 — REST API Project/Config/Metrics
**Status:** ⬚ Not started
**Dependencies:** Task 03, Layer 0 Epic 06 (schema & sqlc)

---

## Description

Implement the `GET /api/metrics/conversation/:id/context/:turn` endpoint that returns the full `ContextAssemblyReport` for a specific turn within a conversation. This endpoint returns the JSON columns from the `context_reports` table (needs, signals, RAG results, brain results, graph results, budget breakdown) along with scalar quality metrics. This is the primary data source for the context inspector debug panel when browsing historical turns.

## Acceptance Criteria

- [ ] `GET /api/metrics/conversation/:id/context/:turn` returns the full context report for the specified turn number
- [ ] Response includes all JSON columns from `context_reports`: `needs_json`, `signals_json`, `rag_results_json`, `brain_results_json`, `graph_results_json`, `budget_breakdown_json`
- [ ] JSON columns are returned as parsed JSON objects (not double-encoded strings) — the frontend receives structured data it can render directly
- [ ] Response includes scalar quality metrics: `context_hit_rate`, `agent_used_search_tool`, `budget_used`, `budget_total`, `included_chunks_count`, `excluded_chunks_count`
- [ ] Response includes latency metrics: `analysis_latency_ms`, `retrieval_latency_ms`, `total_latency_ms`
- [ ] Response includes `agent_read_files_json` — the list of files the agent read via tool calls during this turn (for cross-referencing against assembled context)
- [ ] Returns HTTP 404 with `{"error": "context report not found"}` if the conversation or turn does not exist
- [ ] Turn number is validated as a positive integer — non-numeric values return HTTP 400
- [ ] Endpoint is registered at `/api/metrics/conversation/:id/context/:turn` on the server's router
- [ ] Response is served with `Content-Type: application/json`
