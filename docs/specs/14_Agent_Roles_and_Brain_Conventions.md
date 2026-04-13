# 14 — Agent Roles and Brain Conventions

**Status:** Draft v0.1 **Last Updated:** 2026-04-11 **Author:** Mitchell

---

## Overview

This spec defines the agent role system and brain directory conventions that enable multi-agent orchestration on top of the SirTopham harness. Each role configures which tools an agent can access, which brain paths it can write to, and what safety limits apply. The brain directory structure provides the shared state contract between agents — all coordination flows through the brain, not through message passing.

This spec depends on the headless run command defined in [[13-headless-run]].

---

## Design Principles

### Agents Are Small and Focused

Each agent has one job. A code correctness auditor does not also check performance. An epic decomposer does not also plan implementation details. Keeping agents focused means shorter sessions, less context pollution, cheaper runs, and higher quality output per-role.

### Fresh Context Per Agent

Every agent spawns as a new SirTopham session with no conversation history from prior agents. This eliminates context bias — the auditor forms its own opinion from source material, not from the coder's reasoning. The brain is the only shared state, and each agent reads only what its role requires.

### The Brain Is the API

Agents communicate exclusively through brain documents. There is no message passing, no shared memory, no RPC between agents. The orchestrator reads receipts from the brain. The coder reads plans from the brain. The auditor reads specs from the brain. If it isn't in the brain, it doesn't exist to another agent.

### Humans Are First-Class Participants

The brain is an Obsidian vault. A human can open it at any time, read every document an agent has written, edit specs, correct plans, add context, or override decisions. The system is designed so that a human and agents work from the same corpus with the same conventions.

---

## Brain Directory Structure

```
.brain/
├── specs/                    # Project specifications (human-owned)
│   ├── feature-auth.md
│   └── feature-billing.md
├── architecture/             # System architecture docs (human-owned, arbiter-maintained)
│   ├── overview.md
│   └── data-model.md
├── epics/                    # Decomposed epics (epic decomposer writes, orchestrator reads)
│   ├── auth/
│   │   └── epic.md
│   └── billing/
│       └── epic.md
├── tasks/                    # Decomposed tasks (task decomposer writes, orchestrator reads)
│   ├── auth/
│   │   ├── 01-jwt-middleware.md
│   │   ├── 02-token-refresh.md
│   │   └── 03-role-permissions.md
│   └── billing/
│       └── 01-stripe-integration.md
├── plans/                    # Implementation plans (planner writes, coder reads)
│   ├── auth/
│   │   └── 01-jwt-middleware.md
│   └── billing/
│       └── 01-stripe-integration.md
├── receipts/                 # Agent completion receipts (each role writes to its subdir)
│   ├── orchestrator/
│   ├── epic-decomposer/
│   ├── task-decomposer/
│   ├── planner/
│   ├── coder/
│   ├── correctness/
│   ├── quality/
│   ├── performance/
│   ├── security/
│   ├── integration/
│   ├── tests/
│   ├── resolver/
│   └── arbiter/
├── logs/                     # Append-only agent logs (each role writes to its subdir)
│   ├── orchestrator/
│   ├── coder/
│   └── ...
├── conventions/              # Project conventions and standards (human-owned)
│   ├── coding-standards.md
│   └── testing-strategy.md
└── _log.md                   # Global brain operation log (auto-maintained by harness)
```

### Directory Ownership Rules

| Directory | Owner | Write Access | Purpose |
|---|---|---|---|
| `specs/` | Human | Human, Docs Arbiter (corrections only) | Source of truth for what to build |
| `architecture/` | Human | Human, Docs Arbiter | Source of truth for system design |
| `conventions/` | Human | Human | Coding standards, testing strategy |
| `epics/` | Epic Decomposer | Epic Decomposer, Orchestrator | Feature-level work breakdown |
| `tasks/` | Task Decomposer | Task Decomposer, Orchestrator | Task-level work items |
| `plans/` | Planner | Planner, Coder, Orchestrator | Implementation approach per task |
| `receipts/{role}/` | Each role | Respective role only | Completion records |
| `logs/{role}/` | Each role | Respective role only | Operational logs |
| `_log.md` | Harness | Harness (automatic) | Global operation audit trail |

