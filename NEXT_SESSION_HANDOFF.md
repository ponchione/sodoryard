# Session handoff — sodoryard migration, start of Phase 3

Date: 2026-04-11
Branch: main
Cwd: /home/gernsback/source/sodoryard

> Read this cold. Everything you need to orient yourself is in here. If anything
> in this doc disagrees with current repo state, trust the repo and update this
> doc before acting.

> **Phase 5a (yard paths rename) landed this session.** See the "Phase 5a
> complete" section below. The next task is Phase 3 — SirTopham orchestrator —
> per `docs/specs/15-chain-orchestrator.md`.

---

## What this project is

Migrating `ponchione/sirtopham` (single-binary coding harness) into the
`ponchione/sodoryard` monorepo. GitHub repo has been renamed; local dir is now
`/home/gernsback/source/sodoryard`; git remote already points at
`git@github.com:ponchione/sodoryard.git`.

Target monorepo layout:

- **Tidmouth** — headless engine harness (`cmd/tidmouth/`, renamed from current `cmd/sirtopham/`)
- **SirTopham** — orchestrator binary that runs chains (future `cmd/sirtopham/main.go`, not yet implemented)
- **Knapford** — web dashboard (future `cmd/knapford/main.go`, not yet implemented)

**Full plan: `sodor-migration-roadmap.md`.** Read phases 0–7 before touching code.

Other required reading:

- `conductor-v1-extraction.md` — what to lift from the archived conductor repo
- `docs/specs/13_Headless_Run_Command.md` — spec for the landed `run` command
- `docs/specs/14_Agent_Roles_and_Brain_Conventions.md` — spec for role config + brain write scoping
- `docs/specs/15-chain-orchestrator.md` — spec for the future SirTopham orchestrator (Phase 3)
- `docs/plans/2026-04-11-headless-run-command-implementation-plan.md` — plan that drove the run command
- `docs/plans/2026-04-11-agent-roles-and-brain-conventions-implementation-plan.md` — plan that drove the role/brain-scoping work
- `docs/agent-roles-and-brain-conventions.md` + `docs/agent-role-conductor-boundary.md` — operator-facing boundary docs

---

## Milestones reached

- **Phase 0 — complete.** Tag `v0.1-pre-sodor` at commit `1338611` marks the
  state before any sodoryard migration or brain-rebuild work.
- **Brain system rebuild — complete.** Canonical parser, derived-state indexer,
  heading-aware brain chunks, hybrid runtime brain searcher (keyword + semantic
  + graph/backlink), layout-intent routing, explicit `sirtopham index brain`
  path. Six maintained validation scenarios stay green live on the rebuilt
  runtime. Narrative preserved in git at commits `4a63ad8` and `1338611`.
- **Phase 2 spec-13 headless run command — landed and smoke-tested end to
  end.** The "Phase 2 acceptance items verified live" section below lists
  what the live smoke test proved. Test-only gaps (custom-tools rejection,
  exit-code paths, `--task-file`) were later closed in commit `447bf5e` —
  see "Bugs fixed / debt paid this session" below.
- **Phase 2 spec-14 agent roles and brain conventions — landed.**
  `file:read` tool group split, `internal/role/builder.go`, brain write path
  enforcement, agent prompt stubs in `agents/`, role set wired into
  `yard.yaml` (formerly `sirtopham.yaml`), boundary docs. Prompt stubs are
  operational but minimal — production prompts are Phase 4, not this session.
- **Phase 1 monorepo restructure — complete.** Tag `v0.2-monorepo-structure`
  marks the final Phase 1 commit. Module path is `github.com/ponchione/sodoryard`,
  `cmd/tidmouth/` hosts the headless engine, `cmd/sirtopham/` and `cmd/knapford/`
  are placeholder binaries awaiting Phase 3 and Phase 6, `templates/init/` has
  the scaffold for `yard init`, and the Makefile builds all three binaries.
- **Phase 5a yard paths rename — complete.** Tag `v0.2.1-yard-paths` at the
  most recent Phase 5a commit. Every project now uses the canonical `.yard/`
  state directory with `yard.yaml` config and `yard.db` SQLite file, regardless
  of project basename. The codeintel chunk-label derivation (`ProjectName()`)
  is intentionally kept basename-derived — that concern is split from the
  state dir name. See `docs/plans/2026-04-11-yard-paths-rename-implementation-plan.md`
  for the task-by-task plan that drove this work.

