# Sodoryard

A self-hosted AI coding harness with multi-agent chain orchestration, RAG-powered context assembly, and a persistent project brain. One CLI (`yard`) controls the entire operator surface — from spinning up the web UI to running autonomous agent chains that plan, implement, audit, and resolve code changes.

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
                 |Provider | |Brain | | Code   |
                 |Router   | |(MCP) | | Index  |
                 +----+----+ +--+---+ +---+----+
                      |        |          |
              +-------+--------+----------+-------+
              |                                    |
        +-----+------+                    +--------+--------+
        | SQLite/FTS5 |                    | LanceDB Vectors |
        | (yard.db)   |                    | (semantic search)|
        +--------------+                    +-----------------+
```

The **engine harness** runs individual agent sessions — a single agent with tools, context assembly, and a provider connection. The **chain orchestrator** composes multi-step pipelines: it spawns engine subprocesses, routes work through planning/coding/auditing roles, and tracks progress via receipts.

Both share a common runtime layer (`internal/runtime/`) for provider construction, database setup, brain backends, and context assembly. The `yard` CLI delegates to these runtimes — no business logic lives in the command layer.

## Command Reference

```
yard [--config yard.yaml]
 |-- init                          Project bootstrap
 |-- serve                         Web UI + API server
 |-- run                           Single headless agent session
 |-- index                         Code index build/rebuild
 |-- auth
 |   |-- login codex                Provider login
 |   +-- status                    Provider auth detail
 |-- doctor                        Auth diagnostics with connectivity check
 |-- config                        Show/validate configuration
 |-- chain
 |   |-- start                     Start a new chain execution
 |   |-- status                    Show chain status
 |   |-- logs                      Show chain event log
 |   |-- receipt                   Show orchestrator or step receipt
 |   |-- cancel                    Cancel a running chain
 |   |-- pause                     Pause a running chain
 |   +-- resume                    Resume a paused chain
 |-- brain
 |   |-- index                     Rebuild brain metadata from vault
 |   +-- serve                     Standalone brain MCP server (stdio)
 +-- llm
     |-- status                    Local LLM service health
     |-- up                        Start local LLM services
     |-- down                      Stop local LLM services
     +-- logs                      Show service logs
```

## Key Concepts

### Chain Orchestration

A chain is a multi-agent pipeline. The orchestrator agent reads a task or spec, decomposes it into steps, and spawns engine subprocesses for each step — assigning roles like planner, coder, auditor, or resolver. Each step produces a receipt (structured markdown with frontmatter) stored in the project brain. The orchestrator tracks token budgets, step counts, and wall-clock limits across the entire chain.

Chains support pause/resume semantics and can be cancelled mid-execution. The `yard chain status` command shows progress; `yard chain receipt` retrieves the structured output from any step.

The shipped role set is intentionally themed around the Railway Series / Thomas universe:

| Role | Engine | Why |
|------|--------|-----|
| Orchestrator | Sir Topham Hatt | He's not an engine — he's the boss. Dispatches, evaluates, decides. |
| Planner | Gordon | The big express engine. Thinks he's the most important. Goes first, sets the course for everyone else, and sees the big picture. |
| Epic Decomposer | Edward | Wise, experienced, methodical. Breaks big jobs into sensible pieces. The reliable one everyone trusts with important work. |
| Task Decomposer | Emily | Organized, detail-oriented, and makes sure everything is in order before work starts. |
| Coder | Thomas | The main character. Does the actual branch line work, gets his hands dirty, occasionally goes off-script, but gets the job done. |
| Code Correctness Auditor | Percy | Thomas's best friend but independent. Honest, catches what Thomas missed. Runs the mail — delivering the truth about what actually happened. |
| Code Quality Auditor | James | Vain, cares about appearances. Everything should look right, be polished, and follow proper form. |
| Performance Auditor | Spencer | The sleek silver private engine. Fast, efficiency-obsessed, always measuring himself against others. |
| Security Auditor | Diesel | Suspicious of everything. Looks for hidden problems and bad actors. Doesn't trust easily. |
| Integration Auditor | Toby | The tramway engine who works at the junction between different lines. Lives at the seams and understands how different parts of the railway connect. |
| Test Writer | Rosie | Independent-minded, approaches things from her own angle, and writes tests from the spec rather than from Thomas's implementation. |
| Resolver | Victor | Runs the Steamworks repair shop. You bring him broken things, he fixes them. Targeted, efficient, doesn't redesign — just repairs. |
| Docs Arbiter | Harold | The helicopter. Sees everything from above, checks that the whole railway makes sense as a system, and brings a different perspective entirely. |

In config, these roles are exposed through the `agent_roles` mapping in `yard.yaml`. Stock prompt selections use embedded `builtin:<role>` markers; the checked-in `agents/` directory remains the editable source prompt set and the sync source for embedded defaults.

### Brain

The brain is an Obsidian-compatible vault (`.brain/`) that serves as structured long-term project memory. Agents read and write documents through an MCP (Model Context Protocol) interface — specs, receipts, conventions, architectural decisions. The brain is indexed both relationally (SQLite FTS5 for full-text search) and semantically (LanceDB vectors for embedding-based retrieval).

`yard brain index` rebuilds derived metadata. `yard brain serve` exposes the vault as a standalone MCP server over stdio for external tool integration.

### Context Assembly

Every agent turn starts with context assembly — a RAG pipeline that builds a focused context package from multiple sources:

- **Code search** — semantic similarity over the codebase via LanceDB embeddings
- **Graph relationships** — structural code intelligence from tree-sitter parsing (Go, Python, TypeScript)
- **Brain retrieval** — hybrid search (FTS5 + vector) over the project brain
- **Conventions** — project-specific coding conventions extracted from the brain vault

A budget manager allocates tokens across these sources based on priority and the model's context window. The assembled context is serialized and injected into the conversation, giving agents grounded knowledge about the codebase without manually specifying files.

### Provider Routing

The provider router supports multiple LLM backends with automatic fallback:

- **Anthropic** — Claude models with native credential management and token refresh
- **Codex** — OpenAI Codex subscription integration with Yard-owned OAuth auth
- **OpenAI-compatible** — any API following the OpenAI chat completions spec (local models via Ollama, vLLM, etc.)

Each provider is configured in `yard.yaml` with routing rules that map surfaces (default, fallback) to specific provider/model pairs. The router tracks per-call token usage in SQLite for cost visibility.

## Project Structure

```
cmd/
  yard/           Unified operator CLI (documented public surface)
  tidmouth/       Internal engine binary retained for chain subprocess spawning

