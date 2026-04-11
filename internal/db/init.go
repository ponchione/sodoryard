package db

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"strings"
)

//go:embed schema.sql
var schemaSQL string

const dropSchemaSQL = `
DROP TRIGGER IF EXISTS messages_fts_insert;
DROP TRIGGER IF EXISTS messages_fts_delete;
DROP TRIGGER IF EXISTS messages_fts_update;
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

const messagesFTSInsertTriggerSQL = `
CREATE TRIGGER messages_fts_insert AFTER INSERT ON messages
WHEN NEW.role IN ('user', 'assistant', 'tool')
BEGIN
    INSERT INTO messages_fts(rowid, content) VALUES (NEW.id, NEW.content);
END;`

const messagesFTSDeleteTriggerSQL = `
CREATE TRIGGER messages_fts_delete AFTER DELETE ON messages
WHEN OLD.role IN ('user', 'assistant', 'tool')
BEGIN
    INSERT INTO messages_fts(messages_fts, rowid, content)
    VALUES ('delete', OLD.id, OLD.content);
END;`

const messagesFTSUpdateTriggerSQL = `
CREATE TRIGGER messages_fts_update AFTER UPDATE OF content ON messages
WHEN NEW.role IN ('user', 'assistant', 'tool')
BEGIN
    INSERT INTO messages_fts(messages_fts, rowid, content)
    VALUES ('delete', OLD.id, OLD.content);
    INSERT INTO messages_fts(rowid, content) VALUES (NEW.id, NEW.content);
END;`

// Init recreates the full SQLite schema from scratch.
// WARNING: This drops all existing tables first. Use InitIfNeeded for idempotent setup.
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

// InitIfNeeded creates the schema only if the core tables do not yet exist.
// Safe to call repeatedly — returns (true, nil) if schema was created,
// (false, nil) if it already existed.
func InitIfNeeded(ctx context.Context, db *sql.DB) (created bool, err error) {
	if ctx == nil {
		ctx = context.Background()
	}

	var count int
	err = db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='projects'`,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check existing schema: %w", err)
	}
	if count > 0 {
		return false, nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin schema init transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, schemaSQL); err != nil {
		return false, fmt.Errorf("apply schema: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit schema init transaction: %w", err)
	}
	return true, nil
}

// EnsureMessageSearchIndexesIncludeTools upgrades older databases whose FTS
// triggers only indexed user/assistant rows. It is safe to call repeatedly.
func EnsureMessageSearchIndexesIncludeTools(ctx context.Context, db *sql.DB) error {
	if ctx == nil {
		ctx = context.Background()
	}

	var ftsTableCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='messages_fts'`).Scan(&ftsTableCount); err != nil {
		return fmt.Errorf("check messages_fts table: %w", err)
	}
	if ftsTableCount == 0 {
		return nil
	}

	var triggerSQL sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type='trigger' AND name='messages_fts_insert'`).Scan(&triggerSQL); err != nil {
		if err == sql.ErrNoRows {
			triggerSQL = sql.NullString{}
		} else {
			return fmt.Errorf("inspect messages_fts_insert trigger: %w", err)
		}
	}
	if triggerSQL.Valid && strings.Contains(triggerSQL.String, "'tool'") {
		return nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin messages_fts trigger upgrade transaction: %w", err)
	}
	defer tx.Rollback()

	for _, stmt := range []string{
		`DROP TRIGGER IF EXISTS messages_fts_insert;`,
		`DROP TRIGGER IF EXISTS messages_fts_delete;`,
		`DROP TRIGGER IF EXISTS messages_fts_update;`,
		messagesFTSInsertTriggerSQL,
		messagesFTSDeleteTriggerSQL,
		messagesFTSUpdateTriggerSQL,
		`INSERT INTO messages_fts(messages_fts) VALUES ('rebuild');`,
	} {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("upgrade messages_fts triggers: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit messages_fts trigger upgrade transaction: %w", err)
	}
	return nil
}

// EnsureContextReportsIncludeTokenBudget upgrades older databases whose
// context_reports table predates token_budget_json persistence. It is safe to
// call repeatedly.
func EnsureContextReportsIncludeTokenBudget(ctx context.Context, db *sql.DB) error {
	if ctx == nil {
		ctx = context.Background()
	}
	exists, err := tableHasColumn(ctx, db, "context_reports", "token_budget_json")
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	if _, err := db.ExecContext(ctx, `ALTER TABLE context_reports ADD COLUMN token_budget_json TEXT`); err != nil {
		return fmt.Errorf("add context_reports.token_budget_json: %w", err)
	}
	return nil
}

func EnsureChainSchema(ctx context.Context, db *sql.DB) error {
	if ctx == nil {
		ctx = context.Background()
	}
	const ddl = `
CREATE TABLE IF NOT EXISTS chains (
    id                  TEXT PRIMARY KEY,
    source_specs        TEXT,
    source_task         TEXT,
    status              TEXT NOT NULL DEFAULT 'running',
    summary             TEXT,
    total_steps         INTEGER NOT NULL DEFAULT 0,
    total_tokens        INTEGER NOT NULL DEFAULT 0,
    total_duration_secs INTEGER NOT NULL DEFAULT 0,
    resolver_loops      INTEGER NOT NULL DEFAULT 0,
    max_steps           INTEGER NOT NULL DEFAULT 100,
    max_resolver_loops  INTEGER NOT NULL DEFAULT 3,
    max_duration_secs   INTEGER NOT NULL DEFAULT 14400,
    token_budget        INTEGER NOT NULL DEFAULT 5000000,
    started_at          TEXT NOT NULL DEFAULT (datetime('now')),
    completed_at        TEXT,
    created_at          TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at          TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE TABLE IF NOT EXISTS steps (
    id                  TEXT PRIMARY KEY,
    chain_id            TEXT NOT NULL REFERENCES chains(id),
    sequence_num        INTEGER NOT NULL,
    role                TEXT NOT NULL,
    task                TEXT NOT NULL,
    task_context        TEXT,
    status              TEXT NOT NULL DEFAULT 'pending',
    verdict             TEXT,
    receipt_path        TEXT,
    tokens_used         INTEGER NOT NULL DEFAULT 0,
    turns_used          INTEGER NOT NULL DEFAULT 0,
    duration_secs       INTEGER NOT NULL DEFAULT 0,
    exit_code           INTEGER,
    error_message       TEXT,
    started_at          TEXT,
    completed_at        TEXT,
    created_at          TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_steps_chain ON steps(chain_id);
CREATE INDEX IF NOT EXISTS idx_steps_status ON steps(status);
CREATE TABLE IF NOT EXISTS events (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    chain_id            TEXT NOT NULL REFERENCES chains(id),
    step_id             TEXT REFERENCES steps(id),
    event_type          TEXT NOT NULL,
    event_data          TEXT,
    created_at          TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_events_chain ON events(chain_id);
CREATE INDEX IF NOT EXISTS idx_events_created ON events(created_at);
`
	if _, err := db.ExecContext(ctx, ddl); err != nil {
		return fmt.Errorf("ensure chain schema: %w", err)
	}
	return nil
}

func tableHasColumn(ctx context.Context, db *sql.DB, table string, column string) (bool, error) {
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
	if err != nil {
		return false, fmt.Errorf("inspect table %s: %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name string
		var dataType string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk); err != nil {
			return false, fmt.Errorf("scan table_info(%s): %w", table, err)
		}
		if strings.EqualFold(name, column) {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("iterate table_info(%s): %w", table, err)
	}
	return false, nil
}
