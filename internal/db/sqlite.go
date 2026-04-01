package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

const (
	DriverName          = "sqlite3"
	expectedJournalMode = "wal"
	expectedBusyTimeout = 5000
	expectedForeignKeys = 1
	expectedSynchronous = 1
)

// OpenDB opens a SQLite database with the project's required pragmas.
//
// The project intentionally uses mattn/go-sqlite3 because CGO is already an
// accepted build dependency elsewhere in the stack. Builds/tests use the
// sqlite_fts5 tag so the Layer 0 FTS5 schema is available at runtime.
func OpenDB(ctx context.Context, filePath string) (*sql.DB, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(filePath) == "" {
		return nil, errors.New("open sqlite database: file path is empty")
	}

	if filePath != ":memory:" {
		parent := filepath.Dir(filePath)
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return nil, fmt.Errorf("open sqlite database %q: create parent directory: %w", filePath, err)
		}
	}

	dsn, err := buildDSN(filePath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database %q: build dsn: %w", filePath, err)
	}

	db, err := sql.Open(DriverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database %q: %w", filePath, err)
	}

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("open sqlite database %q: ping: %w", filePath, err)
	}

	if err := applyAndVerifyPragmas(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("open sqlite database %q: %w", filePath, err)
	}

	return db, nil
}

func buildDSN(filePath string) (string, error) {
	query := url.Values{}
	query.Set("_busy_timeout", fmt.Sprintf("%d", expectedBusyTimeout))
	query.Set("_foreign_keys", "on")
	query.Set("_journal_mode", "WAL")
	query.Set("_synchronous", "NORMAL")
	query.Set("_txlock", "immediate")

	if filePath == ":memory:" {
		query.Set("mode", "memory")
		return "file::memory:?" + query.Encode(), nil
	}

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return "", err
	}

	u := url.URL{Scheme: "file", Path: absPath, RawQuery: query.Encode()}
	return u.String(), nil
}

func applyAndVerifyPragmas(ctx context.Context, db *sql.DB) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection for pragma setup: %w", err)
	}
	defer conn.Close()

	var journalMode string
	if err := conn.QueryRowContext(ctx, "PRAGMA journal_mode = WAL;").Scan(&journalMode); err != nil {
		return fmt.Errorf("apply journal_mode pragma: %w", err)
	}
	if strings.ToLower(journalMode) != expectedJournalMode {
		return fmt.Errorf("verify pragma journal_mode=%q (want %q)", journalMode, expectedJournalMode)
	}

	if _, err := conn.ExecContext(ctx, "PRAGMA busy_timeout = 5000;"); err != nil {
		return fmt.Errorf("apply busy_timeout pragma: %w", err)
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = ON;"); err != nil {
		return fmt.Errorf("apply foreign_keys pragma: %w", err)
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA synchronous = NORMAL;"); err != nil {
		return fmt.Errorf("apply synchronous pragma: %w", err)
	}

	var busyTimeout, foreignKeys, synchronous int
	if err := conn.QueryRowContext(ctx, "PRAGMA busy_timeout;").Scan(&busyTimeout); err != nil {
		return fmt.Errorf("verify busy_timeout pragma: %w", err)
	}
	if busyTimeout != expectedBusyTimeout {
		return fmt.Errorf("verify pragma busy_timeout=%d (want %d)", busyTimeout, expectedBusyTimeout)
	}

	if err := conn.QueryRowContext(ctx, "PRAGMA foreign_keys;").Scan(&foreignKeys); err != nil {
		return fmt.Errorf("verify foreign_keys pragma: %w", err)
	}
	if foreignKeys != expectedForeignKeys {
		return fmt.Errorf("verify pragma foreign_keys=%d (want %d)", foreignKeys, expectedForeignKeys)
	}

	if err := conn.QueryRowContext(ctx, "PRAGMA synchronous;").Scan(&synchronous); err != nil {
		return fmt.Errorf("verify synchronous pragma: %w", err)
	}
	if synchronous != expectedSynchronous {
		return fmt.Errorf("verify pragma synchronous=%d (want %d)", synchronous, expectedSynchronous)
	}

	return nil
}