internal/
  runtime/        Shared runtime builders (engine + orchestrator construction)
  agent/          Agent loop, event system, turn execution
  brain/          Brain vault, MCP client/server, indexer, parser
  chain/          Chain store, step tracking, event log
  codeintel/      Tree-sitter parsing, graph store, embedder, semantic search
  codestore/      LanceDB vector store wrapper
  config/         YAML config loading and validation
  context/        Context assembler, retrieval orchestrator, budget manager
  conversation/   Conversation persistence and title generation
  db/             SQLite schema, migrations, sqlc-generated queries
  provider/       Provider interfaces, router, anthropic/codex/openai impls
  role/           Role-based tool registry construction
  server/         HTTP server, WebSocket handler, API endpoints
  spawn/          Engine subprocess spawning for chain steps
  tool/           Tool registry, executor, file/git/shell/brain/search tools

agents/           System prompts for each agent role (13 roles)
web/              React frontend (Vite, TypeScript)
webfs/            Embedded frontend assets (go:embed)
docs/             Specs and implementation plans
```

The retained internal binary name (`tidmouth`) follows a naming convention from the codebase's development history. The operator-facing surface is exclusively `yard`.

## Getting Started

### Build from source

```bash
# Prerequisites: Go 1.25+, Node 22+, Make, GCC (for CGO/SQLite)

# Build all binaries
make all

# Binaries land in bin/
ls bin/
# tidmouth  yard

# Copy the current build into ~/bin for normal shell use
make install-user-bin
```

### Initialize a project

```bash
cd /path/to/your/project

# Bootstrap config and directory structure
yard init

# Index the codebase for semantic retrieval
yard index

# Index the brain vault
yard brain index
```

### Run the web UI

```bash
yard serve
# => http://localhost:8090
```

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

# Read the result
yard chain receipt <chain-id>
```

### Run a single agent

```bash
yard run --role thomas --task "fix the null pointer in auth.go"
```

### Docker

```bash
# Build the image
docker compose build yard

# Run inside container
PROJECT_DIR=/path/to/project docker compose run --rm yard yard init
PROJECT_DIR=/path/to/project docker compose run --rm yard yard serve
PROJECT_DIR=/path/to/project docker compose run --rm yard yard chain start --task "do the thing"
```

## Tech Stack

| Component | Technology |
|-----------|-----------|
| Language | Go 1.25 |
| CLI | Cobra |
| Database | SQLite with FTS5 full-text search |
| Vector store | LanceDB |
| Code parsing | tree-sitter (Go, Python, TypeScript) |
| Frontend | React, Vite, TypeScript |
| Brain interface | Model Context Protocol (MCP) |
| Container | Debian Trixie, multi-stage Docker build |
| LLM providers | Anthropic, OpenAI-compatible, Codex |

## Current status and next session starting point

Current repo state:
- `make test`, `make build`, and `make all` are green on the current tree.
- The unified `yard` CLI is the real operator-facing surface.
- `tidmouth` remains only as the internal engine binary required by the current spawn contract.
- Live packaging/install surfaces no longer ship unsupported `sodoryard` or placeholder `knapford` binaries.
- The remaining active docs are the README, current specs, and `NEXT_SESSION_HANDOFF.md`; stale migration/implementation-plan markdown is being removed rather than treated as archival guidance.

If you are resuming work cold, read in this order:
1. `AGENTS.md`
2. this `README.md`
3. `NEXT_SESSION_HANDOFF.md`
4. `docs/specs/13_Headless_Run_Command.md`
5. `docs/specs/17-yard-containerization.md`
6. `docs/specs/18-unified-yard-cli.md`

First thing to address next session:
- prefer current-truth docs (`README.md`, specs, handoff) over historical planning artifacts
- keep `tidmouth` limited to the internal engine contract (`run`, `index`) unless you explicitly redesign the spawn contract too
- keep operator-facing docs aligned with the actual `yard` / container / runtime surface
- rerun `make test` and `make build` after each narrow slice

Useful commands:
```bash
make test
make build
make install-user-bin
yard index
yard brain index
yard serve
yard chain start --task "<real task>"
yard chain status
yard chain logs <chain-id>
yard chain receipt <chain-id>
```
