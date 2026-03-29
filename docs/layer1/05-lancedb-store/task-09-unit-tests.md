# Task 09: Unit Tests

**Epic:** 05 — LanceDB Store
**Status:** ⬚ Not started
**Dependencies:** Task 07

---

## Description

Write unit tests for all store operations using a temporary LanceDB directory that is cleaned up after each test. These tests validate individual operations in isolation: insert, upsert (update), search, metadata queries, delete, and schema versioning. Each test uses small synthetic data (a handful of chunks with random float32 embeddings of 3584 dimensions).

## Acceptance Criteria

### Setup and Teardown

- [ ] Each test creates a LanceDB store in `t.TempDir()` — automatically cleaned up
- [ ] Helper function to create test chunks: `makeTestChunk(id, name, filePath, language, chunkType string) rag.Chunk` with all fields populated: ID, Name, FilePath, Language, ChunkType, Signature, Body, Description, LineStart, LineEnd, Calls, CalledBy, TypesUsed, ImplementsIfaces, Imports, ContentHash, Embedding (set to non-empty relationship slices and a valid content hash)
- [ ] Helper function to create random embeddings: `makeRandomEmbedding() []float32` returning a 3584-dimension float32 slice

### Insert and Retrieve

- [ ] Test: insert 3 chunks, `GetByFilePath` returns the correct chunks for each file path
- [ ] Test: insert 3 chunks, `GetByName` returns the correct chunk(s) for a given name
- [ ] Test: `GetByFilePath` with a non-existent path returns empty slice, no error
- [ ] Test: `GetByName` with a non-existent name returns empty slice, no error

### Upsert (Update)

- [ ] Test: insert a chunk, then upsert the same chunk ID with a modified `body` and new embedding. `GetByFilePath` returns the updated body. Only one record exists for that ID (no duplicates).

### Vector Search

- [ ] Test: insert 5 chunks with known embeddings. Search with one of the inserted embeddings as the query vector. Verify the matching chunk appears as the top result with the highest similarity score.
- [ ] Test: search with `topK=2` returns exactly 2 results
- [ ] Test: search with a language filter returns only chunks matching that language
- [ ] Test: search with `nil` filter returns results from all languages

### Delete

- [ ] Test: insert 3 chunks for file "a.go" and 2 for file "b.go". `DeleteByFilePath("a.go")`. `GetByFilePath("a.go")` returns empty. `GetByFilePath("b.go")` still returns 2 chunks.
- [ ] Test: `DeleteByFilePath` with a non-existent path returns nil (no error)

### Schema Versioning

- [ ] Test: open a store, insert data, close it. Re-open with the same `SchemaVersion` — data is preserved, `NeedsReindex()` returns false.
- [ ] Test: The test creates a store, inserts data, then closes it. Next, it constructs a new store using an internal test helper that overrides the schema version (the `SchemaVersion` should be a package-level variable, not a constant, to enable test overrides — or use a constructor option `withSchemaVersion(v int)` for testing). After reopening with a different version, verify the table is recreated and previous data is gone. `NeedsReindex()` returns true.

### Relationship Field Serialization

- [ ] Test: insert a chunk with non-empty `Calls`, `CalledBy`, `TypesUsed`, `ImplementsIfaces`, `Imports`. Retrieve via `GetByFilePath`. Verify all relationship fields are correctly round-tripped (Go slice to JSON string to Go slice).
- [ ] Test: insert a chunk with nil/empty relationship slices. Retrieve and verify they come back as empty `[]string{}` (not nil).

### Error Cases

- [ ] Test: `Upsert` with mismatched chunk/embedding slice lengths returns an error
- [ ] Test: `VectorSearch` with a wrong-dimension query vector returns an error
