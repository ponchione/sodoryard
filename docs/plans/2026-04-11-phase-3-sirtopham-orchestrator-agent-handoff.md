# Phase 3 SirTopham Orchestrator — Agent Handoff Companion

Purpose: this is the compact execution companion for `docs/plans/2026-04-11-phase-3-sirtopham-orchestrator-implementation-plan.md`. Hand this file plus the full implementation plan to another coding agent. This companion exists so the receiving agent can start work immediately without re-deriving scope, dependency order, or proof criteria.

## Start here

Read these in order:

1. `AGENTS.md`
2. `NEXT_SESSION_HANDOFF.md`
3. `TECH-DEBT.md`
4. `docs/specs/15-chain-orchestrator.md`
5. `sodor-migration-roadmap.md` (Phase 3 only)
6. `docs/plans/2026-04-11-phase-3-sirtopham-orchestrator-implementation-plan.md`
7. This companion file

Then execute the implementation plan in the dependency order below. Do not reopen architecture unless blocked by code reality.

## One-paragraph objective

Build `cmd/sirtopham` into a working orchestrator binary for roadmap Phase 3. The orchestrator is itself a headless agent session that can use brain tools plus two custom tools: `spawn_agent` and `chain_complete`. It runs ordered chain steps by spawning `tidmouth run` subprocesses, persists chain/step/event state in `.yard/yard.db`, reads engine receipts from the brain, and writes a final orchestrator receipt on completion.

## Implementation-authoritative decisions

These are locked for Phase 3. Do not change them unless explicitly asked.

1. Custom tool name is `spawn_agent`, not `spawn_engine`.
2. Loop termination uses sentinel error `tool.ErrChainComplete`, not a return-value flag.
3. Custom tools are wired through a `BuilderDeps.CustomToolFactory` map.
4. `.yard/yard.db` is the shared SQLite database for Tidmouth and SirTopham state.
5. Roadmap Step 3.2 is implemented as a CLI-driven execution contract, not a new chain-definition YAML format.
6. Phase 3 implements `reindex_before` only. `reindex_after` is deferred.
7. `pause` / `resume` / `cancel` only flip chain status in MVP; `resume` does not auto-restart the old orchestrator session.
8. No Knapford/dashboard work in this phase.
9. No parallel engine execution, chain templates, or chain forking in this phase.

## Do not change

- Do not invent a second config surface for chain definitions.
- Do not add a `--project` flag unless explicitly asked.
- Do not move chain state into a separate `sirtopham.db` file.
- Do not broaden the tool interface just to support `chain_complete`.
- Do not refactor unrelated Tidmouth, web, or Knapford code.
- Do not weaken tests or skip required build/test verification.

## Required execution order

Do the work in this order.

### Checkpoint CP1: shared parsing + state foundation

Task 1: shared receipt parser
- `internal/receipt/types.go`
- `internal/receipt/parser.go`
- `internal/receipt/parser_test.go`
- update `cmd/tidmouth/receipt.go` to delegate validation

Tasks 2-3: schema, sqlc, chain store
- add `chains`, `steps`, `events` tables to `internal/db/schema.sql`
- add `EnsureChainSchema` to `internal/db/init.go`
- call it from `cmd/tidmouth/runtime.go`
- add sqlc query source for chains/steps/events
- implement `internal/chain` store wrappers, limits, and events
- add tests for chain state transitions and limit enforcement

Proof required before moving on:
- `internal/receipt` tests pass
- `cmd/tidmouth` receipt tests still pass
- schema/init/sqlc surfaces are updated coherently
- `internal/chain` tests pass

### Checkpoint CP2: orchestrator runtime primitives

Tasks 4-6: custom tools plumbing, `chain_complete`, `spawn_agent`
- allow `cmd/sirtopham` to register only its custom tools through role builder deps
- keep `cmd/tidmouth` rejecting custom tools by default
- add `tool.ErrChainComplete`
- teach `internal/agent/loop.go` to stop cleanly on that sentinel
- implement `internal/spawn/spawn_agent.go`
- implement subprocess helper and tests
- ensure `spawn_agent` records step rows, chain metrics, and events
- ensure it parses receipts via `internal/receipt`
- wire `reindex_before` support before `tidmouth run`

