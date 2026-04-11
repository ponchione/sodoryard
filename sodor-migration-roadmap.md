# Sodor Migration Roadmap

**Purpose:** Step-by-step plan to migrate from the current `ponchione/sirtopham` repo into the `ponchione/sodoryard` monorepo, rename/restructure packages, and build new components on top.

**Current state:**
- `ponchione/sirtopham` — the engine harness (becomes Tidmouth)
- `ponchione/agent-conductor` — the old pipeline (reference only, being archived)

**Target state:**
- `ponchione/sodoryard` — monorepo containing Tidmouth, SirTopham, and Knapford

---

## Phase 0: Prep (Manual — Mitchell)

Do these by hand before agents touch anything.

1. **Create the `ponchione/sodoryard` repo on GitHub.** Empty, just a README and go.mod with `module github.com/ponchione/sodoryard`.

2. **Archive `ponchione/agent-conductor`.** Mark it archived on GitHub. It's reference material now. The extraction guide doc covers what to pull from it.

3. **Decide on the `ponchione/sirtopham` repo.** Two options:
   - **Option A:** Rename `ponchione/sirtopham` → `ponchione/sodoryard` on GitHub (GitHub handles redirects). Then restructure in place. Preserves full git history.
   - **Option B:** Create `ponchione/sodoryard` fresh, copy code in, archive `ponchione/sirtopham`. Cleaner start, loses inline git history (old repo still exists for reference).
   
   **Recommendation:** Option A. The harness code IS the bulk of Sodor. Renaming preserves 187 commits of history and avoids a messy copy-paste-and-fix-imports phase. The restructuring happens as commits on top of that history.

4. **Cut a tag on current sirtopham.** Tag `v0.1-pre-sodor` so there's a clean marker of the state before migration begins.

---

## Phase 1: Restructure into Monorepo Layout

**Goal:** Move existing sirtopham code into the monorepo directory structure without changing any functionality. Everything should still compile and tests should pass after each step.

**Agent type:** Coder (Thomas) — but manually supervised since this is structural.

### Step 1.1: Update go.mod

Change module path from `github.com/ponchione/sirtopham` to `github.com/ponchione/sodoryard`.

### Step 1.2: Move cmd

```
cmd/sirtopham/ → cmd/tidmouth/
```

Rename the binary. Update package name, all internal references. The `serve` command stays temporarily — it gets removed later when Knapford absorbs it.

### Step 1.3: Move internal packages (no renames yet)

Everything in `internal/` stays as-is for now. The packages don't need renaming — `internal/brain`, `internal/tool`, `internal/context`, etc. are already well-named for the monorepo. Just fix import paths from `github.com/ponchione/sirtopham/internal/...` to `github.com/ponchione/sodoryard/internal/...`.

### Step 1.4: Move supporting files

```
web/              → web/                    # stays (temporary, migrates to Knapford later)
webfs/            → webfs/                  # stays with web
ops/              → ops/                    # stays
scripts/          → scripts/                # stays
docs/specs/       → docs/specs/             # stays
.brain/           → .brain/                 # stays (this is sirtopham's own brain, not a template)
```

### Step 1.5: Create placeholder cmd dirs

```
cmd/sirtopham/    # empty main.go placeholder for the orchestrator
cmd/knapford/     # empty main.go placeholder for the dashboard
```

### Step 1.6: Create agents/ directory

```
agents/
  thomas.md       # placeholder system prompts
  percy.md
  gordon.md
  ... etc
```

These can be stubs initially. Prompt engineering happens later.

### Step 1.7: Create templates/init/ directory

```
templates/init/
  yard.yaml.example
  brain/            # default brain directory structure template
    specs/.gitkeep
    architecture/.gitkeep
    epics/.gitkeep
    tasks/.gitkeep
    plans/.gitkeep
    receipts/.gitkeep
    logs/.gitkeep
    conventions/.gitkeep
```

### Step 1.8: Update Makefile

Update build targets:
- `make tidmouth` → builds `cmd/tidmouth`
- `make sirtopham` → builds `cmd/sirtopham` (placeholder initially)
- `make knapford` → builds `cmd/knapford` (placeholder initially)
- `make all` → builds everything
- `make test` → runs all tests
- `make dev-frontend` → stays (moves to knapford later)

### Step 1.9: Verify

- `make test` passes
- `make tidmouth` produces a working binary
- `tidmouth serve` still works (temporary, proves nothing broke)
- `tidmouth index` still works
- `tidmouth init` still works

### Checkpoint: Tag `v0.2-monorepo-structure`

---

## Phase 2: Implement Headless Run Command

**Goal:** Build `tidmouth run` per spec 13.

