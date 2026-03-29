# Task 01: RetrievalOrchestrator Struct and Parallel Execution Framework

**Epic:** 04 — Retrieval Orchestrator
**Status:** ⬚ Not started
**Dependencies:** Epic 01; Layer 0: Epic 03 (config)

---

## Description

Create the `RetrievalOrchestrator` struct and the parallel execution framework that runs up to five retrieval paths concurrently in v0.1. Use `errgroup.Group` with a context for timeout propagation. Each retrieval path runs in its own goroutine and writes to a dedicated field on the shared `RetrievalResults` struct. The framework handles per-path timeouts (default 5 seconds) so that a slow or failing path does not block the entire assembly. Individual path implementations are stubbed here and filled in by subsequent tasks. Proactive project-brain retrieval is deferred to v0.2.

## Acceptance Criteria

- [ ] `RetrievalOrchestrator` struct defined with constructor accepting dependencies: Layer 1 searcher, structural graph, convention cache access, git-context execution helper, and config
- [ ] Method signature: takes `ContextNeeds`, search queries (`[]string`), and config; returns `RetrievalResults`
- [ ] Five retrieval paths executed via goroutines using `errgroup.Group` with context
- [ ] Per-path timeout: configurable, default 5 seconds. If a path times out, its results are empty and no error is propagated
- [ ] If a path returns an error, the error is logged via structured logging but other paths continue
- [ ] `RetrievalResults` populated by merging results from all completed paths
- [ ] Package compiles with no errors
