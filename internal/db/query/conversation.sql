-- name: ReconstructConversationHistory :many
SELECT role, content, tool_use_id, tool_name
FROM messages
WHERE conversation_id = ? AND is_compressed = 0
ORDER BY sequence;

-- name: ListActiveMessages :many
SELECT id, conversation_id, role, content, tool_use_id, tool_name, turn_number, iteration, sequence,
       is_compressed, is_summary, compressed_turn_start, compressed_turn_end, created_at
FROM messages
WHERE conversation_id = ? AND is_compressed = 0
ORDER BY sequence;

-- name: NextMessageSequence :one
SELECT COALESCE(MAX(sequence) + 1.0, 0.0)
FROM messages
WHERE conversation_id = ?;

-- name: NextTurnNumber :one
SELECT COALESCE(MAX(turn_number), 0) + 1
FROM messages
WHERE conversation_id = ?;

-- name: InsertUserMessage :exec
INSERT INTO messages (
    conversation_id,
    role,
    content,
    turn_number,
    iteration,
    sequence,
    created_at
) VALUES (
    ?,
    'user',
    ?,
    ?,
    1,
    ?,
    ?
);

-- name: TouchConversationUpdatedAt :exec
UPDATE conversations
SET updated_at = ?
WHERE id = ?;

-- name: ListConversations :many
SELECT id, title, updated_at
FROM conversations
WHERE project_id = ?
ORDER BY updated_at DESC
LIMIT ? OFFSET ?;

-- name: ListTurnMessages :many
SELECT id, role, content, tool_use_id, tool_name, turn_number, iteration, sequence
FROM messages
WHERE conversation_id = ?
ORDER BY sequence;

-- name: InsertIterationMessage :exec
INSERT INTO messages (
    conversation_id,
    role,
    content,
    tool_use_id,
    tool_name,
    turn_number,
    iteration,
    sequence,
    created_at
) VALUES (
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    ?
);

-- name: LatestAssistantMessageIDForIteration :one
SELECT id
FROM messages
WHERE conversation_id = ?
  AND turn_number = ?
  AND iteration = ?
  AND role = 'assistant'
ORDER BY sequence DESC
LIMIT 1;

-- name: DeleteIterationMessages :exec
DELETE FROM messages
WHERE conversation_id = ? AND turn_number = ? AND iteration = ? AND role != 'user';

-- name: DeleteIterationToolExecutions :exec
DELETE FROM tool_executions
WHERE conversation_id = ? AND turn_number = ? AND iteration = ?;

-- name: DeleteIterationSubCalls :exec
DELETE FROM sub_calls
WHERE conversation_id = ? AND turn_number = ? AND iteration = ?;

-- name: SearchConversations :many
SELECT c.id, c.title, c.updated_at, m.role, snippet(messages_fts, 0, '<b>', '</b>', '...', 32) AS snippet
FROM messages_fts
JOIN messages m ON m.id = messages_fts.rowid
JOIN conversations c ON c.id = m.conversation_id
WHERE messages_fts.content MATCH ?
ORDER BY rank
LIMIT 20;

-- name: ListAllMessages :many
SELECT id, role, content, tool_use_id, tool_name, turn_number, iteration, sequence,
       is_compressed, is_summary, created_at
FROM messages
WHERE conversation_id = ?
ORDER BY sequence;

-- name: ListMessagePage :many
SELECT id, role, content, tool_use_id, tool_name, turn_number, iteration, sequence,
       is_compressed, is_summary, created_at
FROM (
    SELECT id, role, content, tool_use_id, tool_name, turn_number, iteration, sequence,
           is_compressed, is_summary, created_at
    FROM messages
    WHERE conversation_id = ?
    ORDER BY sequence DESC
    LIMIT ? OFFSET ?
)
ORDER BY sequence;

-- name: InsertConversation :exec
INSERT INTO conversations (id, project_id, title, model, provider, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: GetConversation :one
SELECT id, project_id, title, model, provider, created_at, updated_at
FROM conversations
WHERE id = ?;

-- name: DeleteConversation :exec
DELETE FROM conversations WHERE id = ?;

-- name: SetConversationTitle :exec
UPDATE conversations
SET title = ?, updated_at = ?
WHERE id = ?;

-- name: SetConversationRuntimeDefaults :exec
UPDATE conversations
SET model = ?, provider = ?, updated_at = ?
WHERE id = ?;

-- name: CountConversations :one
SELECT COUNT(*) FROM conversations WHERE project_id = ?;
