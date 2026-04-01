-- name: ListMessagesForCompression :many
SELECT id, conversation_id, role, content, tool_use_id, tool_name, turn_number, iteration, sequence,
       is_compressed, is_summary, compressed_turn_start, compressed_turn_end, created_at
FROM messages
WHERE conversation_id = ?
ORDER BY sequence;

-- name: MarkMessagesCompressedBetweenSequences :exec
UPDATE messages
SET is_compressed = 1
WHERE conversation_id = ?
  AND is_compressed = 0
  AND sequence > ?
  AND sequence < ?;

-- name: InsertCompressionSummary :exec
INSERT INTO messages (
    conversation_id,
    role,
    content,
    turn_number,
    iteration,
    sequence,
    is_compressed,
    is_summary,
    compressed_turn_start,
    compressed_turn_end,
    created_at
) VALUES (
    ?,
    'user',
    ?,
    ?,
    ?,
    ?,
    0,
    1,
    ?,
    ?,
    ?
);

-- name: UpdateMessageContent :exec
UPDATE messages
SET content = ?
WHERE id = ?;

-- name: MarkMessageCompressedByID :exec
UPDATE messages SET is_compressed = 1 WHERE id = ?;
