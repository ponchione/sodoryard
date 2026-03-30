-- name: InsertSubCall :exec
INSERT INTO sub_calls (
    conversation_id, message_id, turn_number, iteration,
    provider, model, purpose,
    tokens_in, tokens_out, cache_read_tokens, cache_creation_tokens,
    latency_ms, success, error_message, created_at
) VALUES (
    ?1, ?2, ?3, ?4,
    ?5, ?6, ?7,
    ?8, ?9, ?10, ?11,
    ?12, ?13, ?14, ?15
);