### Document Naming Conventions

Receipt documents follow the shipped runtime conventions:
```
# direct standalone headless run
receipts/{role}/{chain-id}.md

# orchestrator-managed step run
receipts/{role}/{chain-id}-step-{NNN}.md

# final orchestrator completion receipt
receipts/orchestrator/{chain-id}.md
```

Example:
```
receipts/coder/auth-2026-04-11-step-001.md
receipts/correctness/auth-2026-04-11-step-002.md
receipts/orchestrator/auth-2026-04-11.md
```

The direct-run path stays simple for one-off headless sessions. Orchestrated step runs use a monotonic step number because the orchestrator decides sequencing dynamically at runtime and does not have a durable task-slug contract when it spawns an engine.

Task documents follow the pattern:
```
tasks/{epic-slug}/{NN-task-slug}.md
```

The numeric prefix establishes execution order. The task decomposer is responsible for determining dependencies and ordering.

Plan documents mirror the task path:
```
plans/{epic-slug}/{NN-task-slug}.md
```

---

## Agent Role Definitions

### Orchestrator

The orchestrator is an LLM agent that reads brain state and decides which agent to spawn next. It does not write code, read files, or execute commands. Its only tools are brain access and the custom `spawn_agent` / `chain_complete` tools provided by the conductor binary.

```yaml
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
  brain_deny_paths:
    - "specs/**"
    - "architecture/**"
    - "conventions/**"
  max_turns: 50
  max_tokens: 500000
  timeout: 60m
```

**System prompt guidance:** You are a project coordinator. Read the current brain state — specs, epics, tasks, plans, and receipts. Decide which agent to run next by calling `spawn_agent`. When all work is complete, call `chain_complete`. Never attempt to do the work yourself. Your job is sequencing and judgment, not execution.

---

### Epic Decomposer

Reads project specs and produces structured epic documents. Each epic defines a feature-level scope of work with clear boundaries, success criteria, and dependencies on other epics.

```yaml
epic-decomposer:
  system_prompt: agents/epic-decomposer.md
  tools:
    - brain
  brain_write_paths:
    - "epics/**"
    - "receipts/epic-decomposer/**"
    - "logs/epic-decomposer/**"
  brain_deny_paths:
    - "specs/**"
    - "architecture/**"
    - "conventions/**"
    - "tasks/**"
    - "plans/**"
  max_turns: 20
  max_tokens: 200000
  timeout: 15m
```

**System prompt guidance:** You are a requirements analyst. Read the specs and architecture docs in the brain. Decompose the project into epics — feature-level scopes of work. Each epic should have clear boundaries, acceptance criteria, dependencies on other epics, and a rough complexity estimate. Write each epic to `epics/{slug}/epic.md`. Do not create tasks — that is a separate agent's job.

---

### Task Decomposer

Reads a single epic and produces an ordered list of implementation tasks. Each task is a concrete, actionable work item that a single coder agent can complete in one session.

```yaml
task-decomposer:
  system_prompt: agents/task-decomposer.md
  tools:
    - brain
  brain_write_paths:
    - "tasks/**"
    - "receipts/task-decomposer/**"
    - "logs/task-decomposer/**"
  brain_deny_paths:
    - "specs/**"
    - "architecture/**"
    - "conventions/**"
    - "epics/**"
    - "plans/**"
  max_turns: 20
  max_tokens: 200000
  timeout: 15m
```

