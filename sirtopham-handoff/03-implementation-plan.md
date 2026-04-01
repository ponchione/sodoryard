# SirTopham incorporation plan for Claude Code findings

> For Hermes/SirTopham agent: this is an implementation-oriented plan, but the immediate goal may still be planning/brainstorming. If implementation begins, prefer narrow deterministic slices and validate each with tests before expanding scope.

**Goal:** Incorporate the highest-value findings from `cc-analysis.md` into SirTopham with Go-native package boundaries and minimal architectural churn.

**Architecture:** Add deterministic infrastructure seams around tool-output normalization, file-edit preconditions, turn cleanup, prompt-cache stability, and token budgeting. Do not transliterate Claude Code modules; translate the architecture patterns into SirTopham's existing session/turn/iteration model.

**Tech stack:** Go, SQLite, filesystem artifacts, provider SDK(s), WebSocket streaming already present in SirTopham.

---

## Stage 0: Planning and repository mapping

### Task 0.1: Map the current SirTopham package layout to the proposed package sketch

Objective:
- Identify where the new logic naturally fits in the existing codebase.

Deliverables:
- A short mapping note: proposed package -> actual package/path in SirTopham.

Questions to resolve:
- Where are current tool-result truncation decisions made?
- Where is read-state for file operations currently tracked, if anywhere?
- Where are partial iterations persisted and cleaned up?
- Where is prompt block assembly implemented?
- Where is token budgeting handled today?

### Task 0.2: Choose artifact persistence strategy for oversized tool outputs

Objective:
- Decide where persisted-output references should live.

Options:
- SQLite metadata + file on disk for body
- SQLite-only for small persisted bodies, disk for large bodies
- Disk-only with durable path references in SQLite

Decision criteria:
- easy replay/debugging
- easy cleanup
- prompt/transcript stability
- minimal write amplification

### Task 0.3: Define transcript-visible representation for persisted refs and tombstones

Objective:
- Make sure the model-visible and durable forms are explicit before coding.

Deliverables:
- A short schema note for:
  - persisted output reference blocks
  - cancellation tombstones
  - synthesized tool-result placeholders

---

## Stage 1: Tool output manager

### Task 1.1: Introduce tool-output domain types

Objective:
- Add the core request/result types without wiring behavior everywhere yet.

Files:
- Create or adapt package for tool-output normalization
- Start from `stubs/tooloutput/tooloutput.go`

Required types:
- `ResultEnvelope`
- `VisibleResult`
- `PersistedRef`
- `BudgetPolicy`
- `FormatPolicy`
- `ReplacementReason`

### Task 1.2: Implement per-tool formatting strategies

Objective:
- Normalize output kinds before budget enforcement.

Required behaviors:
- shell/build/test output keeps tail emphasis
- empty output normalizes to a deterministic non-empty success message
- errors may preserve more local context than success output

### Task 1.3: Implement per-result budget enforcement

Objective:
- Replace oversized raw outputs with preview + persisted ref when policy allows.

Checks:
- per-tool limit respected
- preview generation deterministic
- file-read tool policy explicitly handled

### Task 1.4: Implement aggregate visible-budget enforcement across all tool results in the next request

Objective:
- Enforce a second budget pass after per-result normalization.

Required behaviors:
- group the same set of tool results that will actually be visible together
- prefer replacing the largest fresh results first
- memoize replacement decisions by tool-call ID within the turn/request-preparation path

### Task 1.5: Add tests for output normalization and replacement stability

Cover at least:
- one huge shell output
- several medium outputs that only exceed the aggregate budget together
- a file-read result that should not be persisted under policy
- deterministic preview output for the same input

### Task 1.6: Wire the manager into the request preparation path

Objective:
- Ensure the next model request sees normalized tool results instead of raw unbounded results.

Verification:
- inspect durable transcript shape
- inspect model-visible request shape
- verify no regression in streaming UX

---

## Stage 2: File-edit hardening

### Task 2.1: Introduce read-snapshot tracking types

Objective:
- Make read-before-edit a real invariant.

Files:
- Start from `stubs/fileedit/fileedit.go`

Required types:
- `ReadSnapshot`
- `ReadStateStore`
- `EditRequest`
- `EditErrorCode`

### Task 2.2: Record full reads only

Objective:
- Distinguish full-file reads from partial reads or previews.

Required behavior:
- partial read should not satisfy edit preconditions
- full read should capture enough metadata to evaluate staleness later

### Task 2.3: Implement preflight stale-write detection

Objective:
- Catch obvious mismatch between prior read snapshot and current file state.

Suggested checks:
- content fingerprint
- mtime/size hints
- encoding/line-ending normalization policy if SirTopham supports it

