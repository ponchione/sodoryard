# 22 - Sequential Agent Guardrails

**Status:** Implemented guardrails; optional read-only fan-out remains future work
**Last Updated:** 2026-05-08
**Owner:** Mitchell

---

## Overview

SodorYard should remain a **sequential multi-agent harness** for mutating work.

The intended operating model is:

1. One orchestrator decides what should happen next.
2. One spawned implementation-capable agent works at a time.
3. One or more independent read-only agents may inspect state, but they must not mutate source, project state, chain flow, or other agents' work.
4. The orchestrator consumes durable receipts and chain state before dispatching the next mutating step.

This keeps the system auditable. Parallel coders or resolvers can diverge, invalidate each other's context, and make later verification harder. The harness should optimize for coherence and traceability over wall-clock speed.

This spec depends on:

- [[13_Headless_Run_Command]] - internal `tidmouth run` step execution
- [[14_Agent_Roles_and_Brain_Conventions]] - role capabilities and receipt conventions
- [[15-chain-orchestrator]] - chain state, `spawn_agent`, and `chain_complete`
- [[24-electron-desktop-app]] - operator launch, control, and inspection surface
- [[21-web-inspector]] - richer chain and receipt inspection

---

## Non-Goals

These are intentionally out of scope for the near-term design:

- No peer-to-peer agent chat.
- No direct agent negotiation protocol.
- No parallel source-writing agents.
- No live mutation of an already-running agent's model context.
- No best-effort prompt-only safety where a hard runtime guard is reasonable.
- No separate scheduler service outside the current `yard` / `tidmouth` runtime model.

SodorYard can still use durable shared memory through Shunter. Agents should communicate through typed chain state, receipts, events, and explicit brain documents, not through untracked side channels.

---

## Current Baseline

The current implementation already leans sequential:

- `spawn_agent` blocks until the spawned `tidmouth run` subprocess exits.
- The orchestrator receives the receipt content before it can decide the next step.
- Chain steps and receipts are persisted through Shunter-backed project memory.
- The orchestrator prompt already instructs read receipt -> decide -> spawn next.

However, this is partly an emergent property of the current tool implementation. The code should make the intended invariant explicit so future launch modes, manual rosters, or read-only fan-out cannot accidentally introduce parallel writers.

## Implementation Status

The mutating-step guardrails described in this spec are now implemented in the runtime and surfaced through metrics, operator detail, desktop/web inspector views, and chain event logs.

Implemented:

- Built-in role mutation classes and config validation for obvious capability conflicts.
- Project-scoped `source_writer` lock acquisition, heartbeat, release, stale replacement facts, and operator force-release audit events.
- Source-writing step rejection while a live source-writer lock is held.
- Required receipt schema and required body sections, including `## Changed Files` for source-writing roles.
- Synthetic safety receipts for missing step receipts.
- Durable changed-file manifests from read-only `git status --short --untracked-files=all` inspection after source-writing steps.
- Changed-file receipt claim parsing, including `None`, quoted paths, and `old -> new` rename notation.
- Claim-vs-manifest mismatch facts and warnings.
- Audit finding IDs, finding lifecycle facts, resolver-addressed facts, reopened/closed/addressed tracking, and repeated resolver loop warnings.
- Pre-step chain briefings with recent receipts, finding lifecycle, changed-file manifests, lock release facts, and code/brain index facts.
- Post-step guardrail fact events for receipt validity, verdict/finding consistency, changed files, source-writer lock release, validation commands, and index state.
- Shunter/RPC code-index dirty marking after source-writing steps with changed files.
- Metrics warnings for invalid receipts, missing manifests, claim mismatches, unresolved/reopened findings, resolver loops, lock failures, dirty-mark failures, and changed files with a clean code index.
- CLI, desktop, web, and operator/API drilldowns for the durable guardrail facts above.
- Focused tests covering launch-mode flow analyzer edges and synthetic full guardrail event chains.

Remaining future work:

- Optional read-only parallel fan-out. This is still intentionally unimplemented and must remain explicitly classified, receipt-path constrained, and unable to mutate source/project state.
- Additional live dogfooding smoke runs against a configured provider/model can continue to exercise real chains, but they are validation work rather than missing guardrail mechanics.

---

## Strategy Decision

SodorYard should primarily implement these multi-agent strategies:

