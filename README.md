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
 |   |-- approvals                 List chain tool approvals
 |   |-- approve                   Approve a pending tool approval
 |   |-- deny                      Deny a pending tool approval
 |   |-- cancel                    Cancel a running chain
 |   |-- pause                     Pause a running chain
 |   +-- resume                    Resume a paused or approval-waiting chain
 |-- eval
 |   |-- list                      List deterministic evaluation suites
 |   +-- run                       Run a deterministic evaluation suite
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

### Project Memory TypeScript SDK

The web inspector uses generated Shunter bindings from `web/src/generated/yard-project-memory.ts`. Regenerate them after changing the Go project-memory module with:

```bash
make projectmemory-bindings
```

Check that the generated bindings are fresh with:

```bash
make projectmemory-bindings-check
```

`make test` also runs the freshness check so stale generated bindings fail standard validation.

Generated bindings import `@shunter/client`. Until the Shunter TypeScript SDK is published as a normal npm package, Yard resolves that package from the vendored copy in `third_party/shunter-client` via `web/package.json`. This avoids requiring a sibling `../../shunter` checkout for `npm install` or frontend builds. To update the vendored SDK, replace the files under `third_party/shunter-client` from the pinned Shunter release while preserving the package name `@shunter/client`, then run `npm install` in `web/` to refresh the lockfile.

Run the real SDK/runtime smoke with:

```bash
make projectmemory-sdk-smoke
```

That target starts a temporary Project Memory runtime, mounts `/api/project-memory/contract` and `/api/project-memory/subscribe`, and runs the generated TypeScript helpers through the vendored `@shunter/client`. It verifies the module name/version and decodes the `recent_chains` and `recent_chain_events` declared queries against the mounted runtime.

The SDK currently powers the chain list's live Project Memory row counts, REST invalidation, and recent Project Memory event panel. Chain detail now uses SDK-decoded Project Memory `events` rows for event/timeline updates and falls back to REST event polling only if the SDK connection fails. Chain summaries, the chain detail REST snapshot, steps, approvals, approval mutations, receipts, guardrails, and metrics still use Yard's REST APIs.

Shunter follow-up: declared queries/views do not yet accept dynamic arguments. Chain detail therefore uses the general Shunter raw SQL query/view path for the selected chain id while still decoding rows with the generated `events` decoder. The Shunter agent is currently implementing general-purpose runtime/SDK/codegen features intended to accommodate what Sodoryard needs here, including parameterized declared reads. Once those features land, this should move to a declared chain-events helper.

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

### Run the desktop shell MVP

Spec 24's Electron desktop app is in early shell form. The current dev target
starts the Vite renderer and opens Electron against a local `yard serve --dev`
backend, spawning the backend when one is not already reachable:

```bash
make desktop-dev
```

Useful overrides include `YARD_BACKEND_URL` to attach to an existing backend,
`YARD_RENDERER_URL` for a non-default Vite URL, `YARD_BINARY` for the sidecar
binary, and `YARD_PROJECT_DIR` for the backend working directory.

Build a local unpacked desktop package with:

```bash
make desktop-package
# => desktop/out/Yard-linux-x64/yard-desktop
```

The unpacked package is a local development artifact. It includes Electron, the
`yard` sidecar, embedded web assets, and LanceDB libraries; installer/AppImage
packaging remains future work.

### Run the terminal operator console

```bash
yard
```

The TUI uses the shared operator runtime directly. It opens on a Codex-style command console: normal text sends raw chat to the configured provider/model without an agent role prompt, and slash commands run Yard operations inline. Useful commands include `/help`, `/new`, `/status`, `/model`, `/effort [low|medium|high|xhigh]`, `/chains`, `/chain <id>`, `/events <id>`, `/follow <id>`, `/receipt <id> [step]`, `/approvals <id>`, `/approve <id> <approval-id>`, `/deny <id> <approval-id>`, `/preview ...`, `/start ...`, `/pause <id>`, `/resume <id>`, `/cancel <id>`, and `/web <id>`. Command results, chain events, receipts, approval decisions, launch previews, and confirmations render back into the same transcript. Use PageUp/PageDown/Home/End to scroll the console transcript.

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

