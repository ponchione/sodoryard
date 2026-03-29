# Task 06: Unit Tests and Integration Test

**Epic:** 04 — Retrieval Orchestrator
**Status:** ⬚ Not started
**Dependencies:** Task 01, Task 02, Task 03, Task 04, Task 05

---

## Description

Comprehensive unit and integration tests for the retrieval orchestrator. Unit tests use mock implementations of the Layer 1 searcher, structural graph, convention cache access, git-context execution, and file system to verify each retrieval path independently. Integration test exercises the full orchestration with all five v0.1 paths running in parallel.

## Acceptance Criteria

- [ ] Unit test: mocked Layer 1 searcher — provide queries, get ranked RAG hits back
- [ ] Unit test: mocked file reads — explicit file paths return file contents; missing files produce graceful error
- [ ] Unit test: mocked structural graph — symbol queries return callers/callees
- [ ] Unit test: relevance filtering at threshold 0.35 — hits below 0.35 discarded, hits at or above 0.35 retained
- [ ] Unit test: deduplication across RAG and structural graph — no duplicate chunks in output
- [ ] Unit test: one path times out — other paths still return results, no panic
- [ ] Integration test: full orchestration with all five v0.1 paths using mock dependencies — message in, `RetrievalResults` out with all v0.1 source types populated
- [ ] All tests pass: `go test ./internal/context/...`
