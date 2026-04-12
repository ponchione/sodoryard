# Session handoff — sodoryard migration

**Date:** 2026-04-12
**Branch:** main
**Cwd:** /home/gernsback/source/sodoryard

> Read this cold. Everything you need to orient yourself is in here. If anything in this doc disagrees with current repo state, trust the repo and update this doc before acting.

---

## What this project is

Migrating `ponchione/sirtopham` (single-binary coding harness) into the `ponchione/sodoryard` monorepo. The GitHub repo has been renamed; the local directory is `/home/gernsback/source/sodoryard`; the git remote points at `git@github.com:ponchione/sodoryard.git`.

Target monorepo layout (all in place as of this handoff):

- **Tidmouth** — headless engine harness (`cmd/tidmouth/`)
- **SirTopham** — chain orchestrator (`cmd/sirtopham/`)
- **Yard** — unified operator-facing CLI (`cmd/yard/`) — **all 19 operator commands**
- **Knapford** — web dashboard (`cmd/knapford/`, placeholder until Phase 6)

The full migration roadmap is `sodor-migration-roadmap.md`.

---

## Current state of all phases

| Phase | Status | Tag |
|---|---|---|
| 0 — prep | done | `v0.1-pre-sodor` |
| 1 — monorepo restructure | done | `v0.2-monorepo-structure` |
| 2 — headless run command | done | (no separate tag) |
| 3 — SirTopham orchestrator | done | `v0.4-orchestrator` |
| 4 — system prompts | done | — |
| 5a — yard paths rename | done | `v0.2.1-yard-paths` |
| 5b — yard init | done | `v0.5-yard-init` |
| 6 — Knapford dashboard | deferred | — |
| 7 — yard containerization | done | `v0.7-containerization` |
| **8 — unified yard CLI** | **done** | `v0.8-unified-cli` |

---

## Phase 8 — complete

**Tag:** `v0.8-unified-cli`
**Commit range:** `f1cf1b6..14b453e` (8 implementation commits)

**What shipped:**
- `internal/runtime/` package — extracted shared runtime builders from `cmd/tidmouth/` and `cmd/sirtopham/` into 4 files:
  - `helpers.go` — ChainCleanup, EnsureProjectRecord, LoadRoleSystemPrompt, ResolveModelContextLimit
  - `provider.go` — BuildProvider, AliasedProvider, LogProviderAuthStatus, ErrorAsProviderError
  - `engine.go` — EngineRuntime + BuildEngineRuntime, BuildBrainBackend, BuildGraphStore, BuildConventionSource
  - `orchestrator.go` — OrchestratorRuntime + BuildOrchestratorRuntime, NoopContextAssembler, RegistryToolExecutor, BuildOrchestratorRegistry
- `cmd/tidmouth/` and `cmd/sirtopham/` refactored to thin wrappers calling `internal/runtime/`
- All 19 operator commands wired under `cmd/yard/`:
  - `yard init` / `yard install` (existing)
  - `yard serve` — web UI + API server
  - `yard run` — headless agent session
  - `yard index` — code index build/rebuild
  - `yard auth status` — provider auth inspection
  - `yard doctor` — auth diagnostics with ping
  - `yard config` — show/validate config
  - `yard llm status/up/down/logs` — local LLM service management
  - `yard brain index` — brain reindex
  - `yard brain serve` — standalone brain MCP server
  - `yard chain start/status/logs/receipt/cancel/pause/resume` — chain orchestration

**No deviations from plan.** All 16 tasks completed as specified.

**Verified:**
- `make all` green (4 binaries: tidmouth, sirtopham, knapford, yard)
- `make test` / `go test ./...` green
- `yard --help` shows all 11 command groups
- `yard chain --help` shows 7 subcommands
- `yard brain --help` shows 2 subcommands
- `yard llm --help` shows 4 subcommands
- Legacy `tidmouth --help` and `sirtopham --help` unchanged

---

## Phase 4 — complete

**What shipped:** production agent prompts (13 files, ~5KB each) with Thomas & Friends engine names:

| Role | Engine | File |
|---|---|---|
| Orchestrator | Sir Topham Hatt | `sirtophamhatt.md` |
| Planner | Gordon | `gordon.md` |
| Epic Decomposer | Edward | `edward.md` |
| Task Decomposer | Emily | `emily.md` |
| Coder | Thomas | `thomas.md` |
| Correctness Auditor | Percy | `percy.md` |
| Quality Auditor | James | `james.md` |
| Performance Auditor | Spencer | `spencer.md` |
| Security Auditor | Diesel | `diesel.md` |
| Integration Auditor | Toby | `toby.md` |
| Test Writer | Rosie | `rosie.md` |
| Resolver | Victor | `victor.md` |
| Docs Arbiter | Harold | `harold.md` |

