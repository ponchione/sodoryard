# Sodoryard

A self-hosted AI coding harness with a unified operator CLI, headless agent runtime, multi-agent chain orchestration, RAG-powered context assembly, and a persistent project brain. Operators use one public CLI, `yard`, for project bootstrap, the web UI/API server, indexing, auth diagnostics, local LLM services, and autonomous agent chains, including one-step chains for single-agent work. The retained `tidmouth` binary is an internal engine subprocess used by chain execution.

## Architecture

```
                          +-----------+
                          |   yard    |  Unified operator CLI
                          +-----+-----+
                                |
              +-----------------+-----------------+
              |                                   |
      +-------+--------+               +---------+----------+
      | Engine Harness  |               | Chain Orchestrator  |
      | (headless agent |               | (multi-agent        |
      |  sessions)      |               |  pipelines)         |
      +-------+--------+               +---------+----------+
              |                                   |
              +--------+-------+---------+--------+
                       |       |         |
                 +-----+--+ +--+---+ +---+----+
                 |Provider | |Project| | Code   |
                 |Router   | |Memory | | Index  |
                 +----+----+ +--+---+ +---+----+
                      |        |          |
              +-------+--------+----------+-------+
              |                                    |
        +-----+------+                    +--------+--------+
        | Shunter     |                    | LanceDB Vectors |
        | project     |                    | (semantic search)|
        | memory      |                    |                 |
        +--------------+                    +-----------------+
```

The **engine harness** runs individual agent sessions: web conversations started by `yard serve` and internal `tidmouth run` subprocesses spawned by chains. Autonomous operator work is represented as chains, including one-step chains for single-agent work. Each session gets tools, context assembly, conversation persistence, and provider routing.

The **chain orchestrator** composes multi-step pipelines. `yard chain start` creates a chain, runs an orchestrator agent, spawns engine subprocesses for planning/coding/auditing/resolution steps, and records receipts plus event logs in project memory.

Both paths share `internal/runtime/` for provider construction, memory setup, brain backends, and context assembly. The `cmd/yard` package is mostly command wiring plus CLI rendering/control glue; reusable runtime behavior lives under `internal/`.

## Command Reference

```
yard [--config yard.yaml]             Terminal operator console
 |-- init                          Project bootstrap
 |-- serve                         Web UI + API server
 |-- index                         Code index build/rebuild
 |-- auth
 |   |-- login codex                Provider login
 |   +-- status                    Provider auth detail
 |-- doctor                        Auth diagnostics with connectivity check
 |-- config                        Show/validate configuration
 |-- chain
 |   |-- start                     Start a new chain execution
 |   |-- status                    Show chain status
 |   |-- metrics                   Show chain dogfooding metrics
 |   |-- logs                      Show chain event log
 |   |-- receipt                   Show orchestrator or step receipt
 |   |-- cancel                    Cancel a running chain
 |   |-- pause                     Pause a running chain
 |   +-- resume                    Resume a paused chain
 |-- brain
 |   +-- index                     Rebuild derived brain metadata
 |-- llm
 |   |-- status                    Local LLM service health
 |   |-- up                        Start local LLM services
 |   |-- down                      Stop local LLM services
 |   +-- logs                      Show service logs
 +-- completion                    Shell completion scripts
```

## Key Concepts

### Chain Orchestration

A chain is a multi-agent pipeline. The orchestrator agent reads a task or spec, decomposes it into steps, and spawns engine subprocesses for each step, assigning roles like planner, coder, auditor, or resolver. Each step produces a receipt (structured markdown with frontmatter) stored in the project brain. The orchestrator tracks token budgets, step counts, and wall-clock limits across the entire chain.

Chains support pause/resume semantics and can be cancelled mid-execution. The `yard chain status` command shows progress, `yard chain metrics` highlights dogfooding health signals, and `yard chain receipt` retrieves the structured output from any step.

The shipped role set is intentionally themed around the Railway Series / Thomas universe. Commands that accept an agent role can use either the **config key** or the associated persona name, so `yard chain start --role coder` and `yard chain start --role thomas` select the same role.

