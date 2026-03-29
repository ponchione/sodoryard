# Task 03: Per-Conversation Metrics Endpoint

**Epic:** 03 — REST API Project/Config/Metrics
**Status:** ⬚ Not started
**Dependencies:** Task 01, Layer 0 Epic 06 (schema & sqlc)

---

## Description

Implement the `GET /api/metrics/conversation/:id` endpoint that returns aggregated per-conversation metrics. This endpoint queries the `sub_calls`, `tool_executions`, and `context_reports` tables to compute token usage, cache hit rate, tool usage breakdown, and context assembly quality averages. The data is consumed by the frontend's per-conversation metrics display in the settings/metrics panel.

## Acceptance Criteria

- [ ] `GET /api/metrics/conversation/:id` returns a JSON object with aggregated metrics for the specified conversation
- [ ] **Token usage section:** `{tokens_in, tokens_out, cache_read_tokens, total_calls}` aggregated from `sub_calls` WHERE `purpose='chat'` and `conversation_id` matches
- [ ] **Cache hit rate:** Calculated as `cache_read_tokens / tokens_in * 100` (percentage). Returns 0 if no tokens_in. Included in the response as `cache_hit_rate_pct`
- [ ] **Tool usage breakdown:** Array of `[{tool_name, call_count, avg_duration_ms, failure_count}]` aggregated from `tool_executions` for the conversation
- [ ] **Context assembly quality:** `{total_turns, reactive_search_count, avg_hit_rate, avg_budget_used_pct}` aggregated from `context_reports` for the conversation
- [ ] `reactive_search_count` is the count of turns where `agent_used_search_tool` is true
- [ ] Returns HTTP 404 if the conversation does not exist
- [ ] Returns zeroed metrics (not an error) if the conversation exists but has no sub_calls, tool_executions, or context_reports yet (new conversation)
- [ ] Uses sqlc-generated queries or raw SQL aggregations from the data model's Key Query Patterns section
- [ ] Endpoint is registered at `/api/metrics/conversation/:id` on the server's router
