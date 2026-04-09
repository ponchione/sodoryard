# Brain System Rebuild Implementation Plan

> For Hermes: Use subagent-driven-development skill to implement this plan task-by-task.

Goal: Upgrade sirtopham's brain from a keyword-first vault helper into a spec-aligned, robust project memory system with derived metadata, graph state, hybrid retrieval, richer serialization, and durable validation.

Architecture: Keep the vault markdown as the source of truth, then add a derived brain layer under it: parsed document metadata + wikilink graph in SQLite, plus semantic chunk indexing for retrieval. Runtime retrieval should become hybrid (lexical + semantic + structural/metadata) while preserving today's MCP/vault tools and observability surfaces.

Tech Stack: Go, SQLite/sqlc, existing MCP-backed vault backend, existing code embedding/vectorstore stack, context assembly/report store/metrics inspector, repo Makefile test/build flow.

---

## Read-this-first execution context

Before touching code, read these files in this order:
- `BRAIN_SYSTEM_AUDIT_AND_REBUILD.md`
- `docs/specs/09-project-brain.md`
- `docs/specs/06-context-assembly.md`
- `docs/specs/08-data-model.md`
- `docs/v2-b4-brain-retrieval-validation.md`
- `cmd/sirtopham/serve.go`
- `internal/context/retrieval.go`
- `internal/context/budget.go`
- `internal/context/serializer.go`
- `internal/context/analyzer.go`
- `internal/tool/brain_search.go`
- `internal/tool/brain_read.go`
- `internal/tool/brain_write.go`
- `internal/tool/brain_update.go`
- `internal/tool/brain_lint.go`
- `internal/brain/vault/client.go`
- `internal/brain/mcpclient/client.go`
- `internal/db/schema.sql`
- `internal/context/report_store.go`
- `internal/server/metrics.go`

Execution rules:
- Keep edits narrow.
- Prefer `make test` / `make build`.
- If running Go directly, use `-tags sqlite_fts5`.
- Do not refactor unrelated provider/runtime code.
- Preserve the vault as the brain source of truth.
- Preserve current MCP/vault tools unless intentionally improving them.
- Do not leave decorative config fields claiming runtime behavior they do not implement.

---

## Phase map

Phase 0: Lock current contract and add failing tests for the known gaps
Phase 1: Land derived brain metadata + graph persistence
Phase 2: Land brain indexing/freshness workflow
Phase 3: Land hybrid proactive/reactive brain retrieval
Phase 4: Fix brain budget/serialization/spec alignment
Phase 5: Upgrade write/update + lint + observability integration
Phase 6: Expand live validation and docs

Each phase should leave the repo in a working state.

---

## Phase 0: Lock current behavior and expose the spec gaps with tests

### Task 0.1: Add a serializer-order regression test
Objective: Make the current code-vs-brain ordering mismatch explicit.

Files:
- Modify: `internal/context/serializer_test.go`

Step 1: Add a failing test that builds a `BudgetResult` with both `SelectedBrainHits` and `SelectedRAGHits`.

Step 2: Assert the serialized output places `## Project Brain` / `## Project Knowledge` before `## Relevant Code`.

Step 3: Run:
- `go test -tags sqlite_fts5 ./internal/context -run TestMarkdownSerializerBrainAppearsBeforeCode`
Expected now: FAIL

Step 4: Commit after the implementation that makes it pass, not before.

### Task 0.2: Add a richer-brain-serialization failing test
Objective: Prevent a future implementation from staying at one-line bullet snippets.

Files:
- Modify: `internal/context/serializer_test.go`

Step 1: Add a failing test asserting brain serialization includes:
- note path
- title
- match reason or mode when available
- a multi-line excerpt/body block or clearly richer structured content than a single bullet

Step 2: Run:
- `go test -tags sqlite_fts5 ./internal/context -run TestMarkdownSerializerBrainIncludesRichKnowledgeContent`
Expected now: FAIL

### Task 0.3: Lock the current proactive brain report behavior
Objective: Keep current good behavior from regressing while deeper changes land.

Files:
- Modify: `internal/context/assembler_test.go`
- Modify: `internal/server/metrics_test.go`

Step 1: Add/assert tests covering:
- `brain_results` persistence
- `prefer_brain_context` signal flow
- signal stream still includes semantic query + flag entries

Step 2: Run:
- `go test -tags sqlite_fts5 ./internal/context ./internal/server`
Expected: PASS

### Task 0.4: Add a failing test for inert brain config knobs
Objective: Expose that brain-specific config values do not yet change runtime behavior.

