# TECH-DEBT

Last audit refresh: 2026-04-11
Current phase: operationally healthy, plausible daily-driver, but not yet fully proven on real day-to-day use
Verification baseline: `make test` ✅, `make build` ✅ (frontend chunk-size warning + npm audit warning only)

Overall verdict
- The harness is now coherent and real enough to use daily as a single-user coding harness.
- The biggest remaining gaps are no longer stale spec drift; they are real readiness/product gaps around live operator confidence, brain usefulness on real vaults, and a few workflow/UX limitations.
- Future work should focus on proving and strengthening the shipped harness, not reopening already-closed audit mismatches.

## Active gaps worth caring about

### R1. Daily-driver validation is still mostly inferred from tests/builds, not proven on the exact real setup
Priority: high
Area: runtime confidence / operator readiness

Why it matters
- `make test` and `make build` prove the repo is healthy, but they do not prove the exact provider/model, target project, vault, and operator workflow you intend to use every day.
- A harness can be technically healthy and still have annoying daily-use edge cases in first-turn chat, reload/history, cancel/retry, provider switching, retrieval quality, or browser/runtime behavior.

Done means
- run the maintained live validation flow against the exact config/provider/model/project you plan to use daily
- confirm first-turn chat, reload/history, search, settings/model routing, cancellation, and retrieval all behave correctly in the real browser/runtime
- confirm browser console and backend logs stay clean during that pass

Primary references
- `MANUAL_LIVE_VALIDATION.md`
- `docs/v2-b4-brain-retrieval-validation.md`
- `scripts/validate_brain_retrieval.py`

### R2. Brain readiness is good enough to use, but still narrower than an ideal daily-driver memory system
Priority: high
Area: project brain / retrieval quality

Current truth
- proactive brain retrieval is now hybrid at runtime: MCP/vault keyword hits plus the derived semantic brain index can both feed context assembly
- reactive `brain_search` now exposes real semantic/auto runtime behavior and graph-aware labeling, while `brain_read include_backlinks` prefers derived `brain_links` when available
- current operator confidence still depends on note wording quality, graph quality, and on how well the hybrid runtime path matches real vault usage

Why it matters
- the brain can help daily, but it is still easiest on notes with strong textual anchors
- if the real vault grows large or contains fuzzier rationale/debugging notes, keyword-only retrieval may feel brittle compared with the broader product vision
- daily-driver confidence depends on proving that real notes are found reliably, not just the maintained canary scenarios

Done means
- run broader live validation on the real vault/note corpus you actually depend on
- identify whether keyword retrieval is sufficient in practice or whether semantic/index-backed brain retrieval should become a real next product slice
- if keyword-only remains the contract, tighten note-authoring guidance and validation coverage around that reality

Primary references
- `docs/specs/09-project-brain.md`
- `docs/v2-b4-brain-retrieval-validation.md`
- `MANUAL_LIVE_VALIDATION.md`

### R3. Index freshness is still an explicit operator workflow, not a seamless daily-drive experience
Priority: medium
Area: indexing / operator ergonomics

Current truth
- code retrieval freshness depends on running `sirtopham index`
- brain freshness depends on running `sirtopham index brain`
- `serve` does not auto-reindex, but brain stale/clean status is now surfaced in Settings and `/api/project`

Why it matters
- this is acceptable, but it adds friction and is easy to forget during real use
- daily-driving feels better when the operator can trust index freshness without extra ceremony or ambiguity

Done means
- either implement a trustworthy low-surprise auto-refresh/index reminder workflow
- or make the explicit indexing workflow even clearer in the UI/runtime so stale-index states are obvious

Primary references
- `README.md`
- `MANUAL_LIVE_VALIDATION.md`
- `docs/specs/04-code-intelligence-and-rag.md`

### R4. The current UI is usable, but still missing a first-class project/file browsing surface
Priority: medium
Area: web UI / operator experience

Current truth
- conversation, sidebar search, settings, metrics, and streaming are real
- `/api/project/tree` and `/api/project/file` exist
- there is still no dedicated shipped file-browser/code-viewer route

Why it matters
- for daily drive, being able to inspect project structure/files directly in the app would improve operator confidence and reduce tool/context switching
- the backend surface already exists, so this is a product-gap/UX gap more than an architecture gap

Done means
- build a real file-browser/code-viewer experience on top of the existing project endpoints
- or explicitly decide that the harness stays conversation-first and operators should use external editors/file browsers

Primary references
- `docs/specs/07-web-interface-and-streaming.md`
- `internal/server/project.go`
- `web/src/main.tsx`

## Recently closed and should stay closed
- R5 DB close race: fixed by adding WaitGroup to TrackedProvider and DrainTracking() to the router, called before DB close in orchestrator cleanup (2026-04-12, `v0.7-containerization`).
- R6 extra provider registration: fixed by iterating ConfiguredProviders instead of all default Providers in buildOrchestratorRuntime (2026-04-12, `v0.7-containerization`).
- R7 hardcoded receipt metrics: fixed by reading real chain metrics from Store.GetChain before writing orchestrator receipt (2026-04-12, `v0.7-containerization`).
- Spec drift from the 2026-04-08 audit sweep is reconciled for the current shipped scope.
- T6 indexing-contract cleanup is closed.
- T7 sidebar/web-surface reconciliation is closed for the current shipped scope.
- D1-D4 WebSocket/brain/tool/project-identity contract reconciliation is closed.

## Notes
- Keep this file focused on unresolved current gaps only.
- Closed audit items belong in git history and session handoff, not here.
- The maintained six-scenario brain-retrieval validation package is currently green on the live `:8092` runtime; the next best work should come from broader daily-driver validation, ranking/selection quality, or wider real-vault coverage rather than more prompt-family routing churn.