| Config key | Persona | Purpose |
|------------|---------|---------|
| `orchestrator` | Sir Topham Hatt | Dispatches work, evaluates results, and decides chain flow. |
| `planner` | Gordon | Establishes the overall plan. |
| `epic-decomposer` | Edward | Breaks large work into sensible epics. |
| `task-decomposer` | Emily | Turns epics into ordered tasks. |
| `coder` | Thomas | Implements changes. |
| `correctness-auditor` | Percy | Checks behavior and regressions independently. |
| `quality-auditor` | James | Checks maintainability, polish, and code quality. |
| `performance-auditor` | Spencer | Checks performance and efficiency risks. |
| `security-auditor` | Diesel | Checks security and abuse cases. |
| `integration-auditor` | Toby | Checks cross-component integration. |
| `test-writer` | Rosie | Writes tests from the spec instead of from the implementation. |
| `resolver` | Victor | Applies targeted fixes after audit findings. |
| `docs-arbiter` | Harold | Checks whether docs and system-level explanations still make sense. |

These roles live under `agent_roles` in `yard.yaml`. `yard init` seeds all 13 roles with embedded `builtin:<role>` prompt markers, so a generated config works without a prompt directory. The checked-in `agents/` directory is the editable source prompt set and sync source for embedded defaults.

### Brain

The brain is structured long-term project memory for specs, receipts, conventions, architectural decisions, logs, and notes. Shunter-backed project memory (`memory.backend: shunter`, `brain.backend: shunter`) is the base design: normal runtime reads and writes brain documents through Shunter.

`yard brain index` rebuilds derived brain metadata and semantic chunks in `.yard/lancedb/brain` from Shunter documents. `.brain/` and `.yard/yard.db` are not part of the Shunter brain design for new or cleansed projects.

### Context Assembly

Every agent turn starts with context assembly: a RAG pipeline that builds a focused context package from multiple sources:

- **Code search**: semantic similarity over the codebase via LanceDB embeddings
- **Graph relationships**: structural code intelligence from tree-sitter parsing (Go, Python, TypeScript)
- **Brain retrieval**: hybrid keyword and semantic search over the configured project brain backend
- **Conventions**: project-specific coding conventions read from the configured brain backend

A budget manager allocates tokens across these sources based on priority and the model's context window. The assembled context is serialized and injected into the conversation, giving agents grounded knowledge about the codebase without manually specifying files.

### Provider Routing

The provider router supports multiple LLM backends. `routing.default` selects the normal provider/model, and `routing.fallback` can be configured for retryable provider failures.

- **Codex**: OpenAI Codex subscription integration with Yard-owned device-code OAuth auth. `yard init` currently seeds Codex as the default provider with `reasoning_effort: medium`; use `low`, `high`, or `xhigh` for unusually small or complex runs.
- **Anthropic**: Claude models using `ANTHROPIC_API_KEY` or Claude OAuth credentials with token refresh.
- **OpenAI-compatible**: APIs following the OpenAI chat-completions shape, including local services and third-party routers.

Each provider is configured in `yard.yaml` with routing rules that map surfaces (default, fallback) to specific provider/model pairs. The router tracks per-call token usage in SQLite for cost visibility.

## Project Structure

```
cmd/
  yard/           Unified operator CLI (documented public surface)
  tidmouth/       Internal engine binary retained for chain subprocess spawning

internal/
  runtime/        Shared runtime builders (engine + orchestrator construction)
  agent/          Agent loop, event system, turn execution
  brain/          Brain indexer/parser and backend interfaces
  chain/          Chain store, step tracking, event log
  chainrun/       Chain start/resume runner used by `yard chain`
  codeintel/      Tree-sitter parsing, graph store, embedder, semantic search
  codestore/      LanceDB vector store wrapper
  config/         YAML config loading and validation
  context/        Context assembler, retrieval orchestrator, budget manager
  conversation/   Conversation persistence and title generation
  db/             SQLite schema, migrations, sqlc-generated queries
  embeddedprompts/ Built-in role prompt assets
  index/          Code-index service and local-service prechecks
  initializer/    `yard init` scaffolding
  localservices/  Docker Compose local LLM manager
  provider/       Provider interfaces, router, anthropic/codex/openai impls
  role/           Role-based tool registry construction
  server/         HTTP server, WebSocket handler, API endpoints
  spawn/          Engine subprocess spawning for chain steps
  tool/           Tool registry, executor, file/git/shell/brain/search tools

agents/           System prompts for each agent role (13 roles)
ops/llm/          Repo-owned local llama.cpp stack for indexing/local models
web/              React frontend (Vite, TypeScript)
webfs/            Embedded frontend assets (go:embed)
docs/             Specs, validation notes, and design references
```

