# Session handoff — audit follow-through

**Date:** 2026-04-12
**Branch:** main
**Cwd:** /home/gernsback/source/sodoryard

> Read this cold. Everything needed to continue the current audit follow-through is here. If this doc disagrees with the repo, trust the repo and update this doc before acting.

---

## What this repo is

Migrating `ponchione/sirtopham` (single-binary coding harness) into the `ponchione/sodoryard` monorepo. The local directory is `/home/gernsback/source/sodoryard`; the git remote points at `git@github.com:ponchione/sodoryard.git`.

Target monorepo layout remains:
- **Tidmouth** — headless engine harness (`cmd/tidmouth/`)
- **SirTopham** — chain orchestrator (`cmd/sirtopham/`)
- **Yard** — unified operator-facing CLI (`cmd/yard/`)
- **Knapford** — web dashboard placeholder / later `yard serve` expansion

The historical migration roadmap is `sodor-migration-roadmap.md`.

---

## Why this handoff exists

This handoff started from an audit-follow-through session, but the repo has moved since the original note was written.

Current reality as of 2026-04-12 19:07 -04:00:
- the older handoff text below still describes the earlier audit slices accurately
- the repo no longer has a root `AUDIT.md`, so treat references to it as historical context rather than a file you can read now
- a small follow-up UI/runtime contract slice and matching docs/spec reconciliation are now also landed in the working tree

Read in this order before changing code:
1. `AGENTS.md`
2. `TECH-DEBT.md`
3. this file

Use `make test` / `make build` rather than raw Go commands unless you intentionally need a focused command with the right CGO/sqlite flags.

---

## Current working-tree status

As of 2026-04-13 10:49 -04:00:
- `npx vitest run src/hooks/use-context-report.test.tsx` ✅
- `npx tsc --noEmit` ✅
- `make test` ✅
- `make build` ✅
- live `yard serve --config /tmp/my-website-runtime-8092.yaml` browser rerun on `http://localhost:8092` ✅

Current modified files (use live `git status` for the full dirty tree):
- prior in-flight orchestrator/control/docs/web files from the earlier audit follow-through slices are still present
- new files from this slice:
  - `web/src/hooks/use-context-report.ts`
  - `web/src/hooks/use-context-report.test.tsx`
  - `web/src/test/setup.ts`
  - `web/vite.config.ts`
  - `web/package.json`
  - `web/package-lock.json`

What the new runtime-validation follow-up proved:
- exact-setup daily-driver validation ran against the intended `my-website` runtime on `:8092`
- first-turn chat, sidebar/new conversation, reload/history, settings/model routing, search quality, code-retrieval grounding, and the maintained six-scenario brain-retrieval package all passed
- backend websocket cancellation itself is live and emits `turn_cancelled` with reason `user_interrupted`

What this slice changed:
- reproduced a concrete frontend/runtime bug: the context inspector eagerly fetched `/api/metrics/conversation/:id/context/:turn` for the newest live turn before the report existed, causing transient 404s and noisy browser/backend logs during normal first-turn use
- added targeted frontend tests for the intended behavior
- changed `use-context-report` so newest-turn report fetches are briefly deferred while following the live latest turn, and the deferred fetch is cancelled if a real-time `context_debug` report arrives first
- reran the live browser check after rebuilding; the first-turn + inspector path stayed clean with no repeated browser-console 404 noise in the rerun
- a smaller remaining observability issue still exists: expected user-triggered cancellation currently logs `agent loop: turn cancelled: context canceled` at `level=ERROR` on the backend even when the turn correctly emits `turn_cancelled/user_interrupted`

The older dirty-tree inventory below is stale history; use live `git status` over that list.

**Not pushed.** User pushes manually.

---

## What was completed this session

### 1. P0 — chain CLI flags / operator surface
Files:
- `cmd/sirtopham/chain.go`
- `cmd/yard/chain.go`
- `cmd/sirtopham/chain_cli_flags_test.go`
- `cmd/yard/chain_test.go`

What changed:
- added/plumbed `--project`, `--brain`, `--max-resolver-loops`
- added numeric validation for chain start flags
- zero resolver loops are allowed
- tests added for flag presence, plumbing, and validation

