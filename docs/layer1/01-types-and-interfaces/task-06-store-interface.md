# Task 06: Store Interface

**Epic:** 01 — Types and Interfaces
**Status:** ⬚ Not started
**Dependencies:** Task 02, Task 03

---

## Description

Define the `Store` interface that abstracts the LanceDB vector store. The store is responsible for persisting chunks with their embeddings, performing vector similarity search with metadata filters, and supporting metadata-only queries needed for change detection and call graph lookups. The store handles serialization of Go slices to JSON strings for LanceDB's Arrow schema — callers work with native Go types only.

## Acceptance Criteria

- [ ] `Store` interface defined in `internal/codeintel/interfaces.go` with the following methods:
  ```go
  type Store interface {
      // Upsert inserts or updates chunks in the vector store.
      // Implementation: delete-by-ID then insert (LanceDB has no native upsert).
      // chunks must have their Embedding field populated.
      // Returns an error if any chunk fails to persist.
      Upsert(ctx context.Context, chunks []Chunk) error

      // VectorSearch performs cosine similarity search against chunk embeddings.
      // queryEmbedding is the embedded query vector (same dimensionality as stored embeddings).
      // topK is the maximum number of results to return.
      // filter constrains results by language, chunk type, or file path prefix.
      // Returns results sorted by descending similarity score.
      VectorSearch(ctx context.Context, queryEmbedding []float32, topK int, filter Filter) ([]SearchResult, error)

      // GetByFilePath returns all chunks for a given file path.
      // Used for change detection (compare stored content hashes against current)
      // and for building call graph context during description generation.
      GetByFilePath(ctx context.Context, filePath string) ([]Chunk, error)

      // GetByName returns all chunks matching a given symbol name.
      // Used for call graph expansion: given a function name from a Calls/CalledBy
      // list, look up the corresponding chunk to include in search results.
      // May return multiple chunks if the name is ambiguous across files.
      GetByName(ctx context.Context, name string) ([]Chunk, error)

      // DeleteByFilePath removes all chunks for a given file path.
      // Used during re-indexing to clean up stale chunks before upserting
      // new ones from a re-parsed file.
      DeleteByFilePath(ctx context.Context, filePath string) error

      // Close releases resources held by the store (LanceDB connection, etc.).
      Close() error
  }
  ```
- [ ] All methods accept `context.Context` as the first parameter (except `Close`)
- [ ] `VectorSearch` accepts a `Filter` value (not pointer) — zero-value Filter means no filtering
- [ ] `Upsert` accepts a slice of `Chunk` (batch operation, not single-chunk)
- [ ] Doc comments explain the LanceDB implementation strategy (delete-then-insert for upsert, JSON serialization for relationship slices)
- [ ] File compiles cleanly: `go build ./internal/codeintel/...`
