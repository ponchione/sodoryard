# Task 01: SearchOptions and SearchResult Types

**Epic:** 08 — Searcher
**Status:** ⬚ Not started
**Dependencies:** L1-E01 (Searcher interface, SearchResult type, Filter type)

---

## Description

Define the `SearchOptions` struct and the `HopRelationship` metadata type used by the searcher. L1-E01 defines the core `SearchResult` and `Filter` types, but the searcher needs its own options struct to control per-call behavior (topK, filters, hop expansion toggle, budget ratio, max results). This task also defines the `HopRelationship` type that tags hop-expanded results with their relationship to the direct hit (caller or callee). These types live in the searcher package and are the contract between the searcher and its callers (context assembly and the `search_semantic` agent tool).

## Acceptance Criteria

- [ ] File `internal/rag/searcher/types.go` created in the searcher package
- [ ] `SearchOptions` struct defined with the following fields and YAML/JSON tags:
  - `TopK int` — number of results per query (default: 10)
  - `Filters rag.Filter` — optional metadata filters, using the `rag.Filter` type (from L1-E01): `Language string` (empty = all languages), `ChunkType ChunkType` (empty = all types), `FilePathPrefix string` (empty = no path filter)
  - `ExpandHops bool` — whether to perform one-hop call graph expansion (default: true)
  - `HopBudgetRatio float64` — fraction of `MaxResults` allocated to hop results (default: 0.4, meaning 40% hops / 60% direct)
  - `MaxResults int` — overall cap on returned results (default: 30)
- [ ] `DefaultSearchOptions() SearchOptions` function returns a `SearchOptions` with the defaults listed above
- [ ] Calling `Search(ctx, queries, SearchOptions{})` with a zero-value `SearchOptions` behaves identically to calling with `DefaultSearchOptions()` — the `Search` method must apply defaults to zero-value fields
- [ ] `HopRelationship` string type defined with constants:
  - `HopCaller HopRelationship = "caller"` — the hop result calls the direct hit
  - `HopCallee HopRelationship = "callee"` — the hop result is called by the direct hit
  - `HopNone HopRelationship = ""` — the result is a direct vector search hit, not a hop
- [ ] `rankedResult` internal struct (unexported) defined for use by the dedup/re-rank logic:
  - `Chunk rag.Chunk` — the chunk data
  - `BestScore float64` — highest cosine similarity across all matching queries
  - `HitCount int` — number of queries that returned this chunk
  - `MatchedQueries []int` — indices of queries that matched this chunk
  - `HopRelation HopRelationship` — how this result relates to a direct hit (empty for direct hits)
  - `HopSource string` — chunk ID of the direct hit that caused this hop (empty for direct hits)
- [ ] All types compile cleanly with `go build ./internal/rag/searcher/...`
