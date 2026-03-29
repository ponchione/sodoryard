-- name: ReconstructConversationHistory :many
SELECT role, content, tool_use_id, tool_name
FROM messages
WHERE conversation_id = ? AND is_compressed = 0
ORDER BY sequence;

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

-- name: SearchConversations :many
SELECT c.id, c.title, c.updated_at, snippet(messages_fts, 0, '<b>', '</b>', '...', 32) AS snippet
FROM messages_fts
JOIN messages m ON m.id = messages_fts.rowid
JOIN conversations c ON c.id = m.conversation_id
WHERE messages_fts.content MATCH ?
ORDER BY rank
LIMIT 20;
