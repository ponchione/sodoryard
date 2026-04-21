# Chain Detach / Background Execution Protocol Slice

> For Hermes / next worker agent: this is a narrow design-first slice for the Phase 1 blocker discovered during the 2026-04-21 chain observability follow-through. Do not treat this as permission for a broad chain-architecture rewrite. Preserve the current event model and stored `step_output` behavior. Do not push.

Goal
- Define and then implement the smallest control-plane extension that makes first-interrupt detach semantics actually possible for `yard chain start`, `yard chain resume`, and `sirtopham chain`.

Why this slice exists
- The earlier Phase 1 investigation found a hard blocker: the current CLI process runs the orchestrator turn in-process via `agent.NewAgentLoop(...).RunTurn(...)`.
- Because the orchestrator is not already backgrounded, a true “Ctrl-C detaches local watch while the chain continues running” behavior cannot be achieved by signal wiring alone.
- Exiting the CLI process exits the orchestrator itself.

Current grounded evidence
- `cmd/yard/chain.go` and `cmd/sirtopham/chain.go` call the orchestrator loop directly in-process.
- Both commands currently register active execution using the current process PID as `orchestrator_pid`.
- `internal/chain/control.go` and the control/status helpers already model `running`, `pause_requested`, `cancel_requested`, `paused`, and `cancelled` cleanly.
- `cmd/yard/chain.go` and `cmd/sirtopham/logs.go` already support live follow rendering over stored events.
- `docs/specs/15-chain-orchestrator.md` already describes a future-facing command-queue/background-orchestrator direction for resume, but the current implementation is simpler and in-process.

Non-goals
- No general job supervisor.
- No daemon/service manager.
- No websocket/UI redesign.
- No change to stored event payloads.
- No second-interrupt-to-cancel UX in this slice.
- No speculative command queue/table redesign unless the narrow subprocess protocol proves impossible.

Design stance
- Make detach possible by moving only the orchestrator execution lifetime out of the foreground CLI process.
- Keep the existing chain store, events, `active_execution`, and PID-based pause/cancel signaling model.
- Prefer a self-reexec/background-subprocess protocol over a brand-new persistent daemon.
- Keep operator-facing behavior simple:
  - `yard chain start` / `resume` print the chain id immediately
  - they spawn a background orchestrator process for the actual run
  - with `--watch=true`, the foreground process only follows logs
  - first Ctrl-C stops watching and exits cleanly
  - explicit `yard chain cancel <id>` / `pause <id>` still control the real background orchestrator

---

## Proposed narrow protocol

### Operator path
1. Foreground CLI validates config and creates/resumes the chain row exactly as today.
2. Foreground CLI launches a background subprocess of the same binary in a hidden/internal execution mode.
3. Background subprocess becomes the real orchestrator runner, registers `active_execution` with its own PID, and runs the chain loop.
4. Foreground CLI optionally tails events with existing follow logic.
5. Ctrl-C in the foreground CLI cancels only the local watch context; it does not signal the background orchestrator.
6. Future `pause` / `cancel` commands continue using the stored `orchestrator_pid` from the background process.

### Internal protocol surface
Add an internal-only execution mode, for example one of:
- hidden command: `yard chain _run-background`
- hidden command: `sirtopham _run-chain-background`
- hidden flags on existing commands, e.g. `--background-child`

Preferred shape
- Hidden subcommand, not a public flag, so operator help stays clean and child entry is explicit.
- Child receives only already-resolved execution inputs:
  - `chain-id`
  - `config path`
  - whether this is a resumed run
  - operator-selected verbosity/watch need not propagate because the child does not watch
- Child performs the actual `RunTurn(...)` path and finalization.

### PID / active execution contract
Foreground process:
- must not register `active_execution`
- must not write `orchestrator_pid` as itself
- may create/start the chain row if needed

Background child:
- is the only process that calls `registerActiveChainExecution(...)`
- writes its own PID as `orchestrator_pid`
- owns errored-execution cleanup and terminal closure

This preserves the existing pause/cancel signaling assumptions.

### Detach semantics contract
On first Ctrl-C while foreground watch is attached:
- print a message like `detached from live output; chain <id> continues running`
- exit zero
- do not mutate chain status
- do not call cancel/pause helpers
- do not close active execution

### Failure-handshake contract
The foreground CLI should not claim success unless the child actually starts.

Minimum viable handshake:
- child writes/records `active_execution` promptly after launch
- foreground waits briefly for one of:
  - `active_execution` event with a new PID/execution_id
  - immediate child exit/failure
  - timeout
- on success, foreground enters watch mode or exits cleanly if `--watch=false`
- on timeout, foreground returns a startup failure explaining the child never registered execution

Do not overbuild this into a general IPC channel unless the simple event-store handshake proves insufficient.

---

## Recommended implementation slice

### Task 1: codify the protocol in tests first
Objective
- Add regression tests that prove the parent/child split semantics before changing behavior.

Files
- Modify: `cmd/yard/chain_test.go`
- Modify: `cmd/sirtopham/chain_test.go`
- Inspect: current watch tests and active-execution tests