Proof required before moving on:
- custom tools are only available through `cmd/sirtopham`
- `chain_complete` exits the loop cleanly in tests
- `spawn_agent` can record a successful step and a failure path in tests
- `reindex_before` behavior is covered or explicitly queued for Task 11 verification

### Checkpoint CP3: CLI integration

Tasks 7-9: `cmd/sirtopham` CLI and operator commands
- replace placeholder `cmd/sirtopham/main.go`
- add `runtime.go` and `chain.go`
- implement CLI flags: `--specs`, `--task`, `--chain-id`, `--max-steps`, `--max-duration`, `--token-budget`, `--dry-run`
- add `status`, `logs`, `receipt`, `cancel`, `pause`, `resume`
- ensure pause/cancel/resume are simple status flips with event logging
- add tests for command behavior and store transitions

Proof required before moving on:
- `make build` produces `bin/sirtopham`
- `cmd/sirtopham` tests pass
- status transitions work through the chain store
- no regression is introduced to existing Tidmouth commands

### Checkpoint CP4: completion gate

Tasks 10-13: prompt, reindex verification, smoke test, docs alignment
- refine `agents/orchestrator.md` enough to drive a minimal smoke chain
- verify `reindex_before` wiring and add test if missing
- run the real smoke chain from the main plan
- align `docs/specs/15-chain-orchestrator.md` with implementation

Proof required for phase completion:
- smoke chain exits 0
- chain rows/step rows/events appear in `.yard/yard.db`
- engine receipt exists in the brain
- orchestrator receipt exists in the brain
- spec 15 matches shipped implementation names and DB path

## Fast command checklist

Use these as the minimum command proofs at each stage.

CP1:
- `make test 2>&1 | grep -E "internal/receipt|internal/chain|cmd/tidmouth|FAIL" | head -40`

CP2:
- `make test 2>&1 | grep -E "internal/role|internal/agent|internal/spawn|FAIL" | head -40`

CP3:
- `make build`
- `make test 2>&1 | grep -E "cmd/sirtopham|FAIL" | head -40`

CP4:
- `./bin/sirtopham --help`
- run the smoke chain from the implementation plan
- `./bin/sirtopham status <chain-id>`
- `./bin/sirtopham logs <chain-id>`
- `./bin/sirtopham receipt <chain-id>`

## Expected file surface

New packages/files expected by the end of Phase 3:
- `internal/receipt/*`
- `internal/db/query/chains.sql`
- generated `internal/db/chains.sql.go`
- `internal/chain/*`
- `internal/spawn/*`
- `cmd/sirtopham/main.go`
- `cmd/sirtopham/runtime.go`
- `cmd/sirtopham/chain.go`
- `cmd/sirtopham/status.go`
- `cmd/sirtopham/logs.go`
- `cmd/sirtopham/receipt.go`
- `cmd/sirtopham/cancel.go`
- `cmd/sirtopham/pause_resume.go`

Modified files expected by the end of Phase 3:
- `internal/db/schema.sql`
- `internal/db/init.go`
- `cmd/tidmouth/runtime.go`
- `cmd/tidmouth/receipt.go`
- `internal/role/builder.go`
- `internal/role/builder_test.go`
- `internal/tool/registry.go` or new tool error file
- `internal/agent/loop.go`
- `internal/agent/loop_test.go`
- `agents/orchestrator.md`
- `docs/specs/15-chain-orchestrator.md`

## If you get stuck

Stop only if one of these is true:
- a referenced file/package no longer exists and there is no obvious replacement
- the implementation would require changing one of the locked Phase 3 decisions above
- live runtime behavior contradicts the plan in a way that narrow edits cannot fix

Otherwise, adapt locally and continue.

When handing off midstream, update `NEXT_SESSION_HANDOFF.md` with:
- current checkpoint
- exact files touched
- exact failing command/test
- whether the failure is code, schema, or environment
- the next smallest unresolved step

## Final acceptance summary

Phase 3 is done when all of these are true:
- `bin/sirtopham` exists and exposes the required subcommands
- `spawn_agent` and `chain_complete` work inside the orchestrator loop
- chain/step/event state persists in `.yard/yard.db`
- pause/cancel/resume status flips are implemented and enforced at step boundaries
- a smoke chain completes successfully end-to-end
- `docs/specs/15-chain-orchestrator.md` is aligned with what shipped
