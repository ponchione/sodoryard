# TECH-DEBT

Open items for the next phase after the v0.1 harness closeout.

Last sweep: 2026-04-07
Current phase: v0.2 scoping and post-v0.1 cleanup

What is no longer tech debt
- B1/B2/B3/B4/B5/B6/B7 are landed and validated enough to stop carrying as open debt.
- Basic code-path retrieval is proven on `~/source/my-website/`.
- The harness punchlist is effectively cleared for v0.1.
- Provider/config/operator surfaces now respect explicitly configured provider names.

The remaining items below are the real open work.

---

## v0.2 brain scope

The next meaningful product phase is v0.2. The brain is the main unfinished capability that should define that phase.

### V2-B1. Proactive brain retrieval in context assembly
Severity: High

Current state:
- brain tools work reactively: `brain_read`, `brain_write`, `brain_update`, `brain_search`, `brain_lint`
- a first proactive slice is now wired: context assembly runs keyword-backed brain search against the active MCP/vault backend and populates `RetrievalResults.BrainHits`
- assembled-context budgeting/serialization/reporting now treat brain hits as a real category
- proactive filtering now excludes `_log.md` operational brain log hits so they do not compete with real notes in Project Brain context
- brain-directed prompts can now emit a `brain_intent` signal so proactive retrieval can prefer brain context and skip generic code RAG when there are no explicit code references
- remaining gap: query shaping is still only lightly brain-aware; architecture/design questions that do not explicitly mention the brain may still rely on generic code-oriented queries/signals

v0.2 target:
- brain retrieval runs alongside code retrieval during context assembly
- analyzer/query extraction can emit brain-oriented queries/signals
- `RetrievalResults.BrainHits` is populated from the active brain backend
- context assembly reports, the new `/api/metrics/conversation/{id}/context/{turn}/signals` stream endpoint, and the inspector show real proactive brain retrieval decisions

Expected implementation seams:
- `internal/context/*` retrieval orchestration
- `internal/context/budget*` priority fitting
- context-report serialization / inspector payloads
- whichever backend stays canonical for brain retrieval (`mcpclient`/vault path vs any future index-backed path)

Done means:
- a live turn can answer from brain content without the model explicitly calling a brain tool
- inspector shows non-empty brain retrieval evidence for that turn
- budget breakdown includes brain budget use as a first-class category

### V2-B2. Brain-aware budget fitting and serialization
Severity: High

Current state:
- budget order is now effectively explicit files -> proactive brain hits -> top code RAG -> graph -> conventions -> git -> lower-priority code RAG overflow
- serializer emits a distinct Project Brain section and `budget_breakdown` includes a `brain` bucket
- remaining gap: the priority order is implemented but still needs live validation evidence and explicit operator-facing documentation polish

v0.2 target:
- introduce a real brain tier in budget fitting
- decide and document the priority order explicitly
- serialize proactive brain context as a distinct section, not as an implicit side-effect of tool usage

Initial intended ordering:
- explicit files
- proactive brain hits
- top code RAG hits
- structural graph
- conventions
- git
- lower-priority code RAG overflow

Done means:
- `budget_breakdown` can truthfully report brain token usage
- included/excluded brain results have the same diagnostic quality as code hits

### V2-B3. Brain backend/product contract cleanup
Severity: Medium-High

Current state:
- practical runtime path today is MCP/vault-backed brain tools plus MCP/vault-backed proactive keyword retrieval during context assembly
- README/spec/inspector docs have been reconciled to describe that operator-facing truth
- several future-facing config fields and old design notes still imply richer brain indexing/retrieval than the runtime actually uses, so the remaining debt is tightening or deleting those reserved surfaces rather than explaining the old story

v0.2 target:
- pick and document the actual source of truth for brain retrieval
- explicitly state whether v0.2 brain retrieval is:
  - MCP/vault keyword-backed only,
  - MCP + semantic/index-backed hybrid,
  - or an internal indexer with MCP/tooling as the mutation path