**System prompt guidance:** You are a technical project manager. Read the epic at the path provided in your task. Break it into ordered implementation tasks, each scoped so that one agent can complete it in a single session. Consider dependencies between tasks — the ordering matters. Write each task to `tasks/{epic-slug}/{NN-task-slug}.md` with a description, acceptance criteria, relevant spec references, and any prerequisite tasks.

---

### Planner

Reads a single task, examines the codebase (via search tools and brain docs), and writes an implementation plan. The plan describes the approach, which files to create or modify, key design decisions, and potential risks.

```yaml
planner:
  system_prompt: agents/planner.md
  tools:
    - brain
    - search
  brain_write_paths:
    - "plans/**"
    - "receipts/planner/**"
    - "logs/planner/**"
  brain_deny_paths:
    - "specs/**"
    - "architecture/**"
    - "conventions/**"
    - "epics/**"
    - "tasks/**"
  max_turns: 30
  max_tokens: 300000
  timeout: 20m
```

**System prompt guidance:** You are a senior engineer planning implementation. Read the task document and relevant specs from the brain. Use search tools to understand the existing codebase — what patterns are in use, where similar functionality exists, what interfaces you'll need to work with. Write an implementation plan to `plans/{epic-slug}/{task-slug}.md` covering: approach, files to create/modify, key design decisions, risks, and estimated complexity. Do not write code. Your plan will be handed to a coder agent.

---

### Coder

The primary implementation agent. Reads the plan, writes code, updates brain docs as needed. Has full tool access.

```yaml
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
    - "conventions/**"
    - "epics/**"
    - "tasks/**"
  max_turns: 100
  max_tokens: 1000000
  timeout: 45m
```

**System prompt guidance:** You are a software engineer. Read your implementation plan from the brain and execute it. Write clean, tested code that follows the project's conventions. When finished, write a receipt to `receipts/coder/` documenting what you built, what files you changed, any deviations from the plan, and any concerns. If you encounter a problem that makes you question the plan itself, set your verdict to `blocked` and explain the issue — do not try to redesign the approach.

---

### Code Correctness Auditor

Validates that the implementation matches the spec and task requirements. Does the code do what it's supposed to do?

```yaml
correctness-auditor:
  system_prompt: agents/correctness-auditor.md
  tools:
    - brain
    - file
    - git
  brain_write_paths:
    - "receipts/correctness/**"
    - "logs/correctness/**"
  brain_deny_paths:
    - "specs/**"
    - "architecture/**"
    - "conventions/**"
    - "epics/**"
    - "tasks/**"
    - "plans/**"
  max_turns: 30
  max_tokens: 300000
  timeout: 20m
```

**System prompt guidance:** You are a code reviewer focused on correctness. Read the spec, the task definition, and the coder's receipt from the brain. Examine the actual code changes using git_diff and file_read. Your job is to answer: does this implementation satisfy the requirements? Check for missing edge cases, incorrect logic, unhandled errors, and gaps between what the spec requires and what the code does. Write your findings to a receipt with verdict `completed` (no issues), `completed_with_concerns` (minor issues), or `fix_required` (blocking issues).

---

### Code Quality Auditor

Reviews code for maintainability, structure, and adherence to project conventions.

```yaml
quality-auditor:
  system_prompt: agents/quality-auditor.md
  tools:
    - brain
    - file
    - git
  brain_write_paths:
    - "receipts/quality/**"
    - "logs/quality/**"
  brain_deny_paths:
    - "specs/**"
    - "architecture/**"
    - "conventions/**"
    - "epics/**"
    - "tasks/**"
    - "plans/**"
  max_turns: 30
  max_tokens: 300000
  timeout: 20m
```

**System prompt guidance:** You are a code reviewer focused on quality and maintainability. Read the conventions docs and the coder's receipt from the brain. Examine the code changes. Check for: naming clarity, SOLID violations, unnecessary duplication, missing abstractions, test coverage gaps, overly complex functions, and deviations from project conventions. Do not re-check correctness — another auditor handles that. Focus only on whether this code will be easy to maintain and extend.