### 2. P0 — WebSocket error payload contract
Files:
- `internal/server/websocket.go`
- `internal/server/websocket_test.go`

What changed:
- standardized WS error payloads to `message` + optional `recoverable` + optional `error_code`
- tests verify absence of legacy `error` field and correct new shape

### 3. P0 — brain-disabled truly disables brain runtime paths
Files:
- `internal/runtime/engine.go`
- `internal/runtime/engine_test.go`
- `internal/tool/register.go`
- `internal/tool/brain_test.go`
- `internal/role/builder_test.go`
- `cmd/tidmouth/serve_test.go`

What changed:
- disabled brain no longer opens hybrid brain runtime
- disabled brain no longer reads convention-source docs from the vault
- disabled brain no longer registers brain tools
- focused tests added/updated

### 4. P0/P1-ish — chain control semantics materially improved
Files:
- `cmd/sirtopham/chain.go`
- `cmd/sirtopham/cancel.go`
- `cmd/sirtopham/pause_resume.go`
- `cmd/yard/chain.go`
- `internal/spawn/spawn_agent.go`
- `internal/spawn/spawn_agent_test.go`

What changed:
- resume is real now, not just cosmetic status flipping
- `sirtopham resume <chain-id>` and `yard chain resume <chain-id>` restart orchestration for an existing paused chain using stored chain task/specs
- paused/cancelled chains stop new step scheduling before next `spawn_agent`
- transition validation added so terminal chains cannot be resumed/paused/cancelled nonsensically
- best-effort live cancel path added by logging `orchestrator_pid` and signaling active orchestrator process on cancel
- chain run now listens for interrupt signals so cancel can propagate into the running orchestrator turn and then into the current `spawn_agent` subprocess through existing context-driven subprocess cancellation

Important caveat:
- this is better, but still not a full explicit control-plane implementation with `pause_requested` / `cancel_requested` durable flags or a command queue table
- current behavior is practical and materially improved, but not the last word on spec-perfect control semantics

### 5. P0 — structural hop expansion package-aware filtering
Files:
- `internal/codeintel/searcher/searcher.go`
- `internal/codeintel/searcher/searcher_test.go`

What changed:
- hop expansion no longer accepts all same-name symbols blindly
- after `GetByName(ref.Name)`, candidates are filtered against `ref.Package` using low-risk path/package heuristics
- regression test added with duplicated symbol names across packages to prove unrelated package hop hits are excluded

### 6. P1 — receipt fallback step plumbing + `chain_complete` status fidelity
Files:
- `cmd/tidmouth/receipt.go`
- `cmd/tidmouth/receipt_test.go`
- `cmd/yard/run_helpers.go`
- `cmd/yard/run_helpers_test.go`
- `internal/spawn/chain_complete.go`
- `internal/spawn/chain_complete_test.go`
- `cmd/sirtopham/chain.go`
- `cmd/yard/chain.go`

What changed:
- fallback receipts no longer hardcode `step: 1` for step-specific receipt paths
- step number is inferred from paths like `...-step-003.md`
- direct headless run still defaults safely to step 1 for plain `receipts/{role}/{chain-id}.md`
- `chain_complete status=partial` now persists chain status `partial` instead of collapsing to `completed`
- resume logic now treats `partial` as terminal and refuses resume/continue for that state
- focused tests added for fallback step inference and partial status persistence

### 7. Web/runtime contract cleanup + docs reconciliation
Files:
- `web/src/types/events.ts`
- `web/src/pages/conversation.tsx`
- `web/src/components/inspector/context-inspector.tsx`
- `docs/specs/05-agent-loop.md`
- `docs/specs/06-context-assembly.md`
- `docs/specs/07-web-interface-and-streaming.md`

What changed:
- frontend `AgentState` now matches the shipped backend status values
- conversation streaming UI now shows useful labels for `assembling_context`, `waiting_for_llm`, `executing_tools`, `compressing`, and `idle`
- the context inspector now renders explicit load-failure UI instead of silently appearing empty
- specs/docs now describe the shipped status-state contract and the current inspector/report/signal-flow behavior
- stale doc references to a root `AUDIT.md` were cleaned up under `docs/`