---

## Deferred

### Phase 6 — Knapford dashboard

Web dashboard that consumes `.brain/`, `.yard/yard.db`, and chain state. The Phase 7 docker-compose.yaml has a profile-gated `knapford` service slot ready. Once Phase 6 ships, the profile gate is removed and `knapford` becomes a default-on service.

**Status:** the largest remaining phase. Needs decomposition into per-epic specs. Phase 4 prompts are now ready for dogfooding. With Phase 8 done, chain timelines/brain explorer/analytics should be added to `yard serve` rather than a separate binary.

---

## Recent commits

```
HEAD  14b453e  chore(yard): remove unused helper functions from auth.go and llm.go
      a1dc10f  feat(yard): add brain and chain command groups
      29b7228  feat(yard): add serve, run, index, auth, config, llm commands
      0022a7f  refactor(sirtopham): delegate to internal/runtime for shared helpers
      131e18e  refactor(tidmouth): delegate to internal/runtime for shared helpers
      6dfd10f  feat(runtime): extract orchestrator runtime builder into internal/runtime
      02f84fb  feat(runtime): extract engine runtime builder into internal/runtime
      ed5a18d  feat(runtime): extract shared helpers and provider construction into internal/runtime
      1aa7b93  docs: point next session at Phase 8 unified CLI plan
```

- Working tree at handoff time: intended clean; trust `git status` for the current local checkout state.
- `make test`: green
- `make all`: green (4 binaries: tidmouth, sirtopham, knapford, yard)
- Tags: `v0.1-pre-sodor`, `v0.2-monorepo-structure`, `v0.2.1-yard-paths`, `v0.4-orchestrator`, `v0.5-yard-init`, `v0.7-containerization`, `v0.8-unified-cli`
- **Not pushed.** User pushes manually.

---

## Operational notes

### Hard rules

- **Per-step commits** — don't batch multi-task work into one mega-commit.
- **Do not push** — the user pushes manually.
- **Do not skip git hooks** unless the user explicitly asks.

### The unified operator workflow

```bash
# Initialize a project
yard init
yard install

# Index code + brain
yard index
yard brain index

# Start the web UI
yard serve

# Run a chain
yard chain start --task "implement feature X" --config yard.yaml
yard chain status
yard chain logs <chain-id>
yard chain receipt <chain-id>

# Headless single agent
yard run --role thomas --task "fix the bug in auth.go"

# Auth diagnostics
yard auth status
yard doctor

# Local LLM management
yard llm status
yard llm up
yard llm down
yard llm logs

# Config inspection
yard config
```

### Running the containerized railway

```bash
# Build the image
docker compose build yard

# Initialize a project
PROJECT_DIR=/path/to/project docker compose run --rm yard yard init
PROJECT_DIR=/path/to/project docker compose run --rm yard yard install

# Run a chain (needs codex auth mounted)
PROJECT_DIR=/path/to/project docker compose run --rm \
  -v ~/.sirtopham:/root/.sirtopham:ro \
  yard sirtopham chain --config /project/yard.yaml --task "..."
```

### Where to find things

- **Templates:** `internal/initializer/templates/init/` (moved from repo-root `templates/init/` during Phase 5b)
- **Agent prompts:** `agents/` — 13 engine-named `.md` files
- **Runtime builders:** `internal/runtime/` — shared engine + orchestrator runtime construction
- **Specs:** `docs/specs/16-yard-init.md`, `docs/specs/17-yard-containerization.md`, `docs/specs/18-unified-yard-cli.md`
- **Plans:** `docs/plans/2026-04-12-phase-8-unified-yard-cli-implementation-plan.md`
- **Roadmap:** `sodor-migration-roadmap.md`
- **Tech debt:** `TECH-DEBT.md` (R5/R6/R7 closed; R1-R4 remain)

### Codex auth

Tokens in `~/.sirtopham/auth.json` expire 2026-04-13. Re-auth via `codex auth` if needed.

---

## Next session

1. **Daily-driver dogfooding** — point `yard` at a real project, run the full workflow (init → install → index → serve → chain), prove it works end-to-end through the UI.

2. **Phase 6 — Knapford features folded into `yard serve`** — chain timelines, brain explorer, analytics added to the existing web UI rather than a separate binary. Epic decomposition needed first.

3. **TECH-DEBT R1/R2/R3** — daily-driver validation, brain retrieval quality, index freshness UX.

---

## When in doubt

- Trust the repo over this doc.
- Per-step commits with clear messages are always safe.
- Don't push. Don't skip hooks. Don't expand scope.
