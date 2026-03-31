# Layer 3 Implementation Plan

> For Hermes: execute one slice at a time, run the listed targeted tests, commit locally, then stop for approval before starting the next slice.

Goal: implement `internal/context` so Layer 5 can call one turn-start entrypoint and receive a frozen `FullContextPackage`, with report persistence and compression support, without taking hard dependencies on unfinished Layer 5 packages.

Architecture:
- Keep Layer 3 in `internal/context`.
- Reuse existing Layer 0/1/2 code instead of duplicating it:
  - `internal/config.ContextConfig`
  - `internal/codeintel.Searcher`
  - `internal/codeintel.GraphStore`
  - `internal/db` / sqlc models and queries
  - `internal/provider.Provider`
- Avoid importing future Layer 5 runtime types by defining narrow Layer 3 boundary types/interfaces for the few things Layer 3 actually needs at assembly time.
- Keep proactive brain retrieval out of v0.1. Preserve `BrainHit` / report fields, but leave them empty until v0.2.
- Put included/excluded status and exclusion reasons into the JSON payloads stored in `context_reports` so Layer 6 can render them without a schema change.

Current repo reality
- `internal/context` currently only has `doc.go`.
- Layer 1 searcher + structural graph already exist and test green.
- `context_reports` schema already exists, but there are no sqlc write queries for insert/update.
- There is no `internal/conversation` package yet, so Layer 3 cannot import a real `SeenFiles` implementation yet.
- There is no Layer 1 convention-cache implementation yet.
- `go test ./internal/...` is currently blocked by `internal/vectorstore` linker errors around missing `simple_lancedb_*` symbols, so Layer 3 work should use targeted package tests instead of full-repo tests.

Lock these implementation decisions before coding
1. Use `internal/context`, not a second package name like `internal/assembly`.
2. Reuse `config.ContextConfig`; do not create a duplicate Layer 3 config struct.
3. Do not import a future Layer 5 `SessionState` type. Instead define a Layer 3-owned boundary input such as:
   - `type SeenFileLookup interface { Contains(path string) (bool, int) }`
   - `type AssemblyScope struct { ConversationID string; TurnNumber int; SeenFiles SeenFileLookup }`
4. Inject conventions behind a small interface, e.g. `ConventionSource`, and ship a `NoopConventionSource` first so retrieval can compile before Layer 1 adds a real cache.
5. Honor the canonical 50/50 direct-hit vs hop split in Layer 3 by explicitly setting `HopBudgetFraction: 0.5` when calling the existing searcher. Treat the stale 60/40 text in docs as cleanup, not code truth.
6. The pipeline returns `CompressionNeeded`; it does not mutate history itself.

Recommended implementation order
- Slice 1: contracts and boundaries
- Slice 2: turn analyzer
- Slice 3: query extraction + momentum
- Slice 4: retrieval orchestrator
- Slice 5: budget manager + serializer
- Slice 6: compression engine
- Slice 7: assembler + report persistence + quality update

This is intentionally more linear than the docs’ parallel build graph because you want one-problem-at-a-time commits.

---

## Slice 1: Core contracts and Layer 3 boundaries

Objective: create the shared types and interfaces everything else compiles against.

Files:
- Modify: `internal/context/doc.go`
- Create: `internal/context/types.go`
- Create: `internal/context/interfaces.go`
- Create: `internal/context/scope.go`
- Create: `internal/context/types_test.go`

Implement:
- `ContextNeeds`, `Signal`
- `RAGHit`, `BrainHit`, `GraphHit`, `FileResult`
- `RetrievalResults`
- `BudgetResult`
- `FullContextPackage`
- `ContextAssemblyReport`
- `SeenFileLookup`
- `AssemblyScope`
- narrow component interfaces used by the assembler (`TurnAnalyzer`, `QueryExtractor`, `MomentumTracker`, `ConventionSource`, etc.)
- report/result fields needed by Layer 6 UI payloads:
  - included/excluded status
  - exclusion reason
  - threshold/hop metadata where useful

