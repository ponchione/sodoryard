# Layer 3 Slice 4: Retrieval Orchestrator Plan

> For Hermes: implement only this slice, commit locally, then stop.

Goal: add the `internal/context` retrieval orchestrator so Slice 3 query output and Slice 1 contracts can produce concrete pre-budget retrieval results from semantic search, explicit file reads, structural graph lookups, conventions, and git context.

Architecture:
- Keep everything in `internal/context`.
- Reuse Layer 1 interfaces directly: `codeintel.Searcher` and `codeintel.GraphStore`.
- Keep conventions behind `ConventionSource` and provide a `NoopConventionSource` so this slice compiles without a real Layer 1 convention cache.
- Inject local helpers for file reads and git execution so tests can mock I/O cheaply.
- Use the repo-locked 50/50 direct-hit vs hop split when calling the existing searcher, even though some docs still say 60/40.

Tech stack: Go, `internal/context`, `internal/codeintel`, `internal/config`, stdlib `os`, `os/exec`, `filepath`, `sync`, `time`

---

## Scope of this single problem

Deliver only these artifacts:
- `internal/context/retrieval.go`
- `internal/context/conventions.go`
- `internal/context/retrieval_test.go`

Do not implement budget fitting, serialization, report persistence, or compression in this slice.

---

## Task 1: Write failing retrieval tests

Objective: lock down the orchestrator behavior before implementation.

Add tests for:
- semantic search path mapping and `SearchOptions` wiring
- explicit file reads with path-traversal protection and missing-file tolerance
- structural graph mapping into `GraphHit`
- convention and git paths when flagged
- relevance filtering at threshold 0.35
- cross-source dedup where a graph hit matches a RAG hit by symbol/file and the RAG hit records both sources
- one-path timeout while another path still succeeds

Verification:
- `go test ./internal/context/... -run 'TestRetrievalOrchestrator|TestNoopConventionSource'`
- Expected RED failure because orchestrator implementation does not exist yet

---

## Task 2: Implement the orchestrator shell and dependency injection

Objective: create the concrete retriever and noop conventions source.

Create `internal/context/conventions.go` with:
- `NoopConventionSource` implementing `ConventionSource`

Create `internal/context/retrieval.go` with:
- `RetrievalOrchestrator` implementing `Retriever`
- constructor accepting:
  - `codeintel.Searcher`
  - `codeintel.GraphStore`
  - `ConventionSource`
  - project root
- injected helpers for:
  - file reading
  - git log execution
- configurable per-path timeout with a default of 5 seconds

Notes:
- Each path should degrade gracefully: log and return empty results rather than fail the whole retrieval stage.
- Keep brain retrieval empty in v0.1.

Verification:
- `go test ./internal/context/... -run 'TestNoopConventionSource'`

---

## Task 3: Implement the five retrieval paths

Objective: satisfy the path-specific tests with the smallest clean implementation.

Implement:
- semantic search path
  - uses `codeintel.Searcher.Search`
  - `TopK: 10`
  - `MaxResults` from `cfg.MaxChunks` with sane default
  - `EnableHopExpansion: true`
  - `HopBudgetFraction: 0.5`
  - `HopDepth` from `cfg.StructuralHopDepth`
- explicit file read path
  - respects `cfg.MaxExplicitFiles`
  - enforces project-root containment via `filepath.Rel`
  - truncates oversized files and sets `Truncated`
- structural graph path
  - calls `BlastRadius` per symbol
  - uses `cfg.StructuralHopDepth` / `cfg.StructuralHopBudget`
- convention path
  - loads only when `IncludeConventions` is true
- git context path
  - runs `git log --oneline -N` only when `IncludeGitContext` is true

Verification:
- `go test ./internal/context/... -run 'TestRetrievalOrchestrator'`

---

## Task 4: Implement filtering, merge, dedup, and timeout behavior

Objective: finish the post-processing and orchestration lifecycle.

Implement:
- per-path timeout wrapper using child contexts
- RAG threshold filtering using `cfg.RelevanceThreshold` defaulting to 0.35
- cross-source merge logic:
  - initialize RAG hits with source `rag`
  - when a graph hit matches a RAG hit on file path + symbol name, annotate the RAG hit with `graph` in `Sources`
  - omit the duplicate graph entry from final graph results
- preserve other graph hits unchanged

Verification:
- `go test ./internal/context/... -run 'TestRetrievalOrchestrator'`

---

## Task 5: Final verification and commit

Objective: prove Slice 4 is complete and isolated.

Run:
- `go test ./internal/context/... -run 'TestRetrievalOrchestrator|TestNoopConventionSource'`
- `go test ./internal/context/...`

Commit only Slice 4 code:
- `git add internal/context/retrieval.go internal/context/conventions.go internal/context/retrieval_test.go`
- `git commit -m "feat(context): add retrieval orchestrator"`

Stop after commit.

---

## Success criteria

This slice is done when:
- `Retriever` has a concrete `RetrievalOrchestrator`
- all five v0.1 retrieval paths can be exercised with mocks
- RAG hits are threshold-filtered before leaving retrieval
- duplicate graph hits that overlap RAG hits are merged into RAG source annotations
- path failures/timeouts do not fail the whole retrieval stage
- `go test ./internal/context/...` passes
- only Slice 4 code is included in the local commit
