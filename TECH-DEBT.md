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

### R5. SirTopham orchestrator teardown races provider sub-call recorder against DB close
Priority: low
Area: orchestrator runtime / cleanup

Current truth
- `cmd/sirtopham/chain.go` defers `rt.Cleanup()` which closes `.yard/yard.db` immediately on agent loop return
- the provider router writes per-call sub-call records via a goroutine that may still be in flight at that point
- result: the orchestrator emits a stray `ERROR msg="failed to record sub-call for stream" err="sql: database is closed"` line on every clean exit

Why it matters
- the chain itself completes correctly (verified live by Phase 3 smoke chain `phase3-smoke-1`)
- but the noise undermines operator confidence in the "exit 0 = clean" signal and could mask real teardown errors later
- `cmd/tidmouth/run.go` does NOT have this issue, so the fix is to align orchestrator teardown with the engine pattern

Done means
- drain or wait on in-flight sub-call writes before closing the DB in `buildOrchestratorRuntime`'s cleanup closure
- add a regression test that runs an orchestrator chain and asserts no `database is closed` error lines on clean exit

Primary references
- `cmd/sirtopham/runtime.go` (orchestratorRuntime.Cleanup)
- `cmd/sirtopham/chain.go` (defer rt.Cleanup)
- `internal/provider/tracking/` (sub-call store)

### R6. SirTopham auto-registers anthropic + openrouter providers even when not in yard.yaml
Priority: low
Area: orchestrator runtime / startup

Current truth
- `./bin/sirtopham chain --config <yaml>` against a yaml that lists only `codex` still logs `provider registered` for `anthropic` and `openrouter`
- the anthropic provider then fails its `Ping()` startup check and gets unregistered with a credentials warning, every single startup
- this happens regardless of whether anthropic credentials exist on the host

Why it matters
- wasted startup work + a misleading WARN line on every chain run
- operators reading orchestrator logs will assume something is misconfigured
- root cause is somewhere in the orchestrator's provider router init path that defaults-in providers the yaml never asked for

Done means
- only register providers explicitly listed under `providers:` in the loaded config
- no extra WARN lines on a clean orchestrator startup
- test that exercises a codex-only config and asserts the registration log is exactly `[codex]`

Primary references
- `cmd/sirtopham/runtime.go` (buildOrchestratorRuntime provider loop)
- `internal/provider/router/`
- compare with `cmd/tidmouth/runtime.go` provider registration

### R7. chain_complete writes orchestrator receipts with hardcoded zero metrics
Priority: low
Area: orchestrator receipt fidelity

Current truth
- `internal/spawn/chain_complete.go` builds the orchestrator receipt with `turns_used: 0`, `tokens_used: 0`, `duration_seconds: 0` literals
- the real chain metrics are tracked correctly in the chain store and surfaced via `sirtopham status`/`sirtopham receipt`
- the receipt body is therefore informative for the chain summary text but useless for any downstream tool that wants to read run metrics from receipt frontmatter

Why it matters
- spec 13 receipt frontmatter is supposed to be the canonical source of truth
- a Phase 4/Phase 6 dashboard that aggregates receipts will silently undercount everything orchestrators did

Done means
- `chain_complete` reads chain metrics from `Store.GetChain` before writing the receipt and emits the real numbers
- a unit test asserts non-zero metrics flow through to the receipt body when the chain has steps recorded

Primary references
- `internal/spawn/chain_complete.go` (Execute)
- `internal/chain/store.go` (GetChain, ChainMetrics)
- `docs/specs/13_Headless_Run_Command.md` (receipt contract)

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
- Spec drift from the 2026-04-08 audit sweep is reconciled for the current shipped scope.
- T6 indexing-contract cleanup is closed.
- T7 sidebar/web-surface reconciliation is closed for the current shipped scope.
- D1-D4 WebSocket/brain/tool/project-identity contract reconciliation is closed.

## Notes
- Keep this file focused on unresolved current gaps only.
- Closed audit items belong in git history and session handoff, not here.
- The maintained six-scenario brain-retrieval validation package is currently green on the live `:8092` runtime; the next best work should come from broader daily-driver validation, ranking/selection quality, or wider real-vault coverage rather than more prompt-family routing churn.
