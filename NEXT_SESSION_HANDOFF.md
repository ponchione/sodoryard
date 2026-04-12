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
- **Yard** — operator-facing CLI for project bootstrap (`cmd/yard/`, fully shipped — `yard init`)
- **Knapford** — web dashboard (`cmd/knapford/`, placeholder until Phase 6)

The full migration roadmap is `sodor-migration-roadmap.md`. Read phases 0–7 before touching code.

---

## Current state of all phases

| Phase | Status | Tag |
|---|---|---|
| 0 — prep | done | `v0.1-pre-sodor` |
| 1 — monorepo restructure | done | `v0.2-monorepo-structure` |
| 2 — headless run command | done | (no separate tag) |
| 3 — SirTopham orchestrator | done | `v0.4-orchestrator` |
| 4 — system prompts | deferred (handled out-of-band) | — |
| 5a — yard paths rename | done | `v0.2.1-yard-paths` |
| **5b — yard init** | **done** | `v0.5-yard-init` |
| 6 — Knapford dashboard | deferred (waiting on Phase 4) | — |
| **7 — yard containerization** | spec + plan ready, NOT executed | (will be `v0.7-containerization`) |

---

## Phase 5b — complete

**Tag:** `v0.5-yard-init`
**Commit range:** `30217d8..91d57ac` (10 commits on top of `v0.4-orchestrator` cleanup stack)

