//go:build sqlite_fts5
// +build sqlite_fts5

package initializer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	appconfig "github.com/ponchione/sodoryard/internal/config"
	appdb "github.com/ponchione/sodoryard/internal/db"
)

func TestEnsureDatabaseCreatesSchema(t *testing.T) {
	projectRoot := t.TempDir()
	stateDir := filepath.Join(projectRoot, appconfig.StateDirName)
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	created, err := EnsureDatabase(context.Background(), projectRoot, "myproject", stateDir)
	if err != nil {
		t.Fatalf("EnsureDatabase: %v", err)
	}
	if !created {
		t.Errorf("expected created=true on first run")
	}

	dbPath := filepath.Join(stateDir, appconfig.StateDBName)
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("expected %s to exist: %v", dbPath, err)
	}

	// Verify the schema is queryable.
	db, err := appdb.OpenDB(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	var name string
	row := db.QueryRowContext(context.Background(), "SELECT name FROM projects WHERE id = ?", projectRoot)
	if err := row.Scan(&name); err != nil {
		t.Fatalf("project record query: %v", err)
	}
	if name != "myproject" {
		t.Errorf("project name = %q, want %q", name, "myproject")
	}
}

func TestEnsureDatabaseIsIdempotent(t *testing.T) {
	projectRoot := t.TempDir()
	stateDir := filepath.Join(projectRoot, appconfig.StateDirName)
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	if _, err := EnsureDatabase(context.Background(), projectRoot, "myproject", stateDir); err != nil {
		t.Fatalf("first call: %v", err)
	}
	created, err := EnsureDatabase(context.Background(), projectRoot, "myproject", stateDir)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if created {
		t.Errorf("expected created=false on second run")
	}
}
