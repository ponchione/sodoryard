# Brain system audit and rebuild prompt

Date: 2026-04-09
Repo: /home/gernsback/source/sirtopham
Scope: project brain runtime, tooling, retrieval, persistence, observability, and spec alignment

## Executive verdict

The brain system is real, useful, and partially integrated, but it is not yet “what it should be by the spec”.

What is genuinely landed today:
- MCP-backed vault read/write/search is wired into the running app.
- Reactive brain tools exist and are tested.
- Proactive brain retrieval is live inside context assembly.
- Brain-specific intent routing, context reports, signal-stream observability, and inspector support are real.
- Brain linting exists and is already better than a toy implementation.

What is still missing relative to the broader spec/vision:
- no semantic/index-backed brain retrieval
- no live brain indexer or derived-brain pipeline
- no runtime population/use of `brain_documents` or `brain_links`
- no graph-backed backlinks/traversal
- no dedicated brain budget enforcement despite config fields
- no dedicated brain relevance threshold enforcement despite config fields
- serialized brain context is thinner than the spec intent
- serialization order currently contradicts the spec: code is emitted before brain

Bottom line:
The current system is a good v0.2 keyword-first foundation, not a spec-complete brain system. The next work should be a focused brain-only product slice that upgrades it from “vault keyword notes” into “persistent project memory with derived metadata, graph, robust retrieval, and strong observability”.

## Audit evidence

### 1. What is operationally real today

1) Brain backend is wired into serve via MCP/vault
- `cmd/sirtopham/serve.go:63-72` builds the brain backend with `mcpclient.Connect(...)`
- `cmd/sirtopham/serve.go:183-197` injects the brain backend into retrieval and enables brain query logging
- `cmd/sirtopham/serve.go:207` registers brain tools in the live tool registry

2) Brain tools are real
- `internal/tool/register.go:40-56`
- `internal/tool/brain_search.go`
- `internal/tool/brain_read.go`
- `internal/tool/brain_write.go`
- `internal/tool/brain_update.go`
- `internal/tool/brain_lint.go`

3) Proactive brain retrieval is wired into context assembly
- `internal/context/retrieval.go:209-221` runs proactive brain retrieval in parallel
- `internal/context/retrieval.go:327-379` executes brain keyword search over derived query candidates
- `internal/context/budget.go:63-79` gives brain hits first-class budget priority after explicit files
- `internal/context/types.go:53-66`, `148-168` persists brain results in reports

4) Brain-intent routing is real
- `internal/context/analyzer.go:612-720`
- explicit brain intent and three non-explicit families are already implemented:
  - rationale
  - convention
  - history/debugging

5) Observability is real
- `internal/context/report_store.go` persists reports/signals/brain results
- `internal/server/metrics.go:26-28` exposes context + signal-stream endpoints
- `web/src/components/inspector/context-inspector.tsx` renders brain results and signal flow

6) Brain-targeted test surface is healthy
- targeted command run during this audit passed:
  - `go test -tags sqlite_fts5 ./internal/brain/... ./internal/context ./internal/tool ./internal/server`
- full `make test` currently fails, but due to unrelated codex provider model discovery:
  - `internal/provider/codex` / `TestVisibleModelsMatchInstalledCodexAppServer`
  - not a brain-system failure

### 2. Biggest spec gaps

1) Semantic/index-backed brain retrieval is not implemented
Evidence:
- `internal/brain/backend.go` exposes only read/write/patch/keyword-search/list operations
- `internal/context/interfaces.go:39-43` brain retrieval surface is only `SearchKeyword`
- `internal/context/retrieval.go` only calls keyword search
- searches for `MaxBrainTokens|BrainRelevanceThreshold|IncludeGraphHops|GraphHopDepth|EmbeddingModel|ChunkAtHeadings|ReindexOnStartup` found no runtime use outside config/tests/docs

Implication:
The product still behaves like a keyword vault, not a semantic long-term memory system.

2) The schema for derived brain state exists, but runtime does not use it
Evidence:
- schema defines tables in `internal/db/schema.sql:101-124`
- repo search for `brain_documents|brain_links` found runtime references only in schema/init/tests, not in live indexing/retrieval code

Implication:
The system has storage reserved for brain metadata/graph, but no live pipeline populates or consumes it.

3) Serialization order contradicts the current spec
Spec says:
- `docs/specs/09-project-brain.md:399` says brain content is serialized before code chunks
- `docs/specs/06-context-assembly.md:328+` shows Project Knowledge first

Runtime does:
- `internal/context/serializer.go:29-45`
- order is: relevant code -> structural -> brain -> conventions -> git

Implication:
Even when brain retrieval succeeds, the final context gives code higher placement than the current brain spec intends.

4) Brain serialization is too thin
Evidence:
- `internal/context/serializer.go:166-193` serializes each brain hit as one bullet line with path/title/mode/snippet

Implication:
The assembled context gets note references and snippets, not rich structured note content. This weakens the effect of successful retrieval and is below the spec’s richer “Project Knowledge” intention.

