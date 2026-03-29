# Task 03: Change Detection via File Hash Comparison

**Epic:** 07 — Indexing Pipeline
**Status:** ⬚ Not started
**Dependencies:** Task 01, L0-E06 (schema/sqlc — index_state table and queries)

---

## Description

Implement the file-hash-based change detection that compares content hashes against the `index_state` SQLite table. For each file discovered by the walker, compute its SHA-256 content hash and check whether the stored hash matches. Files with unchanged hashes are skipped entirely, avoiding redundant parsing, description generation, and embedding. This is the primary change detection mechanism — it works regardless of git state.

## Function Signatures

```go
// computeFileHash reads a file and returns its SHA-256 hex digest.
func computeFileHash(path string) (string, error)

// filterChangedFiles returns only those FileEntry items whose content hash
// differs from the hash stored in index_state, plus any files not yet indexed.
func (idx *Indexer) filterChangedFiles(ctx context.Context, files []FileEntry) (changed []FileEntry, unchanged []string, err error)
```

## Acceptance Criteria

- [ ] `computeFileHash` reads the file at the given absolute path and returns `hex(sha256(content))`. If the file cannot be read (permission denied, file deleted between walk and hash), `computeFileHash` returns an empty string and the error. The caller (`filterChangedFiles`) skips the file with a warning log and continues processing
- [ ] `filterChangedFiles` loads existing index_state rows for the project via sqlc query: `SELECT file_path, file_hash FROM index_state WHERE project_id = ?`
- [ ] Builds an in-memory map of `relPath → storedHash` from the query results
- [ ] For each `FileEntry`, computes the content hash and compares against the stored hash:
  - **Hash matches:** file is skipped (added to `unchanged` list). Debug log: `"skipping unchanged file" path=<relpath>`
  - **Hash differs or no stored hash:** file is included in `changed` list
- [ ] Returns the `changed` file list (to be parsed) and the `unchanged` file list (for informational/progress purposes)
- [ ] When `IndexerConfig.Force` is true, this function is bypassed entirely — all files are treated as changed
- [ ] Detects deleted files: files present in `index_state` but not in the walker output. Returns or records these for later cleanup (deletion from LanceDB store via `DeleteByFilePath` and removal from `index_state`)
- [ ] Deleted file paths collected into a `deleted []string` return value
- [ ] Context cancellation support
