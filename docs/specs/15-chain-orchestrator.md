# 15 — Chain Orchestrator (SirTopham)

**Status:** Draft v0.1 **Last Updated:** 2026-05-01 **Author:** Mitchell

**Note:** Project names (SirTopham, Tidmouth, Knapford, engine aliases) are working titles subject to renaming. The architecture is name-agnostic.

---

## Overview

The orchestrator is a Go binary that manages chain executions. A chain is an ordered sequence of agent invocations against a shared brain vault, where each agent runs inside the engine harness (Tidmouth) and communicates results through brain documents. The orchestrator is itself an LLM agent — it reads brain state, exercises judgment about what to do next, and dispatches engines via custom tools.

The orchestrator does not write code, read files, run shell commands, or interact with the codebase directly. It reads the brain and spawns engines. That is its entire job.

This spec depends on:
- [[13_Headless_Run_Command]] — the internal chain-step engine that the orchestrator spawns
- [[14_Agent_Roles_and_Brain_Conventions]] — role definitions and brain directory structure
- [[20-operator-console-tui]] — primary operator chain monitoring, controls, receipts, and event-log UI
- [[21-web-inspector]] — rich browser inspection for chain details, receipts, and metrics

---

## Core Concepts

### Chain

A chain is a complete execution of work — from reading specs through decomposition, planning, implementation, auditing, and resolution. A chain has an ID, a source (which specs triggered it), a sequence of steps, and an overall status. Chains are tracked in SQLite.

A chain may contain exactly one step. One-step chains are the canonical representation for autonomous single-agent work, replacing any separate public run model. They use the same SQLite records, event log, receipt conventions, pause/cancel semantics, metrics, and operator inspection surfaces as longer chains.

Chains can be orchestrator-managed, manually rostered, or one-step. Orchestrator-managed chains run the Sir Topham role to decide sequencing dynamically. Manually rostered and one-step chains skip the orchestrator agent and execute their declared steps directly through the same step runner.

### Step

A step is one agent invocation within a chain. Each step records: which engine role was spawned, what task it was given, its receipt path, verdict, token usage, and duration. Steps are ordered but the ordering is dynamic — the orchestrator decides the next step at runtime based on receipt verdicts and brain state.

### The Orchestrator Agent

The orchestrator itself runs as a chain-step engine session. It uses the engine harness with a restricted tool set: brain access plus two custom tools (`spawn_agent` and `chain_complete`). Its system prompt instructs it to read the brain, decide what to do, and dispatch engines.

This means the orchestrator binary does three things:
1. Sets up chain state tracking (SQLite)
2. Registers the custom tools (`spawn_agent`, `chain_complete`)
3. Starts a chain-step engine session with the orchestrator role

Everything else — the judgment, the sequencing, the decision-making — is the LLM agent inside that session.

---

## CLI Interface

### Starting a Chain

```
yard chain start [flags]
```

