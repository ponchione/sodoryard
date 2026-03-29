# Task 01: Graph Schema DDL and Version Management

**Epic:** 09 — Structural Graph
**Status:** ⬚ Not started
**Dependencies:** L0-E04 (sqlite connection), L0-E06 (schema/sqlc patterns)

---

## Description

Define the SQLite DDL for the structural graph tables and implement schema initialization with version checking. The graph tables live in the same SQLite database as the main schema but are managed independently by the graph package. On initialization, the graph package checks a version constant against a `graph_schema_version` metadata row; if the version is missing or mismatched, all graph tables are dropped and recreated. This keeps the graph self-contained while sharing the database connection from L0-E04.

## Schema DDL

The following four tables and their indexes must be created. These are NOT part of the main `schema.sql` from L0-E06 -- they live in a separate DDL string constant within the graph package.

```sql
CREATE TABLE IF NOT EXISTS graph_schema_meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS graph_symbols (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id     TEXT    NOT NULL,
    file_path      TEXT    NOT NULL,
    name           TEXT    NOT NULL,
    qualified_name TEXT    NOT NULL,
    symbol_type    TEXT    NOT NULL CHECK (symbol_type IN ('function', 'method', 'type', 'interface')),
    language       TEXT    NOT NULL,
    line_start     INTEGER NOT NULL,
    line_end       INTEGER NOT NULL,

    UNIQUE(project_id, qualified_name)
);

CREATE INDEX IF NOT EXISTS idx_graph_symbols_file
    ON graph_symbols(project_id, file_path);
CREATE INDEX IF NOT EXISTS idx_graph_symbols_qualified
    ON graph_symbols(project_id, qualified_name);
CREATE INDEX IF NOT EXISTS idx_graph_symbols_type
    ON graph_symbols(project_id, symbol_type);

CREATE TABLE IF NOT EXISTS graph_calls (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id TEXT    NOT NULL,
    caller_id  INTEGER NOT NULL REFERENCES graph_symbols(id) ON DELETE CASCADE,
    callee_id  INTEGER NOT NULL REFERENCES graph_symbols(id) ON DELETE CASCADE,

    UNIQUE(project_id, caller_id, callee_id)
);

CREATE INDEX IF NOT EXISTS idx_graph_calls_caller
    ON graph_calls(project_id, caller_id);
CREATE INDEX IF NOT EXISTS idx_graph_calls_callee
    ON graph_calls(project_id, callee_id);

CREATE TABLE IF NOT EXISTS graph_type_refs (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id TEXT    NOT NULL,
    source_id  INTEGER NOT NULL REFERENCES graph_symbols(id) ON DELETE CASCADE,
    target_id  INTEGER NOT NULL REFERENCES graph_symbols(id) ON DELETE CASCADE,
    ref_type   TEXT    NOT NULL CHECK (ref_type IN ('field', 'parameter', 'return', 'embedding')),

    UNIQUE(project_id, source_id, target_id, ref_type)
);

CREATE INDEX IF NOT EXISTS idx_graph_type_refs_source
    ON graph_type_refs(project_id, source_id);
CREATE INDEX IF NOT EXISTS idx_graph_type_refs_target
    ON graph_type_refs(project_id, target_id);

CREATE TABLE IF NOT EXISTS graph_implements (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id   TEXT    NOT NULL,
    type_id      INTEGER NOT NULL REFERENCES graph_symbols(id) ON DELETE CASCADE,
    interface_id INTEGER NOT NULL REFERENCES graph_symbols(id) ON DELETE CASCADE,

    UNIQUE(project_id, type_id, interface_id)
);

CREATE INDEX IF NOT EXISTS idx_graph_implements_type
    ON graph_implements(project_id, type_id);
CREATE INDEX IF NOT EXISTS idx_graph_implements_interface
    ON graph_implements(project_id, interface_id);
```

## Version Management

- Define a package-level constant: `const GraphSchemaVersion = "1"`
- On `InitSchema(db *sql.DB) error`:
  1. Create `graph_schema_meta` table if not exists.
  2. Query `SELECT value FROM graph_schema_meta WHERE key = 'schema_version'`.
  3. If the row is missing or the value does not equal `GraphSchemaVersion`:
     - `DROP TABLE IF EXISTS` for `graph_implements`, `graph_type_refs`, `graph_calls`, `graph_symbols` (in this order due to foreign keys).
     - Execute the full DDL above.
     - `INSERT OR REPLACE INTO graph_schema_meta (key, value) VALUES ('schema_version', ?)` with the current version.
  4. If the version matches, do nothing (tables already exist and are compatible).
- All DDL execution must happen within a single transaction.

## File Location

`internal/codeintel/graph/schema.go` -- contains the DDL constant string and `InitSchema` function.

## Function Signatures

```go
package graph

import "database/sql"

const GraphSchemaVersion = "1"

// InitSchema ensures the graph tables exist and match the current schema version.
// If the version is missing or mismatched, all graph tables are dropped and recreated.
func InitSchema(db *sql.DB) error
```

## Acceptance Criteria

- [ ] DDL string constant contains all four tables (`graph_symbols`, `graph_calls`, `graph_type_refs`, `graph_implements`) plus `graph_schema_meta`, with the columns, constraints, and indexes enumerated in the AC items above
- [ ] `InitSchema` creates tables from scratch on a fresh database
- [ ] `InitSchema` is idempotent -- calling it twice on an already-initialized database is a no-op
- [ ] `InitSchema` drops and recreates tables when `GraphSchemaVersion` changes (simulate by inserting a different version into `graph_schema_meta` before calling)
- [ ] Foreign key CASCADE deletes work: deleting a row from `graph_symbols` removes referencing rows in `graph_calls`, `graph_type_refs`, and `graph_implements`
- [ ] All DDL runs within a single transaction -- partial failure rolls back cleanly
- [ ] `symbol_type` CHECK constraint rejects values outside `('function', 'method', 'type', 'interface')`
- [ ] `ref_type` CHECK constraint rejects values outside `('field', 'parameter', 'return', 'embedding')`
