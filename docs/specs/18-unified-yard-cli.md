# Spec 18: Unified `yard` CLI

**Phase:** 8 — CLI consolidation
**Status:** approved design, ready for implementation planning
**Date:** 2026-05-01

---

## 1. Problem

The operator previously interacted with three separate binaries:

- `yard` — project bootstrap
- `tidmouth` — engine harness, indexing, web UI, auth
- `sodoryard` — chain orchestrator

That split operator-visible commands across 3 CLIs with no discoverability between them. The original intent (documented in project memory) was a single `yard` prefix for all operator-facing commands.

## 2. Goal

Consolidate all operator-facing commands under `yard`. One binary, one `--help`, one mental model. Under the no-legacy target state, `yard` is the only operator-facing CLI, `sodoryard` is removed as a duplicate public binary, and `tidmouth` is retained only for the minimal internal subprocess contract still required by chain execution.

## 3. Command tree

```
yard [--config yard.yaml]
├── init                        # project bootstrap (exists)
├── serve                       # web UI + API server
├── index                       # code index build/rebuild
├── auth                        # provider auth group
│   ├── login PROVIDER          # provider login flow; currently supports codex
│   └── status                  # auth status detail
├── doctor                      # auth diagnostics
├── config                      # show/validate config
├── chain                       # chain operations group
│   ├── start                   # start a chain execution; --role creates a one-step chain
│   ├── status                  # show chain status
│   ├── logs                    # show chain event log
│   ├── receipt                 # show orchestrator or step receipt
│   ├── cancel                  # cancel a running chain
│   ├── pause                   # pause a running chain
│   └── resume                  # resume a paused chain
├── brain                       # brain operations group
│   ├── index                   # brain indexing
│   └── serve                   # standalone brain MCP server over stdio
└── llm                         # local LLM service management
    ├── status                  # service health check
    ├── up                      # start local LLM services
    ├── down                    # stop local LLM services
    └── logs                    # show service logs
```

### 3.1 Global flags

`--config <path>` is a persistent flag on the root command, defaulting to `yard.yaml`. Every subcommand that needs config inherits it.

### 3.2 Naming decisions

| Current | New | Rationale |
|---|---|---|
| legacy chain start command | `yard chain start` | `chain` becomes a group; `start` is the action |
| legacy chain status command | `yard chain status` | nests under the chain group |
| legacy chain logs command | `yard chain logs` | nests under the chain group |
| legacy chain receipt command | `yard chain receipt` | nests under the chain group |
| legacy chain cancel command | `yard chain cancel` | nests under the chain group |
| legacy chain pause command | `yard chain pause` | nests under the chain group |
| legacy chain resume command | `yard chain resume` | nests under the chain group |
| legacy serve command | `yard serve` | top-level, most-used command |
| single-agent autonomous work | `yard chain start --role <role> --task <task>` | one-step chain; no separate public run model |
| `tidmouth run` | internal only | retained subprocess contract for chain steps until that contract is redesigned |
| `tidmouth index` | `yard index` | top-level, code indexing |
| `tidmouth index brain` | `yard brain index` | brain group owns brain indexing |
| `tidmouth brain-serve` | `yard brain serve` | brain group owns brain MCP |
| legacy auth command | `yard auth` | top-level group |
| legacy auth login command | `yard auth login codex` | provider login under auth group |
| legacy auth status command | `yard auth status` | same nesting |
| `tidmouth doctor` | `yard doctor` | top-level |
| legacy config command | `yard config` | top-level |
| legacy llm command group | `yard llm` | same group structure |
| legacy llm status/up/down/logs | `yard llm status/up/down/logs` | same nesting |

### 3.3 Flags and arguments

Every retained operator-facing subcommand preserves its existing flags and arguments unless this spec explicitly narrows the public surface. Compatibility-only install/substitution flows do not survive the no-legacy cleanup. A standalone public `yard run` command does not survive either; autonomous single-agent work is expressed as `yard chain start --role <role> ...` and recorded as a one-step chain.

Commands that currently take `configPath *string` as a constructor argument will receive it from the root persistent flag instead.

### 3.4 Current command-specific flags