| Strategy | Use? | SodorYard interpretation |
|---|---:|---|
| Delegation | Yes | Orchestrator spawns one bounded role step at a time. |
| Creator-Verifier | Yes | Builder roles produce changes; auditor roles independently verify through receipts and read-only tools. |
| Broadcast | Limited | Use pre-step state briefings and durable chain events. Do not push live context into in-flight agents. |
| Direct Communication | No | Avoid peer-to-peer agent chat; it fragments state. |
| Negotiation | No | No concurrent writers means there is little useful resource negotiation. |

The correct shape is **sequential delegation + sequential creator-verifier**, with optional strictly read-only parallel audit support later.

---

## Core Invariants

### Invariant 1: Only One Source-Mutating Agent Runs Per Project

At any instant, a project may have at most one active source-mutating step.

Source-mutating roles include:

- `coder`
- `resolver`
- `test-writer`
- any custom role whose config marks it as source-mutating

The exact set should be config-driven, but builtin roles should have safe defaults.

### Invariant 2: Orchestrator Is Not A Source Writer

The orchestrator may read and write allowed brain paths and may call `spawn_agent` / `chain_complete`.

It must not:

- edit source files
- run shell commands
- run file search or semantic code search directly
- spawn another orchestrator unless a future spec explicitly adds nested chains

This is already reflected in the prompt and role config. The implementation should keep enforcing it through tool registry construction.

### Invariant 3: Read-Only Parallelism Must Be Explicit

Read-only parallel work is allowed only when each worker has:

- no source-write tools
- no shell tool
- no `spawn_agent`
- no `chain_complete`
- a unique receipt path
- a read-only role classification
- no ability to change chain status

This means future parallel auditors are acceptable. Future parallel coders are not.

### Invariant 4: Receipts Are The Step Contract

Every step must end with a valid receipt. If the agent fails before writing one, the harness must write a synthetic safety receipt.

The orchestrator should reason from receipt metadata and required sections, not from stdout, stderr, or informal logs.

### Invariant 5: Current State Is Pulled At Step Boundaries

Brain and chain state should be refreshed before each step starts. An in-flight model context should not be live-mutated.

SodorYard should use pre-step state briefings to get broadcast-like coherence while preserving sequential execution.

---

## Improvement 1: Writer Lock Enforcement

### Goal

Prevent accidental concurrent source-writing agents even if future code introduces async spawning, parallel launch modes, or multiple operator surfaces.

### Proposed Model

Add a project-scoped source writer lock in Shunter project memory.

Suggested table:

```text
project_locks
- lock_name      string primary key
- owner_chain_id string
- owner_step_id  string
- owner_role     string
- acquired_at_us uint64
- expires_at_us  uint64
- metadata_json  string
```

Suggested lock names:

- `source_writer`

Suggested reducers:

- `acquire_project_lock`
- `release_project_lock`
- `heartbeat_project_lock`
- `force_release_project_lock`

Lock acquisition should be atomic inside Shunter. If the lock is held by another running step, the reducer rejects the acquisition.

### Integration Points

`spawn_agent.prepareStep` should:

1. Resolve the role.
2. Classify the role's mutation behavior.
3. If role is source-mutating, acquire `source_writer`.
4. Start the step only after lock acquisition succeeds.
5. Record the lock owner in the step event log.

`spawn_agent.recordStepOutcome` and all failure paths should:

1. Complete/fail the step.
2. Release `source_writer` when the step owned it.
3. Log release success or failure.

The harness must release the lock on:

- successful completion
- infrastructure failure
- timeout
- missing receipt
- receipt parse failure
- cancellation after the step exits

### Open Questions

- Should lock timeout be role timeout plus grace period, or a fixed project setting?
- Should manual operator force-release be exposed in CLI/Desktop, or only in a repair command?

### Suggested First Slice

Do not build the full lock table first. Start with a conservative guard:

- Before starting a source-mutating step, query active steps in the current chain/project.
- If any source-mutating step is `running`, reject the spawn.
- Add tests proving a second coder/resolver/test-writer cannot start while one is running.

Then move enforcement into an atomic Shunter reducer if cross-process concurrency becomes realistic.

---

## Improvement 2: Role Capability Hardening

### Goal

Make role permissions machine-checkable instead of relying only on prompts.

### Proposed Role Classification

Add role metadata to `agent_roles`:

```yaml
agent_roles:
  coder:
    mutation_class: source_write
  resolver:
    mutation_class: source_write
  test-writer:
    mutation_class: source_write
  correctness-auditor:
    mutation_class: read_only
  quality-auditor:
    mutation_class: read_only
  orchestrator:
    mutation_class: orchestrator
```

