# Spec 18: Unified `yard` CLI

**Phase:** 8 — CLI consolidation
**Status:** approved design, ready for implementation planning
**Date:** 2026-04-12

---

## 1. Problem

The operator currently interacts with three separate binaries:

- `yard` — project bootstrap (2 commands)
- `tidmouth` — engine harness, indexing, web UI, auth (10 commands)
- `sirtopham` — chain orchestrator (7 commands)

This splits 19 operator-visible commands across 3 CLIs with no discoverability between them. The original intent (documented in project memory) was a single `yard` prefix for all operator-facing commands.

## 2. Goal

Consolidate all operator-facing commands under `yard`. One binary, one `--help`, one mental model. Internal binaries (`tidmouth`, `sirtopham`) continue building for subprocess use but are not part of the documented operator surface.

## 3. Command tree

```
yard [--config yard.yaml]
├── init                        # project bootstrap (exists)
├── install                     # agents-dir substitution (exists)
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

`--config <path>` is a persistent flag on the root command, defaulting to `yard.yaml`. Every subcommand that needs config inherits it. This matches the existing pattern on both `tidmouth` and `sirtopham`.

### 3.2 Naming decisions

| Current | New | Rationale |
|---|---|---|
| `sirtopham chain` | `yard chain start` | `chain` becomes a group; `start` is the action |
| `sirtopham status` | `yard chain status` | nests under the chain group |
| `sirtopham logs` | `yard chain logs` | nests under the chain group |
| `sirtopham receipt` | `yard chain receipt` | nests under the chain group |
| `sirtopham cancel` | `yard chain cancel` | nests under the chain group |
| `sirtopham pause` | `yard chain pause` | nests under the chain group |
| `sirtopham resume` | `yard chain resume` | nests under the chain group |
| `tidmouth serve` | `yard serve` | top-level, most-used command |
| `tidmouth run` | `yard run` | top-level, headless single agent |
| `tidmouth index` | `yard index` | top-level, code indexing |
| `tidmouth index brain` | `yard brain index` | brain group owns brain indexing |
| `tidmouth brain-serve` | `yard brain serve` | brain group owns brain MCP |
| `tidmouth auth` | `yard auth` | top-level group |
| `tidmouth auth status` | `yard auth status` | same nesting |
| `tidmouth doctor` | `yard doctor` | top-level |
| `tidmouth config` | `yard config` | top-level |
| `tidmouth llm` | `yard llm` | same group structure |
| `tidmouth llm status/up/down/logs` | `yard llm status/up/down/logs` | same nesting |

### 3.3 Flags and arguments

Every subcommand preserves its existing flags and arguments exactly. No flag renames, no behavior changes. The only addition is the `--config` persistent flag inherited from the root.

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

Today, two runtime builders exist:

- `cmd/tidmouth/runtime.go` — `buildRuntime()` for the engine harness
- `cmd/sirtopham/runtime.go` — `buildOrchestratorRuntime()` for the chain orchestrator

These need to be callable from `cmd/yard/`. Two approaches:

**Approach chosen: extract to internal package.** Move the runtime construction into `internal/runtime/` (or `internal/harness/`) so both `cmd/yard/` and the legacy binaries can call it. This is the right move because:

1. It eliminates code duplication between `cmd/yard/` and `cmd/tidmouth/`/`cmd/sirtopham/`
2. It makes the runtime constructors testable in isolation
3. It keeps the door open for the web UI to start chains by calling the same runtime builder from an HTTP handler

### 4.3 Spawn subprocess path

The chain orchestrator spawns engine subprocesses via `internal/spawn/`. Today, the spawn config references `tidmouth` as the engine binary name (set in `cmd/sirtopham/runtime.go` line 190: `EngineBinary: "tidmouth"`).

**This does not change.** The spawned engine subprocess is `tidmouth run`, not `yard run`. The operator types `yard chain start`, but under the hood the orchestrator still spawns `tidmouth` subprocesses. `tidmouth` must remain on PATH for chains to work. The Makefile already builds it.

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
├── orchestrator.go         # extracted from cmd/sirtopham/runtime.go
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

### 5.2 Modified files

```
cmd/yard/main.go            # register all new subcommands, add --config persistent flag
cmd/tidmouth/runtime.go     # thin wrapper calling internal/runtime/engine.go
cmd/sirtopham/runtime.go    # thin wrapper calling internal/runtime/orchestrator.go
Makefile                    # no changes (already builds all 4 binaries)
```

### 5.3 Unchanged

- All `internal/` packages (except the new `internal/runtime/`)
- `cmd/tidmouth/*.go` subcommand files (they keep working, just undocumented)
- `cmd/sirtopham/*.go` subcommand files (same)
- `cmd/knapford/` (placeholder)
- Web frontend
- Docker infrastructure
- Templates, agent prompts, specs

## 6. What doesn't change

- The internal binary architecture
- The spawn subprocess mechanism (`tidmouth run` stays)
- The web frontend
- Any existing command's flags or behavior
- The chain execution flow
- Brain read/write paths
- Database schema
- Docker infrastructure

## 7. Acceptance criteria

1. `yard --help` shows all command groups (init, install, serve, run, index, auth, doctor, config, chain, brain, llm)
2. `yard serve` starts the web UI identically to `tidmouth serve`
3. `yard chain start --task "..." --config yard.yaml` runs a chain identically to `sirtopham chain --task "..."`
4. `yard brain index` indexes the brain identically to `tidmouth index brain`
5. `yard index` indexes code identically to `tidmouth index`
6. `yard llm status/up/down/logs` behaves identically to `tidmouth llm status/up/down/logs`
7. `yard auth status` behaves identically to `tidmouth auth status`
8. `make all` still builds all 4 binaries
9. `make test` green
10. Existing `tidmouth` and `sirtopham` binaries continue to work unchanged
11. A chain started via `yard chain start` successfully spawns engine subprocesses (proving the spawn path still works)
12. `internal/runtime/` package exists with extracted runtime builders callable from both `cmd/yard/` and `cmd/tidmouth/`/`cmd/sirtopham/`

## 8. Out of scope

- Renaming `tidmouth` or `sirtopham` internal binaries
- Changing the spawn subprocess binary name
- Adding new commands that don't exist today
- UI-driven chain execution (future Phase 6 work, but §4.4 keeps the door open)
- Deprecation warnings on `tidmouth`/`sirtopham` direct usage
- Changing any existing command's flags or behavior

## 9. Tag

`v0.8-unified-cli`
