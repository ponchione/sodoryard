# Task 02: Semantic Code Search and Explicit File Read Paths

**Epic:** 04 — Retrieval Orchestrator
**Status:** ⬚ Not started
**Dependencies:** Task 01; Layer 1: Epic 08 (searcher)

---

## Description

Implement the first two retrieval paths: semantic code search and explicit file reads. The semantic search path calls the Layer 1 searcher with 1-3 queries from query extraction, using multi-query execution with `topK=10` per query, deduplication by chunk ID, hit-count re-ranking, and one-hop call graph expansion (60% direct hits, 40% dependency hops). The explicit file read path reads each file in `ContextNeeds.ExplicitFiles` from disk, with path traversal prevention and size limits.

## Acceptance Criteria

- [ ] **Semantic code search path:** Calls Layer 1 searcher with the 1-3 queries from query extraction
- [ ] Multi-query execution with `topK=10` per query
- [ ] Deduplication by chunk ID across queries
- [ ] Hit-count re-ranking: chunks appearing in multiple query results ranked higher
- [ ] One-hop call graph expansion: 60% budget for direct hits, 40% for dependency hops
- [ ] Returns `[]RAGHit`
- [ ] **Explicit file read path:** For each path in `ContextNeeds.ExplicitFiles`, reads the file from disk
- [ ] Path traversal prevention: files outside the project root are skipped with a logged warning
- [ ] Files exceeding individual size limits are truncated with `Truncated = true` on the `FileResult`
- [ ] Missing files produce a logged warning, not an error (graceful degradation)
- [ ] Returns `[]FileResult`
- [ ] Package compiles with no errors