Files:
- Modify: `internal/context/budget_test.go`
- Possibly modify: `internal/context/retrieval_test.go`

Step 1: Add failing tests for:
- `brain.max_brain_tokens`
- `brain.brain_relevance_threshold`

Step 2: Run targeted tests.
Expected now: FAIL for the new tests.

---

## Phase 1: Derived brain metadata and graph persistence

### Phase goal
Make `brain_documents` and `brain_links` real runtime state derived from the vault.

### Task 1.1: Add a brain parser package
Objective: Parse markdown notes into a canonical derived representation.

Files:
- Create: `internal/brain/parser/document.go`
- Create: `internal/brain/parser/document_test.go`

Document model should include at minimum:
- `Path string`
- `Title string`
- `Content string`
- `ContentHash string`
- `Tags []string`
- `Frontmatter map[string]any` or stable JSON string
- `Wikilinks []ParsedLink`
- `Headings []Heading`
- `TokenCount int`
- `UpdatedAt` when recoverable from frontmatter/content or file metadata input

Step 1: Write failing parser tests for:
- frontmatter extraction
- title extraction
- inline tags + frontmatter tags
- wikilink extraction including `[[target|display]]`
- heading extraction
- content hash stability

Step 2: Implement the parser.

Step 3: Run:
- `go test -tags sqlite_fts5 ./internal/brain/parser`
Expected: PASS

### Task 1.2: Add DB query support for upserting brain metadata
Objective: Give the runtime a clean persistence path for derived brain state.

Files:
- Modify: SQL query source file(s) under `internal/db/` as needed
- Regenerate: sqlc outputs
- Possibly modify: `internal/db/schema.sql` only if truly needed
- Add tests: `internal/db/schema_integration_test.go`

Needed operations:
- upsert/replace one `brain_documents` row by `(project_id, path)`
- delete/rewrite links for one source document
- list brain docs for a project
- fetch brain doc metadata by path

Step 1: Add failing DB integration tests.
Step 2: Add SQL queries and regenerate code.
Step 3: Make tests pass.

### Task 1.3: Add a derived-state indexer service
Objective: Walk the vault and materialize derived brain state.

Files:
- Create: `internal/brain/indexer/indexer.go`
- Create: `internal/brain/indexer/indexer_test.go`

Responsibilities:
- list markdown docs from the vault backend or filesystem-backed vault client
- parse each note
- upsert `brain_documents`
- replace its outgoing rows in `brain_links`
- skip operational docs that should not enter retrieval if that remains the product contract

Step 1: Write failing tests for a tiny sample vault.
Step 2: Implement full rebuild.
Step 3: Implement incremental rebuild by content hash if practical in this slice.
Step 4: Run:
- `go test -tags sqlite_fts5 ./internal/brain/indexer`
Expected: PASS

### Task 1.4: Wire a repo-visible command for brain reindex
Objective: Make derived brain state runnable by an operator and testable.

Files:
- Modify or create: `cmd/sirtopham/index.go` or add a dedicated subcommand if cleaner
- Modify tests under `cmd/sirtopham/`

Contract:
- either extend existing `index` to include brain derived state
- or add a clear `brain-index` / `index --brain` path

Step 1: Add command tests.
Step 2: Implement command wiring.
Step 3: Print useful summary counts.

---

## Phase 2: Brain chunk indexing and freshness contract

### Phase goal
Add semantic/index-backed brain retrieval without replacing the vault source of truth.

### Task 2.1: Define brain chunk storage model
Objective: Decide and implement how semantic brain chunks are stored.

Files:
- Create: `internal/brain/chunks/*.go` or similar
- Possibly modify: vectorstore integration files
- Add tests for chunking behavior

Requirements:
- chunk at headings/sections when configured
- include chunk metadata linking back to note path/title/tags/section heading
- keep brain vectors logically separate from code vectors

Step 1: Write chunking tests.
Step 2: Implement heading-aware chunk generation.
Step 3: Run tests.

### Task 2.2: Build brain semantic indexing path
Objective: Embed brain chunks and persist a retrievable semantic index.

Files:
- Create: `internal/brain/indexer/semantic.go` or equivalent
- Modify integration points needed for embeddings/vectorstore
- Add tests

Requirements:
- use existing embedding runtime where possible
- preserve clean failure behavior if embedding/vector services are unavailable
- allow full rebuild from vault + derived metadata

Step 1: Write failing tests/mocks around indexing orchestration.
Step 2: Implement semantic brain indexing.
Step 3: Run targeted tests.

