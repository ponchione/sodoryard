# Task 02: Report Persistence to SQLite

**Epic:** 06 — Context Assembly Pipeline
**Status:** ⬚ Not started
**Dependencies:** Task 01; Layer 0: Epic 04 (SQLite), Epic 06 (schema/sqlc)

---

## Description

Implement the persistence of `ContextAssemblyReport` to the `context_reports` SQLite table as step 9 of the assembly pipeline. JSON-serialize complex fields (`ContextNeeds`, signals, retrieval results, budget breakdown) using `json.Marshal` and store as TEXT columns. Quality fields (`agent_used_search_tool`, `agent_read_files_json`, `context_hit_rate`) are zero-valued on INSERT and updated after the turn completes via the `UpdateQuality` method.

## Acceptance Criteria

- [ ] `ContextAssemblyReport` persisted to the `context_reports` SQLite table at step 9
- [ ] INSERT includes: `conversation_id`, `turn_number`, latency fields (`analysis_latency_ms`, `retrieval_latency_ms`, `total_latency_ms`), `needs_json`, `signals_json`, `rag_results_json`, `brain_results_json`, `graph_results_json`, `explicit_files_json`, `budget_total`, `budget_used`, `budget_breakdown_json`, `included_count`, `excluded_count`
- [ ] Complex fields serialized via `json.Marshal` and stored as TEXT in SQLite
- [ ] Quality fields (`agent_used_search_tool`, `agent_read_files_json`, `context_hit_rate`) are zero-valued on INSERT
- [ ] JSON fields serialize and deserialize correctly (round-trip: `ContextNeeds` to JSON and back)
- [ ] Package compiles with no errors
