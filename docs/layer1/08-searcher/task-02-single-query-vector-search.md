# Task 02: Single-Query Vector Search

**Epic:** 08 — Searcher
**Status:** ⬚ Not started
**Dependencies:** Task 01, L1-E04 (Embedder.EmbedQuery), L1-E05 (Store.VectorSearch)

---

## Description

Implement the core single-query search path: take a natural-language query string, embed it via the Embedder, run a vector search against the Store, and return results as `rankedResult` structs. This is the building block that the multi-query logic (Task 03) calls once per query. It does not perform deduplication, re-ranking, or hop expansion — those are separate tasks. This task also defines the `Searcher` struct and its constructor.

## Acceptance Criteria

- [ ] `Searcher` struct defined in `internal/rag/searcher/searcher.go` with fields:
  - `embedder rag.Embedder` — embedding client implementing `rag.Embedder` (methods used: `EmbedQuery`)
  - `store rag.Store` — vector store implementing `rag.Store` (methods used: `VectorSearch`, `GetByName`)
- [ ] `NewSearcher(embedder rag.Embedder, store rag.Store) *Searcher` constructor
- [ ] `(s *Searcher) embedAndSearch(ctx context.Context, query string, queryIndex int, opts SearchOptions) ([]rankedResult, error)` method (unexported) that:
  1. Calls `s.embedder.EmbedQuery(ctx, query)` to get the query embedding vector
  2. Calls `s.store.VectorSearch(ctx, embedding, opts.TopK, opts.Filters)` to get the top-K chunks by cosine similarity
  3. Converts each returned chunk + score into a `rankedResult` with:
     - `BestScore` set to the cosine similarity score from the store
     - `HitCount` set to 1
     - `MatchedQueries` set to `[]int{queryIndex}`
     - `HopRelation` set to `HopNone`
     - `HopSource` set to `""`
  4. Returns the slice of `rankedResult` structs
- [ ] If `EmbedQuery` returns an error, the method returns `nil, fmt.Errorf("embedding query %d: %w", queryIndex, err)`
- [ ] If `VectorSearch` returns an error, the method returns `nil, fmt.Errorf("vector search for query %d: %w", queryIndex, err)`
- [ ] If `VectorSearch` returns zero results, the method returns an empty slice and nil error (not an error condition)
- [ ] Context cancellation is respected — both the embed and search calls receive the context
