# Next session handoff

Date: 2026-04-01
Repo: /home/gernsback/source/sirtopham
Branch: main
State: working tree dirty only for NEXT_SESSION_HANDOFF.md, ahead of origin/main by 4 commits

What is actually complete from the Claude Code / sirtopham handoff

This is not a full completion of the entire Claude Code retrofit plan.

What is substantially complete is the highest-priority tool-output slice plus a few adjacent correctness/documentation follow-ups:

1. Aggregate tool-result budgeting in the real agent loop
- Fresh tool results are budgeted in aggregate before they are appended into the next model-visible request.
- Budgeting is deterministic:
  - larger results are reduced first
  - `file_read` is deprioritized for replacement when another tool can absorb the cut
- Main files:
  - `internal/agent/loop.go`
  - `internal/agent/toolresult_budget.go`
  - `internal/agent/loop_compression_test.go`

2. Persisted oversized tool outputs with structured refs
- Oversized non-`file_read` tool outputs can be persisted to disk and replaced with a structured reference + preview.
- Current model-visible format includes:
  - `[persisted_tool_result]`
  - `path=...`
  - `tool=...`
  - `tool_use_id=...`
  - `preview=`
- Tiny-budget fallback preserves the path when possible.
- Main files:
  - `internal/agent/toolresult_budget.go`
  - `internal/agent/toolresult_store.go`
  - `internal/agent/toolresult_budget_test.go`
  - `internal/agent/loop_compression_test.go`

3. Configurable persisted artifact storage root
- Persisted tool-result artifacts are no longer tempdir-only in practice; the root is configurable.
- Config field added:
  - `agent.tool_result_store_root`
- Wired through config -> serve -> agent loop.
- Main files:
  - `internal/config/config.go`
  - `internal/config/config_test.go`
  - `cmd/sirtopham/serve.go`
  - `internal/agent/loop.go`
  - `internal/agent/toolresult_store_config_test.go`

4. Observability for aggregate budgeting
- Aggregate budget helper now returns a report struct.
- Agent loop emits debug logging when fresh tool results are replaced.
- Current report includes:
  - original chars
  - final chars
  - max chars
  - replaced result count
  - persisted result count
  - inline-shrunk result count
  - chars saved
- Main files:
  - `internal/agent/toolresult_budget.go`
  - `internal/agent/toolresult_budget_test.go`
  - `internal/agent/loop.go`

5. Tool execution recording correctness fix
- The normal loop path now correctly passes execution metadata through the tool adapter so `tool_executions` rows are not skipped.
- Main files:
  - `internal/tool/execution_context.go`
  - `internal/tool/adapter.go`
  - `internal/agent/loop.go`
  - `internal/tool/adapter_persistence_test.go`
  - `internal/agent/loop_test.go`

6. API/settings visibility for useful runtime config
- `/api/config` now exposes:
  - `agent.tool_output_max_tokens`
  - `agent.tool_result_store_root`
- Settings page shows those values read-only.
- Main files:
  - `internal/server/configapi.go`
  - `internal/server/configapi_test.go`
  - `web/src/types/metrics.ts`
  - `web/src/pages/settings.tsx`

7. Persistence atomicity contract clarified
- Current contract is now explicitly documented:
  - `PersistIteration` is atomic for `messages`
  - `tool_executions` and `sub_calls` are best-effort and non-atomic relative to message persistence
  - cancellation cleanup still deletes all three together for in-flight iterations
- Main files:
  - `internal/conversation/history.go`
  - `docs/specs/08-data-model.md`
  - `TECH-DEBT.md`

8. File-edit hardening is now substantially complete
- `file_edit` now enforces a real full-read-before-edit invariant.
- Partial `file_read` results do not satisfy edit preconditions.
- Stale-write detection now happens both before edit planning and immediately before write.
- Successful edits clear the saved read snapshot so a fresh read is required before another edit.
- Recovery-oriented error payloads are now much stronger:
  - `invalid_create_via_edit`
  - `not_read_first`
  - `stale_write`
  - `zero_match`
  - `multiple_matches`
- Zero-match failures include a preview of current file content.
- Multiple-match failures include candidate lines plus candidate snippets, including multiline snippets when `old_str` spans lines.
- Match-analysis/helper logic is now separated into its own file with focused unit tests.
- Main files:
  - `internal/tool/file_read.go`
  - `internal/tool/file_read_state.go`
  - `internal/tool/file_edit.go`
  - `internal/tool/file_edit_analysis.go`
  - `internal/tool/file_edit_test.go`
  - `internal/tool/file_edit_analysis_test.go`
  - `internal/tool/register.go`

What is NOT complete from the Claude Code / sirtopham handoff

The overall retrofit handoff is still incomplete. The following major areas remain mostly unimplemented:

1. Cancellation cleanup / transcript invariants
- Existing cancellation cleanup is present and tested.
- The richer Claude-Code-style cleanup model is not implemented:
  - no tombstones/synthesized terminal records for partial state
  - no explicit interrupt-vs-cancel cleanup semantics
  - no `InflightTurn` / `CleanupPlan` subsystem wired into the loop
- Relevant handoff stub:
  - `sirtopham-handoff/stubs/turnstate/turnstate.go`

