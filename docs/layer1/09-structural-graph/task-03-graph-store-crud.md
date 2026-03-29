# Task 03: GraphStore CRUD Operations

**Epic:** 09 — Structural Graph
**Status:** ⬚ Not started
**Dependencies:** Task 01 (schema DDL), Task 02 (domain types), L0-E04 (sqlite connection)

---

## Description

Implement the write-path methods of the `GraphStore` interface: `UpsertSymbols`, `UpsertRelationships`, and `DeleteByFilePath`. These methods populate the graph from analyzer output and support incremental re-indexing by removing stale data for a file before re-inserting. All mutations must be transactional -- a partial failure during upsert should not leave the graph in an inconsistent state.

## File Location

`internal/graph/store.go`

## Constructor

```go
package graph

import "database/sql"

// Store implements the GraphStore interface from internal/rag.
type Store struct {
    db *sql.DB
}

// NewStore creates a new graph Store. It calls InitSchema to ensure
// tables exist and are at the correct version.
func NewStore(db *sql.DB) (*Store, error)
```

`NewStore` must call `InitSchema(db)` (from Task 01) before returning. If `InitSchema` fails, `NewStore` returns the error.

## Method: UpsertSymbols

```go
// UpsertSymbols inserts or updates symbols for a file. Uses INSERT OR REPLACE
// keyed on the UNIQUE(project_id, qualified_name) constraint.
// All symbols in the slice must belong to the same project.
func (s *Store) UpsertSymbols(ctx context.Context, symbols []Symbol) error
```

**Behavior:**
1. Begin a transaction.
2. For each symbol, execute:
   ```sql
   INSERT INTO graph_symbols (project_id, file_path, name, qualified_name, symbol_type, language, line_start, line_end)
   VALUES (?, ?, ?, ?, ?, ?, ?, ?)
   ON CONFLICT(project_id, qualified_name) DO UPDATE SET
       file_path = excluded.file_path,
       name = excluded.name,
       symbol_type = excluded.symbol_type,
       language = excluded.language,
       line_start = excluded.line_start,
       line_end = excluded.line_end;
   ```
3. Commit the transaction.
4. If any insert fails, rollback the entire transaction and return the error.

**Edge cases:**
- Empty slice: return nil immediately (no-op).
- Symbol with empty `QualifiedName`: return an error (qualified name is required for graph resolution).

## Method: UpsertRelationships

```go
// UpsertRelationships inserts relationships between symbols. Symbols referenced
// by qualified name must already exist in graph_symbols. Relationships referencing
// unknown symbols are silently skipped (the referenced symbol may be in an external
// package not yet indexed).
func (s *Store) UpsertRelationships(ctx context.Context, calls []Call, typeRefs []TypeRef, implements []Implements) error
```

**Behavior:**
1. Begin a transaction.
2. For each `Call`:
   - Resolve `CallerQName` to `caller_id` via: `SELECT id FROM graph_symbols WHERE project_id = ? AND qualified_name = ?`
   - Resolve `CalleeQName` to `callee_id` via the same query.
   - If either is not found, skip this call (log at debug level, do not error).
   - Insert:
     ```sql
     INSERT OR IGNORE INTO graph_calls (project_id, caller_id, callee_id)
     VALUES (?, ?, ?);
     ```
3. For each `TypeRef`:
   - Resolve `SourceQName` to `source_id` and `TargetQName` to `target_id`.
   - If either is not found, skip.
   - Insert:
     ```sql
     INSERT OR IGNORE INTO graph_type_refs (project_id, source_id, target_id, ref_type)
     VALUES (?, ?, ?, ?);
     ```
4. For each `Implements`:
   - Resolve `TypeQName` to `type_id` and `InterfaceQName` to `interface_id`.
   - If either is not found, skip.
   - Insert:
     ```sql
     INSERT OR IGNORE INTO graph_implements (project_id, type_id, interface_id)
     VALUES (?, ?, ?);
     ```
5. Commit the transaction.

**Performance note:** Qualified name resolution requires lookups per relationship. For large batches, pre-load all symbols for the project into a `map[string]int64` (qualified_name -> id) at the start of the transaction. The query:
```sql
SELECT id, qualified_name FROM graph_symbols WHERE project_id = ?;
```

This avoids N+1 queries. The map fits comfortably in memory (thousands of symbols at most).

## Method: DeleteByFilePath

```go
// DeleteByFilePath removes all symbols and their relationships for a file.
// Used during re-indexing: delete stale data, then re-insert from fresh parse.
func (s *Store) DeleteByFilePath(ctx context.Context, projectID, filePath string) error
```

**Behavior:**
1. Begin a transaction.
2. Delete symbols for the file:
   ```sql
   DELETE FROM graph_symbols WHERE project_id = ? AND file_path = ?;
   ```
3. The `ON DELETE CASCADE` foreign keys on `graph_calls`, `graph_type_refs`, and `graph_implements` automatically remove referencing relationship rows.
4. Commit the transaction.

**Important:** This relies on `PRAGMA foreign_keys = ON` being set on the database connection (from L0-E04). If foreign keys are not enabled, CASCADE deletes do not fire and orphaned relationship rows accumulate. The `NewStore` constructor should verify foreign keys are enabled by running `PRAGMA foreign_keys` and returning an error if the result is not `1`.

## Acceptance Criteria

- [ ] `NewStore(db)` calls `InitSchema` and returns error if schema init fails
- [ ] `NewStore(db)` verifies `PRAGMA foreign_keys = ON` and returns error if not enabled
- [ ] `UpsertSymbols` inserts new symbols correctly (all columns populated)
- [ ] `UpsertSymbols` updates existing symbols on qualified_name conflict (e.g., file_path or line range changed)
- [ ] `UpsertSymbols` with empty slice is a no-op (returns nil)
- [ ] `UpsertSymbols` returns error for symbol with empty QualifiedName
- [ ] `UpsertSymbols` rolls back on partial failure (e.g., invalid symbol_type)
- [ ] `UpsertRelationships` inserts calls, type refs, and implements correctly
- [ ] `UpsertRelationships` silently skips relationships referencing unknown qualified names
- [ ] `UpsertRelationships` uses bulk symbol lookup (map) rather than N+1 queries
- [ ] `UpsertRelationships` uses `INSERT OR IGNORE` to handle duplicate relationships idempotently
- [ ] `DeleteByFilePath` removes all symbols for the specified file
- [ ] `DeleteByFilePath` cascades to remove all relationships referencing deleted symbols
- [ ] `UpsertSymbols`, `UpsertRelationships`, and `DeleteByFilePath` are each wrapped in a transaction