- `yard index`: `--full` forces a full code-index rebuild, `--json` emits machine-readable results, and `--quiet` suppresses the human summary.
- `yard brain index`: rebuilds brain relational metadata and semantic chunks from the configured vault.
- `yard auth login codex`: starts the OpenAI Codex device-code login flow and writes credentials to Yard's private auth store. Other provider names currently fail with an unsupported-provider error.
- `yard llm status`: accepts `--json` and reports Docker, Compose, network, service health, problems, and remediation.
- `yard llm up`: ensures required services are healthy according to `local_services.mode`; `auto` may create networks and run `docker compose up -d`, while `manual` reports remediation.
- `yard llm down`: runs compose down for the configured stack.
- `yard llm logs`: accepts `--tail` for recent compose logs.
- `yard chain start`: accepts `--task`, `--specs`, optional `--role`, `--watch`, and `--verbosity normal|debug`; it prints the chain ID to stdout immediately and streams progress to stderr when watch is enabled. When `--role` is absent, it creates an orchestrator-managed chain. When `--role` is present, it creates a one-step chain and does not launch the orchestrator agent.
- `yard chain logs`: supports following a running chain and uses the same verbosity model as `chain start`.
- `yard chain status`: without a chain ID lists recent chains; with a chain ID shows chain and step detail.

### 3.5 Local LLM service management contract

`yard llm` is the operator surface for the configured `local_services` stack. The shipped manager reports and acts on these fields:

- `mode`: `off`, `manual`, or `auto`
- `compose_file` and `project_dir`
- Docker capability booleans: `docker_available`, `daemon_available`, `compose_available`, `compose_file_exists`
- `network_status`: map of required Docker network names to availability
- `services`: per-service `name`, `healthy`, `reachable`, `models_ready`, `required`, `base_url`, `health_url`, `models_url`, and optional `detail`
- `required_services`
- `problems` and `remediation`

`yard llm status --json` returns that `StackStatus` shape. `yard llm up` first computes status. In `off` or `manual` mode, unhealthy required services produce a manager error plus remediation. In `auto` mode, the manager may create configured required networks when `auto_create_networks` is true, run `docker compose up -d` for required services, poll health/model readiness until `startup_timeout_seconds`, and then return the final status. `yard llm down` runs compose down for the configured stack, and `yard llm logs --tail N` returns compose logs for configured services.

`yard index` uses this same local-service precheck path before retrieval/runtime validation that depends on local embedding or model services.

## 4. Architecture

### 4.1 Thin routing layer

`cmd/yard/` is a cobra command tree with no business logic. Each subcommand file imports the runtime builder and internal packages it needs, constructs the runtime, and delegates.

The pattern for each command:

```
cmd/yard/serve.go
  → imports internal packages (server, agent, config, db, etc.)
  → calls the same runtime construction + server startup that cmd/tidmouth/serve.go does
  → prints output via cobra's OutOrStdout()
```

### 4.2 Runtime builders

Today, the retained runtime builders are:

- `cmd/tidmouth/runtime.go` — `buildRuntime()` for the engine harness
- `internal/runtime/orchestrator.go` — shared chain orchestrator runtime construction used by `cmd/yard`

These need to be callable from `cmd/yard/`. After the cleanup, only `yard` and the minimal internal `tidmouth` engine wrapper should depend on the shared runtime builders.

**Approach chosen: extract to internal package.** Move the runtime construction into `internal/runtime/` (or `internal/harness/`) so both `cmd/yard/` and the legacy binaries can call it. This is the right move because:

1. It eliminates code duplication between `cmd/yard/` and the retained internal wrappers
2. It makes the runtime constructors testable in isolation
3. It provides the integration point for operator surfaces to start chains by calling the same runtime builder from the TUI or an HTTP handler

### 4.3 Spawn subprocess path

The chain orchestrator spawns engine subprocesses via `internal/spawn/`. Today, the spawn config references `tidmouth` as the engine binary name (now set in `internal/runtime/orchestrator.go`: `EngineBinary: "tidmouth"`).

The no-legacy contract keeps this as an internal implementation detail only. The operator invokes `yard chain start`; the orchestrator may continue spawning `tidmouth run` until that internal contract is redesigned. `tidmouth` is therefore retained only as an internal engine binary, not as a supported public CLI.

### 4.4 Shared operator chain entry points

The chain start logic must remain in `internal/` packages (not in cobra wiring) so that the TUI and any browser handlers can invoke the same code path as `yard chain start`. The live implementation delegates `yard chain start` to `internal/chainrun.Start`, whose shape is:

