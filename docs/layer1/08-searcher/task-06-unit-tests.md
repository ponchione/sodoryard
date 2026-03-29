# Task 06: Unit Tests

**Epic:** 08 — Searcher
**Status:** ⬚ Not started
**Dependencies:** Task 05

---

## Description

Write comprehensive unit tests for the searcher using mock implementations of the `Embedder` and `Store` interfaces. Tests cover the deduplication logic, hit-count re-ranking, hop expansion, budget allocation, default options, error handling, and edge cases. Mock implementations return deterministic, predictable results so that test assertions can verify exact ordering, scores, and hop relationships.

## Acceptance Criteria

### Mock Setup

- [ ] `mockEmbedder` implementing `rag.Embedder` that returns pre-configured embedding vectors. `EmbedQuery` returns a deterministic vector per query string (use a map of query string -> vector). Returns error if configured to do so.
- [ ] `mockStore` implementing `rag.Store` that returns pre-configured search results. `VectorSearch` returns deterministic chunk+score pairs per embedding vector (use a map or ordered list). `GetByName` returns pre-configured chunks per name. Returns error if configured to do so.

### Deduplication and Re-Ranking Tests

- [ ] Test: two queries, overlapping results. Query 0 returns chunks A (0.85), B (0.72), C (0.68). Query 1 returns chunks B (0.79), D (0.74), A (0.70). Verify final order is A (hits=2, score=0.85), B (hits=2, score=0.79), D (hits=1, score=0.74), C (hits=1, score=0.68).
- [ ] Test: three queries, one chunk appears in all three. Verify HitCount=3 and that chunk ranks first regardless of individual scores.
- [ ] Test: two queries, completely disjoint results (no overlap). Verify all chunks have HitCount=1, ordered by BestScore descending.
- [ ] Test: two queries, identical results (complete overlap). Verify deduplication produces one entry per chunk with HitCount=2.
- [ ] Test: single query. Verify all chunks have HitCount=1, ordered by score descending.

### Hop Expansion Tests

- [ ] Test: direct hit has Calls=["FuncA", "FuncB"] and CalledBy=["FuncC"]. Mock store returns chunks for all three. Verify hop results tagged correctly: FuncA and FuncB as HopCallee, FuncC as HopCaller. All have HopSource set to the direct hit's chunk ID.
- [ ] Test: hop result chunk ID matches a direct hit chunk ID. Verify the hop duplicate is dropped; the direct hit is preserved.
- [ ] Test: two direct hits both have the same function in their Calls list. Verify the hop chunk appears only once (first occurrence kept).
- [ ] Test: direct hit has empty Calls and CalledBy. Verify no GetByName calls are made for that hit.
- [ ] Test: GetByName returns no results for a name. Verify the name is silently skipped with no error.
- [ ] Test: GetByName returns an error for one name. Verify the error is logged but processing continues for remaining names.

### Budget Allocation Tests

- [ ] Test: MaxResults=10, HopBudgetRatio=0.4. Verify directBudget=6, hopBudget=4. With 8 direct hits and 6 hop results, verify output has exactly 6 direct hits and 4 hop results (10 total).
- [ ] Test: MaxResults=10, HopBudgetRatio=0.0, ExpandHops=true. Verify all 10 slots go to direct hits, no hop lookups occur.
- [ ] Test: MaxResults=30, HopBudgetRatio=0.4 (defaults). Verify directBudget=18, hopBudget=12.
- [ ] Test: fewer direct hits than directBudget (e.g., 3 direct hits with directBudget=18). Verify all 3 direct hits included, hop expansion still runs.
- [ ] Test: fewer hop results than hopBudget. Verify all available hops included without padding.

### Default Options Tests

- [ ] Test: `Search(ctx, queries, SearchOptions{})` applies defaults: TopK=10, MaxResults=30, ExpandHops=true, HopBudgetRatio=0.4.
- [ ] Test: `Search(ctx, queries, SearchOptions{TopK: 5})` overrides TopK but applies defaults to other fields.
- [ ] Test: `Search(ctx, queries, SearchOptions{ExpandHops: false})` skips hop expansion entirely.

### Edge Cases

- [ ] Test: empty query slice returns error with message "searcher: at least one query is required".
- [ ] Test: nil query slice returns error with message "searcher: at least one query is required".
- [ ] Test: all queries return zero results from the store. Verify empty result slice, nil error.
- [ ] Test: embedder returns an error. Verify error is propagated with query index context.
- [ ] Test: store VectorSearch returns an error. Verify error is propagated with query index context.
- [ ] Test: context cancellation mid-search. Verify the cancelled context is propagated to embedder and store calls.

### Interface Compliance

- [ ] Test: compile-time assertion `var _ rag.Searcher = (*Searcher)(nil)` exists in test file to verify interface satisfaction.

## Work Breakdown

**Part A (~2-3h):** Mock setup (configurable mock Embedder and Store), single-query tests, dedup/rerank tests (6 categories: basic dedup, hit-count ordering, tie-breaking by score, empty results from one query, identical chunks across queries, single query passthrough).

**Part B (~2h):** Hop expansion tests (basic expansion, budget enforcement 60/40, GetByName error handling, no-hops-available), edge case tests (empty queries, nil queries, zero results, embedder error, store error, context cancellation).

This task should be worked in two sessions to stay within the 4-hour budget.
