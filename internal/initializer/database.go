package initializer

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	appconfig "github.com/ponchione/sodoryard/internal/config"
	appdb "github.com/ponchione/sodoryard/internal/db"
)

// EnsureDatabase opens the project's .yard/yard.db file, initialises the
// schema if needed, runs the schema upgrade helpers, and ensures a project
// record exists. Returns true if the schema was created from scratch on
// this call, false if the database was already initialised.
//
// stateDir must already exist on disk — initializer.Run() creates it before
// calling EnsureDatabase.
func EnsureDatabase(ctx context.Context, projectRoot, projectName, stateDir string) (bool, error) {
	dbPath := filepath.Join(stateDir, appconfig.StateDBName)

	database, err := appdb.OpenDB(ctx, dbPath)
	if err != nil {
		return false, fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	created, err := appdb.InitIfNeeded(ctx, database)
	if err != nil {
		return false, fmt.Errorf("init database schema: %w", err)
	}
	if err := appdb.EnsureMessageSearchIndexesIncludeTools(ctx, database); err != nil {
		return false, fmt.Errorf("upgrade message search indexes: %w", err)
	}
	if err := appdb.EnsureContextReportsIncludeTokenBudget(ctx, database); err != nil {
		return false, fmt.Errorf("upgrade context report token budget storage: %w", err)
	}
	if err := appdb.EnsureChainSchema(ctx, database); err != nil {
		return false, fmt.Errorf("ensure chain schema: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	_, err = database.ExecContext(ctx, `
INSERT INTO projects(id, name, root_path, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	name = excluded.name,
	root_path = excluded.root_path,
	updated_at = excluded.updated_at
`, projectRoot, projectName, projectRoot, now, now)
	if err != nil {
		return false, fmt.Errorf("ensure project record: %w", err)
	}

	return created, nil
}