Allowed values:

| Value | Meaning |
|---|---|
| `orchestrator` | May dispatch steps and complete chains. No source tools. |
| `source_write` | May mutate source or test files. Requires writer lock. |
| `brain_write` | May write allowed brain paths but not source files. |
| `read_only` | May read/search only; receipt writing is the only allowed write. |

Builtin defaults should exist so generated configs remain small.

### Tool Registry Rules

The role builder should validate tool groups against mutation class:

| Mutation class | Disallowed tools |
|---|---|
| `read_only` | file write/edit, shell, test runner if it may write, SQLC mutation, `spawn_agent`, `chain_complete` |
| `brain_write` | source write/edit, shell, `spawn_agent`, `chain_complete` |
| `source_write` | `spawn_agent`, `chain_complete` |
| `orchestrator` | source write/edit, shell, git mutation, code search if intentionally brain-only |

The implementation should fail fast during config validation when role tools and mutation class conflict.

### Receipt Exception

Read-only agents still need to write their own receipt. This should be handled as a narrow exception:

- allow `brain_write` / `brain_update` only for the role's receipt paths
- deny arbitrary brain writes unless role config explicitly allows them

### Suggested First Slice

Add `MutationClass` to `AgentRoleConfig`, default builtin roles in config normalization, and validate obvious conflicts in `Config.Validate`.

---

## Improvement 3: Typed Receipt Validation

### Goal

Make receipts reliable enough for the orchestrator, desktop/web inspector, CLI, and future automation to reason over them.

### Required Frontmatter

Every step receipt must include:

```yaml
agent: <role>
chain_id: <chain-id>
step: <number>
verdict: completed | completed_with_concerns | fix_required | blocked | escalate | safety_limit | completed_no_receipt
timestamp: <RFC3339 UTC timestamp>
turns_used: <int>
tokens_used: <int>
duration_seconds: <int>
```

### Required Body Sections

All receipts should include:

- `## Summary`
- `## Changes`
- `## Validation`
- `## Concerns`
- `## Next Steps`

Auditor receipts should additionally include:

- `## Findings`

Resolver receipts should additionally include:

- `## Findings Addressed`

### Harness Behavior

`spawn_agent.readStepReceipt` should parse and validate:

- frontmatter schema
- role matches expected role
- chain id matches current chain
- step number matches expected step
- verdict is allowed
- required sections exist
- usage fields are non-negative

If the receipt is invalid:

1. Mark the step failed.
2. Write or preserve a diagnostic receipt.
3. Log a `step_failed` event with validation details.
4. Return a clear error to the orchestrator.

### Suggested First Slice

Extend `internal/receipt` with section validation and tests. Keep section validation warning-only at first if too many prompts need adjustment.

---

## Improvement 4: Step Transition Rules

### Goal

Formalize allowed chain flow so the orchestrator has guardrails without losing judgment.

### Default Flow

Common feature flow:

```text
epic-decomposer -> task-decomposer -> planner -> coder -> auditors -> resolver? -> re-audit? -> docs-arbiter? -> chain_complete
```

Common bug fix flow:

```text
planner -> coder -> correctness-auditor -> quality-auditor -> resolver? -> re-audit? -> chain_complete
```

One-step flow:

```text
selected-role -> terminal chain closure
```

Manual roster flow:

```text
operator-declared ordered roles -> terminal chain closure
```

### Enforcement Level

The first implementation should be advisory:

- generate warnings in chain metrics
- show warnings in desktop/web inspector
- inject warnings into pre-step briefings

Hard rejection should be limited to clear safety violations:

- mutating step while writer lock is held
- resolver loop budget exceeded
- max steps exceeded
- role not allowed by constrained orchestration
- role config violates mutation class

### Suggested First Slice

Add a `chain.FlowAnalyzer` that reads chain steps and emits warnings:

- coder started before planner
- resolver started without fix_required/blocking finding
- chain completed without auditor after coder
- docs-arbiter missing after docs-impacting changes
- repeated resolver loop for same finding id

Surface those warnings in `yard chain metrics`.

---

## Improvement 5: Audit Finding IDs

### Goal

Make audit findings traceable from auditor -> resolver -> re-auditor -> final summary.

### Finding Format

Auditor receipts should use stable finding IDs:

```text
## Findings

### FIND-correctness-001
Severity: high
Status: open
Evidence: path/to/file.go:123
Summary: The nil case can panic.
Required fix: Guard before dereferencing.
```

