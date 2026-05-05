# 13 — Internal Chain Step Engine

**Status:** Draft v0.1 **Last Updated:** 2026-05-01 **Author:** Mitchell

---

## Overview

The retained `tidmouth run` subcommand is the current internal chain-step engine. It executes one agent session headlessly — no web UI, no HTTP server. It receives a task packet, runs the agent loop to completion, writes a receipt, and exits. The primary consumer is chain execution, and under the no-legacy CLI contract it is treated as an internal engine entrypoint rather than a public operator command.

Target direction: autonomous operator work should be represented as chains, including one-step chains for single-agent work. `yard run` should not remain a distinct execution model. If a convenience command survives temporarily, it should delegate to chain creation and produce a one-step chain. `tidmouth run` may remain as the internal step engine until the spawn contract is redesigned.

This is the internal interface contract between Tidmouth (the harness) and chain step execution. The harness remains responsible for context assembly, tool dispatch, brain access, and LLM interaction. Chain runners are responsible for deciding *what* to run and *what to do* with the result.

---

## Core Concepts

### Chain Step Session

A chain step session is identical to a web UI agent session in runtime machinery but not in control flow. The same agent loop, context assembly, tool executor, and brain wiring apply. The differences:

- Input comes from a chain step task packet, not WebSocket messages.
- Output goes to stdout/stderr, chain events, and a brain receipt document, not directly to a chat transcript.
- The session runs to completion and exits. There is no multi-turn user interaction — the agent works autonomously until it decides the task is done or a safety limit is reached.

### Receipt

Every chain step writes a receipt document to Shunter project memory at a known brain-relative path. The receipt is the durable output contract. The chain runner, orchestrator, browser UI, or operator reads the receipt to determine what happened and what should happen next. See **Receipt Format** below.

### Agent Role

A chain step is configured with a role that determines which tools are available. Roles are defined in the project config and map to tool groups. This enables a chain to run a coder with full tool access, an auditor with read-only file and brain access, or an orchestrator agent with brain-only access plus custom tools.

Custom tools are not globally available just because a role lists `custom_tools`. They require the caller constructing the role registry to provide concrete tool factories. Today, orchestrator-managed chain execution provides `spawn_agent` and `chain_complete`; an ordinary internal step-engine invocation without that orchestrator factory fails fast with a clear "custom_tools are not implemented" configuration error. This prevents a non-orchestrator step from advertising tools it cannot actually execute.

---

## CLI Interface

Current internal step-engine interface:

```
tidmouth run [flags]  # retained internal equivalent for chain spawning
```

Target operator interface:

```bash
yard chain start --role <role> --task <task>
yard chain start --role <role> --specs <paths>
```

The target operator-facing command creates a one-step chain. There is no separate autonomous `yard run` execution model in the desired command surface.

### Required Flags

| Flag | Description |
|---|---|
| `--role <name>` | Agent role config key or associated persona name (determines tool set and system prompt) |
| `--task <string>` | Task description for the agent. Mutually exclusive with `--task-file`. |
| `--task-file <path>` | Path to a file containing the task description. Mutually exclusive with `--task`. |

### Optional Flags

| Flag | Default | Description |
|---|---|---|
| `--chain-id <string>` | auto-generated | Identifier for the chain execution. Used in receipt paths and logging. |
| `--max-turns <int>` | 50 | Maximum turns before forced stop. |
| `--max-tokens <int>` | 500000 | Total token budget across all turns. |
| `--timeout <duration>` | 30m | Wall-clock timeout for the entire session. |
| `--receipt-path <path>` | supplied by chain runner | Brain-relative receipt output path. Chain-managed invocations pass `receipts/{role}/{chain-id}-step-{NNN}.md`; one-step chains pass `step-001`. Low-level engine tests may provide an explicit path, but there is no operator-facing direct-run default. |
| `--quiet` | false | Suppress all stdout except the receipt path on completion. |
| `--project-root <path>` | cwd | Override the project root for file tool operations. |

### Exit Codes

| Code | Meaning |
|---|---|
| 0 | Agent completed normally, receipt written. |
| 1 | Infrastructure error (config, Shunter project memory, provider failure). |
| 2 | Safety limit reached (max turns, max tokens, or timeout). |
| 3 | Agent signaled an explicit escalation (cannot complete task). |

### Stdout Contract

On successful internal step-engine exit (code 0 or 2), the last line of stdout is the brain-relative path to the receipt document. Chain runners use that receipt path to update the step and chain state.

```bash
receipt=$(tidmouth run \
  --role coder \
  --task "implement auth middleware" \
  --chain-id auth-2026-04-11 \
  --receipt-path receipts/coder/auth-2026-04-11-step-001.md \
  2>/dev/null | tail -1)
```

