# Layer 3, Epic 04: Retrieval Orchestrator

**Layer:** 3 — Context Assembly
**Epic:** 04 of 07
**Status:** ⬚ Not started
**Dependencies:** [[layer-3-epic-01-context-assembly-types]]; Layer 1: [[layer-1-epic-01-types]], [[layer-1-epic-08-searcher]], [[layer-1-epic-09-structural-graph]]; Layer 0: [[layer-0-epic-03-config]]

---

## Description

Implement the parallel retrieval orchestrator that executes up to five retrieval paths concurrently, collects their results, and applies relevance filtering. This is the I/O-heavy stage of context assembly — it calls into the Layer 1 searcher (semantic search), reads files from disk (explicit files), queries the structural graph (blast radius), loads cached conventions, and runs `git log`.

The orchestrator runs the currently active retrieval paths in parallel via goroutines, waits for all to complete (or timeout), then merges and deduplicates results across sources. Relevance filtering applies cosine similarity thresholds to discard low-quality RAG hits before passing results to the budget manager (Epic 05). Current runtime already includes proactive keyword-backed brain retrieval from the MCP/vault backend; broader semantic/index-backed brain retrieval remains future work.

---

## Definition of Done

### Retrieval Execution

- [ ] `RetrievalOrchestrator` struct that takes `ContextNeeds`, search queries (from Epic 03), and config, and returns `RetrievalResults`
- [ ] **Five parallel retrieval paths** executed via goroutines with `sync.WaitGroup` or `errgroup.Group`:
  1. **Semantic code search:** Calls Layer 1 searcher with the 1-3 queries from query extraction. Uses multi-query execution with `topK=10` per query, deduplication by chunk ID, hit-count re-ranking, and one-hop call graph expansion (60% direct hits, 40% dependency hops). Returns `[]RAGHit`.
  2. **Explicit file reads:** For each path in `ContextNeeds.ExplicitFiles`, read the file from disk. Truncate files exceeding `MaxExplicitFiles` or individual file size limits. Returns `[]FileResult`.
  3. **Structural graph:** For each symbol in `ContextNeeds.ExplicitSymbols`, query the Layer 1 structural graph for upstream callers, downstream callees, and interface implementations. Configurable depth (`StructuralHopDepth`, default 1) and budget (`StructuralHopBudget`, default 10). Returns `[]GraphHit`.
  4. **Convention cache:** If `ContextNeeds.IncludeConventions` is true, load cached conventions from the Layer 1 convention extractor. Returns convention text string.
  5. **Git context:** If `ContextNeeds.IncludeGitContext` is true, execute `git log --oneline -N` where N is `GitContextDepth`. Returns git log string.
- [ ] **Timeout:** Each retrieval path has a configurable timeout (default 5 seconds). If a path times out, its results are empty — the orchestrator does not fail the entire assembly.
- [ ] **Current v0.2 note:** Proactive project-brain retrieval is already in scope here via the MCP/vault keyword-backed path. Remaining open work is about query shaping, validation packaging, and any future semantic/index-backed expansion — not about proving the brain is still reactive-only.

### Relevance Filtering

- [ ] **Code RAG filtering:** Discard `RAGHit` entries with similarity score below `RelevanceThreshold` (default 0.35)
- [ ] **Merge and dedup:** Combine RAG hits and structural graph hits. Deduplicate by chunk/document ID across sources. When the same code chunk appears from both RAG and structural graph, keep the entry with the higher score and annotate with both sources.

### Tests

- [ ] Unit test with mocked Layer 1 searcher: provide queries → get ranked RAG hits back
- [ ] Unit test with mocked file reads: explicit file paths → file contents returned, missing files → graceful error
- [ ] Unit test with mocked structural graph: symbol → callers/callees returned
- [ ] Unit test: relevance filtering at threshold 0.35 → hits below 0.35 discarded, hits at/above 0.35 retained
- [ ] Unit test: deduplication across RAG and structural graph → no duplicate chunks in output
- [ ] Unit test: one path times out → other paths still return results, no panic
- [ ] Integration test: full orchestration with all five v0.1 paths (can use mocks for external dependencies)

---

## Architecture References

- [[06-context-assembly]] — "Component: Retrieval Execution" section (Parallel Retrieval Paths, Semantic Search Details, Relevance Filtering)
- [[04-code-intelligence-and-rag]] — Searcher interface, multi-query expansion, dependency hop patterns
- [[09-project-brain]] — Current MCP/vault-backed proactive brain retrieval contract plus future expansion questions

---

## Implementation Notes

- The Layer 1 searcher (from `internal/rag/searcher.go`) is the primary dependency. This epic calls into it, not reimplements it. The searcher already handles multi-query expansion, dedup, re-ranking, and dependency hops.
- Proactive project-brain retrieval is no longer deferred: current runtime calls the MCP/vault brain backend for keyword-backed brain hits during context assembly. Older references to an Obsidian REST-only runtime in this epic are historical planning context, not the current operator truth.
- `git log` execution should use the same shell execution pattern as the rest of the codebase — `exec.CommandContext` with the project root as working directory.
- The `errgroup.Group` pattern is ideal for parallel retrieval: each goroutine runs independently, errors are collected, and a context cancellation propagates timeouts.
- File reads for `ExplicitFiles` should verify paths are within the project root (path traversal prevention). Files outside the project root are skipped with a warning.
