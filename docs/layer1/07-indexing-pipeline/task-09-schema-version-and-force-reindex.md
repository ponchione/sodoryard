# Task 09: Schema Version Check and Force Re-index

**Epic:** 07 — Indexing Pipeline
**Status:** ⬚ Not started
**Dependencies:** Task 01, L1-E01 (types — SchemaVersion constant), L1-E05 (LanceDB store)

---

## Description

Implement the schema versioning check that triggers a full re-index when the `SchemaVersion` constant changes, and the force re-index logic that drops and recreates the LanceDB table. Schema version changes happen when the chunk schema or embedding dimensions change — storing data in the old format alongside new-format data would corrupt search results. The force flag (`--force` on `sirtopham index`) also triggers this path.

## Function Signature

```go
// checkSchemaVersion compares the current SchemaVersion against the stored
// version. Returns true if a full re-index is needed (version mismatch or
// no stored version).
func (idx *Indexer) checkSchemaVersion(ctx context.Context) (needsFullReindex bool, err error)

// resetForFullReindex drops the LanceDB chunks table and clears all
// index_state rows for the project.
func (idx *Indexer) resetForFullReindex(ctx context.Context) error
```

## Acceptance Criteria

- [ ] `checkSchemaVersion` calls `store.NeedsReindex()` (from L1-E05 task-03) which compares the stored schema version against the current version. If `NeedsReindex()` returns true, `resetForFullReindex` is called
- [ ] Compares stored version against `rag.SchemaVersion` constant (defined in L1-E01)
- [ ] Returns `true` (needs full re-index) if:
  - Stored version does not match `rag.SchemaVersion`
  - No stored version exists (first index)
  - `IndexerConfig.Force` is true
- [ ] Returns `false` (incremental indexing OK) if stored version matches and Force is false
- [ ] `resetForFullReindex` performs these steps in order:
  1. Calls `store.DropTable(ctx)` or equivalent to delete the LanceDB chunks table entirely
  2. Calls `store.CreateTable(ctx)` or equivalent to recreate the table with current schema
  3. Deletes all `index_state` rows for the project: `DELETE FROM index_state WHERE project_id = ?`
  4. Sets `last_indexed_commit = NULL` on the projects row (forces full git diff on next incremental)
- [ ] `resetForFullReindex` is called at the start of `Run()` (Task 10) before Pass 1, when `checkSchemaVersion` returns true or `Force` is true
- [ ] After reset, the pipeline proceeds as a full index — all files are walked, parsed, described, embedded, and stored
- [ ] Logging: `"schema version mismatch, triggering full re-index" stored=<old> current=<new>` or `"force re-index requested"`