**Agent type:** Coder (Thomas), then auditors.

### Step 2.1: Role config schema

Add `agent_roles` section to config parsing in `internal/config/`. Define the role struct: name, alias, system prompt path, tool groups, brain write/deny paths, limits.

### Step 2.2: Role-based registry builder

New function in `internal/tool/` or `internal/config/` that takes a role config and returns a `*tool.Registry` with only the permitted tools registered.

### Step 2.3: Brain path enforcement

Add allow/deny path lists to `BrainConfig`. Update `BrainWrite.Execute` and `BrainUpdate.Execute` to check paths before writing. Add glob matching support.

### Step 2.4: Read-only file tool group

Split `RegisterFileTools` into `RegisterFileReadTools` and `RegisterFileWriteTools`. Map `file:read` tool group to the read-only variant.

### Step 2.5: Headless agent driver

Adapter that feeds a single user message into the agent loop and runs to completion without WebSocket streaming. Collects the final response and session metrics.

### Step 2.6: Fallback receipt writer

If agent completes without writing a receipt to the brain, the harness writes one with `completed_no_receipt` verdict.

### Step 2.7: `cmd/tidmouth/run.go`

The CLI command. Parses flags, loads role config, builds registry, creates headless session, drives agent loop, handles exit codes, prints receipt path.

### Step 2.8: Verify

- `tidmouth run --role thomas --task "explain what this project does"` runs headlessly and exits
- Receipt appears in `.brain/receipts/`
- Brain path enforcement blocks writes to denied paths
- Safety limits (max turns, timeout) trigger correct exit codes
- `make test` passes

### Checkpoint: Tag `v0.3-headless-run`

---

## Phase 3: Build SirTopham Orchestrator

**Goal:** Build the orchestrator binary that runs chains.

**Agent type:** Coder (Thomas), multiple auditors.

### Step 3.1: Chain state schema

New SQLite schema in `internal/chain/` — chains, steps, events tables. Based on patterns extracted from conductor v1 (see extraction guide).

### Step 3.2: Chain config format

Define the chain YAML format. How chains are declared, step ordering, reindex triggers, resolver loop limits.

### Step 3.3: `spawn_agent` tool

Custom tool implementation in `internal/spawn/`. Execs `tidmouth run` as a subprocess, waits for exit, reads receipt from brain, returns receipt content to the orchestrator agent.

### Step 3.4: `chain_complete` tool

Simple tool that signals the orchestrator's agent loop to stop.

### Step 3.5: Receipt parser

`internal/receipt/` — parses receipt frontmatter from brain docs. Extracts verdict, agent, chain_id, metrics. Used by the orchestrator to read step outcomes.

### Step 3.6: Orchestrator agent loop

The main loop: SirTopham creates a headless Tidmouth session for itself (role: orchestrator, tools: brain + spawn_agent + chain_complete), feeds it the chain task, and lets it run. The orchestrator agent reads the brain, calls spawn_agent to dispatch engines, reads receipts, and decides what's next.

### Step 3.7: Reindex hooks

Before/after step hooks that exec `tidmouth index` as a subprocess.

### Step 3.8: `cmd/sirtopham/main.go`

The CLI: `sirtopham chain --specs auth --project /path/to/project`. Loads config, initializes chain state, starts orchestrator loop.

### Step 3.9: Verify

- `sirtopham chain --specs test-spec` runs a full chain (at minimum: planner → coder → one auditor)
- Chain state is tracked in SQLite
- Receipts appear in brain
- Resolver loops are capped
- Chain completes or escalates correctly

### Checkpoint: Tag `v0.4-orchestrator`

---

## Phase 4: System Prompts

**Goal:** Write production system prompts for all engine roles.

**Agent type:** This is mostly human work with LLM assistance. Not a coding task.

### Step 4.1: Write each engine's system prompt