| Flag | Required | Description |
|---|---|---|
| `--specs <paths>` | Yes (or `--task`) | Comma-separated brain-relative paths to spec docs that define the work |
| `--task <string>` | Yes (or `--specs`) | Free-form task description (for chains that don't start from specs) |
| `--role <name>` | No | Start a one-step chain for this role instead of an orchestrator-managed chain |
| `--project <path>` | No (default: cwd) | Project root directory |
| `--brain <path>` | No (default: config) | Brain vault path override |
| `--chain-id <string>` | No (auto-generated) | Chain execution identifier |
| `--max-steps <int>` | No (default: 100) | Maximum total agent invocations |
| `--max-resolver-loops <int>` | No (default: 3) | Maximum fix-audit cycles per task |
| `--max-duration <duration>` | No (default: 4h) | Wall-clock timeout for entire chain |
| `--token-budget <int>` | No (default: 5000000) | Total token ceiling across all agents |
| `--dry-run` | No | Orchestrator plans the chain but doesn't spawn any engines |

When `--role` is present, `yard chain start` creates a chain with one step, passes the task/spec packet directly to that role, writes the normal step receipt, and marks the chain complete from the step verdict. It does not run the orchestrator agent and does not call `spawn_agent`.

### Inspecting Chains

```
yard chain status [chain-id]          # Show chain status, steps, verdicts
yard chain logs [chain-id]            # Stream chain event log
yard chain receipt [chain-id] [step]  # Show a specific step's receipt
```

### Chain Control

```
yard chain pause [chain-id]           # Pause after current step completes
yard chain resume [chain-id]          # Resume a paused chain
yard chain cancel [chain-id]          # Cancel a running chain
```

### Exit Codes

| Code | Meaning |
|---|---|
| 0 | Chain completed successfully (all steps passed) |
| 1 | Infrastructure error |
| 2 | Chain completed with escalations (human review needed) |
| 3 | Chain hit safety limits (max steps, timeout, token budget) |
| 4 | Chain cancelled by user |

---

## Custom Tools

### spawn_agent

The orchestrator agent's primary tool. Spawns a chain-step engine session and blocks until it completes.

**Input Schema:**

```json
{
  "name": "spawn_agent",
    "description": "Spawn a chain-step engine agent with the given role and task. Blocks until the engine completes. Returns the engine's receipt content.",
  "input_schema": {
    "type": "object",
    "properties": {
      "role": {
        "type": "string",
        "description": "Engine role config key from agent_roles (e.g., 'coder', 'correctness-auditor', 'planner')"
      },
      "task": {
        "type": "string",
        "description": "Task description for the engine. Be specific — include brain doc paths the engine should read."
      },
      "reindex_before": {
        "type": "boolean",
        "description": "Run code/brain reindexing before starting the engine. Use after code changes.",
        "default": false
      }
    },
    "required": ["role", "task"]
  }
}
```

**Implementation:**

1. Validate role exists in config.
2. Check chain safety limits (max steps, token budget, duration).
3. If `reindex_before`: exec `tidmouth index --quiet`; when `brain.enabled` is true, also exec `yard brain index --quiet`; wait for both to complete before spawning the engine.
4. Create a step record in SQLite (status: running).
5. Exec `tidmouth run --role {role} --task {task} --chain-id {chain-id} --receipt-path receipts/{role}/{chain-id}-step-{NNN}.md` as subprocess.
6. Wait for process exit.
7. Read receipt from brain at the expected path.
8. Parse receipt frontmatter (verdict, tokens used, turns used, duration).
9. Update step record (status: completed, verdict, metrics).
10. Update chain-level cumulative metrics (total tokens, total steps).
11. Return receipt content (frontmatter + body) as tool result to the orchestrator agent.

**On engine failure (non-zero exit):**

- Exit code 1 (infrastructure error): Return error to orchestrator agent. It can decide to retry or escalate.
- Exit code 2 (safety limit): Write the receipt with `safety_limit` verdict. Return to orchestrator.
- Exit code 3 (agent escalation): Write the receipt with `escalate` verdict. Return to orchestrator.

**Timeout handling:**

If the engine subprocess exceeds its configured timeout, the orchestrator process requests termination and waits a short grace period. If the engine exits without producing a receipt, `spawn_agent` writes a synthetic `safety_limit` receipt at the expected path so chain state and brain state remain reconcilable.

### chain_complete

Signals the orchestrator agent that the chain is finished.

**Input Schema:**

```json
{
  "name": "chain_complete",
  "description": "Signal that the chain is complete. Provide a summary of what was accomplished.",
  "input_schema": {
    "type": "object",
    "properties": {
      "summary": {
        "type": "string",
        "description": "Summary of the chain execution — what was built, what passed, any remaining concerns."
      },
      "status": {
        "type": "string",
        "enum": ["success", "partial", "failed"],
        "description": "Overall chain outcome."
      }
    },
    "required": ["summary", "status"]
  }
}
```

**Implementation:**

1. Write chain completion receipt to brain at `receipts/orchestrator/{chain-id}.md`.
2. Update chain record in SQLite (status: completed/partial/failed, `receipt_path` set to the completion receipt).
3. Log final chain metrics.
4. Signal the orchestrator's agent loop to stop (return a special tool result that the headless driver recognizes as a termination signal).

---

## Chain State Schema

SQLite database at `.yard/yard.db`.

### chains table

```sql
CREATE TABLE IF NOT EXISTS chains (
    id                  TEXT PRIMARY KEY,
    launch_id           TEXT,           -- optional operator launch that created this chain
    launch_mode         TEXT NOT NULL DEFAULT 'sir_topham_decides',
                                        -- sir_topham_decides,
                                        -- constrained_orchestration,
                                        -- manual_roster, one_step_chain
    source_specs        TEXT,           -- JSON array of spec paths
    source_task         TEXT,           -- free-form task if not spec-driven
    status              TEXT NOT NULL DEFAULT 'running',
                                        -- running, pause_requested, paused,
                                        -- cancel_requested, completed, partial,
                                        -- failed, cancelled
    summary             TEXT,
    receipt_path        TEXT,           -- brain-relative chain-level receipt path
    total_steps         INTEGER NOT NULL DEFAULT 0,
    total_tokens        INTEGER NOT NULL DEFAULT 0,
    total_duration_secs INTEGER NOT NULL DEFAULT 0,
    resolver_loops      INTEGER NOT NULL DEFAULT 0,

    -- Limits
    max_steps           INTEGER NOT NULL DEFAULT 100,
    max_resolver_loops  INTEGER NOT NULL DEFAULT 3,
    max_duration_secs   INTEGER NOT NULL DEFAULT 14400,
    token_budget        INTEGER NOT NULL DEFAULT 5000000,

    -- Timing
    started_at          TEXT NOT NULL,
    completed_at        TEXT,
    created_at          TEXT NOT NULL,
    updated_at          TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_chains_launch ON chains(launch_id);
CREATE INDEX IF NOT EXISTS idx_chains_launch_mode ON chains(launch_mode);
```

### steps table

```sql
CREATE TABLE IF NOT EXISTS steps (
    id                  TEXT PRIMARY KEY,
    chain_id            TEXT NOT NULL REFERENCES chains(id),
    sequence_num        INTEGER NOT NULL,
    role                TEXT NOT NULL,       -- normalized engine role config key (e.g., 'coder')
    persona             TEXT,                -- display persona resolved at step start, when known
    task                TEXT NOT NULL,       -- task given to the engine
    task_context        TEXT,                -- optional resolver-loop grouping key
    status              TEXT NOT NULL DEFAULT 'pending',
                                             -- pending, running, completed, failed
    verdict             TEXT,                -- from receipt frontmatter
    receipt_path        TEXT,                -- brain-relative path to receipt
    tokens_used         INTEGER DEFAULT 0,
    turns_used          INTEGER DEFAULT 0,
    duration_secs       INTEGER DEFAULT 0,
    provider            TEXT,                -- provider resolved for this step, when known
    model               TEXT,                -- model resolved for this step, when known
    selection_reason    TEXT,                -- why this role was selected, when known
    exit_code           INTEGER,
    error_message       TEXT,

    -- Timing
    started_at          TEXT,
    completed_at        TEXT,
    created_at          TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_steps_chain ON steps(chain_id);
CREATE INDEX IF NOT EXISTS idx_steps_status ON steps(status);
```

### events table

```sql
CREATE TABLE IF NOT EXISTS events (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    chain_id            TEXT NOT NULL REFERENCES chains(id),
    step_id             TEXT REFERENCES steps(id),
    event_type          TEXT NOT NULL,
    event_data          TEXT,               -- JSON blob
    created_at          TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_events_chain ON events(chain_id);
CREATE INDEX IF NOT EXISTS idx_events_created ON events(created_at);
```

All chain, step, and event timestamps are application-written RFC3339 UTC strings, matching [[08-data-model]]. Role values persisted in chain/step state are normalized `yard.yaml.agent_roles` config keys; persona names and aliases are accepted only at operator/UI input boundaries.

### Event Types

| Event Type | When | Data |
|---|---|---|
| `chain_started` | Chain begins | specs, limits, `orchestrator_pid`, `execution_id`, `active_execution`, optional `resumed_by`/`continued_by` |
| `step_started` | Engine spawned | role, task, optional `task_context` |
| `step_process_started` | Engine subprocess has a PID | `process_id`, role, active_process |
| `step_process_exited` | Engine subprocess exited | `process_id`, exit_code |
| `step_output` | Spawned engine emitted stdout/stderr | stream, line |
| `step_completed` | Engine exited | verdict, metrics |
| `step_failed` | Engine errored | error, exit code |
| `reindex_started` | Reindex triggered | trigger (before/after which role) |
| `reindex_completed` | Reindex done | duration, files indexed |
| `resolver_loop` | Resolver cycle detected | loop count, `task_context` |
| `safety_limit_hit` | Chain limit reached | which limit, current value |
| `chain_paused` | Human requested or finalized pause | request event: `status: "pause_requested"`; final event: `status: "paused"`, `execution_id`, optional `finalized_from` |
| `chain_resumed` | Human resumed | `resumed_by`, `orchestrator_pid`, `execution_id`, `active_execution` |
| `chain_completed` | Chain finished | summary, status, total metrics, `execution_id`, optional `finalized_from` |
| `chain_cancelled` | Human requested or finalized cancellation | request event: `status: "cancel_requested"`; final event: `status: "cancelled"`, `execution_id`, optional `finalized_from` |

---

## Orchestrator Startup Flow

```
yard chain start --specs specs/auth.md
    │
    ├─ Load project config (yard.yaml)
    ├─ Validate orchestrator role exists in config
    ├─ Create chain record in SQLite
    ├─ Log chain_started event
    │
    ├─ Build orchestrator tool registry:
    │   ├─ RegisterBrainTools (with orchestrator's brain write paths)
    │   ├─ Register spawn_agent (custom, backed by subprocess exec)
    │   └─ Register chain_complete (custom, signals loop termination)
    │
    ├─ Construct orchestrator task message:
    │   "You are managing a new chain execution.
    │    Source specs: specs/auth.md
    │    Chain ID: {chain-id}
    │    Read the specs from the brain and begin orchestrating."
    │
    ├─ Start headless engine session (orchestrator role)
    │   └─ Agent loop runs:
    │       ├─ Orchestrator reads brain docs (specs, existing epics/tasks)
    │       ├─ Calls spawn_agent(role="epic-decomposer", task="decompose specs/auth.md into epics")
    │       │   └─ spawn_agent impl: execs tidmouth run, waits, returns receipt
    │       ├─ Reads receipt, decides next action
    │       ├─ Calls spawn_agent(role="task-decomposer", task="decompose epics/auth/epic.md into tasks")
    │       ├─ ... continues dispatching engines based on brain state ...
    │       ├─ Calls chain_complete(summary="...", status="success")
    │       └─ Agent loop terminates
    │
    ├─ Log chain_completed event
    ├─ Print chain summary to stdout
    └─ Exit with appropriate code
```

---

## Safety Enforcement

### Where Limits Are Enforced

Safety limits are enforced at two levels:

1. **Per-engine limits** — enforced by the internal chain-step engine (Tidmouth). Max turns, max tokens, timeout per individual agent session. Defined in the role config.

2. **Chain-level limits** — enforced by the orchestrator binary (SirTopham). Max total steps, max total tokens, max duration, resolver loop cap. Checked before every `spawn_agent` call.

The orchestrator agent does NOT enforce limits — it doesn't even know about them. The `spawn_agent` tool implementation checks limits before spawning and returns an error if a limit would be exceeded. This prevents a confused or runaway orchestrator agent from burning through resources.

### Limit Check Flow

```
Orchestrator agent calls spawn_agent(role="coder", task="...")
    │
    spawn_agent implementation:
    ├─ Check chain.total_steps < chain.max_steps
    ├─ Check chain.total_tokens < chain.token_budget
    ├─ Check elapsed time < chain.max_duration_secs
    ├─ If role is resolver: check chain.resolver_loops < chain.max_resolver_loops
    │
    ├─ If any limit exceeded:
    │   ├─ Log safety_limit_hit event
    │   └─ Return tool error: "Chain limit exceeded: {which limit}. 
    │      Call chain_complete with status 'partial' or 'failed'."
    │
    └─ If all limits pass: proceed with spawn
```

### Resolver Loop Tracking

The orchestrator binary tracks resolver loops per task (not per chain). When `spawn_agent` is called with a resolver role, the implementation checks how many resolver invocations have already run for the current task context. This requires the orchestrator agent to include task context in the spawn call, or the implementation to infer it from recent step history.

Practical approach: the `spawn_agent` tool accepts an optional `task_context` field that the orchestrator agent sets (e.g., "auth/01-jwt-middleware"). The implementation counts resolver steps with matching task_context in the current chain.

---

## Chain Pause / Resume / Cancel

### Pause

The CLI transitions the chain status from `running` to `pause_requested`. The `spawn_agent` implementation checks control state before spawning. If set:
- If an engine is currently running: let it finish, then pause.
- If no engine is running: pause immediately.
- Log a request `chain_paused` event with `status: "pause_requested"` when the CLI records the request.
- The orchestrator agent's current turn continues but `spawn_agent` returns a special "chain paused" result.

When the active orchestrator execution actually observes the request and stops, the runtime finalizes the chain to `paused` and logs a terminal `chain_paused` event with `status: "paused"`, the active `execution_id` when known, and `finalized_from: "pause_requested"`.

### Resume

The CLI transitions a `paused` chain back to `running`, logs `chain_resumed`, and starts a fresh active execution with a new `execution_id`. Resume is only valid from `paused`; a `pause_requested` chain must first finish pausing, and a `running` chain is already active. There is no separate command queue table. Resume state is represented by the chain status plus event-log payloads such as `resumed_by`, `orchestrator_pid`, `execution_id`, and `active_execution`.

Implementation detail: resuming may require starting a fresh orchestrator agent session with context about what has already been done. The brain contains all the receipts, so the new orchestrator session can read them and continue from where things left off. This is the same "fresh context" principle that applies to all agents.

### Cancel

The CLI transitions `running` or `pause_requested` to `cancel_requested`; paused chains can move directly to `cancelled`. The orchestrator binary:
- Signals any active engine subprocess PID recorded by `step_process_started` before signaling the active orchestrator execution.
- Records a request `chain_cancelled` event with `status: "cancel_requested"` for active chains.
- Signals the latest active orchestrator execution PID from the most recent active `chain_started` or `chain_resumed` event, if present. Missing or already-exited PIDs are ignored.
- Writes a cancellation receipt when the active runtime can do so.
- Logs a terminal `chain_cancelled` event with `status: "cancelled"`, the active `execution_id` when known, and `finalized_from: "cancel_requested"`.
- Exits with code 4.

---

## Orchestrator System Prompt

The orchestrator's system prompt is the most critical prompt in the system. It must:

1. Explain the orchestrator's role and boundaries clearly.
2. Define the available engine roles and what each does.
3. Describe the brain directory structure and conventions.
4. Establish the standard chain flow (decompose → plan → code → audit → resolve → test → arbiter).
5. Define decision criteria for branching (when to spawn resolvers, when to skip optional auditors, when to escalate).
6. Instruct receipt-reading behavior (parse frontmatter, check verdict, read concerns).
7. Emphasize that the orchestrator must never attempt work itself.

The prompt includes the full engine roster with aliases, roles, and capabilities so the orchestrator knows what it can dispatch.

The prompt should NOT hardcode the chain flow as a rigid sequence. The orchestrator should understand the standard flow as a default but exercise judgment — for example, skipping the performance auditor for documentation-only changes, or running the security auditor before the quality auditor for an auth-related change.

---

## Observability

### Stdout / Stderr

`yard chain start` prints the chain ID to stdout as soon as the chain record exists. By default it also watches live progress on stderr (`--watch=true`). Use `--watch=false` when a caller only wants the chain ID. The watch/log rendering supports `--verbosity normal` and `--verbosity debug`; normal mode suppresses noisy `step_output` lines unless they are important, while debug mode shows the full event stream.

```
[chain] Started chain auth-2026-04-11 from specs/auth.md
[chain] Step 1: Spawning epic-decomposer (Edward)
[chain] Step 1: epic-decomposer completed — verdict: completed (14 turns, 22k tokens, 45s)
[chain] Step 2: Spawning task-decomposer (Emily)
[chain] Step 2: task-decomposer completed — verdict: completed (8 turns, 15k tokens, 30s)
[chain] Reindexing before planner...
[chain] Step 3: Spawning planner (Gordon) for tasks/auth/01-jwt-middleware.md
...
[chain] Step 12: All auditors passed for task 01-jwt-middleware
[chain] Step 13: Spawning test-writer (Rosie)
...
[chain] Chain completed — 18 steps, 340k tokens, 12m 30s
```

`yard chain logs --follow <chain-id>` reattaches to the event stream for an existing chain and uses the same verbosity rules. `yard chain status` without a chain ID lists the latest chains; with a chain ID it prints chain and step status.

### SQLite

All state is in SQLite. The events table provides a complete audit trail. Operator surfaces read this state through shared internal services backed by `internal/chain.Store`; see [[20-operator-console-tui]] for the primary TUI contract and [[21-web-inspector]] for browser inspection routes.

### Brain

The orchestrator's own receipt at `receipts/orchestrator/{chain-id}.md` contains the full chain summary. Individual step receipts are in their role-specific subdirectories.

---

## Configuration

### yard.yaml additions

```yaml
orchestrator:
  # Default limits for all chains
  max_steps: 100
  max_resolver_loops: 3
  max_duration: 4h
  token_budget: 5000000

  # Database location
  database: .yard/yard.db

  # Engine binary path (default: tidmouth in PATH)
  engine_binary: tidmouth

  # Index binary path (default: tidmouth in PATH, uses 'index' subcommand)
  index_binary: tidmouth
```

---

## Implementation Notes

### What Exists Today

From the current runtime/orchestrator stack:
- Brain vault client — used directly by orchestrator for receipt reading
- Config loading — extended for orchestrator config
- SQLite patterns — adapted from conductor v1 (see extraction guide)

### What Needs Built

1. **`cmd/yard/chain.go`** plus **`internal/runtime/orchestrator.go`** — CLI entry point and shared runtime construction for chain startup.

2. **`internal/chain/`** — Chain state management. SQLite schema, step tracking, event logging, limit enforcement.

3. **`internal/spawn/`** — `spawn_agent` tool implementation. Subprocess execution, receipt reading, metric aggregation.

4. **`internal/receipt/`** — Receipt frontmatter parser. Extracts verdict, metrics, and structured data from brain receipts.

5. **Chain control commands** — `pause`, `resume`, `cancel`, `status`, `logs`, `receipt` subcommands.

6. **Orchestrator system prompt** — `agents/orchestrator.md`. The most important prompt in the system.

### Dependencies

The orchestrator binary depends on:
- The engine binary (Tidmouth) being available in PATH or configured path
- A brain vault existing at the configured path
- Project config (yard.yaml) with role definitions

It does NOT import engine internals for agent loop execution — it starts its own headless engine session via the same subprocess mechanism it uses for all other engines. The orchestrator is just another engine with special tools.

---

## Future Extensions

Out of scope for initial implementation:

- **Parallel engine execution** — Spawning multiple engines concurrently (e.g., running all auditors in parallel). Requires concurrent brain write safety.
- **Chain templates** — Predefined chain flows that skip the orchestrator's judgment for common patterns.
- **Chain forking** — Splitting a chain into sub-chains for independent epics.
- **Cost-aware routing** — Orchestrator considers token spend when choosing which optional agents to run.
- **Human checkpoint tool** — A `request_human_input` tool that pauses the chain and waits for human guidance through the operator console.
- **Chain resumption** — Restarting a failed or cancelled chain from a specific step rather than from scratch.