5) Brain-specific budget/config knobs are mostly inert
Evidence:
- config defines them in `internal/config/config.go:134-148`
- budget manager uses only global context budgets plus conventions/git caps in `internal/context/budget.go`
- no dedicated use of `brain.max_brain_tokens`
- no dedicated use of `brain.brain_relevance_threshold`

Implication:
The brain budget contract is mostly documentary today, not enforced runtime behavior.

6) Backlinks/graph traversal remain heuristic or absent
Evidence:
- `brain_read` backlink mode heuristically searches by basename in `internal/tool/brain_read.go:103-115`
- no runtime use of `brain_links`

Implication:
The graph-shaped memory part of the spec is not yet real.

7) Keyword retrieval remains intentionally narrow and somewhat brittle
Evidence:
- `internal/brain/vault/client.go:106-170` is normalized substring/path matching over markdown files
- proactive retrieval improves it with candidate shaping in `internal/context/retrieval.go:381-451`, but still remains keyword matching

Implication:
This works for well-worded notes and maintained canaries, but it is not yet robust for fuzzier historical knowledge or weak lexical overlap.

## Current strengths worth preserving

Do not throw these away during the rebuild:
- MCP/vault backend simplicity and local-first design
- good path-safety handling in `internal/brain/vault/client.go`
- already-useful intent shaping in `internal/context/analyzer.go`
- existing signal-stream/report plumbing
- `brain_lint` as a maintenance surface, especially scope handling and deterministic checks
- `_log.md` exclusion from proactive retrieval/tool search paths
- maintained live validation package in `docs/v2-b4-brain-retrieval-validation.md`

## Brain rebuild goals

The target state should be:

1. Vault remains source of truth
- human-readable markdown in the vault stays authoritative
- all derived data is rebuildable from vault contents

2. Derived brain index becomes first-class
- extract frontmatter, tags, titles, headings, wikilinks, backlinks, timestamps, content hash
- populate `brain_documents` and `brain_links`
- add a dedicated semantic/vector representation for brain chunks

3. Retrieval becomes hybrid, not keyword-only
- keyword retrieval remains available and cheap
- semantic search joins it as a real runtime path
- graph expansion/backlinks can refine or broaden candidate sets
- ranking uses a blend of lexical, semantic, structural, tag, and freshness signals

4. Context serialization becomes knowledge-rich
- emit a real Project Knowledge section with meaningful note content
- place it before code when the spec says to do so
- keep it concise, but not so thin that it loses reasoning value

5. Observability remains excellent
- every retrieval source and ranking reason should remain inspectable
- context reports should show lexical hits, semantic hits, graph-expanded hits, ranking reasons, and budget decisions

6. Robustness becomes explicit
- indexing must be incremental and rebuildable
- writes/updates should keep derived state fresh or obviously stale
- stale-index state should be detectable
- validation should cover both happy paths and adversarial cases

## Hard truths to design around

- The current docs already admit that semantic/index-backed brain retrieval is future work. This is not a bug audit; it is a “close the major missing product slice” audit.
- The fastest path is not to replace the vault backend. It is to add a derived indexed layer under it.
- The right architecture is likely:
  - vault markdown = source of truth
  - SQLite = metadata + graph + bookkeeping
  - dedicated brain vectors/chunks = semantic retrieval layer
  - hybrid retrieval/ranking = operator-facing runtime

## Recommended implementation direction

### Phase 0: lock the operator truth and add regression tests

Before major changes:
- add tests asserting current proactive brain retrieval/reporting behavior remains intact
- add a failing test for serializer ordering if we want brain-before-code
- add failing tests for richer brain serialization
- add failing tests for derived metadata persistence once that slice starts

Suggested files:
- `internal/context/serializer_test.go`
- `internal/context/retrieval_test.go`
- `internal/context/assembler_test.go`
- `internal/server/metrics_test.go`
- `scripts/validate_brain_retrieval.py`

### Phase 1: introduce a real derived brain document model

Implement a canonical parser/index layer that can:
- enumerate all vault docs
- parse frontmatter/tags/title/headings/wikilinks
- compute content hash and token count
- upsert into `brain_documents`
- rebuild `brain_links`

Likely new files:
- `internal/brain/indexer/*.go`
- `internal/brain/parser/*.go` or equivalent
- `internal/brain/store/*.go` or SQL helpers

Likely touched files:
- `internal/db/schema.sql` if extra derived fields/indexes are needed
- `internal/db/*.sql.go` or source SQL queries
- `cmd/sirtopham/index.go` and/or a dedicated brain indexing command path

Definition of done:
- `brain_documents` and `brain_links` are actually populated from the vault
- a rebuild from vault produces deterministic derived state
- changed notes update incrementally

### Phase 2: add semantic brain indexing and hybrid retrieval

Implement:
- chunking at headings/sections for brain notes
- brain embeddings stored separately from code embeddings
- hybrid ranking combining:
  - lexical match
  - semantic similarity
  - graph distance/backlinks
  - tag/family match
  - freshness/recency if helpful

