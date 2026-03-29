# Task 04: Upsert Operation

**Epic:** 05 — LanceDB Store
**Status:** ⬚ Not started
**Dependencies:** Task 03

---

## Description

Implement the upsert operation on the `chunks` table. LanceDB does not have native upsert, so upsert is implemented as delete-by-ID then insert. This handles both pure inserts (chunk does not yet exist) and updates (chunk exists with the same ID). The operation accepts a batch of chunks with their pre-computed embedding vectors.

## Acceptance Criteria

- [ ] Method signature: `Upsert(ctx context.Context, chunks []rag.Chunk, embeddings [][]float32) error`
- [ ] For each chunk, delete any existing record with the same `id` value. Use a filter expression: `id = '<chunk_id>'`
- [ ] If no existing record is found for a given ID, the delete is a no-op (no error)
- [ ] After deleting stale records, insert all chunks as a single Arrow record batch using the builder from Task 02
- [ ] The delete-then-insert sequence is per-batch, not per-chunk: delete all stale IDs first, then insert the full batch. This is more efficient than N individual round-trips.
- [ ] Validates `len(chunks) == len(embeddings)` — returns error if mismatched
- [ ] Validates `len(embeddings[i]) == rag.DefaultEmbeddingDims` for each embedding — returns error with chunk ID if wrong dimension
- [ ] Returns a descriptive error if the delete operation fails (includes chunk ID)
- [ ] Returns a descriptive error if the insert operation fails
- [ ] Calling `Upsert` with an empty slice is a no-op (returns nil, no LanceDB call)
- [ ] Implements the `Upsert` method of the `Store` interface from L1-E01