Resolver receipts should map fixes back:

```text
## Findings Addressed

### FIND-correctness-001
Resolution: fixed
Files changed:
- internal/example.go
Validation:
- make test
```

Re-auditor receipts should close or reopen:

```text
### FIND-correctness-001
Status: closed
Evidence: make test passed
```

### Data Extraction

Add a receipt parser helper that extracts:

- finding id
- role/source
- severity
- status
- evidence references
- required fix
- resolver resolution

This does not need to be a new Shunter table immediately. It can start as parsed receipt metadata used by chain metrics and pre-step briefings.

### Suggested First Slice

Update auditor/resolver prompts and add parser tests for finding blocks.

---

## Improvement 6: Changed-File Manifest

### Goal

Make mutating steps auditable without requiring the next agent to infer changes from prose.

### Receipt Section

Mutating receipts should include:

```text
## Changed Files

- internal/foo.go
- internal/foo_test.go
```

The harness should also capture a post-step manifest independently.

### Harness-Captured Manifest

After a source-mutating step exits, before recording completion:

1. Run a read-only git diff/status inspection.
2. Capture changed paths.
3. Store the manifest in step metadata or a chain event.
4. Include it in the pre-step briefing for auditors/resolvers.

Suggested event:

```json
{
  "event_type": "step_changed_files",
  "step_id": "...",
  "payload": {
    "paths": ["internal/foo.go", "internal/foo_test.go"]
  }
}
```

### Important Constraint

The manifest command must not mutate repo state. Prefer porcelain/status and diff name-only commands.

### Suggested First Slice

Add changed-file capture to `spawn_agent` after source-mutating steps. Log a chain event. Do not yet require receipt section validation.

---

## Improvement 7: Pre-Step State Briefing

### Goal

Give each spawned agent a compact current-state packet before it starts, replacing any need for live context broadcast.

### Current Limitation

The agent loop assembles context once at turn start. The context package is frozen for that turn. Brain changes by other processes do not automatically appear in an in-flight model context.

That is acceptable for sequential execution. The right update point is **between steps**.

### Briefing Contents

Before each `tidmouth run`, the harness should generate a deterministic briefing from Shunter:

- chain id and launch task
- current step number and role
- previous receipt paths and verdicts
- open findings by ID
- findings addressed since last step
- changed-file manifest from latest mutating step
- active constraints and budgets
- pause/cancel state
- recommended docs/receipts to read
- index freshness state if available

### Injection Point

`spawn_agent.taskWithHarnessContext` currently appends chain id, step number, receipt path, and receipt requirements.

Extend this into:

```text
Harness context:
- Chain ID: ...
- Step number: ...
- Receipt path: ...

Current chain briefing:
...

Required receipt protocol:
...
```

### Suggested First Slice

Add a `chain.BriefingBuilder` that takes chain store + brain backend and returns markdown. Inject it into spawned task text. Unit-test with fake chain steps and receipts.

---

## Improvement 8: Post-Step Invariant Checks

### Goal

Automatically record enough post-step facts that auditing can start from reliable system observations.

### Checks After Every Step

Record:

- exit code
- duration
- receipt path
- parsed verdict
- token and turn counts
- whether receipt schema validation passed
- whether the chain was paused/cancelled during the step

### Checks After Mutating Steps

Record:

- changed-file manifest
- whether source writer lock was released
- whether code index is now stale
- whether brain index is now stale
- configured validation commands the agent claims it ran

### Checks After Auditor Steps

Record:

- number of findings
- open finding IDs
- verdict/finding consistency

For example, an auditor receipt with open high-severity findings and verdict `completed` should produce a warning.

### Suggested First Slice

Extend `yard chain metrics` health warnings with:

- missing receipt
- invalid receipt
- no changed-file manifest for mutating step
- auditor found open findings but verdict did not indicate follow-up
- resolver ran but no finding IDs were addressed

---

## Read-Only Parallelism Future Option

This is optional and should come after the sequential guardrails land.

### Allowed Shape

The orchestrator may request a read-only audit fan-out:

```text
coder -> [correctness-auditor, quality-auditor, security-auditor] -> orchestrator waits -> resolver?
```

Rules:

- Only `read_only` roles may be fan-out workers.
- Fan-out workers may write only their receipt paths.
- The orchestrator must wait for all fan-out receipts before spawning any mutating role.
- Findings are merged by ID/source role.
- If any fan-out worker fails to produce a valid receipt, the fan-out result is incomplete.

