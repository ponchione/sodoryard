# Task 03: Schema Versioning and Table Initialization

**Epic:** 05 — LanceDB Store
**Status:** ⬚ Not started
**Dependencies:** Task 01, Task 02, L1-E01 (SchemaVersion constant)

---

## Description

Implement schema versioning for the `chunks` table. On store open, check whether the existing table's schema version matches the current `rag.SchemaVersion` constant. If it does not match (or the table does not exist), drop the existing table and create a new one with the current schema. This triggers a full re-index by upstream callers. The schema version is stored as LanceDB table metadata.

## Acceptance Criteria

- [ ] `initTable(ctx context.Context) error` method on `LanceStore` called during `NewLanceStore` after connection is established
- [ ] If the `chunks` table does not exist, create it with the Arrow schema from Task 02 and store `SchemaVersion` in table metadata
- [ ] If the `chunks` table exists, read its stored schema version from metadata
  - If version matches `rag.SchemaVersion`: open the existing table, no data loss
  - If version does not match: drop the existing table, create a new empty table with the current schema and version. Log a warning: `"schema version mismatch (stored=%s, current=%s), recreating table — full re-index required"`
- [ ] `NeedsReindex() bool` method on `LanceStore` returns `true` if the table was recreated during initialization (schema mismatch or first creation), `false` if the existing table was reused
- [ ] Error handling: if table creation fails, `NewLanceStore` returns an error with context (e.g., `"failed to create chunks table: ..."`)
- [ ] Error handling: if table metadata read fails, treat as schema mismatch (drop and recreate)
