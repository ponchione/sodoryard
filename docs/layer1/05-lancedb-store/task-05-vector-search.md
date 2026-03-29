# Task 05: Vector Search with Metadata Filters

**Epic:** 05 — LanceDB Store
**Status:** ⬚ Not started
**Dependencies:** Task 03

---

## Description

Implement cosine similarity vector search on the `chunks` table with configurable top-K and optional metadata filters. This is the primary retrieval path for the RAG pipeline: given a query embedding, return the most semantically similar chunks. The searcher (L1-E08) calls this method with pre-embedded query vectors.

## Acceptance Criteria

- [ ] Method signature: `VectorSearch(ctx context.Context, queryVector []float32, topK int, filter *rag.Filter) ([]rag.SearchResult, error)`
- [ ] Performs cosine similarity search on the `vector` column
- [ ] Returns at most `topK` results, ordered by descending similarity score (most similar first)
- [ ] Each `rag.SearchResult` contains the full `rag.Chunk` (all fields populated) and the cosine similarity `Score` (float32, range 0.0 to 1.0 for cosine similarity)
- [ ] When `filter` is non-nil, applies optional metadata filters as LanceDB `WHERE` expressions:
  - `filter.Language` (non-empty): `language = '<value>'`
  - `filter.ChunkType` (non-empty): `chunk_type = '<value>'`
  - `filter.FilePathPrefix` (non-empty): `file_path LIKE '<value>%'`
  - Multiple non-empty filter fields are combined with `AND`
- [ ] When `filter` is nil, no metadata filtering is applied
- [ ] Relationship fields (`calls`, `called_by`, `types_used`, `implements_ifaces`, `imports`) are deserialized from JSON strings back to `[]string` slices on the returned `Chunk`
- [ ] If a JSON relationship field is empty string or `"null"`, deserialize as empty `[]string{}` (never nil)
- [ ] Validates `len(queryVector) == rag.DefaultEmbeddingDims` — returns error if wrong dimension
- [ ] Returns empty slice (not nil) when no results match
- [ ] Returns a descriptive error on search failure
- [ ] Implements the `VectorSearch` method of the `Store` interface from L1-E01