**What shipped:**
- New `cmd/yard/` top-level binary (`yard init` subcommand only)
- New `internal/initializer/` package — embedded `templates/init/` via `//go:embed all:templates/init`, substitution helpers, Obsidian config writer, gitignore patcher, database bootstrap, `Run()` orchestrator
- `templates/init/` moved from repo root into `internal/initializer/templates/init/` (go:embed requires paths relative to source file's package dir)
- `templates/init/yard.yaml.example` rewritten with all 13 `agent_roles` entries seeded with `{{SODORYARD_AGENTS_DIR}}` placeholders (quoted for YAML validity pre-substitution)
- Deletion of `cmd/tidmouth/init.go` and `cmd/tidmouth/init_test.go`
- Makefile `yard:` target with the same FTS5/lancedb cgo wiring as `tidmouth:` and `sirtopham:`

**Verified live:** smoke test in `/tmp/yard-init-smoke-*` — empty bootstrap, idempotent re-run, `{{SODORYARD_AGENTS_DIR}}` substitution, end-to-end sirtopham chain with receipts produced by both orchestrator and correctness-auditor.

**Implementation deviation from plan:** the plan assumed `//go:embed all:templates/init` would reach `templates/init/` at the repo root from `internal/initializer/`. Go's embed only resolves paths relative to the source file's package directory and does not follow symlinks, so the templates tree was moved to `internal/initializer/templates/init/`. All embed paths and tests remained unchanged because the relative reference `templates/init/` resolves correctly from its new location.

---

## Phase 7 — ready to execute

**Spec:** `docs/specs/17-yard-containerization.md` (407 lines)
**Plan:** `docs/plans/2026-04-11-phase-7-yard-containerization-implementation-plan.md` (1288 lines)

**Depends on:** Phase 5b (fully landed and tagged).

**What it ships:**
- New `cmd/yard/install.go` subcommand — substitutes `{{SODORYARD_AGENTS_DIR}}` in `yard.yaml`
- New `internal/initializer/install.go` + tests
- New `Dockerfile` — three-stage build
- New `docker-compose.yaml` at the repo root
- New `.dockerignore`

**Locked decisions** (do not re-litigate during execution):
1. Headless-only Phase 7
2. `yard install` is the only command that performs the agents-dir substitution
3. `debian:bookworm-slim` runtime base
4. amd64 only
5. Two compose files, share `llm-net` external network
6. `liblancedb_go.so` staged at `/usr/local/lib/` + `ldconfig`
7. Image filesystem: binaries at `/usr/local/bin/`, agent prompts at `/opt/yard/agents/`, project bind-mounted at `/project`
8. Tag is `v0.7-containerization`

**Note for Phase 7 plan:** the plan references `templates/init/` at the repo root in several places (e.g., `.dockerignore` rules, builder stage embed). These references need to be updated to `internal/initializer/templates/init/` since Phase 5b moved the templates.

---

## Deferred (not in this session)

### Phase 4 — system prompts

The 13 agent prompt files in `agents/` are operational stubs. Phase 4 expands them into production prompts. The user is handling Phase 4 out-of-band. Do NOT touch `agents/` files.

### Phase 6 — Knapford dashboard

Web dashboard. Deferred until Phase 4 prompts are ready for dogfooding. The Phase 7 docker-compose.yaml has a profile-gated `knapford` service slot waiting for Phase 6.

---

## Recent commits

```
HEAD  91d57ac  refactor(tidmouth): remove init subcommand, replaced by yard init
      8d4afd4  build: add yard binary target with FTS5 and lancedb cgo flags
      f2d71e2  feat(yard): add cmd/yard binary with init subcommand
      d9010ae  feat(initializer): add Run() entrypoint that orchestrates init
      21768b8  feat(initializer): add EnsureDatabase for .yard/yard.db bootstrap
      bfe7318  feat(initializer): add EnsureGitignoreEntries for .yard/ and .brain/
      c88b95e  feat(initializer): add EnsureObsidianConfig for .brain/.obsidian/
      1b6da3f  feat(initializer): add placeholder substitution at copy time
      d0b6495  feat(initializer): embed templates/init/ via go:embed
      30217d8  feat(templates/init): seed all 13 agent_roles in yard.yaml template
      7866938  docs: rewrite NEXT_SESSION_HANDOFF.md from scratch        ← pre-5b
```

- Working tree: clean
- `make test`: green
- `make all`: green (4 binaries: tidmouth, sirtopham, knapford, yard)
- Tags: `v0.1-pre-sodor`, `v0.2-monorepo-structure`, `v0.2.1-yard-paths`, `v0.4-orchestrator`, `v0.5-yard-init`
- **Not pushed.** User pushes manually.

---

## Operational notes

### Hard rules

- **Per-step commits** — don't batch multi-task work into one mega-commit.
- **Do not push** — the user pushes manually.
- **Do not skip git hooks** unless the user explicitly asks.
- **Do not touch `agents/`** — Phase 4 prompts are being handled out-of-band.
- **Do not touch `cmd/sirtopham/` or `internal/{chain,spawn,receipt}/`** beyond the TECH-DEBT items unless the user explicitly asks.
- **Do not modify `ops/llm/docker-compose.yml`** in Phase 7.

### Pre-flight checks before any smoke test

Local llama.cpp services (only needed if dogfooding against local LLM):

```bash
curl -s --max-time 3 http://localhost:12434/v1/models | head -c 80
curl -s --max-time 3 http://localhost:12435/v1/models | head -c 80
```

Codex auth:

```bash
./bin/tidmouth auth status
```

Should show `codex (codex): healthy` with non-expired tokens. As of 2026-04-12 the tokens expire 2026-04-13 — re-auth via `codex auth` if needed.

### Where to find things

- **Current product specs:** `docs/specs/01-15` (numbered) plus `docs/specs/16-yard-init.md` and `docs/specs/17-yard-containerization.md`.
- **Ready-to-execute implementation plans:** `docs/plans/2026-04-11-phase-7-yard-containerization-implementation-plan.md`. Phase 5b plan is now historical (completed).
- **Roadmap:** `sodor-migration-roadmap.md` (overall phase plan, still authoritative).
- **Tech debt:** `TECH-DEBT.md` (R5/R6/R7 are open Phase 3 follow-ups; R1–R4 are pre-existing).
- **Live validation procedures:** `MANUAL_LIVE_VALIDATION.md` at the repo root.
- **Templates:** `internal/initializer/templates/init/` (moved from repo-root `templates/init/` during Phase 5b).

### Tools and services this project uses

- **llama.cpp** at `localhost:12434` (qwen-coder-7b) and `localhost:12435` (nomic-embed-code). Managed via `ops/llm/docker-compose.yml`. Optional.
- **Codex auth** stored in `~/.sirtopham/auth.json`. Status check: `./bin/tidmouth auth status`.
- **LanceDB** via `lib/linux_amd64/liblancedb_go.so`. Tests need the env vars set by `make test`.
- **sqlc** generates `internal/db/*.sql.go` from `internal/db/query/*.sql`.

### Next session

The next session should pick one of:

1. **Execute Phase 7** — work through `docs/plans/2026-04-11-phase-7-yard-containerization-implementation-plan.md` task by task. Note: plan references to `templates/init/` need updating to `internal/initializer/templates/init/`. End state: `Dockerfile` and `docker-compose.yaml` at the repo root, `yard install` command exists, container smoke chain passes end-to-end, tag `v0.7-containerization`.

2. **TECH-DEBT cleanup** — work through R5/R6/R7 (Phase 3 orchestrator follow-ups). Each is small (one to two commits).

Phase 4 (prompts) and Phase 6 (Knapford) are NOT for this conversation series.

---

## When in doubt

- Trust the repo over this doc.
- Per-step commits with clear messages are always safe.
- Don't push. Don't skip hooks. Don't expand scope.