---

## Repo state at handoff

```
HEAD  c080551 fix(runtime): bootstrap db schema before running upgrade helpers   ← tag v0.2.1-yard-paths
      b5ed4d4 chore: update gitignore and comment references to yard paths
      88e453e refactor: rename repo config files to yard.yaml
      3cf5239 refactor(init): generate canonical .yard/yard.db and yard.yaml
      d12b69c refactor(cli): default --config flag to yard.yaml
      afdb951 refactor(indexstate): resolve state file under canonical .yard dir
      23cb30e docs(config): document DefaultConfigFilename unused parameter
      1d37c9c refactor(config): hardcode yard state dir and database name
      b092dc0 docs: add phase 5a yard paths rename implementation plan
      8f4b12d docs: refine rename debt notes and phase 1 smoke test phrasing
      2ed9fe8 refactor(config): derive state-dir exclude pattern per project
      447bf5e refactor(tidmouth): extract newRootCmd and cover run headless gaps
      fedd1e1 build: rework Makefile for three-binary monorepo   ← tag v0.2-monorepo-structure
      1338611 docs: refresh brain validation handoff   ← tag v0.1-pre-sodor
```

- Working tree: clean
- `make test`: green
- `make build`: green
- Tags: `v0.1-pre-sodor`, `v0.2-monorepo-structure`, `v0.2.1-yard-paths`
- **Not pushed.** User pushes manually.

---

## Bugs fixed / debt paid across sessions — read before starting Phase 3

### 0. Runtime DB schema bootstrap (phase 5a task 7)

File: `cmd/tidmouth/runtime.go`, commit `c080551`.

`buildAppRuntime` opened the database and immediately called
`EnsureMessageSearchIndexesIncludeTools` + `EnsureContextReportsIncludeTokenBudget`,
which assume the base schema already exists. Against a pre-initialized state
dir this worked fine, but against a fresh `.yard/yard.db` — the common case
after the phase 5a rename — it failed at the second helper with
`no such table: context_reports`. `OpenDB` creates the file via `MkdirAll` but
does not create any tables.

Fix: call `appdb.InitIfNeeded` right after `OpenDB`. That helper is a no-op
when the schema already exists, so this is safe for initialized projects and
correct for uninitialized ones. **If you add new schema-upgrade helpers in
`buildAppRuntime`, they must run AFTER `InitIfNeeded`.**

Surfaced by the phase 5a task 7 regression smoke test against `my-website`.

### Phase 2 era bugs (preserved from earlier handoff)


### 1. Closure capture-by-reference in cleanup chains

File: `cmd/sirtopham/runtime.go` (→ `cmd/tidmouth/runtime.go` after Step 1.2).

`buildAppRuntime` used a single `prevCleanup` variable that was reassigned four
times and captured by four closures. At teardown the shared variable held the
last extension (`func5`, the brainBackend cleanup closure), so `func5` called
itself recursively until the goroutine stack hit 1 GB. The bug had been latent
for weeks because `sirtopham serve` is SIGINT-terminated and never runs the
deferred cleanup chain; `sirtopham run` is the first command with a normal exit
path that exercises it.

Fix: `chainCleanup(prev, next)` helper that takes `prev` as a value parameter,
giving each extension a fresh copy. **If you extend the cleanup chain again,
use `chainCleanup` — never reassign a shared `prev` variable.**

### 2. IsError silently swallowed by mcpclient

File: `internal/brain/mcpclient/client.go`.

Vault methods (`ReadDocument`, `WriteDocument`, `PatchDocument`, `SearchKeyword`,
`ListDocuments`) ignored `res.IsError`. When a handler returned a Go error
(e.g. `vault.ReadDocument` returning `"Document not found: <path>"`), MCP
packaged it as `CallToolResult{IsError: true, Content: [TextContent{...}]}`
and returned it as an RPC success. Our client ran `decodeStructured` on empty
`StructuredContent` and returned `("", nil)` — silently. This made `receipt.go`
believe a nonexistent receipt file existed with empty content and fail the
"missing YAML frontmatter" check instead of taking the fallback-receipt path.
It would also have hidden any handler-side write/patch error.