### 8. Receipt-path contract cleanup
Files:
- `internal/receipt/path.go`
- `internal/receipt/path_test.go`
- `cmd/tidmouth/receipt.go`
- `cmd/yard/run_helpers.go`
- `cmd/sirtopham/receipt.go`
- `internal/spawn/spawn_agent.go`
- `internal/spawn/chain_complete.go`
- `docs/specs/13_Headless_Run_Command.md`
- `docs/specs/14_Agent_Roles_and_Brain_Conventions.md`
- `docs/specs/15-chain-orchestrator.md`
- `agents/*.md` receipt-path references for the shipped role prompts

What changed:
- introduced one shared receipt-path helper package so the shipped path conventions are defined in one place
- direct headless runs now explicitly use `receipts/{role}/{chain-id}.md`
- orchestrator-managed step runs now explicitly use `receipts/{role}/{chain-id}-step-{NNN}.md`
- final orchestrator receipts now explicitly use `receipts/orchestrator/{chain-id}.md`
- fallback step inference now reuses the shared path parser instead of duplicating local regex/path logic in multiple commands
- specs and agent prompts now describe the shipped runtime contract instead of the older task-slug or `{chain_id}-{step}` variants

### 9. Narrow chain control-plane follow-through
Files:
- `internal/chain/control.go`
- `internal/chain/control_test.go`
- `cmd/sirtopham/chain.go`
- `cmd/sirtopham/cancel.go`
- `cmd/sirtopham/chain_cli_flags_test.go`
- `cmd/yard/chain.go`
- `cmd/yard/chain_test.go`
- `internal/spawn/spawn_agent.go`
- `internal/spawn/spawn_agent_test.go`

What changed:
- introduced shared control-state helpers for `pause_requested` and `cancel_requested`
- pausing a running chain now records `pause_requested` instead of pretending the chain is already paused mid-step
- cancelling a running chain now records `cancel_requested` before the best-effort live interrupt path
- resume now accepts `pause_requested` as well as `paused`
- `spawn_agent` now stops new scheduling when a chain is in requested-stop states, not just already-finalized paused/cancelled states
- orchestrator run cleanup now finalizes requested stop states to durable terminal states (`paused` / `cancelled`) once the current turn exits

### 10. Exact-setup daily-driver validation + context-inspector latest-turn fetch hardening
Files:
- `web/src/hooks/use-context-report.ts`
- `web/src/hooks/use-context-report.test.tsx`
- `web/src/test/setup.ts`
- `web/vite.config.ts`
- `web/package.json`
- `web/package-lock.json`
- `NEXT_SESSION_HANDOFF.md`

What changed:
- ran the maintained daily-driver validation flow against the intended `my-website` runtime on `http://localhost:8092`
- confirmed first-turn chat, sidebar/new conversation, reload/history, settings/model routing, search quality, code-retrieval grounding, and the maintained six-scenario brain-retrieval package on the live runtime
- reproduced a concrete bug where the inspector eagerly fetched newest-turn context endpoints before the report existed, causing transient `/context/:turn` and `/context/:turn/signals` 404s in browser console and backend logs
- added targeted frontend tests proving latest-turn fetches are deferred and cancelled when a live `context_debug` report arrives first
- updated `use-context-report` to defer newest-turn fetches briefly while following the live latest turn, which eliminated the repeated first-turn 404 noise in the rerun after rebuild

### 11. Expected-cancellation log-severity cleanup
Files:
- `internal/server/websocket.go`
- `internal/server/websocket_test.go`
- `NEXT_SESSION_HANDOFF.md`

What changed:
- reproduced a second concrete observability bug: expected user-triggered cancellation emitted `turn_cancelled/user_interrupted` correctly but still logged `msg="run turn"` at `level=ERROR`
- added a regression test proving websocket-run turns that return `agent.ErrTurnCancelled` do not emit the generic run-turn error log
- updated the websocket handler to classify `agent.ErrTurnCancelled` as expected control flow and log it as `run turn cancelled` at info level instead of error
- reran the targeted websocket test, full `make test`, and `make build`
- reran the direct websocket cancellation probe and confirmed the live runtime still emits `turn_cancelled` with reason `user_interrupted`, persists the interrupted tool tombstone, and still returns sanitized `/api/conversations/search?q=interrupted` snippets

---

