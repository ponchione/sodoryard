# Task 09: Query Files — Analytics Queries

**Epic:** 06 — Schema & sqlc Code Generation
**Status:** ⬚ Not started
**Dependencies:** Task 07, Task 02, Task 03

---

## Description

Write SQL query files for the analytics query patterns from doc 08: token usage aggregation, cache hit rate calculation, tool usage breakdown, and context assembly quality aggregation.

## Query Patterns (from doc 08)

**Per-conversation token usage:**
```sql
SELECT SUM(tokens_in) as total_in, SUM(tokens_out) as total_out,
       SUM(cache_read_tokens) as total_cache_hits, COUNT(*) as total_calls,
       SUM(latency_ms) as total_latency_ms
FROM sub_calls WHERE conversation_id = ? AND purpose = 'chat';
```

**Cache hit rate:**
```sql
SELECT SUM(cache_read_tokens) * 100.0 / NULLIF(SUM(tokens_in), 0) as cache_hit_pct
FROM sub_calls WHERE conversation_id = ? AND purpose = 'chat';
```

**Tool usage breakdown:**
```sql
SELECT tool_name, COUNT(*) as call_count, AVG(duration_ms) as avg_duration,
       SUM(CASE WHEN success = 0 THEN 1 ELSE 0 END) as failure_count
FROM tool_executions WHERE conversation_id = ? GROUP BY tool_name;
```

**Context assembly quality:**
```sql
SELECT COUNT(*) as total_turns, SUM(agent_used_search_tool) as reactive_search_turns,
       AVG(context_hit_rate) as avg_hit_rate, AVG(budget_used) as avg_budget_used
FROM context_reports WHERE conversation_id = ?;
```

## Acceptance Criteria

- [ ] Per-conversation token usage aggregation query matching doc 08 pattern (filters on `purpose = 'chat'`)
- [ ] Cache hit rate calculation query using `NULLIF` to avoid division by zero
- [ ] Tool usage breakdown by name query with call count, avg duration, and failure count
- [ ] Context assembly quality aggregation query with reactive search turns and avg hit rate
- [ ] All queries use sqlc annotations and pass `sqlc generate`