---

### Performance Auditor

Identifies performance issues in the implementation.

```yaml
performance-auditor:
  system_prompt: agents/performance-auditor.md
  tools:
    - brain
    - file
    - git
  brain_write_paths:
    - "receipts/performance/**"
    - "logs/performance/**"
  brain_deny_paths:
    - "specs/**"
    - "architecture/**"
    - "conventions/**"
    - "epics/**"
    - "tasks/**"
    - "plans/**"
  max_turns: 20
  max_tokens: 200000
  timeout: 15m
```

**System prompt guidance:** You are a performance engineer. Examine the code changes for performance issues: N+1 queries, unnecessary allocations, blocking calls that should be async, missing indexes implied by query patterns, unbounded loops, large in-memory collections, and inefficient algorithms. Only flag concrete, actionable issues — not theoretical concerns. Write findings to a receipt.

---

### Security Auditor

Identifies security vulnerabilities in the implementation.

```yaml
security-auditor:
  system_prompt: agents/security-auditor.md
  tools:
    - brain
    - file
    - git
  brain_write_paths:
    - "receipts/security/**"
    - "logs/security/**"
  brain_deny_paths:
    - "specs/**"
    - "architecture/**"
    - "conventions/**"
    - "epics/**"
    - "tasks/**"
    - "plans/**"
  max_turns: 20
  max_tokens: 200000
  timeout: 15m
```

**System prompt guidance:** You are a security engineer. Examine the code changes for vulnerabilities: injection vectors (SQL, command, path traversal), authentication and authorization bypass, missing input validation, secrets in code, insecure defaults, and OWASP Top 10 issues. Only flag concrete vulnerabilities with clear exploit paths — not theoretical concerns. Write findings to a receipt.

---

### Integration Auditor

Checks whether changes break contracts with the rest of the system.

```yaml
integration-auditor:
  system_prompt: agents/integration-auditor.md
  tools:
    - brain
    - file
    - git
  brain_write_paths:
    - "receipts/integration/**"
    - "logs/integration/**"
  brain_deny_paths:
    - "specs/**"
    - "architecture/**"
    - "conventions/**"
    - "epics/**"
    - "tasks/**"
    - "plans/**"
  max_turns: 20
  max_tokens: 200000
  timeout: 15m
```

**System prompt guidance:** You are an integration engineer. Examine the code changes for contract violations: changed function signatures that have callers, database schema changes without migrations, modified API contracts, broken import paths, changed config schemas, and missing dependency updates. Your focus is the seams between this change and the rest of the system. Write findings to a receipt.

---

### Test Writer

Writes tests based on the spec and task requirements — not from the implementation.

```yaml
test-writer:
  system_prompt: agents/test-writer.md
  tools:
    - brain
    - file
    - git
    - shell
  brain_write_paths:
    - "receipts/tests/**"
    - "logs/tests/**"
  brain_deny_paths:
    - "specs/**"
    - "architecture/**"
    - "conventions/**"
    - "epics/**"
    - "tasks/**"
    - "plans/**"
  max_turns: 50
  max_tokens: 500000
  timeout: 30m
```

**System prompt guidance:** You are a test engineer. Read the spec, the task definition, and the conventions docs from the brain. Write tests that validate the requirements defined in the spec — derive test cases from the spec, not from reading the implementation. You may use file_read to understand interfaces and types you need to work with, but your test logic should come from the requirements. Run your tests with the shell tool to verify they compile and execute. Write a receipt documenting test coverage and any requirements that are difficult to test.

---

### Resolver

Makes targeted fixes based on auditor findings. A specialized fixer agent that reads auditor receipts and addresses specific issues without reimplementing features.