```go
func Start(ctx context.Context, cfg *config.Config, opts chainrun.Options, deps chainrun.Deps) (*chainrun.Result, error)
```

The Cobra command owns CLI-only work: loading flags, printing the chain ID, and streaming watch output. Chain creation, resume handling, active-execution registration, orchestrator loop setup, control-state finalization, and chain exit-code mapping live in the internal chain runner. TUI behavior is specified in [[20-operator-console-tui]], and browser inspection behavior is specified in [[21-web-inspector]].

## 5. What changes

### 5.1 New files

```
internal/runtime/
├── engine.go               # extracted from cmd/tidmouth/runtime.go
├── orchestrator.go         # shared chain orchestrator runtime construction
└── helpers.go              # shared helpers (buildProvider, ensureProjectRecord, etc.)

cmd/yard/
├── serve.go                # delegates to internal/runtime + internal/server
├── index.go                # delegates to internal/runtime + indexer
├── auth.go                 # auth + auth status subcommands
├── doctor.go               # auth diagnostics
├── config_cmd.go           # show/validate config (config.go is taken by Go convention)
├── chain.go                # chain group: start, one-step start, status, logs, receipt, cancel, pause, resume
├── brain.go                # brain group: index, serve
└── llm.go                  # llm group: status, up, down, logs
```

`cmd/yard/install.go` is intentionally absent from the target state; compatibility-only install/substitution flows are removed rather than preserved.

### 5.2 Modified files

```
cmd/yard/main.go            # register all new subcommands, add --config persistent flag
cmd/tidmouth/runtime.go     # thin wrapper calling internal/runtime/engine.go
internal/chainrun/          # shared chain starter handles orchestrated and one-step chains
Makefile                    # builds the retained artifact set after CLI cleanup
```

### 5.3 Unchanged

- Web frontend
- Docker infrastructure
- Templates, agent prompts, specs

The no-legacy target state does not preserve duplicated public command trees in `cmd/sirtopham/`, does not require `cmd/tidmouth/*.go` to remain operator-usable beyond the internal engine contract, and does not treat placeholder surfaces like `cmd/knapford/` as protected by this spec.

## 6. What doesn't change

- The runtime/business-logic split between CLI wiring and `internal/` packages
- The current internal spawn subprocess mechanism until a separate redesign replaces it (`tidmouth run` stays internal)
- The web frontend
- Any retained operator-facing command's flags or behavior, except removal of standalone public run
- The orchestrator-managed chain execution flow
- Brain read/write paths
- Database schema
- Docker infrastructure

## 7. Acceptance criteria

1. `yard --help` shows all supported operator command groups (init, serve, index, auth, doctor, config, chain, brain, llm) and does not advertise a standalone `run` command
2. `yard serve` starts the supported web UI/API server flow
3. `yard chain start --task "..." --config yard.yaml` runs the supported chain flow through the unified CLI
4. `yard chain start --role coder --task "..." --config yard.yaml` creates a one-step chain with a normal chain record, one step record, event log, receipt path, status output, and logs output
5. `yard brain index` indexes the brain identically to `tidmouth index brain`
6. `yard index` indexes code identically to `tidmouth index`
7. `yard llm status/up/down/logs` exposes the supported local-service management flow from the unified CLI
8. `yard auth status` exposes provider auth state from the unified CLI
9. `yard auth login codex` starts the supported Codex login flow from the unified CLI
10. `make build` and `make all` build the supported artifact set for the no-legacy target state; compatibility-only binaries are not acceptance requirements
11. `make test` green
12. No acceptance criterion requires `sodoryard` to remain as a working public binary
13. A chain started via `yard chain start` successfully spawns engine subprocesses (proving the internal spawn path still works)
14. `internal/runtime/` package exists with extracted runtime builders callable from `cmd/yard/` and any retained minimal internal engine wrapper

## 8. Out of scope

- Renaming the retained internal `tidmouth` engine binary
- Changing the spawn subprocess binary name in the same slice as public CLI cleanup
- Adding unrelated new top-level commands beyond separately specified surfaces such as `yard tui`
- Building the TUI or web-inspector surfaces themselves; this CLI spec only preserves the shared internal entry points used by [[20-operator-console-tui]] and [[21-web-inspector]]
- Deprecation warnings or compatibility aliases for removed legacy surfaces
- Changing retained operator-facing command behavior beyond the explicit chain-only run consolidation in this spec

## 9. Tag

`v0.8-unified-cli`