In non-quiet mode, progress information (turn count, tool calls, token usage) streams to stderr.

---

## Agent Role Configuration

Roles are defined in `yard.yaml` under a new `agent_roles` section:

```yaml
agent_roles:
  coder:
    system_prompt: agents/coder.md
    tools:
      - brain
      - file
      - git
      - shell
      - search
    brain_write_paths:
      - "plans/**"
      - "receipts/coder/**"
      - "logs/coder/**"
    brain_deny_paths:
      - "specs/**"
      - "architecture/**"
    max_turns: 100
    max_tokens: 1000000

  auditor:
    system_prompt: agents/auditor.md
    tools:
      - brain
      - file
      - git
    brain_write_paths:
      - "receipts/auditor/**"
      - "logs/auditor/**"
    brain_deny_paths:
      - "plans/**"
      - "specs/**"
      - "architecture/**"
    max_turns: 30
    max_tokens: 300000

  arbiter:
    system_prompt: agents/arbiter.md
    tools:
      - brain
    brain_write_paths:
      - "specs/**"
      - "architecture/**"
      - "receipts/docs-arbiter/**"
      - "logs/docs-arbiter/**"
    max_turns: 20
    max_tokens: 200000

  orchestrator:
    system_prompt: agents/orchestrator.md
    tools:
      - brain
    custom_tools:
      - spawn_agent
      - chain_complete
    brain_write_paths:
      - "receipts/orchestrator/**"
      - "logs/orchestrator/**"
      - "plans/**"
    max_turns: 50
    max_tokens: 500000
```

### Tool Group Mapping

The `tools` list maps to existing registration functions:

| Group | Registers | Purity |
|---|---|---|
| `brain` | `brain_search`, `brain_read`, `brain_write`, `brain_update`, `brain_lint` | mixed |
| `file` | `file_read`, `file_write`, `file_edit` | mixed |
| `file:read` | `file_read` | pure |
| `git` | `git_status`, `git_diff` | pure |
| `shell` | `shell` | mutating |
| `search` | `search_text`, `search_semantic` | pure |
| `directory` | `list_directory`, `find_files` | pure |
| `test` | `test_run` | mutating |
| `sqlc` | `db_sqlc` | mutating |

### Brain Path Enforcement

When `brain_write_paths` is configured, `BrainWrite.Execute` and `BrainUpdate.Execute` validate the target path against the allow list before writing. Paths use glob syntax (`**` for recursive match). If a write is attempted to a path not in the allow list, the tool returns a failure result explaining the restriction.

When `brain_deny_paths` is configured, those paths are blocked even if they match an allow pattern. Deny takes precedence over allow.

Brain read operations (`brain_read`, `brain_search`) are never restricted — every agent can read the full brain. `brain_lint` can inspect the full brain, but may become mutating when operation logging is enabled and requires explicit `allow_model_calls: true` before running contradiction checks. The context bias firewall is achieved through *what the system prompt tells the agent to focus on*, not by hiding information.

---

## Execution Flow

### 1. Initialization

```
yard chain start --role coder --task "implement JWT auth" --chain-id auth-2026-04-11
    │
    ├─ Create one-step chain
    ├─ Load config and open Shunter project memory, or connect to the parent memory endpoint
    ├─ Validate role exists in agent_roles config
    ├─ Build tool registry for role (tool groups + brain path scoping)
    ├─ Initialize provider (same routing as serve mode)
    ├─ Create/link internal session transcript record for the chain step
    └─ Load system prompt from role config path
```

### 2. Session Start

The first turn is constructed from the task description. The agent loop runs identically to serve mode:

```
Turn 1:
  user message = task description (from --task or --task-file)
  context assembly runs (brain retrieval, code context if search tools enabled)
  agent loop iterates until text-only response or safety limit
```

### 3. Autonomous Execution

Unlike serve mode, there is no user to send follow-up messages. The agent must complete its work within the first turn. The system prompt should instruct the agent to:

1. Read relevant brain docs and code to understand the task.
2. Plan its approach.
3. Execute the work (code changes, brain updates, etc.).
4. Write its receipt to the brain.
5. Produce a final text summary.

If the agent produces a text-only response (ending the turn) without writing a receipt, the harness writes a fallback receipt capturing the agent's final text response and marking the verdict as `completed_no_receipt`.

### 4. Completion

On turn completion or safety limit:

