# Sodoryard Stability Closeout Plan

> For Hermes: use this as the fresh-session execution plan for finishing the highest-value remaining stability gaps. Work one slice at a time with strict TDD. Keep edits narrow. Prefer `make test` and `make build` for final verification.

Goal
- Finish the remaining gaps that matter most for real daily use, without reopening already-closed architecture/spec churn.

Architecture / planning stance
- Treat Sodoryard as operationally real already, but not yet fully confidence-closed.
- Prioritize: real runtime stability, control-plane proof, operator confidence, and real-vault usefulness.
- Do not start with a command-queue rewrite or other large architecture changes unless a narrower slice proves insufficient.

Tech stack / constraints
- Go backend with sqlite_fts5 and LanceDB CGO requirements
- CLI surfaces: `cmd/sirtopham`, `cmd/yard`, `cmd/tidmouth`
- Web UI under `web/`
- Brain/retrieval path uses project-local `.brain` plus MCP/vault integration
- Validation commands should prefer:
  - `make test`
  - `make build`
- If using focused Go commands, include Makefile-equivalent CGO settings.

Non-goals
- No broad refactors just for elegance
- No speculative multi-operator architecture work unless a concrete bug demands it
- No pushing

---

## Phase 0 — Baseline / reality check

Objective
- Start every fresh session by proving the current tree still matches the documented state before taking a new slice.

Files
- Read: `AGENTS.md`
- Read: `README.md` (especially the current status / next session section)

Steps
1. Run `git status --short --branch`.
2. Re-read the latest handoff and this plan.
3. If the tree still contains the current chain-control WIP, do not discard it.
4. Run the most recent focused control-plane suites first if touching that area.
5. Only then begin the next slice.

Verification
- Current tree understood
- No unrelated churn introduced

---

## Phase 1 — Final control-plane proof on realistic long-running turns

Why this is first
- The core control semantics are much stronger now, but remaining uncertainty is mostly about proof under realistic in-flight timing, not obvious missing helper logic.
- This is the last high-value control-plane confidence pass before stopping control churn.

### Slice 1.1 — Blocked in-flight `runChain` / `yardRunChain` harness

Objective
- Prove that real command flow, not just helper entrypoints, leaves no stale active execution during pause/cancel interruption timing.

Files
- Modify: `cmd/sirtopham/chain_test.go`
- Modify: `cmd/yard/chain_test.go`
- Maybe modify: `cmd/sirtopham/chain.go`
- Maybe modify: `cmd/yard/chain.go`

Plan
1. Add failing tests first for a blocked loop harness:
   - one `sirtopham` case
   - one `yard` case
2. Use a fake agent loop that blocks until the test sets chain status to `pause_requested` / `cancel_requested` or directly to `paused` / `cancelled` depending on the scenario.
3. Drive the actual `runChain(...)` / `yardRunChain(...)` path as far as possible without starting real external processes.
4. Assert:
   - terminal user-facing message is correct
   - matching terminal event is present for the active `execution_id`
   - `LatestActiveExecution(...)` returns empty at the end
5. Implement only the minimum seam needed for deterministic blocking if the current loop/runtime seams are not enough.

Verification commands
- Focused:
  - `go test -tags sqlite_fts5 ./cmd/sirtopham ./cmd/yard -run 'Test.*(RunChain|YardRunChain).*ActiveExecution.*' -count=1`
- Broader:
  - `go test -tags sqlite_fts5 ./internal/chain ./cmd/sirtopham ./cmd/yard ./internal/spawn -count=1`
- Final:
  - `make test`
  - `make build`

Done means
- Real command-flow interruption timing is proven, not just helper logic.

Decision gate after Slice 1.1
- If this passes cleanly, stop adding more chain-control helpers unless a fresh real-use bug appears.
- If this reveals a real sequencing bug that cannot be fixed narrowly, then document the precise remaining gap before considering a larger control surface.

---

## Phase 2 — Daily-driver operator confidence gaps

Why this matters
- `README.md` now says the big remaining work is mostly proving/stabilizing real use, not audit mismatch cleanup.

### Slice 2.1 — Repeated real-use runtime soak on the intended setup

Objective
- Confirm the intended runtime remains stable across repeated normal use, not just a single maintained validation pass.

References
- `docs/manual-live-validation.md`
- `docs/v2-b4-brain-retrieval-validation.md`
- latest `README.md`

Plan
1. Use the intended `my-website` runtime already documented in `README.md` unless reality has changed.
2. Re-run the maintained validation flow.
3. Add at least one longer mixed session that exercises:
   - first turn
   - reload/history
   - settings/model routing
   - cancellation
   - search
   - retrieval/context inspector
4. Log only concrete failures; do not open-endedly polish if the run is clean.

Done means
- Either a concrete reproduced bug is found and queued as the next slice, or the runtime is judged stable enough to stop this area.

### Slice 2.2 — Clean up any runtime annoyance found in Slice 2.1

Objective
- Fix one concrete annoyance at a time, only if reproduced in the intended runtime.

Plan
1. Write a failing regression first.
2. Implement the minimum fix.
3. Re-run the exact runtime behavior plus normal test/build validation.

