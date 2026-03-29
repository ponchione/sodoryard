# Task 14: Integration Tests — Full Pipeline

**Epic:** 07 — Indexing Pipeline
**Status:** ⬚ Not started
**Dependencies:** Task 10, Task 11, Task 12, Task 13

---

## Description

Write integration tests that run the full three-pass pipeline against temporary project directories with real files. These tests verify the end-to-end flow: file discovery through to chunks stored in LanceDB with embeddings and index_state rows in SQLite. Uses mock or real implementations of parsers, embedder, describer, and store depending on test scope.

## Acceptance Criteria

### Full Index Test

- [ ] Create a temp directory containing:
  - `main.go` — a Go file with 2 functions (e.g., `main()` and `handleRequest()`)
  - `utils/helper.ts` — a TypeScript file with 1 exported function
  - `scripts/process.py` — a Python file with 1 function
  - `docs/README.md` — a Markdown file with 2 `##` sections
- [ ] Initialize a git repo in the temp directory with one commit
- [ ] Run the full pipeline with `Force = true`
- [ ] Verify exact chunk counts from the test fixture: `main.go` (2 functions: `main`, `helper`) yields 2 Go chunks, `utils/helper.ts` (1 function: `greet`) yields 1 TypeScript chunk, `scripts/process.py` (1 function: `parse`) yields 1 Python chunk, `docs/README.md` (2 sections: `# Overview`, `## Usage`) yields 2 Markdown chunks. Optionally add a `notes.txt` (falls through to fallback chunker) and verify fallback chunks are produced
- [ ] Verify each chunk has a non-empty embedding vector. If using a mock embedder, configure it to return vectors of length 8 (sufficient for testing) and verify each chunk's embedding vector has exactly 8 float32 elements. If using the real embedder, verify length 3584
- [ ] Verify `index_state` has rows for all 4 files with correct `file_hash` and `chunk_count`
- [ ] Verify `projects.last_indexed_commit` is set to the HEAD SHA

### Incremental Index Test

- [ ] After the full index test, modify `main.go` (change one function body) and commit
- [ ] Run the pipeline again with `Force = false`
- [ ] Verify only `main.go` is re-processed (check that the describer/embedder are called only for `main.go` chunks — use a counting mock or spy)
- [ ] Verify unchanged files (`helper.ts`, `process.py`, `README.md`) are NOT re-processed
- [ ] Verify the `index_state` row for `main.go` has an updated `file_hash` and `last_indexed_at`
- [ ] Verify `index_state` rows for unchanged files are untouched

### Deleted File Test

- [ ] After the incremental test, delete `scripts/process.py` and commit
- [ ] Run the pipeline again
- [ ] Verify the Python chunks are removed from the store (`store.DeleteByFilePath` called for `scripts/process.py`)
- [ ] Verify the `index_state` row for `scripts/process.py` is deleted

### Schema Version Re-index Test

- [ ] After the deleted file test, change the `SchemaVersion` constant (or mock the stored version to differ from current)
- [ ] Run the pipeline
- [ ] Verify a full re-index occurred: all remaining files re-processed, store table was dropped and recreated, all `index_state` rows were deleted and re-inserted
- [ ] Verify that `persistIndexState` wrote file hashes to the `index_state` table by querying the SQLite database directly after indexing completes (this also validates task-08 and task-09 indirectly via integration testing)

### Force Re-index Test

- [ ] Without changing any files, run the pipeline with `Force = true`
- [ ] Verify all files are re-processed (no skipping from hash comparison)
- [ ] Verify `index_state` rows are all refreshed

### Error Resilience Test

- [ ] Include a file that causes the parser to fail (e.g., a `.go` file with invalid syntax that also fails tree-sitter fallback — or a binary file matching include globs)
- [ ] Run the pipeline
- [ ] Verify the pipeline completes successfully, skipping the problematic file
- [ ] Verify other files are indexed normally
- [ ] Verify no `index_state` row exists for the failed file

### Context Cancellation Test

- [ ] Start the pipeline and cancel the context after Pass 1 completes but before Pass 3 finishes
- [ ] Verify the pipeline returns `context.Canceled` error
- [ ] Verify no `index_state` was persisted (partial state not written)

### Test Infrastructure Notes

- [ ] Tests use mock implementations for expensive external calls (LLM descriptions, embedding HTTP). The describer mock returns canned descriptions. The embedder mock returns fixed-length random vectors
- [ ] Tests use a real temporary LanceDB instance (temp directory) or a mock store, depending on CGo availability in the test environment
- [ ] Tests use a real temporary SQLite database for `index_state` and `projects` queries
- [ ] All temporary directories and databases are cleaned up after tests

## Work Breakdown

**Part A (~2-3h):** Happy-path tests: full index from scratch, incremental index (modify one file), deleted file cleanup. These share the same test fixture setup.

**Part B (~2h):** Edge-case tests: schema version mismatch triggers full reindex, force reindex flag, per-file error resilience, context cancellation mid-pipeline.

This task should be worked in two sessions to stay within the 4-hour budget.