Fix: `toolResultError(res)` helper called after every `session.CallTool` in the
vault methods. The vault's exact phrase `"Document not found: <path>"` is
preserved in the resulting Go error, so `receipt.go`'s
`strings.Contains(err.Error(), "Document not found")` check keeps working.
**Don't add new vault methods without that check.**

### 3. Role receipt/logs allow-list convention

File: `sirtopham.yaml`.

The default receipt path is `receipts/{role}/{chain-id}.md`. Seven role
allow-lists used shortened directory names (`receipts/correctness/**`,
`receipts/tests/**`, `receipts/arbiter/**`, etc.) that did not match the
default. Fallback receipts would have been rejected by the brain write policy.
Fix applied for all seven mismatched roles. **Every role's `brain_write_paths`
must include `receipts/{full-role-name}/**` and `logs/{full-role-name}/**`.
Enforce this convention on every future role config edit.**

### 4. Stale `project_root` in sirtopham.yaml

`project_root:` pointed at the old `/home/gernsback/source/sirtopham` dir and
config loading failed on `brain.vault_path` resolution. Fixed as a one-line
commit. Phase 1 will rename the file itself.

---

## Phase 2 acceptance items verified live

The smoke test proved:

- `sirtopham run` exists and is CLI-registered
- `--role` selects a config-defined role
- Role `system_prompt` path resolves (absolute + project-root-relative both work)
- Role-scoped tool registry is enforced
- Brain write allow/deny policy is enforced on writes/updates only
- Command runs one headless turn via existing `AgentLoop`
- Receipt validated if present
- Last stdout line is the receipt path on exit 0
- `make test` passes
- `make build` passes

Not verified live (lower risk; should be covered by `cmd/sirtopham/run_test.go`):

- `--quiet` suppresses stderr progress
- Exit 2 (safety limit: timeout / max-turns / max-tokens)
- Exit 3 (explicit escalation verdict)
- `custom_tools` runtime rejection
- `--task-file` variant
- Hard `--max-tokens` enforcement mid-turn

---

## Next task: Phase 3 — SirTopham orchestrator

**Goal:** Build `cmd/sirtopham/` into the real chain orchestrator binary per
`docs/specs/15-chain-orchestrator.md`. Today it is a stub `main.go` that
prints a placeholder.

**Design is already written.** Read `docs/specs/15-chain-orchestrator.md`
before touching code. Key pieces Phase 3 introduces:

1. `internal/receipt/` — frontmatter parser. Pure, no upstream deps. Ship first.
2. `internal/chain/` — SQLite schema (`chains`, `steps`, `events` tables),
   migrations, state transitions, limit enforcement, event logging. Schema
   patterns adapted from conductor v1 per `conductor-v1-extraction.md`.
3. `internal/spawn/` — `spawn_engine` tool implementation. Execs `tidmouth run`
   as a subprocess, waits, reads receipt from brain, updates chain state.
4. `chain_complete` tool + headless driver termination signal — small, but has
   to hook into the existing `AgentLoop` cleanly. Watch out for the closure
   capture-by-reference bug pattern from the Phase 2 fix (use `chainCleanup`).
5. `cmd/sirtopham/` CLI — `chain`, `status`, `logs`, `receipt`, `pause`,
   `resume`, `cancel` subcommands. Boot the orchestrator as a headless Tidmouth
   session with the custom tool registry.
6. `agents/orchestrator.md` — enough to run a minimal chain end-to-end.
   NOT the production prompt; that iteration is Phase 4.
7. Reindex hooks (`reindex_before` boolean on `spawn_engine`) — last, because
   you need the rest working before you can prove the hook is useful.

**Database path:** `.yard/yard.db` is the Tidmouth database. Spec 15 also
references `.yard/sirtopham.db` but the user locked the decision at
`.yard/yard.db` (single shared DB for now — reevaluate only if concurrent
writers become a problem). Update spec 15 on first Phase 3 touch.

**User-chosen constraints (do not deviate without asking):**