```yaml
resolver:
  system_prompt: agents/resolver.md
  tools:
    - brain
    - file
    - git
    - shell
  brain_write_paths:
    - "receipts/resolver/**"
    - "logs/resolver/**"
  brain_deny_paths:
    - "specs/**"
    - "architecture/**"
    - "conventions/**"
    - "epics/**"
    - "tasks/**"
    - "plans/**"
  max_turns: 50
  max_tokens: 500000
  timeout: 30m
```

**System prompt guidance:** You are a bug fixer. Read the auditor receipts provided in your task to understand what issues were found. Make targeted fixes — do not refactor or reimagine the implementation, just address the specific issues flagged. Run tests after your changes to ensure you haven't broken anything. Write a receipt documenting which issues you fixed and which remain (if any).

---

### Docs Arbiter

Validates brain document consistency and maintains architecture docs. The only agent allowed to write to `specs/` and `architecture/` (for corrections, not wholesale changes).

```yaml
docs-arbiter:
  system_prompt: agents/docs-arbiter.md
  tools:
    - brain
  brain_write_paths:
    - "specs/**"
    - "architecture/**"
    - "receipts/arbiter/**"
    - "logs/arbiter/**"
  brain_deny_paths:
    - "epics/**"
    - "tasks/**"
    - "plans/**"
  max_turns: 20
  max_tokens: 200000
  timeout: 15m
```

**System prompt guidance:** You are a technical writer and documentation auditor. Read the specs, architecture docs, and recent agent receipts from the brain. Check for: stale references to code that no longer exists, contradictions between docs, specs that were partially implemented differently than described, and missing documentation for new components. Make corrections to specs and architecture docs where the code is clearly correct and the doc is stale. For ambiguous cases, write concerns to your receipt and set verdict to `completed_with_concerns` so a human can review. Never change the intent of a spec — only correct factual inaccuracies.

---

## Orchestration Flow

### Full Feature Flow

```
Human writes/updates specs in .brain/specs/
Human triggers chain: conductor run --specs auth,billing

Orchestrator reads specs/
  │
  ├─ spawns Epic Decomposer
  │    reads specs/, architecture/
  │    writes epics/{feature}/epic.md
  │
  ├─ for each epic:
  │    ├─ spawns Task Decomposer
  │    │    reads epics/{feature}/epic.md, specs/
  │    │    writes tasks/{feature}/{NN-slug}.md
  │    │
  │    ├─ for each task (in order):
  │    │    ├─ Reindex (deterministic, not an agent)
  │    │    │    sirtopham index --quiet
  │    │    │
  │    │    ├─ spawns Planner
  │    │    │    reads tasks/{feature}/{task}.md, specs/, architecture/
  │    │    │    uses search tools for codebase context
  │    │    │    writes plans/{feature}/{task}.md
  │    │    │
  │    │    ├─ spawns Coder
  │    │    │    reads plans/{feature}/{task}.md
  │    │    │    writes code, writes receipt
  │    │    │
  │    │    ├─ Reindex (code changed, update RAG)
  │    │    │
  │    │    ├─ spawns auditors (can run in parallel in future)
  │    │    │    ├─ Code Correctness Auditor
  │    │    │    ├─ Code Quality Auditor
  │    │    │    ├─ Performance Auditor
  │    │    │    ├─ Security Auditor
  │    │    │    └─ Integration Auditor
  │    │    │
  │    │    ├─ Orchestrator reads all auditor receipts
  │    │    │    ├─ if fix_required: spawns Resolver
  │    │    │    │    ├─ re-runs relevant auditors on fixed code
  │    │    │    │    └─ (max N resolver loops before escalate)
  │    │    │    └─ if all pass: continue
  │    │    │
  │    │    ├─ spawns Test Writer
  │    │    │    writes tests, runs them
  │    │    │
  │    │    └─ task complete
  │    │
  │    └─ epic complete
  │
  ├─ spawns Docs Arbiter (once at end)
  │    validates brain consistency across all changes
  │
  └─ chain_complete
```