The retained internal binary name (`tidmouth`) follows a naming convention from the codebase's development history. The operator-facing surface is exclusively `yard`.

## Getting Started

### Build from source

```bash
# Prerequisites: Go 1.25.5+, Node 22+/npm, Make, GCC (for CGO/SQLite),
# and the checked-in LanceDB library under lib/linux_amd64/.

# Build the retained runnable artifact set
make build

# Binaries land in bin/
ls bin/
# tidmouth  yard

# Run the full test suite with the same CGO/LanceDB settings used by CI/local builds
make test

# Copy the current build into ~/bin for normal shell use
make install-user-bin
```

### Initialize a project

```bash
cd /path/to/your/project

# Bootstrap config and directory structure
yard init

# Confirm the configured provider/model and auth state
yard config
yard auth status
yard doctor

# For the default Codex provider, log in if needed
yard auth login codex
```

`yard init` creates `yard.yaml`, `.yard/` Shunter/runtime/LanceDB state roots, and `.gitignore` entries. It does not create `.brain/` or `.yard/yard.db` for new Shunter-mode projects. It is safe to rerun and does not overwrite existing files.

### Build retrieval indexes

Code and brain semantic indexing expect the configured embedding service to be reachable. The generated config points at the repo-owned local stack on `localhost:12435` and defaults `local_services.mode` to `manual`.

```bash
# Check local service readiness and remediation
yard llm status

# If you set local_services.mode: auto, Yard can start the configured stack
yard llm up

# Build retrieval indexes before runtime smoke tests
yard index
yard brain index
```

The local stack lives in `ops/llm/` and expects these model files in `ops/llm/models/`: `Qwen2.5-Coder-7B-Instruct-Q6_K_L.gguf` and `nomic-embed-code.Q8_0.gguf`.

### Run the web UI

```bash
yard serve
# => http://localhost:8090
```

For frontend/backend development, use two terminals:

```bash
make dev-backend
make dev-frontend
```

### Run the terminal operator console

```bash
yard
```

The TUI uses the shared operator runtime directly. It opens on a raw chat screen for talking to the configured provider/model without an agent role prompt, tools, or chain orchestration. It also shows readiness metadata, recent chains, chain details, chain and receipt filters, receipt content, live event following, pause/cancel controls, receipt handoff to `$PAGER` or `$EDITOR`, web-inspector target handoffs that do not start `yard serve`, and launch preview/start flows for one-step, manual-roster, and orchestrated chains.

### Run a chain

```bash
# Start a multi-agent chain
yard chain start --task "implement user authentication"

# `yard chain start` prints the chain ID immediately on stdout
# and streams live progress on stderr by default.
# Use `--watch=false` when you only want the ID.
yard chain start --watch=false --task "implement user authentication"

# Reattach to an already-running chain
yard chain logs --follow <chain-id>
yard chain status
yard chain metrics <chain-id>

# Read the result
yard chain receipt <chain-id>
```

### Run a one-step chain

```bash
yard chain start --role coder --task "fix the null pointer in auth.go"
# Persona aliases work too:
yard chain start --role thomas --task "fix the null pointer in auth.go"
```

### Docker

```bash
# Build the image
docker compose build yard

# The repo-root compose file joins the external llm-net network.
# Create it once if your Docker host does not already have it.
docker network create llm-net

# Run inside container. Indexing requires container-reachable embedding URLs;
# see the note below if using the local LLM stack.
PROJECT_DIR=/path/to/project docker compose run --rm yard yard init
PROJECT_DIR=/path/to/project docker compose run --rm yard yard index
PROJECT_DIR=/path/to/project docker compose run --rm yard yard brain index
PROJECT_DIR=/path/to/project docker compose run --rm yard yard chain start --task "do the thing"
```