# Approval wait mode is opt-in. Approval-required tools still fail closed by
# default, but this mode pauses the chain in waiting_approval.
yard chain start --allow-approval-wait --task "perform a risky operation"
yard chain approvals <chain-id>
yard chain approve <chain-id> <approval-id> --reason "reviewed"
yard chain resume <chain-id>
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
- Implemented TUI/operator work includes a Codex-style slash-command console, `/new` session reset, `/effort` reasoning-effort switching for Codex providers, raw provider/model chat, readiness metadata, recent chain and detail output, receipt content, scrollable console history, event following, pause/resume/cancel controls, TUI approval list/approve/deny commands, web-inspector target handoffs, built-in and custom launch presets, persistent current launch drafts, launch role-list add/remove/clear controls, and launch preview/start for one-step, manual-roster, orchestrated, and constrained-orchestration chains.
- Spec 23 approval work now surfaces approval-required tool results as `approval_required` chain events, derives durable approval state from the event log, records `approval_decision` events, supports `yard chain approvals|approve|deny`, TUI `/approvals|/approve|/deny`, and browser chain-detail approval controls, can opt into `waiting_approval` with `yard chain start --allow-approval-wait` or TUI `/start --allow-approval-wait`, and propagates decided approvals into resumed spawned agents so matching approved shell calls can run while denied calls return a denial tool result. Exact paused-turn replay of an approved tool call remains future work.
- Spec 23 prompt metadata work now keeps all checked-in built-in role prompts and embedded prompt assets synced with frontmatter for role key, persona, expected configured tools, receipt schema, recommended max turns, and structured-finding expectations. `yard config` warns on tool/schema/max-turn drift, but the metadata remains validation/documentation only; runtime tool registration and limits still come from `yard.yaml`.
- Spec 23 eval work now supports saved baselines and append-only JSONL history entries via `yard eval run <suite> --append-history <path>`.
- Daily-driver final touches now include actionable runtime readiness in the TUI, in-console pause/resume/cancel controls, and browser inspector routes for chains, approvals, and metrics. Browser chain detail and `/api/chains/{id}/metrics` now expose the same dogfooding metrics summary used by `yard chain metrics`. The TUI intentionally does not grow a project file browser; code review stays in the operator's IDE.
- Spec 24 backend-enabling work is partially implemented: the server exposes desktop capabilities, runtime status, local-service controls, launch draft/preset/preview/start APIs, chain snapshots/events/receipts/control APIs, project-memory contract/token endpoints, and Shunter protocol mounting. The generated project-memory binding includes parameterized `chain_events` and `live_chain_events`, and the web chain detail uses those generated helpers instead of renderer-built raw SQL.
- Spec 24 desktop shell work has started under `desktop/`. `make desktop-dev` builds `bin/yard`, starts the Vite renderer, opens Electron, starts or attaches to a local backend, shows startup/failure states, persists recent project/window state, and passes backend/project-memory metadata through a minimal preload bridge. `make desktop-package` now creates a local unpacked package under `desktop/out/` with Electron, the `yard` sidecar, embedded web assets, and LanceDB libraries. Electron opens on the new `/dashboard` route, which combines runtime readiness, recent chains, recent conversations, and Project Memory activity. The dashboard also exposes local-service readiness details plus start/stop/log actions through the backend runtime APIs. The `/launch` workbench now assembles launch requests, previews them, starts chains through the backend launch API, routes to the started chain detail, and can attach backend-validated project files from the project tree or from the `/project` browser handoff. Chain detail includes pause/resume/cancel controls, and receipts have a desktop detail route. The `/project` route can browse the backend-safe project tree, filter files, preview file contents, validate selected launch attachments, choose project files through an Electron native file dialog, and hand attachments to `/launch`. The `/settings` route now shows project/runtime routing, provider model metadata, auth status, read-only fallback/agent settings, and saves backend-validated default provider/model overrides through `/api/config`.
- The remaining active docs are the README, current specs, and `TUI_IMPLEMENTATION_PLAN.md`; stale migration/implementation-plan markdown is being removed rather than treated as archival guidance. If a future `NEXT_SESSION_HANDOFF.md` exists in a checkout, prefer it over historical planning artifacts.

If you are resuming work cold, read in this order:
1. `AGENTS.md`
2. this `README.md`
3. `docs/specs/13_Headless_Run_Command.md`
4. `docs/specs/17-yard-containerization.md`
5. `docs/specs/18-unified-yard-cli.md`
6. `docs/specs/20-operator-console-tui.md`
7. `docs/specs/21-web-inspector.md`
8. `docs/specs/23-genkit-patterns-for-yard.md`
9. `docs/specs/24-electron-desktop-app.md`
10. `TUI_IMPLEMENTATION_PLAN.md`

First thing to address next session:
- prefer current-truth docs (`README.md`, specs, handoff) over historical planning artifacts
- keep `tidmouth` limited to the internal engine contract (`run`, `index`) unless you explicitly redesign the spawn contract too
- keep operator-facing docs aligned with the actual `yard` / container / runtime surface
- keep TUI-first docs clear about target behavior versus already-implemented commands
- for spec 24, continue from settings auth/login follow-through and any remaining chain/receipt polish
- use dogfooding runs and `yard chain metrics <chain-id>` to decide non-desktop slices; likely candidates are exact paused-turn approval replay/resume semantics, richer receipt rendering, launch-history ergonomics, performance/ergonomics tuning for small chains, or deeper TUI/web surfacing of the same chain health report
- rerun `make test` and `make build` after each narrow slice

Useful commands:
```bash
make test
make build
make desktop-dev
make desktop-build
make desktop-test
make desktop-package
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
yard chain approvals <chain-id>
```