Each file in `agents/` needs a thorough system prompt that covers:
- Role identity and boundaries (what you do, what you don't do)
- Brain interaction protocol (what to read, where to write, receipt format)
- Tool usage guidance (which tools to use and when)
- Quality criteria specific to the role
- Output expectations (receipt structure, verdict criteria)

### Step 4.2: Test each role in isolation

Run `tidmouth run --role {role} --task {test-task}` for each role with a known test project. Verify the agent stays in scope, writes receipts correctly, and uses tools appropriately.

### Step 4.3: Test full chain flow

Run a complete chain on a small real task. Iterate on prompts based on where agents go off-script.

### Checkpoint: Tag `v0.5-agents-online`

---

## Phase 5: Yard Init

**Goal:** Build the `yard init` command that bootstraps a new project for railway use.

**Agent type:** Coder (Thomas).

### Step 5.1: Init command

New top-level CLI (or subcommand of tidmouth): `yard init` or `tidmouth init --yard`. Creates:
- `.brain/` with the full directory structure from the template
- `.yard/` with empty SQLite databases
- `yard.yaml` with default config (provider settings, role overrides)
- `.gitignore` entries for `.yard/` state files

### Step 5.2: Verify

- `cd /tmp/test-project && yard init` creates everything
- `tidmouth run --role thomas --task "read the brain and describe the project structure"` works against the fresh init
- `sirtopham chain` can start against the initialized project

### Checkpoint: Tag `v0.6-init`

---

## Phase 6: Knapford Dashboard

**Goal:** Build the web dashboard.

**Agent type:** Coder (Thomas) for backend, frontend specialist work for the UI.

### Step 6.1: Backend server skeleton

`cmd/knapford/main.go` — Go HTTP server, serves static frontend, exposes REST API and WebSocket endpoints. Reads brain vault, chain DB, tidmouth DB.

### Step 6.2: Migrate existing web UI components

Pull conversation view, tool call display, context inspector components from `web/` as a starting point. Restructure around the chain model.

### Step 6.3: Build chain view

Chain list, step timeline, live progress, chain flow visualization.

### Step 6.4: Build agent drilldown

Conversation view, context assembly report, receipt display.

### Step 6.5: Build brain explorer

Directory tree, document viewer/editor, metadata panel, search.

### Step 6.6: Build review queue

Escalation display, approval/rejection actions, guidance input.

### Step 6.7: Build ad-hoc engine sessions

Role selector, interactive chat via WebSocket bridge to headless Tidmouth.

### Step 6.8: Build analytics

Chain stats, agent stats, cost tracking, brain analytics.

### Step 6.9: Remove `tidmouth serve`

Once Knapford fully replaces the old web UI, remove the `serve` command from Tidmouth. Tidmouth is headless-only from this point.

### Checkpoint: Tag `v0.7-knapford`

---

## Phase 7: Containerization

**Goal:** Package everything as a Docker image and docker-compose stack.

### Step 7.1: Dockerfile

Multi-stage build:
- Build stage: compile all three Go binaries, build frontend
- Runtime stage: slim image with binaries + frontend assets + agent prompts

### Step 7.2: docker-compose.yaml

```yaml
services:
  yard:
    build: .
    volumes:
      - ${PROJECT_DIR:-.}:/project
    ports:
      - "8080:8080"    # Knapford
    environment:
      - YARD_PROJECT=/project
```

### Step 7.3: Verify

- `docker-compose up` starts Knapford
- Chain execution works inside the container against a mounted project
- Brain vault changes are visible on the host filesystem

### Checkpoint: Tag `v1.0-sodor`

---

## Phase Summary

| Phase | What | Depends On | Estimated Effort |
|---|---|---|---|
| 0 | Prep (manual) | Nothing | 30 min |
| 1 | Monorepo restructure | Phase 0 | Medium — mostly mechanical renames and import fixes |
| 2 | Headless run command | Phase 1 | Medium — spec 13 is detailed, most plumbing exists |
| 3 | SirTopham orchestrator | Phase 2 | Large — new binary, new schema, spawn_agent tool |
| 4 | System prompts | Phase 2 | Medium — iterative prompt engineering |
| 5 | Yard init | Phase 1 | Small — template copy + config generation |
| 6 | Knapford dashboard | Phases 2, 3 | Large — full web app, but migrates existing components |
| 7 | Containerization | Phases 1-6 | Small — standard Docker multi-stage build |

Phases 3, 4, and 5 can run in parallel once Phase 2 is complete.
Phase 6 can begin as soon as Phase 3 is functional (chain data to display).

---

## Agent Assignment Summary

For dispatching work to your local agents:

| Phase | Work Type | Notes |
|---|---|---|
| 1 | Mechanical refactor | High volume of import path changes. Good candidate for a single focused agent session. Verify with `make test` after. |
| 2 | Feature build (spec 13) | Break into steps 2.1-2.7 as individual agent tasks. Each is a focused, testable unit. |
| 3 | Feature build (new) | Needs a spec written first (spec 15 — the orchestrator spec). Then decompose into tasks like Phase 2. |
| 4 | Prompt engineering | Iterative human + LLM work. Not a good fit for fully autonomous agents. |
| 5 | Small feature | Single agent session. |
| 6 | Full app build | Largest phase. Decompose into epics (backend, chain view, brain explorer, review queue, etc.) then tasks. Good candidate for using the railway itself once Phase 3 is online. |
| 7 | DevOps | Single agent session. |