Important repo-specific choices:
- Use `internal/db.Message` as the history/storage model import for analyzer/momentum boundaries.
- Reuse `config.ContextConfig` instead of redefining config.
- Treat `FullContextPackage.Frozen` as an API invariant marker; do not over-engineer impossible runtime immutability.

Verification:
- `go test ./internal/context/...`

Commit:
- `feat(context): add layer3 contracts`

---

## Slice 2: Rule-based turn analyzer

Objective: turn a user message into deterministic retrieval needs with good signal traces.

Files:
- Create: `internal/context/analyzer.go`
- Create: `internal/context/analyzer_test.go`

Implement:
- file path extraction
- symbol extraction
- modification intent detection
- creation intent detection
- git context detection
- continuation detection flagging
- stopword set for PascalCase/camelCase false positives
- complete `Signals` trace population

Notes:
- This slice should not generate semantic queries yet.
- Keep the extraction order exactly as documented: file refs -> symbol refs -> modification -> creation -> git -> continuation.

Verification:
- `go test ./internal/context/... -run 'TestRuleBasedAnalyzer|TestAnalyze'`

Commit:
- `feat(context): add rule-based turn analyzer`

---

## Slice 3: Query extraction and momentum

Objective: fill `ContextNeeds.SemanticQueries`, `MomentumFiles`, and `MomentumModule` from message/history only.

Files:
- Create: `internal/context/query.go`
- Create: `internal/context/momentum.go`
- Create: `internal/context/query_test.go`
- Create: `internal/context/momentum_test.go`

Implement:
- cleaned-message query source
- technical-keyword query source
- momentum-enhanced query source
- max-3-query cap and overlap suppression
- explicit file/symbol exclusion from query text
- recent-history scan for file paths in tool-use inputs and tool results
- longest-common-directory-prefix logic

Notes:
- Parse persisted assistant/tool message JSON using the existing message model instead of inventing a second content format.
- Momentum should only influence weak-signal / continuation turns.

Verification:
- `go test ./internal/context/... -run 'TestQuery|TestMomentum'`

Commit:
- `feat(context): add query extraction and momentum tracking`

---

## Slice 4: Retrieval orchestrator

Objective: execute all v0.1 retrieval paths in parallel and return enriched retrieval payloads.

Files:
- Create: `internal/context/retrieval.go`
- Create: `internal/context/conventions.go`
- Create: `internal/context/retrieval_test.go`

Implement:
- orchestrator constructor with injected dependencies:
  - `codeintel.Searcher`
  - `codeintel.GraphStore`
  - `ConventionSource`
  - project root / git executor helper
  - `config.ContextConfig`
- parallel retrieval paths:
  - semantic search
  - explicit file reads
  - structural graph lookup
  - conventions
  - git log
- per-path timeout handling
- project-root path traversal protection for explicit file reads
- relevance filtering at `RelevanceThreshold`
- dedup + enrichment so JSON payloads can later show:
  - score
  - included/excluded status
  - exclusion reason
  - source (`rag`, `graph`, `brain` reserved)

Repo-specific decisions:
- Use the existing searcher; do not reimplement ranking logic.
- Pass `HopBudgetFraction: 0.5` explicitly from Layer 3.
- Start with `NoopConventionSource` returning `""` so this slice is not blocked on missing Layer 1 convention code.

Verification:
- `go test ./internal/context/... -run 'TestRetrieval|TestOrchestrator'`

Commit:
- `feat(context): add retrieval orchestrator`

---

## Slice 5: Budget manager and serializer

Objective: choose what fits, explain what got cut, and render the stable markdown block for cache block 2.

Files:
- Create: `internal/context/budget.go`
- Create: `internal/context/serializer.go`
- Create: `internal/context/budget_test.go`
- Create: `internal/context/serializer_test.go`

Implement:
- token accounting from model context limit, reserved blocks, history tokens, and `MaxAssembledTokens`
- priority order:
  1. explicit files
  2. top RAG hits
  3. structural hits
  4. conventions
  5. git context
  6. lower-ranked RAG hits
- soft caps for conventions and git
- `CompressionNeeded` flag when history crosses threshold
- deterministic markdown serializer with:
  - grouped code chunks by file
  - description before code
  - language-tagged fences
  - optional conventions section
  - optional git section
  - `[previously viewed in turn N]` annotations via `SeenFileLookup`