Required test coverage
1. Foreground start path does not register itself as `active_execution` when launching a background child.
2. Child/background path is the code path that registers `active_execution` with its own PID.
3. Ctrl-C during foreground watch exits cleanly without marking the chain failed when background execution is active.
4. `--watch=false` returns after successful child-start handshake.
5. Explicit cancel still targets the child PID and closes the active execution normally.

### Task 2: extract shared orchestrator execution core
Objective
- Separate “prepare/launch child” from “run orchestrator turn as the real worker”.

Files
- Modify: `cmd/yard/chain.go`
- Modify: `cmd/sirtopham/chain.go`

Required refactor boundary
Introduce a helper boundary equivalent to:
- `prepare chain row / resume state`
- `launch background child`
- `run chain execution as child`

The child execution helper should own:
- runtime build
- registry build
- conversation creation
- active execution registration
- turn execution
- requested-status finalization
- errored-execution cleanup

The foreground helper should own:
- config load/validation
- chain ID selection
- resume gating
- child launch
- handshake wait
- optional event follow

### Task 3: implement hidden child command/protocol
Objective
- Add a hidden internal entrypoint for the background child.

Files
- Modify: `cmd/yard/chain.go` and/or `cmd/yard/main.go`
- Modify: `cmd/sirtopham/chain.go` and/or `cmd/sirtopham/main.go`

Requirements
- Hidden from normal help output.
- Accepts enough resolved args to run the orchestrator without re-entering the parent launch path.
- Reuses the same execution core in both binaries.
- Prevents recursive child spawning.

### Task 4: implement startup handshake
Objective
- Ensure the parent does not detach blindly before the child actually owns execution.

Files
- Modify: `cmd/yard/chain.go`
- Modify: `cmd/sirtopham/chain.go`
- Inspect: `internal/chain/control.go`

Preferred handshake
- parent polls chain events/store for a freshly registered active execution not equal to the parent PID
- use a short bounded timeout
- surface clean startup errors if child exits early or never registers

### Task 5: rewire watch interruption semantics narrowly
Objective
- After backgrounding exists, make Ctrl-C cancel only local watch/follow.

Files
- Modify: `cmd/yard/chain.go`
- Modify: `cmd/sirtopham/chain.go`
- Modify: tests in both packages

Requirements
- foreground watch uses an interrupt-bound context
- child run context does not inherit that watch cancellation
- message on detach is explicit and operator-friendly
- no raw `agent loop: turn cancelled` on simple detach

---

## File surface expected for the real implementation
- `cmd/yard/chain.go`
- `cmd/yard/main.go` if a hidden child command is needed there
- `cmd/yard/chain_test.go`
- `cmd/sirtopham/chain.go`
- `cmd/sirtopham/main.go` if a hidden child command is needed there
- `cmd/sirtopham/chain_test.go`
- Possibly a tiny shared helper under `internal/runtime/` or `internal/chain/` only if needed to avoid duplication

Avoid touching unrelated packages unless the child-launch boundary makes it unavoidable.

---

## Open questions to resolve during implementation
1. Binary choice
- Should `yard chain start` spawn `yard` as the child, or spawn `sirtopham` directly?
- Preferred default: each binary should spawn itself into hidden child mode so behavior remains symmetric and install/runtime expectations stay simple.

2. Output/logging inheritance
- Child stdout/stderr should not spam the parent terminal directly.
- Prefer event-log observability over inherited stdio in the child path.

3. Resume semantics
- Resumed runs should use the same background-child launch path.
- The foreground resume command should remain a launcher + watcher, not the real worker.

4. Startup race behavior
- Decide whether handshake success means “child process exists” or “child has written active_execution”.
- Preferred: require `active_execution` registration, not mere process existence.

---

## Acceptance criteria for this slice
1. `yard chain start --watch` launches a background orchestrator child and follows logs.
2. Pressing Ctrl-C once while watching exits with a detach message and leaves the chain `running`.
3. `yard chain status` / `yard chain logs --follow <id>` can reattach after detach.
4. `yard chain cancel <id>` still terminates the real child PID.
5. `yard chain resume --watch` uses the same launcher/child protocol.
6. `sirtopham chain` matches the same semantics.
7. No failed-execution closure occurs from simple foreground detach.
8. Stored events and `step_output` payloads remain unchanged.
9. `make test` and `make build` pass.

---

## Stop rules
- If a self-reexec background child cannot be made reliable without introducing a general daemon/service layer, stop and document that exact blocker instead of widening scope silently.
- If the startup handshake cannot be made trustworthy via the existing chain store/events, document the minimum extra protocol needed before adding new schema.
- Do not redesign pause/cancel semantics in this slice.

---

## Suggested next-worker kickoff prompt
- Implement the narrow background-execution protocol in `docs/plans/2026-04-21-chain-detach-background-execution-protocol-slice.md`. Work test-first. Preserve the existing chain store/events model and make the background child, not the foreground CLI, own `active_execution` registration and orchestrator PID. The foreground command should become launcher + optional watcher only; first Ctrl-C must detach local watch without canceling the chain. Keep edits narrow and finish with `make test` and `make build`.
