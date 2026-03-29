# Task 05: Relevance Filtering and Merge/Dedup

**Epic:** 04 — Retrieval Orchestrator
**Status:** ⬚ Not started
**Dependencies:** Task 02, Task 03

---

## Description

Implement post-retrieval relevance filtering and cross-source merge/deduplication. Code RAG hits below `RelevanceThreshold` (default 0.35) are discarded. After filtering, results from RAG and structural graph are merged and deduplicated by chunk/document ID. When the same code chunk appears from both RAG and structural graph, the entry with the higher score is kept and annotated with both sources.

## Acceptance Criteria

- [ ] **Code RAG filtering:** `RAGHit` entries with similarity score below `RelevanceThreshold` (default 0.35) are discarded
- [ ] **Merge and dedup:** Results combined across RAG and structural graph sources
- [ ] Deduplication by chunk/document ID: when the same chunk appears from both RAG and structural graph, the entry with the higher score is kept
- [ ] Duplicate entries annotated with both source origins for observability
- [ ] Filtered/deduplicated results written back to `RetrievalResults`
- [ ] Package compiles with no errors
