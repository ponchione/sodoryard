# Chain Observability Follow-Through Checklist

> For Hermes / next worker agent: use this as the execution checklist for the chain observability/control follow-through surfaced by the 2026-04-21 live run on `my-website`. Work in narrow slices with strict TDD. Keep edits narrow. Prefer `make test` and `make build` for final verification. Do not push.

Goal
- Fix the real correctness and operator-UX issues exposed by the fresh chain run without discarding the value of high-fidelity debug output.

Architecture / planning stance
- Preserve the existing event-stream model and child `step_output` event capture.
- Fix behavior primarily at control-flow and CLI rendering layers before changing storage/event contracts.
- Daily-use mode should be quieter by default.
- Full raw child output must remain available via an explicit debug/verbosity switch.

Tech stack / constraints
- Go backend with sqlite_fts5 and LanceDB CGO requirements
- CLI surfaces: `cmd/yard`, `cmd/sirtopham`, `cmd/tidmouth`
- Context analyzer under `internal/context/`
- Chain control/event logic under `internal/chain/`
- Prefer verification with:
  - `make test`
  - `make build`
- If using focused Go commands, include `-tags sqlite_fts5` and the repo's CGO/LanceDB env as needed.

Non-goals
- No broad chain-architecture rewrite
- No event schema churn unless rendering-only filtering proves insufficient
- No automatic reindexing redesign unless the specific `reindex_before` audit proves a real bug
- No push

Source evidence to keep in mind
- Live run showed child `step_output` works, so observability plumbing is fundamentally alive.
- Ctrl-C during `yard chain start --watch` ended with `agent loop: turn cancelled: context canceled`, which is the highest-priority correctness bug.
- Context analyzer accepted bogus slash prose (`receipts/state`, `add/update`) as explicit file paths.
- Planner step was seeded with dead historical receipt paths and wasted retrieval/tool work on them.
- Child bootstrap/provider chatter is useful for debugging but too noisy for normal operator use.

---

## Phase 0 — Fresh-session baseline

Objective
- Confirm current tree/repo state before taking the first slice.

Files
- Read: `AGENTS.md`
- Read: `README.md` (especially current status / next session)
- Read: this checklist
- Read: `docs/plans/2026-04-21-chain-observability-next-worker-brief.md`

Steps
1. Run `git status --short --branch`.
2. Re-read the two checklist/handoff docs before editing.
3. Inspect whether any partially landed work already touches:
   - `cmd/yard/chain.go`
   - `cmd/sirtopham/chain.go`
   - `internal/context/analyzer.go`
4. If the tree already contains related WIP, do not overwrite it blindly.

Verification
- Current tree understood
- No unrelated churn introduced

---

## Phase 1 — Fix Ctrl-C / detach semantics first

Why this is first
- This is the only issue from the observed run that can directly turn a normal operator action into an incorrect failed execution.

Objective
- Make Ctrl-C during `yard chain start` / `yard chain resume` / `sirtopham chain` detach from live watch output instead of implicitly failing the run.

Current known evidence
- `cmd/yard/chain.go` wraps the run context with `signal.NotifyContext(..., os.Interrupt)` and passes that same context into `RunTurn(...)`.
- `cmd/sirtopham/chain.go` mirrors the same pattern.
- `handleYardChainRunInterruption(...)` / `handleChainRunInterruption(...)` only convert interruption into success when the persisted chain state has already become `paused` or `cancelled`.
- Otherwise the deferred errored-execution closure path can mark the execution failed.

Files
- Modify: `cmd/yard/chain.go`
- Modify: `cmd/sirtopham/chain.go`
- Modify: `cmd/yard/chain_test.go`
- Modify: `cmd/sirtopham/chain_test.go`
- Inspect: `internal/chain/control.go`

Checklist
1. Add failing tests first for both CLIs proving that Ctrl-C during watch does not close the active execution as failed.
2. Drive the real `yardRunChain(...)` / `runChain(...)` path with a blocking fake loop where the command is interrupted while the chain is still running.
3. Refactor signal handling so the first interrupt detaches local watch/output instead of canceling the underlying orchestrator turn context.
4. Ensure the CLI exits cleanly with the chain ID still usable for reattach.
5. Keep explicit `chain cancel` / `pause` semantics unchanged.
6. Do not silently reinterpret detach as cancel.
7. Mirror the fix in both `yard` and `sirtopham`.

Recommended acceptance language
- Something like: `detached from live output; chain <id> continues running`
- Avoid raw `agent loop: turn cancelled` surfacing for simple detach.