Stop rule
- If the runtime soak is clean, skip this slice entirely.

---

## Phase 3 — Brain usefulness / real-vault confidence

Why this matters
- The brain is usable, but `README.md` still identifies broader real-vault usefulness as a remaining meaningful gap.

### Slice 3.1 — Broader real-vault retrieval proof

Objective
- Determine whether the current brain behavior is genuinely good enough on the real note corpus.

Files likely to inspect
- `internal/context/...`
- `internal/brain/...`
- `docs/specs/09-project-brain.md`
- `docs/v2-b4-brain-retrieval-validation.md`

Plan
1. Run a wider real-vault probe beyond the maintained canary scenarios.
2. Collect examples of:
   - clearly good hits
   - misses
   - noisy hits
3. If the current behavior is good enough, document that and stop.
4. If one narrow failure mode dominates, take exactly one focused TDD slice against that failure mode.

Candidate narrow fixes only if demanded by evidence
- ranking cleanup
- tag/query handling cleanup
- operational-note filtering
- analyzer routing for explicit brain-oriented prompts

Done means
- Either “good enough in practice” is demonstrated, or one concrete next bugfix slice is defined from evidence.

---

## Phase 4 — Index freshness / operator ergonomics

Why this matters
- The explicit indexing workflow is acceptable but still a real daily-use friction point.

### Slice 4.1 — Make stale-index state more obvious or easier to recover from

Objective
- Improve operator confidence around whether code and brain indexes are fresh.

Likely files
- `internal/server/project.go`
- `web/src/pages/settings.tsx`
- `README.md`

Plan
1. Inspect what freshness metadata is already exposed.
2. Choose one low-surprise improvement:
   - clearer stale/clean indicators
   - clearer operator docs/workflow
   - lightweight reminder path
3. Add failing frontend/backend test if practical.
4. Implement minimal operator-facing improvement.

Done means
- The operator can easily tell when reindexing is needed.

Stop rule
- Do not implement surprising automatic reindexing unless strongly justified.

---

## Phase 5 — File browsing / operator UX gap

Why this matters
- The backend already exposes project tree/file surfaces; the remaining gap is mostly a product/UI completion gap.

### Slice 5.1 — First-class project/file browsing surface

Objective
- Add a minimal usable file-browser/code-viewer route or explicitly defer it.

Likely files
- `internal/server/project.go`
- `web/src/main.tsx`
- relevant web page/component files
- `docs/specs/07-web-interface-and-streaming.md`

Plan
1. Audit the existing backend endpoints and current web routes.
2. If the backend contract is already enough, add the thinnest useful UI path.
3. Keep the initial viewer read-only and simple.
4. Validate with frontend build plus one real browser check.

Done means
- Either the app has a basic first-class file viewer, or there is an explicit conscious decision to keep the harness conversation-first.

---

## Phase 6 — Closeout / declare current phase stable

Objective
- Once the above slices are either completed or explicitly skipped by evidence, rewrite the top-level docs to reflect reality rather than ongoing generic churn.

Files
- `README.md`

Plan
1. Remove already-finished control-plane hardening items from active “open” framing.
2. Keep only genuinely unresolved current gaps.
3. Rewrite the next-session recommendation around the highest-value remaining product/runtime work, not old audit items.
4. If the runtime soak was clean and the remaining items are non-blocking product gaps, say so plainly.

Done means
- Fresh sessions can start from an accurate, low-noise state.

---

## Suggested execution order for fresh sessions

1. Phase 0 baseline check
2. Phase 1.1 blocked in-flight command-flow harness
3. Phase 2.1 real runtime soak
4. Only if runtime finds issues: Phase 2.2 targeted bugfix
5. Phase 3.1 real-vault brain confidence pass
6. Phase 4.1 stale-index/operator confidence improvement
7. Phase 5.1 file browser/code viewer
8. Phase 6 closeout docs rewrite

---

## Stop conditions

Stop fixing and start using when all of the following are true:
- Phase 1.1 passes
- the intended runtime soak is clean or only shows minor annoyances
- no concrete real-vault brain bug is blocking daily work
- index freshness workflow is understandable enough for normal use
- remaining work is mostly UX/product improvement, not correctness/stability

At that point, Sodoryard should be treated as stable enough for real use, with future work driven by actual operator experience rather than speculative closeout churn.

---

## Commands cheat sheet

Preferred full validation
```bash
make test
make build
```

Useful focused validation
```bash
CGO_ENABLED=1 \
CGO_LDFLAGS="-L$(pwd)/lib/linux_amd64 -llancedb_go -lm -ldl -lpthread" \
LD_LIBRARY_PATH="$(pwd)/lib/linux_amd64" \
go test -tags sqlite_fts5 ./internal/chain ./cmd/sirtopham ./cmd/yard ./internal/spawn
```

---

## Final guidance for the fresh session

If a fresh session needs a single best starting point, start here:
- Phase 1.1 — blocked in-flight `runChain` / `yardRunChain` harness

Reason
- It is the last meaningful control-plane confidence gap left after the recent lifecycle/terminalization hardening.
- If it passes cleanly, stop deep control-plane work and shift to real-use/operator-confidence slices.