## Verification status
- multiple focused package test runs for touched areas
- `npx vitest run src/hooks/use-context-report.test.tsx` ✅
- `npx tsc --noEmit` ✅
- `make test` ✅
- `make build` ✅
- live browser rerun against `yard serve --config /tmp/my-website-runtime-8092.yaml` on `http://localhost:8092` ✅

---

## Audit state after this session

The original ranked list lived in a root `AUDIT.md` during the earlier audit pass; that file is not present in the current repo snapshot, so treat the summary below as the historical audit record.

### Substantially addressed in code
- P0 #2 chain CLI missing flags
- P0 #3 WebSocket error payload shape
- P0 #4 brain-disabled behavior
- P0 #5 structural hop expansion looseness
- P1 #6 receipt contract cleanup
- P1 #7 `chain_complete` partial-status collapse

### Improved but not fully closed
- P0 #1 chain pause/resume/cancel semantics
  - resume is real now
  - pause/cancel requested states now exist (`pause_requested`, `cancel_requested`) so running chains no longer pretend they are already terminal mid-step
  - `spawn_agent` now respects requested-stop states before scheduling another engine
  - cancel still relies on best-effort pid/event-based live signaling rather than a first-class durable command queue or stronger orchestration coordination

### Still clearly open / best next work
Likely next highest-value items now:
1. if you still want deeper control semantics after the daily-driver and observability cleanup, move from requested-state strings + best-effort pid signaling to a first-class durable command/control surface
2. any additional doc cleanup should now be narrow and evidence-driven rather than another broad audit sweep
3. optional polish: if you care about even quieter logs, decide whether the new `run turn cancelled` info log should remain or whether the existing `turn cancelled` event log is already sufficient by itself

---

## Recommended next slice

The exact-setup daily-driver validation slice is now done, the highest-value concrete validation failure (latest-turn inspector 404 noise) is fixed, and expected user-triggered cancellation no longer emits the bogus generic run-turn error. Next best move:

### Option A — deeper chain control architecture (recommended)
If you want to keep pushing the control plane:
- move from pid/event-based best-effort signaling toward a first-class durable control surface
- consider a command queue or equivalent durable mechanism rather than encoding all intent in the status string
- add stronger end-to-end tests proving pause/cancel timing semantics across long-running orchestrator turns

### Option B — smaller runtime/UI polish
If you want another narrow operator-facing polish slice instead:
- audit whether the remaining cancellation logging should be one line or two
- do another browser pass on settings/search/cancel after a cold restart
- keep any follow-up scoped to one reproduced annoyance at a time

---

## Specific reading order for the next agent

1. `TECH-DEBT.md`
2. this file
3. if taking the runtime-validation slice: `MANUAL_LIVE_VALIDATION.md` and `docs/v2-b4-brain-retrieval-validation.md`
4. if taking the deeper control-plane slice: `internal/chain/control.go`, `cmd/sirtopham/chain.go`, `cmd/sirtopham/cancel.go`, `cmd/yard/chain.go`, and `internal/spawn/spawn_agent.go`

---

## Commands to use

Preferred:
```bash
make test
make build
```

Useful focused commands:
```bash
CGO_ENABLED=1 \
CGO_LDFLAGS="-L$(pwd)/lib/linux_amd64 -llancedb_go -lm -ldl -lpthread" \
LD_LIBRARY_PATH="$(pwd)/lib/linux_amd64" \
go test -tags sqlite_fts5 ./cmd/sirtopham ./cmd/yard ./internal/server ./internal/spawn ./internal/codeintel/searcher
```

---

## Constraints / reminders

- Do not push.
- Keep edits narrow.
- Prefer `make test` / `make build`.
- If a doc disagrees with the repo, trust the repo and patch the doc.
- If you touch the audit-follow-through areas again, update this handoff before stopping.

---

## Bottom line

The repo is in a good continuation state:
- frontend typecheck/build, `make test`, and `make build` are green
- the narrow UI/runtime contract cleanup and matching docs reconciliation are landed in the working tree
- the receipt-path contract is now aligned across shared helpers, runtime code, specs, and shipped role prompts
- running-chain stop requests now use explicit requested states before finalizing to `paused` / `cancelled`
- next best move is exact-setup daily-driver validation unless you explicitly want to keep deepening the control plane
