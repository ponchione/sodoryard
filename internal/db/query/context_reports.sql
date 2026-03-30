-- name: InsertContextReport :exec
INSERT INTO context_reports (
    conversation_id,
    turn_number,
    analysis_latency_ms,
    retrieval_latency_ms,
    total_latency_ms,
    needs_json,
    signals_json,
    rag_results_json,
    brain_results_json,
    graph_results_json,
    explicit_files_json,
    budget_total,
    budget_used,
    budget_breakdown_json,
    included_count,
    excluded_count,
    agent_used_search_tool,
    agent_read_files_json,
    context_hit_rate,
    created_at
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
);

-- name: UpdateContextReportQuality :exec
UPDATE context_reports
SET
    agent_used_search_tool = ?,
    agent_read_files_json = ?,
    context_hit_rate = ?
WHERE conversation_id = ?
  AND turn_number = ?;

-- name: GetContextReportByTurn :one
SELECT
    id,
    conversation_id,
    turn_number,
    analysis_latency_ms,
    retrieval_latency_ms,
    total_latency_ms,
    needs_json,
    signals_json,
    rag_results_json,
    brain_results_json,
    graph_results_json,
    explicit_files_json,
    budget_total,
    budget_used,
    budget_breakdown_json,
    included_count,
    excluded_count,
    agent_used_search_tool,
    agent_read_files_json,
    context_hit_rate,
    created_at
FROM context_reports
WHERE conversation_id = ?
  AND turn_number = ?;
