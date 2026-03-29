# L1-E08 — Searcher

**Layer:** 1 — Code Intelligence
**Epic:** 08
**Status:** ⬜ Not Started
**Dependencies:** L1-E04 (embedding client), L1-E05 (LanceDB store)

---

## Description

Implement the semantic searcher that accepts natural language queries and returns ranked code chunks via vector similarity search. The searcher goes beyond basic embed-and-search: it supports multi-query expansion (multiple queries per search, each with `topK=10`), deduplication and re-ranking by hit count (chunks matching multiple queries rank higher), and one-hop call graph expansion (for each direct hit, pull in functions it calls and functions that call it).

This is the retrieval engine that [[06-context-assembly]] calls during per-turn context assembly and that the `search_semantic` agent tool ([[05-agent-loop]]) delegates to. The interface must serve both use cases. Ports from topham's `internal/rag/searcher.go`, adapted from work-order-aware search to a conversational query interface.

---

## Package

`internal/rag/searcher/` — semantic search with multi-query expansion and hop expansion.

---

## Definition of Done

### Basic Search

- [ ] Implements the `Searcher` interface from [[L1-E01-types-and-interfaces]]
- [ ] `Search(ctx, queries []string, opts SearchOptions) ([]SearchResult, error)` — accepts one or more queries, returns ranked results
- [ ] Each query is embedded via the embedding client ([[L1-E04-embedding-client]]) with the query prefix
- [ ] Each embedded query runs a vector search against LanceDB ([[L1-E05-lancedb-store]]) with `topK` from options (default 10)
- [ ] Results include: chunk data, cosine similarity score, matched query index

### Multi-Query Deduplication & Re-Ranking

- [ ] When multiple queries are provided, results are deduplicated by chunk ID
- [ ] Chunks matching multiple queries get a higher rank based on hit count (number of queries that returned this chunk)
- [ ] Ties broken by best similarity score across all matching queries
- [ ] Final ordering: hit count descending, then similarity score descending

### One-Hop Call Graph Expansion

- [ ] For the top-ranked direct hits, look up functions each hit calls (`Calls` field) and functions that call each hit (`CalledBy` field) via the store's `GetByName` method
- [ ] Budget allocation: configurable split between direct hits and hop results (default 60% direct, 40% hops per [[04-code-intelligence-and-rag]])
- [ ] Hop results are tagged with their relationship to the direct hit (caller/callee) in the `SearchResult` metadata
- [ ] Hop results are deduplicated against direct hits — a chunk that's both a direct hit and a hop doesn't appear twice

### Search Options

- [ ] `SearchOptions` struct with: `TopK int`, `Filters Filter` (language, chunk_type, file_path prefix), `ExpandHops bool` (default true), `HopBudgetRatio float64` (default 0.4), `MaxResults int` (overall cap on returned results)
- [ ] Options have sensible defaults — calling `Search(ctx, queries, SearchOptions{})` with zero-value options should work with defaults

### Interface Boundaries

- [ ] The searcher is callable by context assembly ([[06-context-assembly]]) with multiple queries derived from the turn analyzer
- [ ] The searcher is callable by the `search_semantic` agent tool with a single user-provided query
- [ ] Both call paths use the same `Search` method — the difference is in the number of queries and options, not different methods

### Testing

- [ ] Unit tests with a mock store and mock embedder: verify deduplication logic, hit count re-ranking, hop expansion
- [ ] Unit tests for budget allocation: verify the 60/40 split between direct hits and hop results
- [ ] Unit tests for edge cases: no results, single query, all queries return the same chunks, hop expansion finds no connected functions
- [ ] Integration test (requires LanceDB with seeded data): insert test chunks with known relationships, run multi-query search, verify ranking and hop expansion produce expected results

---

## Architecture References

- [[04-code-intelligence-and-rag]] — "Component: Searcher" (basic search, work-order-aware search, adaptation for sirtopham)
- [[06-context-assembly]] — "Component: Retrieval Execution" → "Semantic Search Details" (how context assembly calls the searcher)
- [[05-agent-loop]] — "Tool Set" → `search_semantic` (the agent tool that delegates to this searcher)
- topham source: `internal/rag/searcher.go`

---

## Notes

- topham's `SearchForWorkOrder` method accepts a structured work order with title, target module, acceptance criteria, and known files. sirtopham replaces this with a simpler interface that accepts `[]string` queries. The multi-query expansion and dedup/re-ranking logic is preserved — only the input structure changes.
- The searcher does NOT perform relevance threshold filtering. That's the context assembly layer's responsibility ([[06-context-assembly]] applies a cosine similarity threshold of 0.35). The searcher returns all results with their scores; the caller decides what's relevant enough.
- The `GetByName` call for hop expansion can be expensive if the store contains many chunks with similar names. The store should support efficient name lookups — this may require a name index in LanceDB or a supplementary data structure.
- The one-hop expansion uses the `Calls` and `CalledBy` fields stored on each chunk. These fields are populated during indexing (Pass 2 of [[L1-E07-indexing-pipeline]]). The searcher assumes these fields are accurate and current.