Important data-shape choice:
- Persist inclusion/exclusion detail inside the JSON result payloads themselves so Layer 6 can browse excluded items even though `context_reports` only has scalar `included_count` / `excluded_count` columns.

Verification:
- `go test ./internal/context/... -run 'TestBudget|TestSerializer'`

Commit:
- `feat(context): add budget manager and serializer`

---

## Slice 6: Compression engine

Objective: compress persisted history safely, independently of the future agent loop implementation.

Files:
- Create: `internal/db/query/message_compression.sql`
- Regenerate: `internal/db/*.go`
- Create: `internal/context/compression.go`
- Create: `internal/context/compression_test.go`

Implement:
- preflight char-count check
- post-response prompt-token check
- provider-error trigger detection (`400 context_length_exceeded`, `413`)
- head-tail preservation algorithm over persisted messages
- summary generation through injected `provider.Provider`
- fallback truncation when summarization fails
- cascading compression support
- orphaned tool-use sanitization by rewriting assistant content JSON when matching tool შედეგ messages were compressed
- return value that lets the caller invalidate cached prompt history

Likely sqlc queries needed:
- list active messages with sequence/turn metadata for compression
- mark a message range compressed
- insert compression summary message
- update assistant content after orphan sanitization
- fetch surviving tool result IDs when sanitizing

Verification:
- `go test -tags sqlite_fts5 ./internal/context/... ./internal/db/... -run 'TestCompression|TestSanitize'`

Commit:
- `feat(context): add compression engine`

---

## Slice 7: Assembler, report persistence, and quality update

Objective: wire all components into the single turn-start API Layer 5 will eventually call.

Files:
- Create: `internal/db/query/context_reports.sql`
- Regenerate: `internal/db/*.go`
- Create: `internal/context/assembler.go`
- Create: `internal/context/report_store.go`
- Create: `internal/context/assembler_test.go`

Implement:
- `ContextAssembler` constructor
- `Assemble(ctx, message, history, scope, modelContextLimit, historyTokenCount)` returning:
  - `*FullContextPackage`
  - `compressionNeeded bool`
  - `error`
- full orchestration flow:
  1. analyze
  2. add momentum
  3. extract queries
  4. retrieve
  5. fit budget
  6. serialize
  7. freeze
  8. persist report
- `UpdateQuality(...)` to fill:
  - `agent_used_search_tool`
  - `agent_read_files_json`
  - `context_hit_rate`

Likely sqlc queries needed:
- insert context report
- update context report quality fields
- optionally fetch context report by conversation/turn for test verification

Verification:
- `go test -tags sqlite_fts5 ./internal/context/... ./internal/db/... -run 'TestAssembler|TestUpdateQuality'`

Commit:
- `feat(context): add context assembler and report persistence`

---

## What to defer on purpose

Do not pull these into Layer 3 v0.1:
- proactive brain retrieval
- a real conventions cache implementation if it does not already exist
- Layer 5 concrete session/runtime types
- full-repo `go test ./...` cleanup for the vectorstore linker problem
- UI/debug-panel work beyond shaping JSON payloads correctly

---

## Minimal acceptance bar for “Layer 3 is implemented enough to start Layer 5 wiring”

All of the following should be true:
- `internal/context` has a stable public API.
- `ContextAssembler.Assemble(...)` can produce a frozen context package from faked history/search dependencies.
- `ContextAssemblyReport` persists and updates correctly in SQLite.
- Compression works against persisted message rows and survives orphan-tool sanitization tests.
- Targeted Layer 3 + DB tests are green with `-tags sqlite_fts5`.
- No Layer 3 code imports an unfinished Layer 5 package.

---

## Recommended first implementation slice

Start with Slice 1 only.

Why:
- every other epic depends on it
- it forces the key repo-specific boundary decisions early
- it avoids getting trapped by unfinished Layer 5 types or missing convention-cache code
- it is a clean first local commit

If Slice 1 lands cleanly, the next most natural move is Slice 2 (turn analyzer).
