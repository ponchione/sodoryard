# Task 03: Multi-Query Deduplication and Re-Ranking

**Epic:** 08 — Searcher
**Status:** ⬚ Not started
**Dependencies:** Task 02

---

## Description

Implement the multi-query execution, deduplication, and hit-count re-ranking logic. When the searcher receives multiple queries (e.g., 3 queries from context assembly's query extraction), it runs each query through `embedAndSearch` (Task 02), then merges results by deduplicating on chunk ID and re-ranking based on how many queries returned each chunk. Chunks that appear in results for multiple queries are considered more relevant and rank higher. This is the core ranking algorithm that differentiates multi-query search from naive single-query search.

## Acceptance Criteria

- [ ] `(s *Searcher) searchAndMerge(ctx context.Context, queries []string, opts SearchOptions) ([]rankedResult, error)` method (unexported) that:
  1. Calls `embedAndSearch` for each query (index 0 through len(queries)-1) with the provided options
  2. Merges all results into a single slice, deduplicating by chunk ID (`Chunk.ID`)
  3. For duplicate chunks (same ID from different queries):
     - `HitCount` is the total number of queries that returned this chunk
     - `BestScore` is the maximum cosine similarity score across all matching queries
     - `MatchedQueries` is the union of all query indices that returned this chunk
  4. Sorts the merged results by: `HitCount` descending first, then `BestScore` descending as tiebreaker
  5. Returns the sorted, deduplicated slice
- [ ] Deduplication uses a `map[string]*rankedResult` keyed by `Chunk.ID` for O(1) merge
- [ ] Sorting is stable — when both HitCount and BestScore are equal, original insertion order is preserved
- [ ] If any single `embedAndSearch` call fails, the method returns the error immediately (fail-fast, do not return partial results)
- [ ] If all queries return zero results, the method returns an empty slice and nil error
- [ ] Single-query input (len(queries) == 1) works correctly — produces results with HitCount=1 for all chunks, sorted by BestScore descending
- [ ] Example expected behavior with 2 queries:
  - Query 0 returns chunks A (score 0.85), B (score 0.72), C (score 0.68)
  - Query 1 returns chunks B (score 0.79), D (score 0.74), A (score 0.70)
  - Merged result order: A (hits=2, best=0.85), B (hits=2, best=0.79), D (hits=1, best=0.74), C (hits=1, best=0.68)
