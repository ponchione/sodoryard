# 13 — Headless Run Command

**Status:** Draft v0.1 **Last Updated:** 2026-04-11 **Author:** Mitchell

---

## Overview

The `tidmouth run` subcommand executes a single agent session headlessly — no web UI, no HTTP server. It receives a task, runs the agent loop to completion, and exits. The primary consumer is the chain orchestrator, and under the no-legacy CLI contract it is treated as an internal engine entrypoint rather than a public operator command. Operators use `yard run`; orchestrator internals may continue to use `tidmouth run` until the spawn contract is redesigned.

This is the internal interface contract between Tidmouth (the harness) and the chain orchestrator. The harness remains responsible for context assembly, tool dispatch, brain access, and LLM interaction. The orchestrator is responsible for deciding *what* to run and *what to do* with the result.

---

## Core Concepts

### Headless Session

A headless session is identical to a web UI session in every way except input/output. The same agent loop, context assembly, tool executor, and brain wiring apply. The only differences:

- Input comes from CLI flags and/or a task file, not WebSocket messages.
- Output goes to stdout/stderr and a brain receipt document, not a streaming UI.
- The session runs to completion and exits. There is no multi-turn user interaction — the agent works autonomously until it decides the task is done or a safety limit is reached.

### Receipt

Every headless run writes a receipt document to the brain vault at a known path. The receipt is the sole output contract. The orchestrator (or a human) reads the receipt to determine what happened and what should happen next. See **Receipt Format** below.

### Agent Role

A headless run is configured with a role that determines which tools are available. Roles are defined in the project config and map to tool groups. This enables the orchestrator to spawn a coder with full tool access, an auditor with read-only file and brain access, or an orchestrator agent with brain-only access plus custom tools.

---

## CLI Interface

```
tidmouth run [flags]
```

### Required Flags

| Flag | Description |
|---|---|
| `--role <name>` | Agent role from config (determines tool set and system prompt) |
| `--task <string>` | Task description for the agent. Mutually exclusive with `--task-file`. |
| `--task-file <path>` | Path to a file containing the task description. Mutually exclusive with `--task`. |

### Optional Flags

| Flag | Default | Description |
|---|---|---|
| `--chain-id <string>` | auto-generated | Identifier for the chain execution. Used in receipt paths and logging. |
| `--brain <path>` | config default | Override the brain vault path. |
| `--max-turns <int>` | 50 | Maximum turns before forced stop. |
| `--max-tokens <int>` | 500000 | Total token budget across all turns. |
| `--timeout <duration>` | 30m | Wall-clock timeout for the entire session. |
| `--receipt-path <path>` | `receipts/{role}/{chain-id}.md` | Override the brain-relative receipt output path for a direct headless run. Orchestrator-managed runs typically pass a step-specific path like `receipts/{role}/{chain-id}-step-{NNN}.md`. |
| `--quiet` | false | Suppress all stdout except the receipt path on completion. |
| `--project-root <path>` | cwd | Override the project root for file tool operations. |

### Exit Codes

| Code | Meaning |
|---|---|
| 0 | Agent completed normally, receipt written. |
| 1 | Infrastructure error (config, vault, provider failure). |
| 2 | Safety limit reached (max turns, max tokens, or timeout). |
| 3 | Agent signaled an explicit escalation (cannot complete task). |

### Stdout Contract

On successful exit (code 0 or 2), the last line of stdout is the brain-relative path to the receipt document. This allows simple scripting:

```bash
receipt=$(tidmouth run --role coder --task "implement auth middleware" 2>/dev/null | tail -1)
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
      - "receipts/arbiter/**"
      - "logs/arbiter/**"
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
| `git` | `git_status`, `git_diff` | pure |
| `shell` | `shell` | mutating |
| `search` | `search_text`, `search_semantic` | pure |

### Brain Path Enforcement

When `brain_write_paths` is configured, `BrainWrite.Execute` and `BrainUpdate.Execute` validate the target path against the allow list before writing. Paths use glob syntax (`**` for recursive match). If a write is attempted to a path not in the allow list, the tool returns a failure result explaining the restriction.

When `brain_deny_paths` is configured, those paths are blocked even if they match an allow pattern. Deny takes precedence over allow.

Brain read operations (`brain_read`, `brain_search`, `brain_list`) are never restricted — every agent can read the full brain. The context bias firewall is achieved through *what the system prompt tells the agent to focus on*, not by hiding information.

---

## Execution Flow

### 1. Initialization

```
tidmouth run --role coder --task "implement JWT auth" --chain-id auth-2026-04-11
    │
    ├─ Load config, resolve brain vault path
    ├─ Validate role exists in agent_roles config
    ├─ Build tool registry for role (tool groups + brain path scoping)
    ├─ Initialize provider (same routing as serve mode)
    ├─ Create conversation in DB (type: headless)
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

Receipts are markdown documents with YAML frontmatter, stored in the brain vault. The frontmatter is machine-parseable; the body is human-readable context.

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
- Brain vault client with read/write/patch/search/list (`vault/client.go`) — no changes needed.
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

- **`spawn_agent` custom tool** — Used by the orchestrator agent role. Implemented by the conductor/orchestrator layer, not by the engine binary. The engine contract provided here is the headless `tidmouth run` entrypoint that the conductor calls.
- **Multi-turn headless sessions** — Allowing the orchestrator to send follow-up messages mid-session. Not needed initially; agents should be self-directed within a single turn.
- **Parallel agent execution** — Running multiple agents concurrently against the same brain. Requires brain-level write locking or conflict resolution. Deferred.
- **Brain write hooks** — Triggering events when specific brain paths are written (e.g., auto-spawning the arbiter when `specs/` changes). Deferred.