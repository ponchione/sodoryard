# Task 05: Search Method Integration

**Epic:** 08 — Searcher
**Status:** ⬚ Not started
**Dependencies:** Task 03, Task 04

---

## Description

Wire the building blocks together into the public `Search` method that implements the `Searcher` interface from L1-E01. This method applies defaults to zero-value options, orchestrates the multi-query search (Task 03) and optional hop expansion (Task 04), converts internal `rankedResult` structs to the public `SearchResult` type, and returns the final ranked result slice. This is the single entry point consumed by both context assembly and the `search_semantic` agent tool.

## Acceptance Criteria

- [ ] `(s *Searcher) Search(ctx context.Context, queries []string, opts SearchOptions) ([]codeintel.SearchResult, error)` method that:
  1. Applies defaults to zero-value fields in `opts`:
     - If `opts.TopK == 0`, set to 10
     - If `opts.MaxResults == 0`, set to 30
     - If `opts.HopBudgetRatio == 0.0` and `opts.ExpandHops` is true, set to 0.4 (distinguish from an explicit 0.0 by checking `ExpandHops`)
  2. Validates input: if `queries` is nil or empty, returns `nil, fmt.Errorf("searcher: at least one query is required")`
  3. Calls `s.searchAndMerge(ctx, queries, opts)` to get deduplicated, re-ranked direct hits
  4. If `opts.ExpandHops` is true (default), calls `s.expandHops(ctx, directHits, opts)` to get the combined direct + hop results
  5. If `opts.ExpandHops` is false, truncates direct hits to `opts.MaxResults`
  6. Converts each `rankedResult` to a `codeintel.SearchResult` (from L1-E01) with: `Chunk` set to `rankedResult.Chunk`, `Score` set to `rankedResult.BestScore`, `HitCount` set to `rankedResult.HitCount`, `MatchedBy` set to a comma-joined representation of the matched query indices, `FromHop` set to `rankedResult.HopRelation != HopNone`
  7. Returns the converted slice
- [ ] The `*Searcher` type satisfies the `codeintel.Searcher` interface (compile-time check: `var _ codeintel.Searcher = (*Searcher)(nil)`)
- [ ] Both call paths work through the same `Search` method:
  - Context assembly calls with multiple queries (1-3) and default options
  - `search_semantic` agent tool calls with a single query (`[]string{userQuery}`) and may override options (e.g., `ExpandHops: false` for simple lookups)
- [ ] Default behavior with zero-value options: `Search(ctx, []string{"auth middleware"}, SearchOptions{})` embeds the query, searches with topK=10, expands hops with 60/40 split, returns up to 30 results
- [ ] The searcher does NOT perform relevance threshold filtering — it returns all results with their scores. Threshold filtering is the caller's responsibility (context assembly applies 0.35 threshold per spec 06)
- [ ] Error from `searchAndMerge` is propagated with context: `fmt.Errorf("searcher: %w", err)`
- [ ] Error from `expandHops` is propagated with context: `fmt.Errorf("searcher: hop expansion: %w", err)`