### Reindexing

Reindexing is a deterministic operation, not an agent. The orchestrator binary (conductor) calls `sirtopham index` as a subprocess at two points:

1. **Before the planner** — ensures the planner's search tools reflect the current codebase state.
2. **After the coder** — ensures auditors' context assembly picks up the new/changed files.

This is handled by the conductor, not by any agent. It is a configuration option in the chain definition:

```yaml
reindex_triggers:
  - before: planner
  - after: coder
  - after: resolver
```

---

## Tool Access Summary

| Role | brain | file | git | shell | search | custom |
|---|---|---|---|---|---|---|
| Orchestrator | R/W (scoped) | - | - | - | - | spawn_agent, chain_complete |
| Epic Decomposer | R/W (scoped) | - | - | - | - | - |
| Task Decomposer | R/W (scoped) | - | - | - | - | - |
| Planner | R/W (scoped) | - | - | - | R | - |
| Coder | R/W (scoped) | R/W | R | R/W | R | - |
| Correctness Auditor | R/W (scoped) | R | R | - | - | - |
| Quality Auditor | R/W (scoped) | R | R | - | - | - |
| Performance Auditor | R/W (scoped) | R | R | - | - | - |
| Security Auditor | R/W (scoped) | R | R | - | - | - |
| Integration Auditor | R/W (scoped) | R | R | - | - | - |
| Test Writer | R/W (scoped) | R/W | R | R/W | - | - |
| Resolver | R/W (scoped) | R/W | R | R/W | - | - |
| Docs Arbiter | R/W (scoped) | - | - | - | - | - |

Note: "R/W (scoped)" means the agent can read all brain docs but can only write to paths allowed by its `brain_write_paths` config. All brain reads are unrestricted.

### Read-Only File Access

Auditor roles need `file` and `git` tools but should not modify code. This requires a new concept: **read-only tool groups.** The role config should support:

```yaml
tools:
  - brain
  - file:read     # file_read only, no file_write or file_edit
  - git            # git_status and git_diff are already read-only
```

Implementation: `RegisterFileTools` is split into `RegisterFileReadTools` (registers only `file_read`) and `RegisterFileWriteTools` (registers `file_write` and `file_edit`). The role-based registry builder maps `file:read` to the read-only variant.

---

## Safety Limits

### Per-Role Defaults

Each role defines its own limits in the config. The headless `run` command enforces these, and CLI flags can tighten (but never loosen) them.

### Resolver Loop Cap

The orchestrator must track how many resolver cycles have run for a given task. After a configurable maximum (default: 3), the orchestrator stops the loop and escalates to human review. This prevents infinite fix-audit-fix cycles.

```yaml
orchestrator_limits:
  max_resolver_loops: 3
  max_tasks_per_chain: 50
  max_chain_duration: 4h
```

### Token Budget Awareness

The orchestrator should track cumulative token spend across all agents in a chain. If the total approaches a configurable ceiling, the orchestrator can choose to stop spawning optional auditors (performance, quality) and only run the critical path (correctness, security).

---

## Future Extensions

Out of scope for initial implementation:

- **Parallel auditor execution** — Running all five auditors concurrently against the same codebase snapshot. Requires no brain write conflicts (already satisfied since each writes to its own receipt subdir) but needs the orchestrator to wait for all to complete before reading results.
- **Conditional auditor selection** — The orchestrator decides which auditors to run based on what changed (e.g., skip performance auditor if the change is purely documentation).
- **Cross-epic dependency tracking** — Tasks in one epic may depend on tasks in another epic. The task decomposer flags these, and the orchestrator respects the ordering.
- **Incremental re-planning** — If the resolver makes significant changes, re-run the planner to update the plan before continuing to the next task.
- **Human checkpoints** — Configurable pause points where the orchestrator stops and waits for human approval before continuing (e.g., after epic decomposition, before first coder agent).