### Why This Is Not First

Sequential audits are easier to debug and audit. Parallel read-only fan-out is a performance optimization, not an architectural requirement.

---

## Implementation Backlog

### Slice A: Role Mutation Classes

Files likely involved:

- `internal/config/config.go`
- `internal/config/config_test.go`
- `internal/initializer/templates/init/yard.yaml.example`
- `internal/role/builder.go`
- `internal/role/builder_test.go`

Deliverables:

- Add `mutation_class` to role config.
- Default builtin roles.
- Validate tool groups against mutation class.
- Update seeded config template.
- Tests for invalid read-only role with mutating tools.

### Slice B: Sequential Source-Writer Guard

Files likely involved:

- `internal/spawn/spawn_agent.go`
- `internal/spawn/spawn_agent_test.go`
- `internal/chain/state.go`
- `internal/chain/projectmemory_store.go`
- `internal/projectmemory/chain_reducers.go`

Deliverables:

- Classify source-mutating roles.
- Prevent a second active source-mutating step from starting.
- Emit a chain event when the guard blocks a spawn.
- Tests for coder/coder and coder/resolver conflicts.

### Slice C: Receipt Schema And Sections

Files likely involved:

- `internal/receipt`
- `internal/headless/headless.go`
- `internal/spawn/spawn_agent.go`
- `agents/*.md`

Deliverables:

- Validate required frontmatter.
- Add section validation helper.
- Update prompts with required receipt sections.
- Warn or fail on invalid sections according to rollout mode.

### Slice D: Finding IDs

Files likely involved:

- `internal/receipt`
- `internal/operator/chains.go`
- `web/src/pages/chain-detail.tsx`
- `agents/percy.md`
- `agents/james.md`
- `agents/spencer.md`
- `agents/diesel.md`
- `agents/toby.md`
- `agents/victor.md`

Deliverables:

- Auditor finding block convention.
- Resolver finding-addressed convention.
- Parser for finding IDs/status/severity.
- Metrics warnings for unresolved findings.

### Slice E: Changed-File Manifest

Files likely involved:

- `internal/spawn/spawn_agent.go`
- `internal/spawn/subprocess.go`
- `internal/chain/events.go`
- `internal/operator/chains.go`
- `web/src/pages/chain-detail.tsx`

Deliverables:

- Read-only changed-file capture after mutating steps.
- `step_changed_files` event.
- Display changed paths in metrics/chain detail.
- Tests around empty/non-empty manifests.

### Slice F: Pre-Step Briefing

Files likely involved:

- `internal/chain`
- `internal/spawn/spawn_agent.go`
- `internal/spawn/spawn_agent_test.go`
- `internal/receipt`

Deliverables:

- Build chain briefing from steps, receipts, findings, events, and budgets.
- Inject briefing into spawned task.
- Unit tests with fake receipts and events.

### Slice G: Post-Step Health Checks

Files likely involved:

- `internal/operator/chains.go`
- `cmd/yard/chain_render.go`
- `internal/server/chains.go`
- `web/src/pages/chain-detail.tsx`

Deliverables:

- Add health warnings for receipt validity, manifests, unresolved findings, and suspicious verdicts.
- Surface warnings consistently in CLI, desktop, and web inspector.

---

## Suggested Execution Order

1. Role mutation classes.
2. Sequential source-writer guard.
3. Receipt schema validation.
4. Changed-file manifest.
5. Finding IDs.
6. Pre-step briefing.
7. Post-step health checks.
8. Optional read-only parallel fan-out.

This order puts hard safety first, then improves auditability, then improves orchestration quality.

---

## Acceptance Criteria

The work is successful when:

- A second source-mutating step cannot start while one is active.
- Read-only roles cannot be configured with source-mutating tools.
- Every completed step has a parsed, schema-valid receipt or a synthetic safety receipt.
- Mutating steps produce a changed-file manifest.
- Auditor findings can be traced to resolver receipts by ID.
- Each spawned step receives a compact current-state briefing.
- Chain metrics expose warnings for missing/invalid receipts, unresolved findings, missing manifests, and suspicious flow.
- No direct communication or negotiation path is introduced.

---

## Design Principle

Prefer one durable, inspectable source of truth over many live coordination channels.

Shunter project memory is the ledger. Receipts are the contract. The orchestrator is the scheduler. Agents should remain replaceable workers with bounded capabilities and explicit outputs.