2. Prompt-cache latching
- No explicit prompt block / cache-latch subsystem from the handoff is implemented.
- Stable-vs-dynamic prompt bytes are not modeled as their own seam yet.
- Relevant handoff stub:
  - `sirtopham-handoff/stubs/promptcache/promptcache.go`

3. Better token-budget accounting
- No `BudgetTracker`-style reserve + estimate + reconcile implementation from the handoff is wired into requests.
- Current system still does not fully embody the handoff’s token-budget plan.
- Relevant handoff stub:
  - `sirtopham-handoff/stubs/tokenbudget/tokenbudget.go`

4. A few tool-output subtleties are only partially done
- There is no dedicated `ToolOutputManager` package boundary yet; the logic currently lives directly in agent-loop helper code.
- No explicit shell/build/test tail-preserving formatter strategy is implemented as a first-class subsystem.
- No formal memoization-by-tool-call-ID subsystem exists beyond the deterministic current-pass behavior.

5. File-write freshness policy is still unresolved
- `file_write` remains the explicit overwrite/create escape hatch.
- The stronger read-state/stale-write contract has been implemented for `file_edit`, not for `file_write`.
- If future correctness work focuses on broader mutation safety, decide whether `file_write` should stay intentionally unconstrained or gain a related freshness policy.

Bottom-line assessment

The right way to describe status now is:
- the top recommendation from the Claude Code handoff is implemented enough to be useful
- the entire Claude Code / sirtopham handoff is NOT complete
- treat the current state as “phase 1 complete”, not “handoff complete”

What is worth double-checking next session

Only a few things feel worth active verification before doing more implementation:

1. Real end-to-end oversized-output behavior
- Verify with a real conversation / real large tool output that:
  - output is persisted under the configured artifact root
  - the next model-visible request gets the structured persisted ref
  - transcript/UI behavior is still sane

2. Budget/config semantics
- Double-check that runtime config exposure is not confusing between:
  - per-tool output cap (`tool_output_max_tokens`)
  - aggregate next-message fresh-tool-result budgeting (`MaxToolResultsPerMessageChars` in the agent loop)
- If this feels confusing, either document it better or expose the aggregate cap explicitly too.

3. Artifact lifecycle / cleanup policy
- Persisted tool results now accumulate under a configurable root.
- Decide whether this is acceptable as-is or whether old artifacts need cleanup/retention behavior.

4. Cancellation edge cases under stress
- If cancellation correctness matters next, add adversarial tests around:
  - cancel during tool execution
  - cancel after assistant tool-call start but before tool result persistence
  - crash/restart between message commit and analytics write

Recommended next implementation slice

Unless priorities changed, the best next Claude-handoff-aligned slice is:
- cancellation cleanup / transcript invariants

Why this should be next
- File-edit hardening is now in good shape and no longer looks like the highest-risk deterministic gap.
- Cancellation cleanup is the next major correctness area from the handoff that can still cause confusing durable state.
- It is a better next investment than speculative prompt-cache or broader token-budget architecture.

Suggested exact next-session plan

1. Read these first
- `sirtopham-handoff/03-implementation-plan.md`
- `sirtopham-handoff/01-priority-recommendations.md`
- `sirtopham-handoff/stubs/turnstate/turnstate.go`
- current cancellation/cleanup paths in the agent loop and conversation persistence
- current tests covering cancellation and iteration cleanup behavior

2. Confirm current cancellation-path realities
- What gets persisted before cancellation cleanup runs
- Which partial assistant/tool states can currently survive interruption
- How current cleanup interacts with message persistence vs analytics persistence

3. Implement cancellation cleanup in narrow TDD slices
Minimum worthwhile target:
- explicit structured model of in-flight assistant/tool state
- deterministic cleanup plan for interrupted turns
- no transcript entry appears complete when it was interrupted mid-stream
- no stranded in-progress tool state after cleanup

4. Validate with focused tests first, then broader suite
At minimum run targeted cancellation-path tests and then the relevant broader package suites.

What not to do next session unless there is strong evidence it is worth it
- Do not jump into prompt-cache-latching architecture first.
- Do not do broad token-budget architecture work first.
- Do not reopen file-edit work unless a concrete bug shows up.
- Do not describe the Claude handoff as complete.

Useful recent commits
- `af62069` chore: checkpoint provider and tool-output retrofit work
- `3903a66` feat(agent): structure persisted tool result references
- `80207f0` feat(agent): configure persisted tool result storage root
- `454ad96` feat(agent): report aggregate tool-result budget savings
- `78d7f25` docs: clarify iteration analytics persistence contract
- `5033cf4` feat(settings): expose tool result storage config
- `62d9285` feat(tool): require full reads before file edits
- `f5a7a76` feat(tool): clarify file edit recovery errors
- `6f7c610` feat(tool): enrich file edit disambiguation errors
- `02b0718` feat(tool): add file edit candidate snippets
- `f2e8ecc` feat(tool): improve multiline file edit diagnostics
- `e7de3e3` refactor(tool): separate file edit match analysis

Suggested first commands next session
- `git status --short --branch`
- `go test ./internal/server ./internal/agent/... ./internal/config ./internal/conversation ./cmd/sirtopham && go test -tags sqlite_fts5 ./internal/tool/...`
- then inspect cancellation cleanup codepaths and begin the cancellation/transcript-invariants slice
