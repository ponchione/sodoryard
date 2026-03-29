-- name: GetConversationTokenUsage :one
SELECT
    CAST(COALESCE(SUM(tokens_in), 0) AS INTEGER) AS total_in,
    CAST(COALESCE(SUM(tokens_out), 0) AS INTEGER) AS total_out,
    CAST(COALESCE(SUM(cache_read_tokens), 0) AS INTEGER) AS total_cache_hits,
    COUNT(*) AS total_calls,
    CAST(COALESCE(SUM(latency_ms), 0) AS INTEGER) AS total_latency_ms
FROM sub_calls
WHERE conversation_id = ? AND purpose = 'chat';

-- name: GetConversationCacheHitRate :one
SELECT
    CAST(COALESCE(SUM(cache_read_tokens) * 100.0 / NULLIF(SUM(tokens_in), 0), 0.0) AS REAL) AS cache_hit_pct
FROM sub_calls
WHERE conversation_id = ? AND purpose = 'chat';

-- name: GetConversationToolUsage :many
SELECT
    tool_name,
    COUNT(*) AS call_count,
    AVG(duration_ms) AS avg_duration,
    SUM(CASE WHEN success = 0 THEN 1 ELSE 0 END) AS failure_count
FROM tool_executions
WHERE conversation_id = ?
GROUP BY tool_name;

-- name: GetConversationContextQuality :one
SELECT
    COUNT(*) AS total_turns,
    SUM(agent_used_search_tool) AS reactive_search_turns,
    AVG(context_hit_rate) AS avg_hit_rate,
    AVG(budget_used) AS avg_budget_used
FROM context_reports
WHERE conversation_id = ?;