- Per-step commits as needed — not one big commit
- Do NOT auto-push — the user pushes manually
- Phase 3 work happens on `main` directly (same pattern as Phases 1 and 5a)
- Regression smoke test (see below) must still pass as the final gate
  before tagging `v0.4-orchestrator`
- The Phase 4 production prompts are OUT OF SCOPE for Phase 3 — stub prompts
  in `agents/orchestrator.md` are fine for the first smoke test

**Where Phase 5b and Phase 4 sit:** Phase 5b (`yard init` CLI) lands after
Phase 3 because its smoke test exercises a real chain. Phase 4 (prompt
engineering) happens during/after Phase 3 and is mostly human work with LLM
assistance, not autonomous agent work. See `sodor-migration-roadmap.md` for
the full phase layout.

---

## Historical: Phase 1 step plan (for archaeology)

The Phase 1 step plan below was the plan that drove the monorepo restructure.
Phase 1 is complete as of tag `v0.2-monorepo-structure` at commit `fedd1e1`,
and Phase 5a is complete as of tag `v0.2.1-yard-paths` at commit `c080551`.
Everything below this line is preserved as reference.

### Step 1.1 — `go.mod` module path rename

- Change `module github.com/ponchione/sirtopham` → `module github.com/ponchione/sodoryard`
- Sweep-rewrite every `github.com/ponchione/sirtopham/...` import to
  `github.com/ponchione/sodoryard/...` (use `gofmt -r` or a sed pass; verify
  with `grep -r 'ponchione/sirtopham' --include '*.go'`)
- `make build` + `make test` must stay green
- Commit: `refactor: rename module to github.com/ponchione/sodoryard`

### Step 1.2 — move `cmd/sirtopham/` → `cmd/tidmouth/`

- Use `git mv` to preserve history for every file in `cmd/sirtopham/`:
  `main.go`, `serve.go`, `run.go`, `runtime.go`, `receipt.go`, `run_progress.go`,
  `index.go`, `init.go`, `auth.go`, `config.go`, `llm.go`, `doctor.go`,
  `brain_serve.go`, plus all `*_test.go` siblings
- Leave `cmd/sirtopham/` to be re-created in Step 1.5 as the orchestrator
  placeholder
- Makefile will still point at `./cmd/sirtopham` briefly — Step 1.8 fixes that
- `make build` (may need a one-line Makefile tweak first) + `make test`
- Commit: `refactor: move cmd/sirtopham to cmd/tidmouth`

### Step 1.3 — `internal/` packages

- Stay in place. Import-path fix was done in Step 1.1.
- No action. Verify.

### Step 1.4 — supporting files

- `web/`, `webfs/`, `ops/`, `scripts/`, `docs/specs/`, `.brain/` all stay.
- No action. Verify.

### Step 1.5 — placeholder `cmd/sirtopham/` (orchestrator) and `cmd/knapford/` (dashboard)

- Create minimal `main.go` in each — e.g. `package main; func main() { fmt.Println("sirtopham orchestrator placeholder") }` and similar for knapford
- These compile and produce binaries that print a stub message
- Commit: `feat: add sirtopham and knapford binary placeholders`

### Step 1.6 — `agents/` directory

- Already exists with 13 operational stubs. No action. Verify.

### Step 1.7 — `templates/init/` (in scope per user)

- Create `templates/init/sodor.yaml.example` (a minimal example config)
- Create `templates/init/brain/{specs,architecture,epics,tasks,plans,receipts,logs,conventions}/.gitkeep`
- This template is what `sodor init` will copy into new projects in Phase 5
- Commit: `feat: add templates/init scaffold for sodor init`

### Step 1.8 — Makefile updates

- Rename the `make sirtopham` target to `make tidmouth` → builds `./cmd/tidmouth`
- Add `make sirtopham` → builds `./cmd/sirtopham` (new placeholder)
- Add `make knapford` → builds `./cmd/knapford` (new placeholder)
- `make all` builds all three
- `make test` / `make build` / `make dev-frontend` / `make dev-backend` still work
- The `dev-backend` target currently runs `sirtopham serve --dev`; rename to
  `tidmouth serve --dev` for now (serve gets removed entirely later in Phase 6)