For browser access to `yard serve` from a one-shot container, publish the port and explicitly allow the all-interfaces bind. Only use this on a trusted network:

```bash
PROJECT_DIR=/path/to/project docker compose run --rm -p 8090:8090 yard yard serve --host 0.0.0.0 --allow-external
```

For `yard index` or `yard brain index` inside the container, make sure the mounted project's `yard.yaml` points embedding and local-service URLs at addresses reachable from that container. If the repo-owned local LLM stack is running on the same `llm-net` network, use service names such as `http://nomic-embed:12435` instead of `localhost`.

## Tech Stack

| Component | Technology |
|-----------|-----------|
| Language | Go 1.25.5 |
| CLI | Cobra |
| Project memory | Shunter |
| Structured fallback stores | SQLite with FTS5 full-text search |
| Vector store | LanceDB |
| Code parsing | tree-sitter (Go, Python, TypeScript) |
| TUI | Bubble Tea, Bubbles, Lip Gloss |
| Web inspector | React, Vite, TypeScript, Tailwind CSS |
| Brain interface | Shunter project memory |
| Container | Debian Trixie, multi-stage Docker build |
| LLM providers | Anthropic, OpenAI-compatible, Codex |

## Current status and next session starting point

Current repo state:
- `make test` and `make build` are green on the current tree; `make all` is an alias for `make build`.
- The unified `yard` CLI is the real operator-facing surface.
- `tidmouth` remains only as the internal engine binary required by the current spawn contract.
- Live packaging/install surfaces no longer ship unsupported `sodoryard` or placeholder `knapford` binaries.
- The active UI direction is terminal-first: bare `yard` now starts the daily-driver operator console, while `yard serve` remains the browser/API surface for rich inspection. This direction is specified in `docs/specs/20-operator-console-tui.md` and `docs/specs/21-web-inspector.md`.
- Implemented TUI/operator work includes raw provider/model chat, readiness metadata, recent chain and detail views, chain and receipt filtering, receipt summaries/content, event following, pause/cancel controls, receipt opening through `$PAGER`/`$EDITOR`, web-inspector target handoffs, built-in and custom launch presets, persistent current launch drafts, launch role-list add/remove/clear controls, and launch preview/start for one-step, manual-roster, orchestrated, and constrained-orchestration chains.
- Daily-driver final touches now include actionable runtime readiness in the TUI, in-console pause/resume/cancel controls, and read-only browser inspector routes for chains and metrics. The TUI intentionally does not grow a project file browser; code review stays in the operator's IDE.
- The remaining active docs are the README, current specs, `NEXT_SESSION_HANDOFF.md`, and `TUI_IMPLEMENTATION_PLAN.md`; stale migration/implementation-plan markdown is being removed rather than treated as archival guidance.

If you are resuming work cold, read in this order:
1. `AGENTS.md`
2. this `README.md`
3. `NEXT_SESSION_HANDOFF.md`
4. `docs/specs/13_Headless_Run_Command.md`
5. `docs/specs/17-yard-containerization.md`
6. `docs/specs/18-unified-yard-cli.md`
7. `docs/specs/20-operator-console-tui.md`
8. `docs/specs/21-web-inspector.md`
9. `TUI_IMPLEMENTATION_PLAN.md`

First thing to address next session:
- prefer current-truth docs (`README.md`, specs, handoff) over historical planning artifacts
- keep `tidmouth` limited to the internal engine contract (`run`, `index`) unless you explicitly redesign the spawn contract too
- keep operator-facing docs aligned with the actual `yard` / container / runtime surface
- keep TUI-first docs clear about target behavior versus already-implemented commands
- use dogfooding runs and `yard chain metrics <chain-id>` to decide the next slice; likely candidates are richer receipt rendering, launch-history ergonomics, or deeper TUI surfacing of the same chain health report
- rerun `make test` and `make build` after each narrow slice

Useful commands:
```bash
make test
make build
make install-user-bin
yard index
yard brain index
yard serve
yard
yard chain start --task "<real task>"
yard chain status
yard chain metrics <chain-id>
yard chain logs <chain-id>
yard chain receipt <chain-id>
```
