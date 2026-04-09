-- name: UpsertBrainDocument :exec
INSERT INTO brain_documents (
    project_id,
    path,
    title,
    content_hash,
    tags,
    frontmatter,
    token_count,
    created_by,
    source_conversation_id,
    created_at,
    updated_at
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
)
ON CONFLICT(project_id, path) DO UPDATE SET
    title = excluded.title,
    content_hash = excluded.content_hash,
    tags = excluded.tags,
    frontmatter = excluded.frontmatter,
    token_count = excluded.token_count,
    created_by = excluded.created_by,
    source_conversation_id = excluded.source_conversation_id,
    updated_at = excluded.updated_at;

-- name: GetBrainDocumentByPath :one
SELECT id, project_id, path, title, content_hash, tags, frontmatter, token_count,
       created_by, source_conversation_id, created_at, updated_at
FROM brain_documents
WHERE project_id = ?
  AND path = ?;

-- name: ListBrainDocumentsByProject :many
SELECT id, project_id, path, title, content_hash, tags, frontmatter, token_count,
       created_by, source_conversation_id, created_at, updated_at
FROM brain_documents
WHERE project_id = ?
ORDER BY path;

-- name: DeleteBrainDocumentByPath :exec
DELETE FROM brain_documents
WHERE project_id = ?
  AND path = ?;

-- name: DeleteBrainLinksForSource :exec
DELETE FROM brain_links
WHERE project_id = ?
  AND source_path = ?;

-- name: InsertBrainLink :exec
INSERT INTO brain_links (
    project_id,
    source_path,
    target_path,
    link_text
) VALUES (
    ?, ?, ?, ?
);
