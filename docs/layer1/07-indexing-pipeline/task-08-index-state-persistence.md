# Task 08: Index State Persistence

**Epic:** 07 — Indexing Pipeline
**Status:** ⬚ Not started
**Dependencies:** Task 01, Task 03, Task 05, L0-E06 (schema/sqlc — index_state and projects tables)

---

## Description

Implement the persistence of indexing state after a successful pipeline run. This writes updated file hashes and chunk counts to the `index_state` SQLite table and updates `last_indexed_commit` and `last_indexed_at` in the `projects` table. This state is what enables incremental indexing on subsequent runs — without it, every run would be a full re-index.

## Function Signatures

```go
// persistIndexState writes the index state for all successfully processed files
// and cleans up state for deleted files.
func (idx *Indexer) persistIndexState(ctx context.Context, fileStates []FileState, deletedPaths []string) error

// updateProjectCommit updates the last_indexed_commit and last_indexed_at
// on the projects table.
func (idx *Indexer) updateProjectCommit(ctx context.Context, commitSHA string) error
```

Where `FileState` is:
```go
type FileState struct {
    RelPath    string
    FileHash   string
    ChunkCount int
}
```

## Acceptance Criteria

- [ ] `FileState` struct defined with `RelPath`, `FileHash` (hex SHA-256), and `ChunkCount`
- [ ] `persistIndexState` collects `FileState` entries during Pass 1 (one per changed file that was successfully parsed) and writes them after Pass 3 completes
- [ ] For each `FileState`, executes sqlc upsert query against `index_state`:
  ```sql
  INSERT INTO index_state (project_id, file_path, file_hash, chunk_count, last_indexed_at)
  VALUES (?, ?, ?, ?, ?)
  ON CONFLICT(project_id, file_path)
  DO UPDATE SET file_hash = excluded.file_hash,
                chunk_count = excluded.chunk_count,
                last_indexed_at = excluded.last_indexed_at
  ```
- [ ] For each deleted file path, executes sqlc delete query: `DELETE FROM index_state WHERE project_id = ? AND file_path = ?`
- [ ] All `index_state` writes execute within a single SQLite transaction for atomicity
- [ ] `updateProjectCommit` executes sqlc update query:
  ```sql
  UPDATE projects SET last_indexed_commit = ?, last_indexed_at = ?, updated_at = ? WHERE id = ?
  ```
- [ ] `updateProjectCommit` is called only after all index_state writes succeed
- [ ] The `commitSHA` comes from `currentGitCommit` (Task 04). If the project is not a git repo, `last_indexed_commit` is left unchanged (NULL)
- [ ] Timestamps use ISO8601 format (`time.Now().UTC().Format(time.RFC3339)`)
- [ ] On force re-index, all existing `index_state` rows for the project are deleted before inserting new ones:
  `DELETE FROM index_state WHERE project_id = ?`
- [ ] If the SQLite transaction fails (e.g., disk full, database locked), `persistIndexState` returns the error. The orchestrator (Task 10) logs this as an error but does not fail the entire pipeline — indexing results are already in the vector store
