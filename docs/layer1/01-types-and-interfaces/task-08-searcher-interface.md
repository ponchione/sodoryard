# Task 08: Searcher Interface and Search Options

**Epic:** 01 — Types and Interfaces
**Status:** ⬚ Not started
**Dependencies:** Task 03

---

## Description

Define the `Searcher` interface and its associated `SearchOptions` configuration struct. The Searcher is the primary entry point for code retrieval — it is called by both the `search_semantic` agent tool (doc 05) and the context assembly retrieval path (doc 06). It must support multiple queries with per-query topK, hit-count re-ranking across queries, one-hop call graph expansion, deduplication, and budget allocation between direct hits and dependency hops.

## Acceptance Criteria

- [ ] `SearchOptions` struct defined in `internal/rag/types.go` with the following fields:
  - `TopK               int`     — maximum results per query in vector search (default: 10)
  - `Filter             Filter`  — metadata constraints applied to all queries
  - `MaxResults         int`     — total results to return after deduplication and re-ranking (default: 30, from `max_rag_results` config)
  - `EnableHopExpansion bool`    — whether to perform one-hop call graph expansion on top hits
  - `HopBudgetFraction float64` — fraction of MaxResults allocated to hop expansion (default: 0.4, i.e., 60% direct hits / 40% dependency hops)
  - `HopDepth          int`     — call graph expansion depth (default: 1)
- [ ] `Searcher` interface defined in `internal/rag/interfaces.go` with the following method:
  ```go
  type Searcher interface {
      // Search executes one or more semantic queries against the code index.
      //
      // For each query in queries:
      //   1. Embed the query with the retrieval prefix
      //   2. Run vector search with topK and filter from opts
      //
      // After all queries complete:
      //   3. Deduplicate by chunk ID — chunks matching multiple queries
      //      accumulate hit counts
      //   4. Re-rank by hit count (descending), breaking ties by best
      //      similarity score (descending)
      //   5. If opts.EnableHopExpansion is true, take the top
      //      (MaxResults * (1 - HopBudgetFraction)) direct hits, then
      //      for each, look up functions it calls and functions that call
      //      it via the Store's GetByName method
      //   6. Return up to opts.MaxResults results
      //
      // Returns an empty slice (not nil) if no results meet the criteria.
      Search(ctx context.Context, queries []string, opts SearchOptions) ([]SearchResult, error)
  }
  ```
- [ ] The `Search` method accepts multiple queries (not single) to support both simple agent tool calls (1 query) and context assembly multi-query expansion (up to 3 queries)
- [ ] Doc comment explicitly describes the deduplication and re-ranking algorithm (hit count, then best score as tiebreaker)
- [ ] Doc comment describes the budget allocation between direct hits and hop expansion
- [ ] File compiles cleanly: `go build ./internal/rag/...`