- Commit: `build: rename make targets for monorepo binaries`

### Step 1.9 — verify and tag

- `make build` produces `bin/tidmouth`, `bin/sirtopham`, `bin/knapford`
- `make test` green
- `./bin/tidmouth run --help` works
- **Regression smoke test (see below) passes end to end on `./bin/tidmouth run`**
- Tag `v0.2-monorepo-structure` on the final Phase 1 commit
- Do not push

### Rename debt deferred out of Phase 1

The user decision on 2026-04-11 to use `yard` as the operator-facing CLI
prefix (rather than `sodor` or `sodoryard`) reshapes these items — they
move from "derive filename from project dir name" to "hardcoded `yard`
constant", which lands them closer to Phase 5 (`yard init`) scaffolding
than to Phase 1 mechanical restructuring. Phase 1 now stays minimal.

- `internal/config/config.go:484` — `DatabasePath()` currently returns
  `filepath.Join(c.StateDir(), "sirtopham.db")`, and `StateDir()` returns
  `.<ProjectName()>/` which resolves to `.sodoryard/` in practice. Eventually
  this becomes `.yard/yard.db` (hardcoded). Deferred to Phase 5.
- Config filename `sirtopham.yaml` → `yard.yaml` (hardcoded, not derived
  from project dir name). The Tidmouth CLI currently keeps its explicit default
  at `sirtopham.yaml` so Phase 1/2 behavior stays aligned with the checked-in
  config; the broader filename model still gets replaced rather than reconciled
  in Phase 5. Phase 1 leaves `sirtopham.yaml` in place.
- Review whether `project_root:` in the config should stay absolute or move
  to relative / derived. Non-blocking; defer.

### What NOT to rename in Phase 1

At this point there is zero `sodor` CLI prefix in Go code — the `yard`
decision is currently a docs-only alignment. Proper nouns that stay
unchanged in Phase 1 (and, per user, forever):

- Repo name: `sodoryard` / `ponchione/sodoryard`
- Module path: `github.com/ponchione/sodoryard` (target of Step 1.1)
- Binary names: `tidmouth`, `sirtopham`, `knapford`
- Historical / projected tag names: `v0.1-pre-sodor`, `v1.0-sodor`
- "Sodor" as project concept in prose (the island theming that gave us
  Tidmouth, Knapford, SirTopham)

The actual `yard` scaffolding (`yard init` CLI, `.yard/` state dir,
`yard.yaml` config, `YARD_PROJECT` env var, `yard` compose service) lives
in Phase 5, not Phase 1.

---

## Regression smoke test — run after major Phase 1 steps and at the end

### Pre-flight

Local llama.cpp services must be up:

```bash
curl -s --max-time 3 http://localhost:12434/v1/models | head -c 80
curl -s --max-time 3 http://localhost:12435/v1/models | head -c 80
```

Both should return a `{"models":[...]}` JSON response. If they're down:
`cd ops/llm && docker compose up -d` (or ask the user).

Codex auth must be healthy:

```bash
./bin/sirtopham auth status    # or ./bin/tidmouth auth status after Step 1.2
```

Should show `codex (codex): healthy` with non-expired tokens.

### Throwaway config

Write `/tmp/my-website-smoke.yaml`:

```yaml
project_root: /home/gernsback/source/my-website
log_level: info
log_format: text

server:
  host: localhost
  port: 8093
  dev_mode: false
  open_browser: false

routing:
  default:
    provider: codex
    model: gpt-5.4-mini

providers:
  codex:
    type: codex
    model: gpt-5.4-mini

index:
  include:
    - "**/*.ts"
    - "**/*.tsx"
    - "**/*.js"
    - "**/*.jsx"
    - "**/*.json"
    - "**/*.html"
    - "**/*.css"
    - "**/*.md"
  exclude:
    - "**/.git/**"
    - "**/.my-website/**"
    - "**/.brain/**"
    - "**/node_modules/**"
    - "**/dist/**"
    - "**/build/**"
    - "**/.next/**"

brain:
  enabled: true
  vault_path: .brain
  log_brain_queries: true

agent_roles:
  correctness-auditor:
    system_prompt: /home/gernsback/source/sodoryard/agents/correctness-auditor.md
    tools:
      - brain
      - file:read
    brain_write_paths:
      - "receipts/correctness-auditor/**"
      - "logs/correctness-auditor/**"
    max_turns: 10
    max_tokens: 50000

local_services:
  enabled: false

embedding:
  base_url: http://localhost:12435
  model: nomic-embed-code
  batch_size: 32
  timeout_seconds: 30
  query_prefix: "Represent this query for searching relevant code: "
```