- remove ambiguity about which fields are active vs reserved

Done means:
- README, handoff, TECH-DEBT, and the brain spec all tell the same story
- operators can tell what brain setup is required and what behavior to expect
- stale root-level execution docs from the v0.1 harness phase are removed or explicitly archived

### V2-B4. Brain retrieval validation package
Severity: Low
Status: Landed 2026-04-07

Current state:
- the maintained package now lives at:
  - `docs/v2-b4-brain-retrieval-validation.md`
  - `scripts/validate_brain_retrieval.py`
- the default canary prompt (`What is the runtime brain proof canary phrase?`) now gives one repeatable live proof against the `:8092` my-website runtime that:
  - answers the brain-only fact `ORBIT LANTERN 642`
  - completes without explicit tool detours
  - persists `needs.semantic_queries`, `brain_results`, and non-zero `budget_breakdown.brain`
  - exposes the same signal/query flow through `/api/metrics/conversation/{id}/context/{turn}/signals`

Remaining note:
- this package intentionally proves the current operator-facing MCP/vault keyword path, not a future semantic/index-backed brain path

---

## Post-v0.1 hardening that still matters

### H1. `file_write` has no stale-write safety model
Severity: Medium

`file_edit` participates in read-state / stale-write protection. `file_write` does not. An agent can overwrite existing content it never read.

Fix direction:
- require a recent read before overwriting existing non-empty files, or
- route `file_write` through the same read-state invariants as `file_edit`

### H2. Prompt-cache latching is still absent
Severity: Medium

This is mostly cost/efficiency debt, not correctness debt.

Fix direction:
- separate stable prompt prefix from dynamic suffix
- apply provider prompt-cache controls on the stable segments where supported

### H3. Token-budget reserve/estimate/reconcile tracking is still thin
Severity: Medium

Budgeting works, but there is no explicit reserve -> estimate -> reconcile tracker.

Fix direction:
- add a dedicated budget tracker with output headroom reservation
- reconcile planned vs actual token use
- expose discrepancies in observability surfaces

### H4. Local LLM stack UX is correct but still a bit rough
Severity: Low-Medium

Still worth cleaning later:
- container names are global and can conflict across multiple repo-owned stacks on one machine
- `llm up` surfaces raw Docker conflict text for stale container-name conflicts
- `llm status` always prints remediation lines, even when healthy

This is not an architecture blocker anymore.

### H5. Security hardening remains deferred for localhost-only usage
Severity: Low for single-user localhost / High for any networked deployment

Still deferred:
- gate any insecure TLS behavior behind dev mode only
- replace shell denylist substring matching with token-aware matching
- validate git refs defensively
- audit/finish LanceDB filter escaping

---

## Cleanup / docs debt

### D1. Reconcile v0.1-complete docs vs v0.2-active docs
Severity: Low

Status: mostly done. The root README, handoff, TECH-DEBT, brain spec, context-assembly spec, and inspector/metrics docs now describe the current proactive-brain and signal-flow observability story.

Remaining cleanup direction:
- continue deleting or archiving stale one-off root docs from the old harness closeout phase
- keep future doc updates honest about keyword-backed brain retrieval vs any still-hypothetical semantic/index path
- avoid reintroducing old REST-only language unless the runtime actually depends on it again

### D2. Decide whether to stop registering unused built-in providers internally
Severity: Low-Medium

Operator-facing surfaces are now filtered correctly for explicitly scoped project YAMLs, which fixes the practical bug. Internally, built-in provider defaults may still be present in the merged config.

Fix direction:
- either keep this as an intentional internal-defaults model and document it
- or tighten config loading/build-provider registration so explicitly scoped configs do not carry unused built-ins at all

This is now cleanup, not a user-facing blocker.

---

## Status note

v0.1 is in good enough shape to stop treating every remaining issue as a harness blocker.
The next serious implementation phase should be v0.2 brain work, with only selective hardening/polish taken ahead of it when there is concrete evidence it matters.
