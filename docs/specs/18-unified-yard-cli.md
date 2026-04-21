# Spec 18: Unified `yard` CLI

**Phase:** 8 — CLI consolidation
**Status:** approved design, ready for implementation planning
**Date:** 2026-04-12

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
├── run                         # single headless agent session
├── index                       # code index build/rebuild
├── auth                        # provider auth group
│   └── status                  # auth status detail
├── doctor                      # auth diagnostics
├── config                      # show/validate config
├── chain                       # chain operations group
│   ├── start                   # start a new chain execution
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
| `tidmouth run` | `yard run` | top-level, headless single agent |
| `tidmouth index` | `yard index` | top-level, code indexing |
| `tidmouth index brain` | `yard brain index` | brain group owns brain indexing |
| `tidmouth brain-serve` | `yard brain serve` | brain group owns brain MCP |
| legacy auth command | `yard auth` | top-level group |
| legacy auth status command | `yard auth status` | same nesting |
| `tidmouth doctor` | `yard doctor` | top-level |
| legacy config command | `yard config` | top-level |
| legacy llm command group | `yard llm` | same group structure |
| legacy llm status/up/down/logs | `yard llm status/up/down/logs` | same nesting |

### 3.3 Flags and arguments

Every retained operator-facing subcommand preserves its existing flags and arguments exactly. Compatibility-only install/substitution flows do not survive the no-legacy cleanup.

Commands that currently take `configPath *string` as a constructor argument will receive it from the root persistent flag instead.

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
3. It keeps the door open for the web UI to start chains by calling the same runtime builder from an HTTP handler

### 4.3 Spawn subprocess path

The chain orchestrator spawns engine subprocesses via `internal/spawn/`. Today, the spawn config references `tidmouth` as the engine binary name (now set in `internal/runtime/orchestrator.go`: `EngineBinary: "tidmouth"`).

The no-legacy contract keeps this as an internal implementation detail only. The operator invokes `yard chain start`; the orchestrator may continue spawning `tidmouth run` until that internal contract is redesigned. `tidmouth` is therefore retained only as an internal engine binary, not as a supported public CLI.

### 4.4 Future: UI-driven chains

The chain start logic must remain in `internal/` packages (not in cobra wiring) so that a future HTTP handler at `/api/chain/start` can invoke the same code path. The Phase 8 implementation should verify that `yard chain start` delegates to a function signature like:

```go
func StartChain(ctx context.Context, cfg *config.Config, opts ChainStartOpts) error
```

The actual extraction of `buildOrchestratorRuntime` into a shared package (§4.2) naturally enables this. No additional work needed in Phase 8 beyond the extraction itself.

## 5. What changes

### 5.1 New files

```
internal/runtime/
├── engine.go               # extracted from cmd/tidmouth/runtime.go
├── orchestrator.go         # shared chain orchestrator runtime construction
└── helpers.go              # shared helpers (buildProvider, ensureProjectRecord, etc.)

cmd/yard/
├── serve.go                # delegates to internal/runtime + internal/server
├── run.go                  # delegates to internal/runtime + agent loop
├── index.go                # delegates to internal/runtime + indexer
├── auth.go                 # auth + auth status subcommands
├── doctor.go               # auth diagnostics
├── config_cmd.go           # show/validate config (config.go is taken by Go convention)
├── chain.go                # chain group: start, status, logs, receipt, cancel, pause, resume
├── brain.go                # brain group: index, serve
└── llm.go                  # llm group: status, up, down, logs
```

`cmd/yard/install.go` is intentionally absent from the target state; compatibility-only install/substitution flows are removed rather than preserved.

### 5.2 Modified files

```
cmd/yard/main.go            # register all new subcommands, add --config persistent flag
cmd/tidmouth/runtime.go     # thin wrapper calling internal/runtime/engine.go
Makefile                    # builds the retained artifact set after CLI cleanup
```

### 5.3 Unchanged

- All `internal/` packages (except the new `internal/runtime/`)
- Web frontend
- Docker infrastructure
- Templates, agent prompts, specs

The no-legacy target state does not preserve duplicated public command trees in `cmd/sirtopham/`, does not require `cmd/tidmouth/*.go` to remain operator-usable beyond the internal engine contract, and does not treat placeholder surfaces like `cmd/knapford/` as protected by this spec.

## 6. What doesn't change

- The runtime/business-logic split between CLI wiring and `internal/` packages
- The current internal spawn subprocess mechanism until a separate redesign replaces it (`tidmouth run` stays internal)
- The web frontend
- Any retained operator-facing command's flags or behavior
- The chain execution flow
- Brain read/write paths
- Database schema
- Docker infrastructure

## 7. Acceptance criteria

1. `yard --help` shows all supported operator command groups (init, serve, run, index, auth, doctor, config, chain, brain, llm)
2. `yard serve` starts the supported web UI/API server flow
3. `yard chain start --task "..." --config yard.yaml` runs the supported chain flow through the unified CLI
4. `yard brain index` indexes the brain identically to `tidmouth index brain`
5. `yard index` indexes code identically to `tidmouth index`
6. `yard llm status/up/down/logs` exposes the supported local-service management flow from the unified CLI
7. `yard auth status` exposes provider auth state from the unified CLI
8. `make all` builds the supported artifact set for the no-legacy target state; compatibility-only binaries are not acceptance requirements
9. `make test` green
10. No acceptance criterion requires `sodoryard` to remain as a working public binary
11. A chain started via `yard chain start` successfully spawns engine subprocesses (proving the internal spawn path still works)
12. `internal/runtime/` package exists with extracted runtime builders callable from `cmd/yard/` and any retained minimal internal engine wrapper

## 8. Out of scope

- Renaming the retained internal `tidmouth` engine binary
- Changing the spawn subprocess binary name in the same slice as public CLI cleanup
- Adding new commands that don't exist today
- UI-driven chain execution (future Phase 6 work, but §4.4 keeps the door open)
- Deprecation warnings or compatibility aliases for removed legacy surfaces
- Changing any retained operator-facing command's flags or behavior

## 9. Tag

`v0.8-unified-cli`
