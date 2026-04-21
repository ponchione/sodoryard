# Next Worker Agent Brief — Chain Observability Follow-Through

Read this first
1. `AGENTS.md`
2. `README.md`
3. `docs/plans/2026-04-21-chain-observability-followthrough-checklist.md`
4. If needed for historical context: `docs/plans/2026-04-13-sodoryard-stability-closeout-plan.md`

Objective
- Follow through on the issues exposed by the 2026-04-21 live chain run: incorrect Ctrl-C semantics, bogus slash-path extraction, stale historical receipt injection, and overly noisy default log presentation.

What is already known
- Child `step_output` streaming is working; do not rip it out.
- The highest-priority bug is Ctrl-C during `yard chain start --watch` leading to `agent loop: turn cancelled: context canceled` and likely failed-execution cleanup.
- The context analyzer is incorrectly accepting slash prose like `add/update` and `receipts/state` as explicit file refs.
- Planner task seeding included dead historical receipt paths and wasted retrieval/tool work.
- The user wants noisy bootstrap output preserved as a debug option, but normal use should be quieter by default.

Locked decisions / anti-goals
- Preserve the event-stream model and `step_output` event capture.
- Prefer CLI render-time filtering over changing what gets stored.
- Do not broaden into a chain-architecture rewrite.
- Do not push.
- Do not remove high-fidelity output entirely; make it opt-in debug/verbosity behavior.

Recommended implementation order
1. Fix Ctrl-C / detach semantics in `cmd/yard/chain.go` and `cmd/sirtopham/chain.go` with tests first.
2. Tighten `internal/context/analyzer.go` slash-path heuristics with tests first.
3. Trace and fix the source of dead historical receipt injection.
4. Add `--verbosity=normal|debug` (or equivalent if a narrower existing pattern is better) for watch/log rendering.
5. Audit the suspicious `reindex_before` behavior only after the above are stable.

Proof required before moving on
- After Phase 1: tests prove Ctrl-C detaches without failing the chain.
- After Phase 2: tests prove `add/update` and `receipts/state` no longer become explicit files, while valid repo paths still do.
- After Phase 3: prompt/task construction no longer injects dead receipt paths.
- After Phase 4: normal mode is quieter; debug mode shows full child `step_output`.

Minimal command checklist
- Focused tests while iterating:
  - `go test -tags sqlite_fts5 ./cmd/yard ./cmd/sirtopham -count=1`
  - `go test -tags sqlite_fts5 ./internal/context -count=1`
- Final:
  - `make test`
  - `make build`

Expected file surface
- `cmd/yard/chain.go`
- `cmd/yard/chain_test.go`
- `cmd/sirtopham/chain.go`
- `cmd/sirtopham/chain_test.go`
- `cmd/sirtopham/logs.go`
- `internal/context/analyzer.go`
- `internal/context/analyzer_test.go`
- Possibly one prompt/task-building location once dead receipt injection source is found

Stop / escalate criteria
- If the dead historical receipt injection source is not obvious after targeted search, stop and document the precise candidate locations rather than guessing.
- If render-time filtering proves too brittle for normal-vs-debug output, document why before changing event payload/schema.
- If fixing detach semantics appears to require a broader control-plane redesign, stop and explain the narrow blocker.

Deliverable expectation
- Land the highest-value narrow slices with tests and updated operator docs if CLI flags/behavior change.

Copy-paste kickoff prompt for the next worker agent
- Investigate and implement the chain observability follow-through checklist in `docs/plans/2026-04-21-chain-observability-followthrough-checklist.md`. Work in narrow TDD slices, starting with Ctrl-C/detach semantics in `cmd/yard/chain.go` and `cmd/sirtopham/chain.go`. Preserve `step_output` event capture. Make default operator output quieter only at render time, with an explicit debug/verbosity path that still exposes full child output. Keep edits narrow, run focused tests while iterating, and finish with `make test` and `make build` if code changes land.
