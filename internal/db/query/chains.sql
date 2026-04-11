-- name: CreateChain :exec
INSERT INTO chains (
    id,
    source_specs,
    source_task,
    status,
    max_steps,
    max_resolver_loops,
    max_duration_secs,
    token_budget
) VALUES (?, ?, ?, 'running', ?, ?, ?, ?);

-- name: GetChain :one
SELECT * FROM chains WHERE id = ?;

-- name: ListChains :many
SELECT * FROM chains ORDER BY created_at DESC LIMIT ?;

-- name: UpdateChainStatus :exec
UPDATE chains
SET status = ?, updated_at = datetime('now')
WHERE id = ?;

-- name: UpdateChainMetrics :exec
UPDATE chains
SET total_steps = ?,
    total_tokens = ?,
    total_duration_secs = ?,
    resolver_loops = ?,
    updated_at = datetime('now')
WHERE id = ?;

-- name: CompleteChain :exec
UPDATE chains
SET status = ?,
    summary = ?,
    completed_at = datetime('now'),
    updated_at = datetime('now')
WHERE id = ?;

-- name: CreateStep :exec
INSERT INTO steps (
    id,
    chain_id,
    sequence_num,
    role,
    task,
    task_context,
    status
) VALUES (?, ?, ?, ?, ?, ?, 'pending');

-- name: StartStep :exec
UPDATE steps
SET status = 'running',
    started_at = datetime('now')
WHERE id = ?;

-- name: CompleteStep :exec
UPDATE steps
SET status = ?,
    verdict = ?,
    receipt_path = ?,
    tokens_used = ?,
    turns_used = ?,
    duration_secs = ?,
    exit_code = ?,
    error_message = ?,
    completed_at = datetime('now')
WHERE id = ?;

-- name: GetStep :one
SELECT * FROM steps WHERE id = ?;

-- name: ListStepsByChain :many
SELECT * FROM steps WHERE chain_id = ? ORDER BY sequence_num ASC;

-- name: CountResolverStepsForTaskContext :one
SELECT COUNT(*) FROM steps
WHERE chain_id = ? AND role = 'resolver' AND task_context = ?;

-- name: CreateEvent :exec
INSERT INTO events (chain_id, step_id, event_type, event_data)
VALUES (?, ?, ?, ?);

-- name: ListEventsByChain :many
SELECT * FROM events WHERE chain_id = ? ORDER BY id ASC;

-- name: ListEventsByChainSince :many
SELECT * FROM events
WHERE chain_id = ? AND id > ?
ORDER BY id ASC;
