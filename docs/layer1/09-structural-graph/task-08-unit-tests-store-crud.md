# Task 08: Unit Tests for Store CRUD Operations

**Epic:** 09 — Structural Graph
**Status:** ⬚ Not started
**Dependencies:** Task 01 (schema), Task 03 (CRUD implementation)

---

## Description

Write unit tests for the graph store's CRUD operations: `InitSchema`, `UpsertSymbols`, `UpsertRelationships`, and `DeleteByFilePath`. Each test uses an in-memory SQLite database (`":memory:"`) with foreign keys enabled. Tests verify correct insertion, upsert-on-conflict, relationship resolution, cascade deletes, and error handling for invalid input.

## File Location

`internal/graph/store_test.go`

## Test Helper

```go
func newTestDB(t *testing.T) *sql.DB {
    t.Helper()
    db, err := sql.Open("sqlite3", ":memory:")
    require.NoError(t, err)
    _, err = db.Exec("PRAGMA foreign_keys = ON")
    require.NoError(t, err)
    t.Cleanup(func() { db.Close() })
    return db
}

func newTestStore(t *testing.T) *Store {
    t.Helper()
    db := newTestDB(t)
    store, err := NewStore(db)
    require.NoError(t, err)
    return store
}
```

## Test Cases

### InitSchema Tests

**TestInitSchema_FreshDatabase:**
- Call `InitSchema` on a fresh in-memory DB.
- Verify all four tables exist by querying `sqlite_master`.
- Verify `graph_schema_meta` contains `schema_version` = `GraphSchemaVersion`.

**TestInitSchema_Idempotent:**
- Call `InitSchema` twice on the same DB.
- Verify no error on second call.
- Insert a symbol, call `InitSchema` again, verify the symbol still exists.

**TestInitSchema_VersionMismatch:**
- Call `InitSchema` on a fresh DB.
- Insert a symbol.
- Manually update `graph_schema_meta` to a different version.
- Call `InitSchema` again.
- Verify the symbol is gone (tables were dropped and recreated).
- Verify `graph_schema_meta` now contains the current `GraphSchemaVersion`.

### UpsertSymbols Tests

**TestUpsertSymbols_Insert:**
- Insert 3 symbols: `auth.ValidateToken` (function), `auth.Claims` (type), `auth.Validator` (interface).
- Query `graph_symbols` directly and verify all fields match.

**TestUpsertSymbols_UpdateOnConflict:**
- Insert symbol `auth.ValidateToken` at lines 10-20.
- Upsert same qualified name with lines 15-30.
- Verify only 1 row exists with updated line range (15-30).

**TestUpsertSymbols_EmptySlice:**
- Call `UpsertSymbols` with empty slice.
- Verify nil error returned.
- Verify `graph_symbols` table is empty.

**TestUpsertSymbols_EmptyQualifiedName:**
- Call `UpsertSymbols` with a symbol whose `QualifiedName` is `""`.
- Verify error is returned.

**TestUpsertSymbols_InvalidSymbolType:**
- Call `UpsertSymbols` with `SymbolType = "invalid"`.
- Verify error is returned (CHECK constraint violation).
- Verify no symbols were inserted (transaction rolled back).

### UpsertRelationships Tests

**TestUpsertRelationships_Calls:**
- Seed 3 symbols: A, B, C.
- Insert calls: A->B, A->C, B->C.
- Query `graph_calls` directly and verify 3 rows with correct caller/callee IDs.

**TestUpsertRelationships_TypeRefs:**
- Seed symbols: `auth.Handler` (function), `http.Request` (type).
- Insert type ref: `auth.Handler` references `http.Request` with `RefField`.
- Query `graph_type_refs` and verify 1 row with correct source/target/ref_type.

**TestUpsertRelationships_Implements:**
- Seed symbols: `auth.TokenService` (type), `auth.Validator` (interface).
- Insert implements: `auth.TokenService` implements `auth.Validator`.
- Query `graph_implements` and verify 1 row.

**TestUpsertRelationships_UnknownCallee:**
- Seed symbol A only.
- Insert call A -> B where B does not exist.
- Verify no error (silently skipped).
- Verify `graph_calls` is empty.

**TestUpsertRelationships_DuplicateIgnored:**
- Seed symbols A and B.
- Insert call A->B twice.
- Verify only 1 row in `graph_calls` (INSERT OR IGNORE).

### DeleteByFilePath Tests

**TestDeleteByFilePath_RemovesSymbols:**
- Seed 2 symbols from `"auth/middleware.go"` and 1 from `"auth/service.go"`.
- Delete by file path `"auth/middleware.go"`.
- Verify 1 symbol remains (the one from `service.go`).

**TestDeleteByFilePath_CascadesRelationships:**
- Seed symbols A (middleware.go) and B (service.go).
- Insert call A->B.
- Delete by file path for A's file.
- Verify `graph_calls` is empty (cascade delete).
- Verify B still exists in `graph_symbols`.

**TestDeleteByFilePath_NonexistentFile:**
- Call `DeleteByFilePath` for a file that has no symbols.
- Verify no error (no-op).

## Acceptance Criteria

- [ ] All tests pass with `go test ./internal/graph/... -v`
- [ ] Tests use in-memory SQLite (no file cleanup needed)
- [ ] `TestInitSchema_FreshDatabase` verifies all 5 tables exist in `sqlite_master`
- [ ] `TestInitSchema_Idempotent` verifies data survives a second `InitSchema` call
- [ ] `TestInitSchema_VersionMismatch` verifies data is dropped on version change
- [ ] `TestUpsertSymbols_Insert` verifies all column values for 3 different symbol types
- [ ] `TestUpsertSymbols_UpdateOnConflict` verifies line range update on duplicate qualified_name
- [ ] `TestUpsertSymbols_EmptyQualifiedName` verifies error returned
- [ ] `TestUpsertSymbols_InvalidSymbolType` verifies transaction rollback on constraint violation
- [ ] `TestUpsertRelationships_Calls` verifies 3 call edges inserted correctly
- [ ] `TestUpsertRelationships_UnknownCallee` verifies silent skip (no error, no row)
- [ ] `TestUpsertRelationships_DuplicateIgnored` verifies idempotent insert
- [ ] `TestDeleteByFilePath_CascadesRelationships` verifies ON DELETE CASCADE behavior
- [ ] No test leaks database connections (all use `t.Cleanup`)