```
    ├─ Verify receipt exists at expected path
    │   ├─ If missing: write fallback receipt
    │   └─ If present: validate frontmatter has required fields
    ├─ Log final metrics (turns, tokens, duration, tool calls)
    ├─ Print receipt path to stdout
    └─ Exit with appropriate code
```

---

## Receipt Format

Receipts are markdown documents with YAML frontmatter, stored as Shunter brain documents. The frontmatter is machine-parseable; the body is human-readable context.

### Required Frontmatter Fields

```yaml
---
agent: coder                          # role name
chain_id: auth-2026-04-11             # chain execution identifier
step: 1                               # step number in chain (if applicable)
verdict: completed                    # see verdict values below
timestamp: 2026-04-11T14:32:00Z       # UTC completion time
turns_used: 12                        # total turns consumed
tokens_used: 48000                    # total tokens consumed
duration_seconds: 142                 # wall-clock duration
---
```

### Verdict Values

| Verdict | Meaning | Orchestrator Action |
|---|---|---|
| `completed` | Task finished successfully. | Advance to next step. |
| `completed_with_concerns` | Task finished but agent flagged issues. | Advance, but concerns are available in receipt body. |
| `completed_no_receipt` | Fallback — agent didn't write its own receipt. | Treat as completed, but flag for review. |
| `fix_required` | Agent identified problems that need correction. | Route to appropriate fix agent. |
| `blocked` | Agent cannot proceed without external input. | Stop chain, notify human. |
| `escalate` | Agent determined the task is beyond its scope. | Stop chain, notify human. |
| `safety_limit` | Harness killed the session (turns/tokens/timeout). | Stop chain, notify human. |

### Receipt Body

The body is free-form markdown authored by the agent. Recommended sections:

```markdown
## Summary
What the agent did in 2-3 sentences.

## Changes
- Files created/modified with brief descriptions.
- Brain docs created/modified.

## Concerns
Any issues, ambiguities, or risks identified.

## Next Steps
What the agent recommends the next agent (or human) should do.
```

---

## Implementation Notes

### What Exists Today

- Tool registry with per-group registration (`register.go`) — no changes needed.
- Tool executor with purity-based dispatch (`executor.go`) — no changes needed.
- Brain backend read/write/patch/search/list through Shunter project memory.
- Brain tools with operation logging (`brain_write.go`, `brain_log.go`) — needs path enforcement added.
- Context assembly pipeline (`context/assembler.go`) — no changes needed.
- Provider routing and model selection (`provider/`) — no changes needed.
- Agent loop (`agent/`) — needs a headless driver (no WebSocket, single-turn).

### What Needs Built

1. **`cmd/tidmouth/run.go`** — CLI command wiring for the internal engine entrypoint. Parses flags, loads role config, builds registry, creates headless session, drives agent loop, handles exit codes.

2. **Role-based registry builder** — Function that reads `agent_roles` config and constructs a `tool.Registry` with only the specified tool groups. Small, likely in `internal/config` or a new `internal/role` package.

3. **Brain path enforcement** — Add optional allow/deny path lists to `BrainConfig`. `BrainWrite.Execute` and `BrainUpdate.Execute` check paths before writing. Glob matching via `filepath.Match` or `doublestar` for `**` support.

4. **Headless agent driver** — Adapter that feeds a single user message into the agent loop and collects the result without WebSocket streaming. May already be close to possible with existing `agent` package internals.

5. **Fallback receipt writer** — If agent completes without writing a receipt, the harness writes one with `completed_no_receipt` verdict containing the agent's final text response and session metrics.

6. **Config schema update** — Add `agent_roles` section to `yard.yaml` parsing.

### What Does NOT Need Built

- No changes to the tool implementations themselves (brain, file, git, shell, search).
- No changes to context assembly or retrieval.
- No changes to the provider layer.
- No changes to the web UI or serve command.
- No new dependencies.

---

## Future Extensions

These are explicitly out of scope for the initial implementation but inform design decisions:

- **`spawn_agent` custom tool** — Used by the orchestrator agent role. Implemented by the conductor/orchestrator layer, not by ordinary direct step execution. The target public operator contract is chain-only, with single-agent work represented as one-step chains; the current internal chain-spawn contract still calls the retained `tidmouth run` engine entrypoint until that subprocess contract is redesigned.
- **Multi-turn headless sessions** — Allowing the orchestrator to send follow-up messages mid-session. Not needed initially; agents should be self-directed within a single turn.
- **Parallel agent execution** — Running multiple agents concurrently against the same brain. Requires brain-level write locking or conflict resolution. Deferred.
- **Brain write hooks** — Triggering events when specific brain paths are written (e.g., auto-spawning the arbiter when `specs/` changes). Deferred.
