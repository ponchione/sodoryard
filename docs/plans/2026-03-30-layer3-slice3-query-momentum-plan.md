# Layer 3 Slice 3: Query Extraction and Momentum Plan

> For Hermes: implement only this slice, commit locally, then stop.

Goal: add deterministic query extraction and recent-history momentum tracking in `internal/context` so later retrieval orchestration can receive focused semantic queries plus weak-turn continuity hints.

Architecture:
- Keep everything in `internal/context`.
- Reuse existing contracts from Slice 1 and analyzer helpers from Slice 2 where useful.
- Parse persisted assistant/tool message content using the existing message storage format from `docs/specs/08-data-model.md`; do not invent a second history format.
- Keep this slice pure string/JSON processing only: no DB writes, no I/O, no searcher calls.

Tech stack: Go, `internal/context`, `internal/config`, `internal/db`, `internal/provider`

---

## Scope of this single problem

Deliver only these artifacts:
- `internal/context/query.go`
- `internal/context/momentum.go`
- `internal/context/query_test.go`
- `internal/context/momentum_test.go`

Do not implement retrieval orchestration, budget fitting, serialization, report persistence, or compression in this slice.

---

## Task 1: Write failing query-extraction tests

Objective: lock down the query behavior before writing implementation code.

Add tests for:
- filler stripping and cleaned primary query
- multi-sentence split into up to 2 cleaned queries
- technical-keyword supplementary query
- momentum-enhanced query using `MomentumModule`
- explicit file/symbol exclusion from all produced queries
- max-3-query cap

Verification:
- `go test ./internal/context/... -run 'TestHeuristicQueryExtractor|TestExtractQueries'`
- Expected RED failure because extractor implementation does not exist yet

---

## Task 2: Write failing momentum tests

Objective: lock down recent-history scanning against the real persisted message shapes.

Add tests for:
- assistant `tool_use` blocks with `file_read` / `file_edit` path inputs
- tool results from `search_text` / `search_semantic` with file paths in result text
- deduped `MomentumFiles`
- `MomentumModule` longest common directory prefix
- no-tool-activity empty momentum
- strong-signal turns ignoring stale momentum
- weak-signal integration path: analyzer -> tracker -> extractor

Verification:
- `go test ./internal/context/... -run 'TestHistoryMomentumTracker|TestMomentumIntegration'`
- Expected RED failure because tracker implementation does not exist yet

---

## Task 3: Implement minimal query extractor

Objective: satisfy the query tests with the smallest clean implementation.

Create `internal/context/query.go` with:
- `HeuristicQueryExtractor` implementing `QueryExtractor`
- cleaned-query source:
  - sentence splitting on `.`, `?`, `!`
  - filler phrase stripping via a small static list
  - punctuation cleanup
  - explicit file/symbol removal
  - ~50-word cap
- technical-keyword source:
  - underscore/camelCase/PascalCase/dot-notation terms
  - HTTP methods and status codes
  - programming domain terms
  - overlap suppression
- momentum-enhanced source:
  - prepend `MomentumModule` to the primary cleaned query
- source ordering with max 3 queries:
  - primary cleaned query
  - supplementary cleaned query (if present)
  - technical or momentum query as space allows, while keeping distinct queries only

Notes:
- Keep explicit entities out of semantic queries entirely.
- Reuse same-package helpers from Slice 2 where it reduces duplication.

Verification:
- `go test ./internal/context/... -run 'TestHeuristicQueryExtractor|TestExtractQueries'`

---

## Task 4: Implement minimal momentum tracker

Objective: satisfy momentum tests without overbuilding future retrieval logic.

Create `internal/context/momentum.go` with:
- `HistoryMomentumTracker` implementing `MomentumTracker`
- lookback filtering by the last N turns using `config.ContextConfig.MomentumLookbackTurns` with default 2
- assistant-message parsing using `provider.ContentBlocksFromRaw`
- tool-use path extraction for `file_read`, `file_write`, `file_edit`
- tool-result path extraction for `search_text`, `search_semantic` via file-path regex reuse
- deduped `MomentumFiles`
- `MomentumModule` longest common directory prefix
- strong-signal guard: explicit files/symbols clear or skip momentum

Verification:
- `go test ./internal/context/... -run 'TestHistoryMomentumTracker|TestMomentumIntegration'`

---

## Task 5: Final verification and commit

Objective: prove Slice 3 is complete and isolated.

Run:
- `go test ./internal/context/... -run 'TestHeuristicQueryExtractor|TestHistoryMomentumTracker|TestMomentumIntegration'`
- `go test ./internal/context/...`

Commit only Slice 3 code:
- `git add internal/context/query.go internal/context/momentum.go internal/context/query_test.go internal/context/momentum_test.go`
- `git commit -m "feat(context): add query extraction and momentum tracking"`

Stop after commit.

---

## Success criteria

This slice is done when:
- `QueryExtractor` has a concrete implementation producing 1-3 deterministic queries
- `MomentumTracker` can derive `MomentumFiles` and `MomentumModule` from persisted history rows
- weak-signal turns can generate a momentum-enhanced query
- strong-signal turns do not carry stale momentum into queries
- `go test ./internal/context/...` passes
- only Slice 3 code is included in the local commit
