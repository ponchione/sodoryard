# Task 06: Metadata Queries — GetByFilePath and GetByName

**Epic:** 05 — LanceDB Store
**Status:** ⬚ Not started
**Dependencies:** Task 03

---

## Description

Implement metadata-only query methods that retrieve chunks without vector search. `GetByFilePath` returns all chunks for a given file path (used during re-indexing for change detection). `GetByName` returns chunks matching a symbol name (used by the searcher's one-hop call graph expansion to look up functions referenced in a chunk's `Calls` or `CalledBy` lists).

## Acceptance Criteria

### GetByFilePath

- [ ] Method signature: `GetByFilePath(ctx context.Context, filePath string) ([]codeintel.Chunk, error)`
- [ ] Returns all chunks where `file_path = '<filePath>'` (exact match, not prefix)
- [ ] Results are ordered by `line_start` ascending (top of file first)
- [ ] All `Chunk` fields are populated, including deserialized relationship fields (JSON string to `[]string`)
- [ ] Returns empty slice (not nil) when no chunks match the file path
- [ ] Returns a descriptive error on query failure

### GetByName

- [ ] Method signature: `GetByName(ctx context.Context, name string) ([]codeintel.Chunk, error)`
- [ ] Returns all chunks where `name = '<name>'` (exact match)
- [ ] Multiple chunks may share a name (e.g., same function name in different files/packages). All matches are returned.
- [ ] All `Chunk` fields are populated, including deserialized relationship fields
- [ ] Returns empty slice (not nil) when no chunks match the name
- [ ] Returns a descriptive error on query failure

### Shared

- [ ] Both methods scope queries to the store's `projectName`: all queries include `project_name = '<projectName>'` in the filter
- [ ] Both methods implement their respective `Store` interface methods from L1-E01
- [ ] JSON deserialization of relationship fields handles empty strings, `"null"`, and `"[]"` uniformly as empty `[]string{}`