### Task 2.3: Make freshness explicit
Objective: Remove ambiguity about whether brain derived state is fresh.

Files:
- Modify: command/runtime wiring
- Modify: inspector/report payloads if needed
- Add tests

Pick one explicit contract:
- synchronous refresh on writes/updates, or
- explicit dirty/stale state with required reindex

Do not leave it implicit.

Step 1: Add a failing test/doc assertion for the chosen contract.
Step 2: Implement it.
Step 3: Surface freshness state somewhere operator-visible if stale states can exist.

---

## Phase 3: Hybrid brain retrieval runtime

### Phase goal
Upgrade both proactive and reactive brain retrieval from keyword-only to hybrid.

### Task 3.1: Expand the context retrieval interface
Objective: Let proactive context assembly consume richer brain retrieval than `SearchKeyword`.

Files:
- Modify: `internal/context/interfaces.go`
- Modify: `internal/context/retrieval.go`
- Add tests: `internal/context/retrieval_test.go`

New retrieval result fields should support:
- lexical score
n- semantic score
- final score
- match sources (`keyword`, `semantic`, `graph`, `tag`, etc.)
- tags/metadata
- maybe section heading / excerpt source

Step 1: Add failing tests for hybrid result selection.
Step 2: Change interfaces and orchestrator implementation.
Step 3: Make tests pass.

### Task 3.2: Implement hybrid reactive `brain_search`
Objective: Make tool behavior match runtime capability.

Files:
- Modify: `internal/tool/brain_search.go`
- Add tests: `internal/tool/brain_test.go` / `brain_search` tests

Requirements:
- `mode=keyword` remains deterministic lexical-only
- `mode=semantic` becomes real
- `mode=auto` becomes a true hybrid ranking mode
- tag filters should use real metadata, not read-whole-note fallback when indexed data is available

Step 1: Write failing tests showing `semantic` no longer falls back to keyword notice.
Step 2: Implement hybrid search.
Step 3: Ensure current guidance messaging only remains where still true.

### Task 3.3: Add graph/backlink expansion in retrieval
Objective: Make wikilink structure a first-class retrieval booster.

Files:
- Modify: retrieval/index packages
- Possibly modify: `internal/tool/brain_read.go` for real backlinks
- Add tests

Requirements:
- use `brain_links`
- support backlinks and limited graph hops when configured
- keep graph hop limits small and explainable

Step 1: Add failing graph retrieval tests.
Step 2: Implement backlink expansion.
Step 3: Run targeted tests.

---

## Phase 4: Budgeting, ranking, and serialization alignment

### Phase goal
Make the assembled Project Knowledge section spec-aligned and genuinely useful.

### Task 4.1: Enforce brain-specific budget controls
Objective: Make current config fields real.

Files:
- Modify: `internal/context/budget.go`
- Modify: `internal/context/budget_test.go`

Requirements:
- `brain.max_brain_tokens` actually caps or strongly governs selected brain content
- `brain.brain_relevance_threshold` affects brain semantic/hybrid selection
- if graph hops are enabled, their token effect is included in brain budgeting or clearly separated

Step 1: Make Phase 0 failing tests pass.
Step 2: Add more tests for mixed brain/code pressure.

### Task 4.2: Fix serializer ordering
Objective: Match the current spec direction.

Files:
- Modify: `internal/context/serializer.go`
- Modify: `internal/context/serializer_test.go`

Requirements:
- place Project Brain / Project Knowledge before Relevant Code when that remains the desired contract
- keep deterministic output ordering

Step 1: Make the Phase 0 ordering test pass.

### Task 4.3: Upgrade brain serialization richness
Objective: Put usable knowledge into the assembled context, not only references.

Files:
- Modify: `internal/context/serializer.go`
- Modify: report/result types if needed
- Modify tests

Desired output shape per hit:
- path/title
- section or note role if known
- concise excerpt/body block
- why it matched / what retrieval source(s) produced it
- tags or relationships only when useful

Step 1: Make the Phase 0 richer-serialization test pass.
Step 2: Keep output concise enough for budget discipline.

### Task 4.4: Improve ranking explainability in reports
Objective: Make hybrid retrieval debuggable.

Files:
- Modify: `internal/context/types.go`
- Modify: `internal/context/report_store.go`
- Modify: `internal/server/metrics.go`
- Modify frontend types/UI if required

Add fields or structured metadata for:
- lexical score
- semantic score
- graph expansion reason
- final ranking reason/source list

Step 1: Add tests on report persistence/decoding.
Step 2: Implement pass-through.
Step 3: Add inspector display only if low-risk and clearly useful.

