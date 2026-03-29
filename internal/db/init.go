package db

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
)

//go:embed schema.sql
var schemaSQL string

const dropSchemaSQL = `
DROP TRIGGER IF EXISTS messages_fts_insert;
DROP TRIGGER IF EXISTS messages_fts_delete;
DROP TABLE IF EXISTS messages_fts;
DROP TABLE IF EXISTS brain_links;
DROP TABLE IF EXISTS brain_documents;
DROP TABLE IF EXISTS context_reports;
DROP TABLE IF EXISTS sub_calls;
DROP TABLE IF EXISTS tool_executions;
DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS index_state;
DROP TABLE IF EXISTS conversations;
DROP TABLE IF EXISTS projects;
`

// Init recreates the full SQLite schema from scratch.
func Init(ctx context.Context, db *sql.DB) error {
	if ctx == nil {
		ctx = context.Background()
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin schema init transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, dropSchemaSQL); err != nil {
		return fmt.Errorf("drop existing schema: %w", err)
	}
	if _, err := tx.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit schema init transaction: %w", err)
	}
	return nil
}