### Task 2.4: Implement critical-section stale-write recheck

Objective:
- Revalidate immediately before write.

Required behavior:
- preflight pass alone is insufficient
- write should fail safely if the file changed after preflight

### Task 2.5: Tighten deterministic error payloads

Objective:
- Make error messages do teaching work for the model.

Required error classes:
- file not read first
- file missing
- invalid create-via-edit
- zero matches
- multiple matches
- stale write

### Task 2.6: Add tests for race and self-correction paths

Cover at least:
- edit after partial read -> reject
- edit after full read -> pass
- file changes after read -> stale-write reject
- multiple matches -> include disambiguation hints

---

## Stage 3: Cancellation cleanup and transcript invariants

### Task 3.1: Model in-flight assistant and tool-call state explicitly

Objective:
- Make cleanup operate on structured state rather than ad hoc conditionals.

Files:
- Start from `stubs/turnstate/turnstate.go`

Required types:
- `InflightTurn`
- `InflightToolCall`
- `CleanupReason`
- `CleanupPlan`

### Task 3.2: Separate `interrupt` from generic `cancel`

Objective:
- Preserve the semantic distinction between user interruption and system abort.

Possible reasons:
- `cancel`
- `interrupt`
- `stream_failure`
- `tool_failure`
- `shutdown`

### Task 3.3: Compute cleanup plans for partial assistant/tool state

Objective:
- Decide how to finalize the durable transcript when the live stream ends mid-iteration.

Required behaviors:
- completed units remain durable
- incomplete assistant output is tombstoned or discarded according to policy
- started tool calls missing a terminal state get a deterministic finalization record if needed

### Task 3.4: Apply cleanup transactionally with existing iteration persistence

Objective:
- Preserve SirTopham's atomicity guarantees.

Verification:
- no stranded in-progress tool records
- no assistant message appears complete when it was interrupted mid-stream
- next turn can proceed cleanly

### Task 3.5: Add cancellation-path tests

Cover at least:
- interrupt during assistant streaming
- cancel during shell execution
- stream failure after tool start but before tool end

---

## Stage 4: Prompt-cache latching

### Task 4.1: Introduce explicit prompt block types

Objective:
- Make stable and dynamic sections visible in the code model.

Files:
- Start from `stubs/promptcache/promptcache.go`

Required types:
- `StableBlock`
- `TurnBlock`
- `RenderedPrompt`
- `LatchState`

### Task 4.2: Latch cache-relevant request shape

Objective:
- Prevent accidental cache busting from silent drift.

Candidate latched fields:
- model ID
- tool schema inventory and ordering
- stable prompt block bytes
- output style / reasoning flags if cache-relevant

### Task 4.3: Preserve identical stable bytes across retry/fork/subagent paths

Objective:
- Make cache reuse intentional instead of incidental.

Verification:
- same stable inputs produce byte-identical rendered stable prompt blocks

---

## Stage 5: Budget tracker refinement

### Task 5.1: Introduce a response reserve concept

Objective:
- Stop treating the entire context window as prompt budget.

### Task 5.2: Reconcile estimated prompt size with actual provider usage

Objective:
- Improve future enforcement and debugging.

### Task 5.3: Add overflow decision logic

Objective:
- On request-too-large failures, choose between:
  - compress history
  - replace more tool outputs with persisted refs
  - reduce optional context blocks

---

## Suggested first coding slice if the agent wants immediate implementation

Pick only these tasks first:
- Task 1.1
- Task 1.2
- Task 1.3
- Task 1.4
- Task 1.5

That yields a coherent and high-leverage `tooloutput` subsystem before touching unrelated concerns.

## Suggested planning output if the agent is not coding yet

If the SirTopham agent is in planning mode, ask it to produce:
- current-package-to-proposed-package mapping
- persisted-output storage decision memo
- transcript schema proposal for persisted refs and tombstones
- first-slice implementation plan with exact files and tests

## Explicit deferrals

Defer until later unless a concrete need appears:
- semantic loop detection beyond current heuristics
- LLM-generated tool-use summaries
- Anthropic-specific cache-edit machinery
- heavy permission/classifier systems
- transport-level streaming redesign

## Success criteria

This effort is successful if SirTopham gains:
- deterministic handling of large tool outputs beyond naive truncation
- stronger file-edit safety invariants
- cleaner transcript state after interrupts/cancellation
- more explicit prompt-cache stability rules

This effort is not successful if it produces:
- a broad TypeScript-to-Go mimicry layer
- a large design with no narrow first implementation slice
- Anthropic-specific cleverness without local architectural payoff