### The command

```bash
./bin/tidmouth run \
  --config /tmp/my-website-smoke.yaml \
  --role correctness-auditor \
  --task "Use brain_search to list the notes in the vault. Then use brain_write to create a receipt at receipts/correctness-auditor/smoke-test-p1.md. The receipt must use valid spec-13 YAML frontmatter with exactly these fields: agent, chain_id, step, verdict, timestamp, turns_used, tokens_used, duration_seconds. Set step to the integer 1. Set verdict to completed. After writing the receipt, stop." \
  --chain-id smoke-test-p1 \
  --max-turns 6 \
  --timeout 3m
```

(Before Step 1.2 finishes, use `./bin/sirtopham run` instead of `./bin/tidmouth run`.)

Note: `step` must be the integer `1`; the receipt validator enforces spec-13 field types and will reject string values like `final`.

### Pass criteria

- Exit code: `0`
- Final stdout line: `receipts/correctness-auditor/smoke-test-p1.md`
- File exists on disk:
  `/home/gernsback/source/my-website/.brain/receipts/correctness-auditor/smoke-test-p1.md`
- File has valid spec-13 YAML frontmatter

**If the smoke test fails after a Phase 1 step, STOP and diagnose before
continuing.** Use a new chain-id each time (`smoke-test-p1-a`, `-p1-b`, …) so
previous invalid-receipt artifacts don't interfere.

---

## Tools and services this project uses

- **llama.cpp** at `localhost:12434` (qwen-coder model) and `localhost:12435`
  (nomic-embed-code model). Managed via `ops/llm/docker-compose.yml` when
  `local_services.enabled: true`.
- **Codex/ChatGPT auth** stored in `~/.sirtopham/auth.json`. Status check:
  `./bin/sirtopham auth status` (or `./bin/tidmouth auth status` post-1.2).
  Expires 2026-04-13 at the time of this handoff — re-auth before then.
- **LanceDB** via `lib/linux_amd64/liblancedb_go.so`. Tests need the env vars
  set by `make test` (or `CGO_ENABLED=1 CGO_LDFLAGS="-L$PWD/lib/linux_amd64 -llancedb_go -lm -ldl -lpthread" LD_LIBRARY_PATH="$PWD/lib/linux_amd64"`).
- **sqlc** generates `internal/db/*.sql.go` from `internal/db/query/*.sql`. If
  you change SQL, regenerate (`sqlc generate` from `sqlc.yaml`).

---

## Hard rules for this session

- Per-step commits. Don't batch Phase 1 into one mega-commit.
- Don't push. The user pushes manually.
- Don't expand agent prompt stubs — Phase 4 work, not this session.
- Don't start Phase 3 (orchestrator) until Phase 1 tag `v0.2-monorepo-structure` is cut.
- Don't rename `sodor`/`Sodor` outside the repo/module scope (see "What NOT to rename" above).
- Don't delete the `v0.1-pre-sodor` tag.
- If you discover a bug mid-Phase-1 that isn't a restructure bug, fix it in its
  own commit labeled `fix(...)` rather than hiding it inside a `refactor(...)`
  commit. Per-step hygiene.
- If the smoke test breaks after a step, stop and diagnose. Don't proceed to
  the next step until it's green again.

---

## Pointer to the narrative of prior work

The brain-rebuild narrative that used to live in this file is preserved in git
at commit `4a63ad8` (the big megacommit) and its predecessor `1338611`. If you
need the phase 0–3 brain rebuild story for archaeology, `git show 1338611 --
NEXT_SESSION_HANDOFF.md` is the place to look. The code under
`internal/brain/parser`, `internal/brain/indexer`, `internal/brain/chunks`,
`internal/context/brain_search.go`, and `internal/context/analyzer.go` is where
that work actually lives.