Verification commands
- Focused:
  - `go test -tags sqlite_fts5 ./cmd/yard ./cmd/sirtopham -run 'Test.*(Interrupt|CtrlC|Detach|RunChain).*' -count=1`
- Broader:
  - `go test -tags sqlite_fts5 ./internal/chain ./cmd/yard ./cmd/sirtopham -count=1`
- Final for this slice:
  - `make test`

Done means
- Ctrl-C during live watch no longer produces a failed chain execution by default.
- The chain remains runnable/reattachable after detaching.
- Explicit cancel still behaves as cancel.

Stop rule
- Do not add a second-interrupt-to-cancel UX in this slice unless required to complete the first-interrupt detach fix cleanly.

---

## Phase 2 — Tighten slash-path extraction in the context analyzer

Why this is next
- The observed bogus file refs (`receipts/state`, `add/update`) are real quality regressions that pollute retrieval and logs.

Objective
- Reject slash-delimited prose as explicit file refs while preserving legitimate repo-relative file paths.

Current known evidence
- `internal/context/analyzer.go`:
  - `extractFileReferences(...)`
  - `normalizePathToken(...)`
  - current logic accepts nearly any slash-containing token that avoids a few rejection paths.
- Existing tests already cover related cases like:
  - rejecting `search/title/runtime`
  - rejecting vault-rooted note paths
  - keeping `internal/server/websocket.go`

Files
- Modify: `internal/context/analyzer.go`
- Modify: `internal/context/analyzer_test.go`
- Inspect: `internal/context/momentum.go`
- Inspect optionally: `internal/context/retrieval.go`

Checklist
1. Add failing tests first for the exact bad shapes seen in the live run:
   - `add/update`
   - `receipts/state`
2. Add keep-tests for clearly valid paths:
   - `internal/server/websocket.go`
   - `docs/specs/15-chain-orchestrator.md`
   - `receipts/planner/<id>-step-001.md`
3. Refactor path heuristics into clearer helpers instead of one broad slash acceptance path.
4. Require stronger evidence for slash tokens with no extension and no explicit relative prefix.
5. Prefer repo-anchor heuristics (`internal/`, `cmd/`, `web/`, `docs/`, `agents/`, etc.) over broad slash acceptance.
6. Preserve rejection-signal visibility with a concrete reason like `low_confidence_slash_path` if useful.
7. Verify `momentum.go` behavior still works with the tightened normalization path.

Verification commands
- Focused:
  - `go test -tags sqlite_fts5 ./internal/context -run 'TestRuleBasedAnalyzer.*(Slash|Vault|Path)' -count=1`
- Broader:
  - `go test -tags sqlite_fts5 ./internal/context -count=1`

Done means
- Bogus prose slash tokens no longer become explicit files.
- Real repo file references still do.
- Analyzer signals remain interpretable.

Stop rule
- Do not over-tighten so far that ordinary anchored repo paths regress.

---

## Phase 3 — Stop injecting nonexistent historical receipt paths

Why this matters
- The planner was explicitly told to read dead receipt paths and then wasted cycles failing to retrieve them.

Objective
- Only seed prior receipt references into step tasks when they actually exist.

Unknown to resolve first
- Find the exact code or prompt source assembling the `Relevant brain context to read first:` block and the stale receipt references.

Likely files to inspect first
- `cmd/yard/chain.go`
- `cmd/sirtopham/chain.go`
- `internal/spawn/spawn_agent.go`
- `agents/` prompt assets
- any orchestrator/runtime prompt-builder helpers that reference prior receipts or "start from scratch" text

Checklist
1. Search for the exact prompt fragments seen in the captured run, especially:
   - `Relevant brain context to read first:`
   - `No existing receipts/state were found for this chain`
   - stale `receipts/planner/`, `receipts/coder/`, `receipts/correctness-auditor/` path patterns
2. Add a failing test around the prompt/task construction path once the source is found.
3. Implement existence-aware filtering:
   - include only receipt paths that actually exist
   - omit the whole block if none exist
4. Replace misleading fallback wording with a clean statement when history is unavailable.
5. Re-run the prompt-building test and, if practical, one live dry run that prints/logs the planner step task.

Verification commands
- Focused: exact package test once source is identified
- Broader:
  - `go test -tags sqlite_fts5 ./cmd/yard ./cmd/sirtopham ./internal/spawn -count=1`

Done means
- Fresh chains no longer instruct agents to read nonexistent historical receipts.
- Retrieval warnings from injected dead receipt paths disappear.

Stop rule
- Do not redesign the overall historical-context strategy; just make it existence-aware and truthful.

---

## Phase 4 — Add normal vs debug log presentation