Runtime contract:
- proactive context assembly should use a hybrid retriever, not just keyword search
- `brain_search` should support true `auto`/`semantic`, not compatibility fallthrough

Likely touched files:
- `internal/context/interfaces.go`
- `internal/context/retrieval.go`
- `internal/tool/brain_search.go`
- new brain index storage/retrieval packages
- config plumbing for currently inert brain settings

Definition of done:
- `brain.max_brain_tokens`, `brain.brain_relevance_threshold`, `brain.include_graph_hops`, `brain.graph_hop_depth`, `brain.chunk_at_headings`, and relevant index settings have real runtime effect or are removed

### Phase 3: make assembled brain context actually useful

Change serialization to:
- emit Project Knowledge before Relevant Code when appropriate per spec
- include compact but substantive note excerpts/sections, not only one-line bullets
- preserve note path, title, and why it matched
- annotate graph relationships where helpful

Likely touched files:
- `internal/context/serializer.go`
- `internal/context/serializer_test.go`
- potentially report payload types if richer explanation metadata is added

Definition of done:
- a matched note materially helps answer a question without requiring a follow-up `brain_read`

### Phase 4: strengthen write/update and freshness guarantees

Implement one of two explicit contracts:
- synchronous freshness: writes/updates refresh derived brain state immediately
- asynchronous freshness: writes/updates mark the derived state dirty and the UI/runtime clearly reflects staleness

Do not leave this ambiguous.

Also improve:
- write metadata capture (`created_by`, source conversation/session, tags/frontmatter extraction)
- replace-section safety and heading targeting tests
- operational logging that does not pollute knowledge retrieval

### Phase 5: expand validation and robustness testing

Add validation for:
- lexical-only wins
- semantic-only wins
- graph/backlink wins
- note rename/move handling
- stale index detection/recovery
- malformed frontmatter
- large vault performance
- irrelevant-note suppression
- contradictory notes surfaced by lint or ranking explanations

Keep the maintained live validation package, but expand it into a true brain validation suite.

## Non-goals / things not to do

- do not replace the vault as source of truth with opaque DB-only state
- do not add a giant speculative memory architecture without first making derived metadata + hybrid retrieval real
- do not weaken observability in the name of abstraction
- do not ship semantic retrieval without validation that proves it outperforms the current keyword path on real vault notes
- do not leave config fields as decorative placeholders if they claim runtime behavior

## Copy-paste prompt for the implementation agent

Use this prompt verbatim or as the starting point for a coding agent:

"""
Audit and rebuild the sirtopham project brain so it matches the intended spec direction and is robust in daily use.

Context:
- Repo: /home/gernsback/source/sirtopham
- Follow AGENTS.md and RTK.md
- Prefer `make test` / `make build`; if running Go directly use `-tags sqlite_fts5`
- The current brain runtime is MCP/vault-backed and keyword-first
- The vault markdown is the source of truth; do not replace that
- The goal is to make the brain a real persistent project memory system, not just reactive note tools plus keyword snippets

First, read and use these files as the primary audit/spec surface:
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

Current audit conclusions you should verify and then act on:
1. proactive brain retrieval is real, but keyword-only
2. `brain_documents` / `brain_links` schema exists but is not populated/used at runtime
3. serializer order currently puts code before brain, which contradicts the current spec text
4. brain serialization is too thin to be truly useful
5. brain-specific config knobs are mostly inert
6. backlinks/graph traversal are not a real runtime retrieval source yet

Your job:
- produce a precise implementation plan for making the brain spec-aligned and robust
- then implement the highest-leverage slices, keeping edits narrow and verified

Target architecture:
- vault markdown remains source of truth
- derived brain metadata/index layer is added under it
- SQLite stores brain document metadata and wikilink graph
- semantic brain chunks/index are added as a real runtime retrieval source
- retrieval becomes hybrid: lexical + semantic + graph + tags/metadata
- assembled context gets a real Project Knowledge section that is actually useful
- observability remains excellent

Required deliverables:
1. populate and use derived brain metadata (`brain_documents`, `brain_links`)
2. implement a real brain indexing/reindexing path
3. implement semantic or hybrid brain retrieval in runtime, not docs only
4. make config fields either real or remove/re-scope them
5. fix serializer ordering/content so brain context is strong and spec-aligned
6. expand tests and live validation for the new contract

Constraints:
- preserve current MCP/vault tool behavior unless intentionally improving it
- do not regress existing proactive retrieval/reporting behavior
- do not introduce hidden stale-index behavior; make freshness explicit
- do not hand-wave validation; run tests and report exact results

Validation expectations:
- targeted Go tests for brain/context/tool/server packages
- updated live validation docs/scripts if runtime contract changes
- clear final summary of what is now true, what remains future work, and any operator-facing behavior changes
"""

## Practical next step

The best next execution slice is:
1. add a failing serializer-order/spec-alignment test
2. design and land the derived `brain_documents` + `brain_links` population path
3. only then add hybrid retrieval on top of that derived layer

That sequence gives a stable foundation instead of bolting semantic retrieval onto an unstructured keyword-only substrate.
