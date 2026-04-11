package index

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"time"

	"github.com/ponchione/sodoryard/internal/config"
)

type projectState struct {
	LastIndexedCommit string
	HasIndexedCommit  bool
}

type fileState struct {
	FilePath   string
	FileHash   string
	ChunkCount int
}

func ensureProjectRecord(ctx context.Context, db *sql.DB, cfg *config.Config) error {
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now().UTC().Format(time.RFC3339)
	name := filepath.Base(cfg.ProjectRoot)
	_, err := db.ExecContext(ctx, `
INSERT INTO projects(id, name, root_path, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	name = excluded.name,
	root_path = excluded.root_path,
	updated_at = excluded.updated_at
`, cfg.ProjectRoot, name, cfg.ProjectRoot, now, now)
	if err != nil {
		return fmt.Errorf("ensure project record: %w", err)
	}
	return nil
}

func loadProjectState(ctx context.Context, db *sql.DB, projectID string) (projectState, error) {
	var state projectState
	var commit sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT last_indexed_commit FROM projects WHERE id = ?`, projectID).Scan(&commit); err != nil {
		return state, fmt.Errorf("load project index state: %w", err)
	}
	if commit.Valid {
		state.LastIndexedCommit = commit.String
		state.HasIndexedCommit = true
	}
	return state, nil
}

func loadFileStates(ctx context.Context, db *sql.DB, projectID string) (map[string]fileState, error) {
	rows, err := db.QueryContext(ctx, `SELECT file_path, file_hash, chunk_count FROM index_state WHERE project_id = ?`, projectID)
	if err != nil {
		return nil, fmt.Errorf("load file index state: %w", err)
	}
	defer rows.Close()

	states := make(map[string]fileState)
	for rows.Next() {
		var state fileState
		if err := rows.Scan(&state.FilePath, &state.FileHash, &state.ChunkCount); err != nil {
			return nil, fmt.Errorf("scan file index state: %w", err)
		}
		states[state.FilePath] = state
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate file index state: %w", err)
	}
	return states, nil
}

func deleteFileStates(ctx context.Context, tx *sql.Tx, projectID string, paths []string) error {
	for _, path := range paths {
		if _, err := tx.ExecContext(ctx, `DELETE FROM index_state WHERE project_id = ? AND file_path = ?`, projectID, path); err != nil {
			return fmt.Errorf("delete index state for %s: %w", path, err)
		}
	}
	return nil
}

func upsertFileStates(ctx context.Context, tx *sql.Tx, projectID string, indexedAt time.Time, states []fileState) error {
	indexedAtStr := indexedAt.UTC().Format(time.RFC3339)
	for _, state := range states {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO index_state(project_id, file_path, file_hash, chunk_count, last_indexed_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(project_id, file_path) DO UPDATE SET
	file_hash = excluded.file_hash,
	chunk_count = excluded.chunk_count,
	last_indexed_at = excluded.last_indexed_at
`, projectID, state.FilePath, state.FileHash, state.ChunkCount, indexedAtStr); err != nil {
			return fmt.Errorf("upsert index state for %s: %w", state.FilePath, err)
		}
	}
	return nil
}

func updateProjectMetadata(ctx context.Context, tx *sql.Tx, projectID, revision string, indexedAt time.Time) error {
	_, err := tx.ExecContext(ctx, `
UPDATE projects
SET last_indexed_commit = ?,
    last_indexed_at = ?,
    updated_at = ?
WHERE id = ?
`, nullableRevision(revision), indexedAt.UTC().Format(time.RFC3339), indexedAt.UTC().Format(time.RFC3339), projectID)
	if err != nil {
		return fmt.Errorf("update project index metadata: %w", err)
	}
	return nil
}

func nullableRevision(revision string) any {
	if revision == "" {
		return nil
	}
	return revision
}