---

## Phase 5: Write/update integration and lint robustness

### Phase goal
Make note mutation cooperate with derived state and maintenance workflows.

### Task 5.1: Define post-write/post-update derived-state behavior
Objective: Ensure brain writes do not silently desync the system.

Files:
- Modify: `internal/tool/brain_write.go`
- Modify: `internal/tool/brain_update.go`
- Possibly add small helper service in `internal/brain/`
- Add tests

Requirements:
- after write/update, either refresh derived state immediately or mark it stale explicitly
- do not silently leave indexed retrieval outdated

Step 1: Add failing tests.
Step 2: Implement chosen contract.

### Task 5.2: Upgrade `brain_read` backlinks
Objective: Replace basename heuristic when graph data exists.

Files:
- Modify: `internal/tool/brain_read.go`
- Add tests

Requirements:
- if derived graph exists, backlinks come from `brain_links`
- fallback heuristic only when graph data is unavailable and that fallback is intentional

### Task 5.3: Upgrade `brain_lint` to use derived metadata where appropriate
Objective: Keep lint fast and authoritative once metadata/graph exist.

Files:
- Modify: `internal/brain/analysis/*.go`
- Modify: `internal/tool/brain_lint.go`
- Add tests

Potential improvements:
- use persisted metadata for tags/links when available
- add stale-index/freshness checks
- keep contradictions opt-in and bounded

---

## Phase 6: Validation, docs, and operator contract cleanup

### Phase goal
Leave behind a maintained, explicit, testable brain runtime contract.

### Task 6.1: Expand live validation package
Objective: Prove the new runtime, not just the keyword canaries.

Files:
- Modify: `docs/v2-b4-brain-retrieval-validation.md`
- Modify: `scripts/validate_brain_retrieval.py`
- Possibly add more scenarios or flags

Scenarios to add:
- lexical-only canary
- semantic-only or weak-lexical-overlap scenario
- graph/backlink-assisted scenario
- stale-index/freshness scenario if applicable

### Task 6.2: Reconcile specs/docs with shipped runtime
Objective: Make docs tell the truth after the implementation lands.

Files:
- Modify: `docs/specs/09-project-brain.md`
- Modify: `docs/specs/06-context-assembly.md`
- Modify: `docs/specs/08-data-model.md`
- Modify: `TECH-DEBT.md` if items close or change
- Optionally update: `NEXT_SESSION_HANDOFF.md` at session end

### Task 6.3: Final verification pass
Objective: Ensure the new brain contract is actually stable.

Run at minimum:
- `go test -tags sqlite_fts5 ./internal/brain/... ./internal/context ./internal/tool ./internal/server`
- `make test`
- `make build`
- updated live validation command(s)

Expected:
- brain-targeted tests pass
- any unrelated failures are clearly called out separately
- docs reflect operator truth

---

## Suggested implementation order for the first serious execution session

If doing this across multiple sessions, use this exact order:
1. Phase 0.1 + 0.2 + 0.4 failing tests
2. Phase 1.1 parser
3. Phase 1.2 DB upsert/query path
4. Phase 1.3 derived metadata/graph indexer
5. Phase 1.4 command wiring
6. Phase 2.1 + 2.2 semantic chunk/index path
7. Phase 3.1 + 3.2 hybrid retrieval
8. Phase 3.3 graph expansion
9. Phase 4.1 + 4.2 + 4.3 budgeting/serialization fixes
10. Phase 5 write/update/backlink/lint integration
11. Phase 6 validation/docs cleanup

That order avoids building semantic retrieval on top of nonexistent derived metadata.

---

## Success criteria

The brain rebuild is done when all of the following are true:
- vault markdown remains the source of truth
- `brain_documents` and `brain_links` are populated and used
- hybrid brain retrieval is a real runtime path
- `brain_search` supports true semantic/auto behavior
- brain-specific config knobs have real effect or are removed
- assembled context gives rich project knowledge before code when required by the spec
- writes/updates do not silently desync derived brain state
- signal/report/inspector surfaces explain why a brain hit was selected
- live validation proves lexical, semantic, and graph-assisted retrieval behavior
- docs describe the actual shipped contract

---

## Output expectations for whoever executes this plan

At the end of each phase, record:
- what changed
- exact files touched
- exact tests run
- pass/fail results
- what remains open

If the work spans sessions, finish by updating:
- `NEXT_SESSION_HANDOFF.md`
- `TECH-DEBT.md` only for still-open meaningful gaps
