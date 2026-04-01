# Proposed Go package sketch for SirTopham additions

This is not a claim about SirTopham's existing package layout. It is a proposed set of boundaries that let the agent incorporate the Claude Code findings cleanly.

The goal is to separate stable deterministic infrastructure from provider-specific or UI-specific concerns.

## Design principles

- Keep domain logic in pure packages where possible.
- Isolate filesystem/process/provider interactions behind narrow interfaces.
- Prefer explicit result objects over side effects.
- Make cache stability and transcript invariants visible in types, not just comments.
- Keep model-facing prompt transformations deterministic and replayable.

## Proposed packages

### `internal/agent/tooloutput`
Responsibility:
- Normalize tool results before they enter the next model request.
- Enforce per-result and aggregate visible budgets.
- Persist oversized outputs and replace them with previews and references.
- Apply tool-specific formatting strategies.

Core concepts:
- `ResultEnvelope`
- `VisibleResult`
- `PersistedRef`
- `Formatter`
- `BudgetPolicy`
- `Manager`

Likely dependencies:
- token/budget estimation package
- session storage or artifact store
- transcript/message package

Should not depend on:
- WebSocket transport
- provider SDK details beyond rough sizing helpers

### `internal/agent/fileedit`
Responsibility:
- Enforce read-before-edit and stale-write invariants.
- Normalize exact-match editing behavior.
- Produce deterministic, LLM-self-correcting error payloads.

Core concepts:
- `ReadSnapshot`
- `ReadStateStore`
- `EditRequest`
- `MatchResult`
- `PreconditionChecker`
- `Editor`

Likely dependencies:
- filesystem abstraction
- transcript or operation logger

Should not depend on:
- prompt rendering
- LLM provider code

### `internal/agent/turnstate`
Responsibility:
- Track in-flight turn/iteration records.
- Clean up incomplete assistant/tool state on cancel, interrupt, or stream failure.
- Preserve transcript invariants.

Core concepts:
- `InflightTurn`
- `InflightToolCall`
- `CleanupReason`
- `CleanupPlan`
- `CleanupExecutor`

Likely dependencies:
- persistence layer
- tool execution registry
- process management abstraction

Should not depend on:
- prompt assembly details

### `internal/promptcache`
Responsibility:
- Build stable prompt blocks.
- Distinguish stable session blocks from turn-dynamic blocks.
- Latch cache-relevant request settings.
- Expose reproducible byte slices/strings for reuse across retries/forks/subagents.

Core concepts:
- `StableBlock`
- `TurnBlock`
- `RenderedPrompt`
- `LatchState`
- `RequestShape`

Likely dependencies:
- prompt templates
- context assembly
- tool schema registry

Should not depend on:
- transport streaming

### `internal/tokenbudget`
Responsibility:
- Estimate request cost before send.
- Reserve output budget.
- Reconcile estimated vs actual usage.
- Expose compression triggers and overflow recovery hints.

Core concepts:
- `BudgetTracker`
- `Estimate`
- `UsageSnapshot`
- `ReservePolicy`
- `OverflowDecision`

Likely dependencies:
- promptcache renderer
- transcript formatter
- tooloutput package

## Suggested interaction model

### Request preparation path

1. Conversation state is read.
2. Prompt blocks are rendered via `internal/promptcache`.
3. Fresh tool results are normalized through `internal/agent/tooloutput`.
4. `internal/tokenbudget` estimates total prompt size and response reserve.
5. If necessary, compression or persisted-output substitutions are applied.
6. Provider request is sent.

### File edit path

1. `file_read` stores a `ReadSnapshot` in `internal/agent/fileedit`.
2. `file_edit` consults the snapshot store.
3. Preconditions are checked.
4. Exact-match edit is attempted.
5. A second stale-write check occurs in the write critical section.
6. Structured success or error payload is returned.

### Cancellation path

1. A cancellation or interrupt arrives.
2. Running process groups are signaled.
3. In-flight assistant and tool-call records are inspected.
4. `internal/agent/turnstate` computes a cleanup plan.
5. Partial records are tombstoned, completed records remain durable, and missing tool end-state is synthesized if needed.
6. Transcript is left coherent for the next turn.

## Package boundaries to preserve

These separations are worth defending even if SirTopham's current structure is different:

- tool-result normalization vs transcript storage
- file-edit safety checks vs raw filesystem writes
- prompt-block rendering vs provider request construction
- live stream events vs durable turn records
- cancellation signaling vs cleanup policy

## Suggested data contracts

### Tool output normalization contract
Input:
- tool call ID
- tool name
- raw output
- output kind (`text`, `json`, `shell`, `diff`, `error`)
- per-tool policy

Output:
- model-visible text
- optional persisted ref
- replacement reason (`per_result_budget`, `aggregate_budget`, `tool_policy`)
- preview metadata

### File-edit precondition contract
Input:
- requested edit
- stored read snapshot
- current file metadata/content

Output:
- pass/fail
- structured error code
- self-correction hints
- optional diff preview on success

### Turn cleanup contract
Input:
- in-flight assistant/tool execution state
- cleanup reason
- persistence handles

Output:
- cleanup plan
- records to tombstone
- synthesized tool results if required
- final durable state classification

## What not to over-engineer yet

Avoid introducing all of these at once:
- separate persistence backends for every artifact type
- provider-specific tokenizers for every vendor before a single common tracker exists
- a generalized workflow engine for cleanup rules
- semantic loop detection tied to embeddings or extra model calls

## Minimal viable package subset

If the agent wants the smallest package footprint that still captures the value of the analysis, start with:
- `internal/agent/tooloutput`
- `internal/agent/fileedit`
- `internal/agent/turnstate`

Then add:
- `internal/promptcache`
- `internal/tokenbudget`

## Decision checkpoints the SirTopham agent should resolve

1. Where should persisted oversized outputs live?
   - local filesystem artifact dir
   - SQLite artifact table
   - hybrid metadata-in-SQLite plus file-on-disk storage

2. Should file-read outputs ever be replaced with persisted refs?
   - never
   - only above an extreme threshold
   - only when a stable reread path is guaranteed

3. How should tombstones appear in the durable transcript?
   - hidden internal record only
   - explicit attachment/message type
   - synthetic tool-result block

4. What should be the single source of truth for budget enforcement?
   - character estimate first, actual usage second
   - provider tokenizer where available, estimation fallback otherwise

5. Which cache-relevant request fields should latch at session start?
   - model ID
   - tools schema inventory
   - cache markers / prompt block shape
   - reasoning mode / output style flags

## Bottom line

The best package structure is one that lets SirTopham absorb Claude Code's strongest ideas as deterministic infrastructure, not as a line-by-line port.