Why this is fourth
- The user explicitly wants the noisy output preserved as a debug-capable mode, but normal usage should be less noisy.

Objective
- Keep full event capture in storage, but render a quieter operator-focused view by default and expose a debug/verbose mode for full fidelity.

Design stance
- Filter at CLI render/presentation time first, not at event emission/storage time.
- Preserve `step_output` events in the DB unchanged.

Recommended operator contract
- Default watch/log-follow mode: `normal`
- Optional explicit debug mode: full raw child `step_output`
- Use one consistent flag surface across `yard` and `sirtopham`

Recommended flag
- `--verbosity=normal|debug`

Why this flag
- Extensible
- Better than a one-off boolean
- Leaves room for `quiet` later if desired

Files
- Modify: `cmd/yard/chain.go`
- Modify: `cmd/sirtopham/logs.go`
- Modify maybe: `cmd/sirtopham/chain.go`
- Modify: `cmd/yard/chain_test.go`
- Modify: `cmd/sirtopham/chain_test.go`
- Inspect: `internal/spawn/spawn_agent.go`

Checklist
1. Add render options / verbosity plumbing for both CLIs.
2. Keep all non-`step_output` events visible in normal mode.
3. For `step_output`, suppress only clearly low-value bootstrap/status chatter in normal mode.
4. Initial likely-suppress list in normal mode:
   - `provider registered`
   - `registered provider`
   - `provider failed Ping() startup validation`
   - `brain backend: MCP (in-process)`
   - `status: waiting_for_llm`
   - `status: executing_tools`
   - maybe `status: assembling_context` if still too noisy
5. Never suppress warnings/errors or step failure output in normal mode.
6. In `debug` mode, render all `step_output` exactly as stored.
7. Wire the same verbosity option into:
   - `yard chain start --watch`
   - `yard chain resume --watch`
   - `yard chain logs --follow`
   - `sirtopham chain`
   - `sirtopham logs --follow` if present / equivalent path
8. Add tests locking normal-vs-debug behavior.

Verification commands
- Focused:
  - `go test -tags sqlite_fts5 ./cmd/yard ./cmd/sirtopham -run 'TestFormatChainEvent|Test.*Verbosity|Test.*StepOutput' -count=1`
- Broader:
  - `go test -tags sqlite_fts5 ./cmd/yard ./cmd/sirtopham -count=1`
- Final for this slice:
  - `make test`

Done means
- Normal mode is materially quieter and still useful.
- Debug mode restores full child output.
- Stored events remain complete.

Stop rule
- Do not introduce new event schema fields unless render-only filtering proves insufficient.

---

## Phase 5 — Audit unexpected `reindex_before` behavior

Why this is last
- It is suspicious from the captured run, but not yet proven to be a real bug.

Objective
- Determine whether `reindex_started` during the planner step was correct, prompt drift, or an actual orchestration bug.

Files
- Inspect: `internal/spawn/spawn_agent.go`
- Search all prompt/task/tool-call sites that set `reindex_before`
- Inspect any orchestrator prompt assets or helpers that mention reindexing behavior

Checklist
1. Search all callers / prompt examples / tool invocations that mention `reindex_before`.
2. Determine whether the planner step was explicitly spawned with `reindex_before=true` despite instructions to skip reindexing.
3. If confirmed, add a narrow regression test and fix only that decision path.
4. If not confirmed, document findings and leave behavior unchanged.

Verification commands
- Focused package tests after the source is identified
- Final if a code fix lands:
  - `make test`

Done means
- Either the surprise reindex is fixed, or it is explicitly ruled out as a bug.

---

## Final verification package

After the intended slices land, run:
1. `make test`
2. `make build`
3. Manual/operator smoke:
   - start a chain with watch enabled
   - press Ctrl-C once and confirm detach behavior
   - reattach with `yard chain logs --follow <chain-id>`
   - compare normal vs debug verbosity modes
   - confirm no bogus explicit-file warnings for prose slash tokens in a representative prompt

Optional focused live smoke
- Use the same `my-website` style task from the captured run if practical.
- Cancel explicitly via `yard chain cancel <id>` rather than Ctrl-C when testing true cancellation.

Acceptance summary
- Ctrl-C detach semantics are correct
- explicit cancel semantics remain correct
- bogus slash-file extraction is fixed
- dead historical receipt injection is fixed
- normal mode is quieter by default
- debug mode preserves full fidelity

Suggested execution order for the next worker
1. Phase 1 only
2. Phase 2 only
3. Phase 3 only
4. Phase 4 only
5. Phase 5 only if still needed

If time is limited
- Land Phase 1 first, then Phase 2.
- Phase 4 is the best UX payoff after those.
