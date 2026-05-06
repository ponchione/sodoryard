# 16 — Yard Init

**Status:** Historical init design, superseded by Shunter-native `yard init`
**Owner:** Mitchell Ponchione
**Last Updated:** 2026-04-29
**Roadmap phase:** 5b
**Depends on:** Phase 1 (monorepo restructure), Phase 5a (yard paths rename)

**Supersession note:** This draft predates the Shunter base design. Current `yard init` creates `yard.yaml`, `.yard/` Shunter/runtime/LanceDB roots, and `.gitignore` entries; it does not create `.brain/` or `.yard/yard.db`.

---

## 1. Goal

Ship `yard init` as the canonical, top-level operator command for bootstrapping any new project. In the current Shunter-native design, a single `yard init` invocation seeds providers, agent role configuration, Shunter project-memory roots, and `.gitignore` hygiene without creating legacy `.brain/` or `.yard/yard.db` state.

The command lives in a new `cmd/yard` binary, not as a Tidmouth subcommand. This aligns the operator-facing CLI with the `yard` brand that Phase 5a already locked in across `yard.yaml`, `.yard/`, and `YARD_PROJECT`.

## 2. Why this spec exists

Three things are wrong with the current `cmd/tidmouth/init.go` that this spec resolves:

1. **It's branded wrong.** Operators bootstrap projects with `tidmouth init`, then everything else they touch is named `yard.*`. The internal binary name leaks into the only operator entry point that nothing else mentions.
2. **It lies about the railway shape.** The current init creates `.brain/notes/` and `.brain/.obsidian/` and stops there. The 8 railway brain section directories (`specs/`, `architecture/`, `epics/`, `tasks/`, `plans/`, `receipts/`, `logs/`, `conventions/`) are not created. The seeded `yard.yaml` does not contain any `agent_roles`, so a freshly initialized project cannot run the internal engine contract or `yard chain start` against itself without the operator hand-writing 13 role blocks first.
3. **It has a parallel source of truth.** `cmd/tidmouth/init.go:generateConfigYAML()` is a 100-line string-builder that ignores `templates/init/yard.yaml.example` entirely. The two have already drifted: the inline generator uses `anthropic`/Claude as the default provider with no agent_roles section, the template file uses `codex` with only two roles seeded. Whichever wins, neither actually matches the railway's needs.

Phase 5b makes one thing the source of truth (`templates/init/`), one binary the entry point (`yard`), and one invocation the bootstrap (`yard init`).

## 3. Locked decisions

Each decision below is the locked outcome of a brainstorm round. The reasoning is preserved so future-me can judge whether it still holds when context changes.

### 3.1 CLI surface — new `cmd/yard` binary, not a Tidmouth subcommand

`cmd/yard/main.go` is a new fourth binary in the monorepo. Its only command in this phase is `yard init`. Future phases may add `yard chain`, `yard status`, `yard up`, etc.; this spec does not commit to any of them.

**Why:** Phase 5a (`v0.2.1-yard-paths`) locked `yard` as the operator-facing brand for the state dir, the config file, the env var, and the eventual docker-compose service name. Bootstrapping is the most operator-facing thing the railway does. Keeping it as `tidmouth init` makes the only operator entry point inconsistent with everything else the operator touches.

**Cost:** A fourth binary in `bin/`, a fourth Makefile target, a fourth CGO link line. The thin-wrapper-around-internal-package shape keeps the binary tiny — most logic lives in `internal/initializer/`, so the cost is one cobra registration plus a `func main` that calls `Execute`.

**Alternatives considered:**
- **Keep `tidmouth init`, expand it.** Smallest delta, but locks in the brand inconsistency forever.
- **Both `yard` and `tidmouth init`, sharing internal code.** Two CLI entry points, dual source-of-truth at the operator level — exactly the kind of parallel-truth problem this spec is trying to remove.

### 3.2 Role seeding — all 13 roles, every time

The seeded `yard.yaml` contains `agent_roles` entries for every role in `agents/`:

`orchestrator`, `coder`, `planner`, `test-writer`, `resolver`, `correctness-auditor`, `integration-auditor`, `performance-auditor`, `security-auditor`, `quality-auditor`, `docs-arbiter`, `epic-decomposer`, `task-decomposer`.

Each role has `system_prompt`, `tools`, `custom_tools` (orchestrator only), `brain_write_paths`, `brain_deny_paths`, `max_turns`, and `max_tokens` populated with sensible defaults. Stock roles use explicit `builtin:<role>` prompt selectors, so a fresh config is self-contained and runnable without an external prompt directory. Operators only need a filesystem prompt path when intentionally overriding a stock built-in prompt.

**Why:** the operator-facing promise of `yard init` is "this works the moment you finish typing it" — within the boundary of "you also told it where your provider credentials live." A freshly initialized project must be able to immediately run any role or any chain. Seeding fewer than all 13 means every operator's first-day workflow includes "discover which roles weren't seeded and add them by hand."

**Cost:** the seeded yaml is large (~250 lines). The yaml is generated, not hand-edited, so its length never matters to a human reading it.

**Placeholder convention:** any unsubstituted value uses `{{NAME}}` syntax. The shipped `yard.yaml` template now substitutes `{{PROJECT_ROOT}}` / `{{PROJECT_NAME}}` at copy time and uses explicit `builtin:<role>` markers for stock agent prompts, so no prompt-path placeholder edit is required for normal operation.

**Alternatives considered:**
- **Critical-loop-only subset (orchestrator + 6 engines).** Saves a few dozen yaml lines but creates a "which roles exist?" knowledge requirement.
- **No roles seeded — provider and paths only.** Smallest yaml but breaks the "init produces a working project" promise.

### 3.3 Source of truth — `templates/init/` embedded via `go:embed`

`templates/init/` is the canonical source for everything init writes. The directory tree is embedded into the `yard` binary at build time via `//go:embed`. At runtime, init walks the embedded filesystem and writes its contents to the new project, performing minimal substitution at copy time.

**Why:** template-as-source-of-truth lets you edit yaml comments and starter content without recompiling Go. Embedding means the binary has zero runtime filesystem dependency — `yard` works the same whether it lives in `bin/`, `~/bin/`, or a Docker image. The combination is exactly the use case `embed.FS` exists for.

**Cost:** ~5KB added to the binary size (a yaml file plus 8 empty `.gitkeep` markers). Negligible.

**File layout** (relative to repo root):

```
templates/init/
├── yard.yaml.example          # the seeded config, with {{PLACEHOLDERS}}
└── brain/
    ├── architecture/.gitkeep
    ├── conventions/.gitkeep
    ├── epics/.gitkeep
    ├── logs/.gitkeep
    ├── plans/.gitkeep
    ├── receipts/.gitkeep
    ├── specs/.gitkeep
    └── tasks/.gitkeep
```

Notes on what is and isn't in the template:

- `templates/init/yard.yaml.example` is rewritten in this phase to contain all 13 `agent_roles` entries (today it has only 2). It is the file that ends up at `<project_root>/yard.yaml` after substitution.
- `templates/init/brain/notes/` is **not** in the template. The 8 railway section directories are; the operator's free-form notes directory is created by init code, not by template walking. (Distinguishing the two avoids confusion about which directories are railway-mandated vs operator-discretionary.)
- `.brain/.obsidian/` and its config files (`app.json`, `core-plugins.json`, etc.) are not in the template. Obsidian config is created by init code (it lives in maps in Go, not in the template tree). Reason: Obsidian config files contain JSON, and JSON-in-template gets ugly with the substitution syntax.

**Alternatives considered:**
- **Inline `generateConfigYAML()` stays, expanded to 250 lines.** Editing seeded yaml requires recompiling Go. Painful for the iteration the operator will do during Phase 4.
- **Template lives on disk, init reads it from `templates/init/` at runtime.** Works only when the binary runs from a sodoryard checkout. Breaks the moment the binary moves to `~/bin/` or a Docker image.

### 3.4 Substitutions performed at copy time

When init copies `templates/init/yard.yaml.example` to `<project_root>/yard.yaml`, exactly two substitutions happen:

| Placeholder | Substitution |
|---|---|
| `{{PROJECT_ROOT}}` | absolute path returned by `os.Getwd()` at the time `yard init` runs |
| `{{PROJECT_NAME}}` | basename of `os.Getwd()`, used in the yaml header comment |

Every other `{{PLACEHOLDER}}` token is left as-is for the operator to substitute by hand. In the shipped stock template, prompt-path placeholders are no longer part of the default flow because stock roles use embedded `builtin:<role>` markers. The operator workflow after `yard init` is:

1. Open the generated `yard.yaml`.
2. Confirm the seeded `builtin:<role>` prompt selections are acceptable, or replace a specific role's `system_prompt` with a filesystem path if a custom prompt override is desired.
3. Confirm the provider block matches their auth setup (default is codex, see 3.5).
4. Run `yard init` again to verify it's a no-op (idempotency check, see 3.6).

**Why minimal substitution:** the only required substitutions for a fresh stock config are still `{{PROJECT_ROOT}}` and `{{PROJECT_NAME}}`. Built-in prompt selectors remove the need for any prompt-dir substitution in the normal path, while still allowing explicit filesystem overrides when an operator wants them. Keeping init to these two substitutions preserves portability across repo checkouts, standalone binaries, and Docker images.

**Alternatives considered:**
- **Resolve `{{SODORYARD_AGENTS_DIR}}` from `$SODORYARD_HOME` env var.** Still unnecessary for stock configs now that embedded built-ins cover the default prompt set.
- **Auto-detect sodoryard from the running `yard` binary's location.** Still brittle when the binary lives outside a checkout.
- **Keep stock prompts as filesystem-only paths in generated configs.** Simpler at first glance, but it makes fresh projects depend on an external prompt directory and turns prompt-path setup into required operator ceremony.

Current shipped implementation note: the repo keeps `agents/` as the editable source prompt set and copies those files into an embedded asset directory at build time. A sync test guards against drift between the repo-root prompts and the embedded defaults.

### 3.5 Default provider — codex / gpt-5.5

The seeded `yard.yaml` defaults to:

```yaml
routing:
  default:
    provider: codex
    model: gpt-5.5

providers:
  codex:
    type: codex
    model: gpt-5.5
    reasoning_effort: medium
```

**Why:** codex is the path that worked on the maintainer's host when this spec was written (verified by the Phase 3 `phase3-smoke-1` smoke chain on 2026-04-11), uses the existing local Codex auth store, and requires no environment variable setup at the operator level. Anthropic was the previous default in `cmd/tidmouth/init.go` but currently failed its `Ping()` startup check on the same host with `Claude credentials file missing accessToken field`.

`gpt-5.5` matches the runtime-pinned Codex daily-driver model so generated config, `/api/config`, and the actual request payload report the same model. `reasoning_effort: medium` keeps daily-driver dogfood runs bounded by default; operators can raise it for complex work.

**Out of scope:** `yard init --provider <name>` flag, multi-provider seeding, or any first-run wizard. The operator edits `yard.yaml` after init if they want a different provider.

### 3.6 Old code disposition — delete `tidmouth init` outright

`cmd/tidmouth/init.go`, `cmd/tidmouth/init_test.go`, and the `newInitCmd` registration in `cmd/tidmouth/main.go` are deleted as part of the Phase 5b commit stack. There is no deprecation alias, no "tidmouth init is deprecated, use yard init" message, no transitional period.

**Why:** the repo's stated rules in `AGENTS.md` and CLAUDE-style instructions explicitly prohibit backwards-compatibility hacks for removed code. `tidmouth init` has exactly one user (the maintainer), the deletion is local to two files, and the replacement ships in the same commit stack. Anyone who types `tidmouth init` after this lands gets `unknown command "init"` from cobra and re-learns `yard init` in two seconds.

**Migration of init logic to `internal/initializer/`:** the parts of `cmd/tidmouth/init.go` worth keeping (Obsidian config maps, `.gitignore` patcher, database bootstrap, `mkdirReport` helper) move into a new `internal/initializer/` package. Tests for the init logic move from `cmd/tidmouth/init_test.go` to `internal/initializer/initializer_test.go`. `cmd/yard/init.go` becomes a thin cobra wrapper that calls `initializer.Run(ctx, opts)` and prints the report.

## 4. Component architecture

```
cmd/yard/
├── main.go           # cobra root command, ~30 lines
└── init.go           # cobra subcommand wrapper for yard init, ~50 lines

internal/initializer/
├── initializer.go    # the Run() entrypoint and supporting types
├── templates.go      # the //go:embed declaration and the FS walker
├── substitute.go     # placeholder substitution at copy time
├── obsidian.go       # the .obsidian config maps and writer (moved from cmd/tidmouth/init.go)
├── gitignore.go      # the .gitignore patcher (moved from cmd/tidmouth/init.go)
└── initializer_test.go

templates/init/
├── yard.yaml.example  # rewritten to contain all 13 agent_roles
└── brain/
    └── {8 section dirs}/.gitkeep

cmd/tidmouth/
├── main.go            # newInitCmd registration removed
└── (init.go, init_test.go deleted)

Makefile
└── new yard target using the same CGO_BUILD_ENV / GOFLAGS_DB conventions as the other retained binaries
```

`internal/initializer.Run` returns a structured report (created/skipped/error per file) so `cmd/yard/init.go` can print operator-friendly status without re-walking the filesystem.

## 5. CLI surface

Exactly one command in this phase:

```
yard init [flags]

Initialize the current directory for railway use.

Flags:
  --config string   Override the config filename (default "yard.yaml")
  -h, --help        Show help
```

**No `--project-root` flag.** `yard init` always operates on `os.Getwd()`. If the operator wants to init a different directory, they `cd` into it first. Reason: keeping the surface small means there's only one valid invocation pattern, which means there's only one thing to test, document, and reason about.

**No `--force` / `--reset` flag.** Re-running `yard init` against an already-initialized project is a no-op (see 3.6 idempotency). If the operator wants to start over, they delete `.yard/` and `.brain/` themselves. Reason: same as above plus "destructive flags belong to operator-confirmed destructive commands, not bootstrap commands."

**No interactive prompts.** No "what provider? what model? where's sodoryard installed?" wizard. `yard init` is non-interactive and produces deterministic output for the same working directory. Reason: easier to script, easier to test, no terminal-vs-pipe edge cases.

## 6. What `yard init` produces

After `yard init` runs in `<project_root>` and exits with code 0, the following exists:

```
<project_root>/
├── yard.yaml                     # rendered from templates/init/yard.yaml.example
├── .gitignore                    # patched (or created) with .yard/ and .brain/
├── .yard/
│   ├── yard.db                   # SQLite, schema initialized
│   └── lancedb/
│       ├── code/                 # empty, ready for `yard index`
│       └── brain/                # empty, ready for `yard brain index`
└── .brain/
    ├── .obsidian/
    │   ├── app.json
    │   ├── appearance.json
    │   ├── community-plugins.json
    │   └── core-plugins.json
    ├── notes/                    # operator's free-form notes (created, empty)
    ├── architecture/
    │   └── .gitkeep
    ├── conventions/
    │   └── .gitkeep
    ├── epics/
    │   └── .gitkeep
    ├── logs/
    │   └── .gitkeep
    ├── plans/
    │   └── .gitkeep
    ├── receipts/
    │   └── .gitkeep
    ├── specs/
    │   └── .gitkeep
    └── tasks/
        └── .gitkeep
```

Plus a row in `yard.db.projects` with `id = root_path = <project_root>` and `name = <basename>`.

The terminal output is a list of created/skipped lines, one per file or directory:

```
Initializing yard in /home/operator/source/myproject

  config     yard.yaml (created)
  mkdir      .yard/ (created)
  database   .yard/yard.db (schema created)
  mkdir      .yard/lancedb/code (created)
  mkdir      .yard/lancedb/brain (created)
  mkdir      .brain/ (created)
  vault      .brain/.obsidian/ (obsidian config ready)
  mkdir      .brain/notes/ (created)
  mkdir      .brain/architecture/ (created)
  mkdir      .brain/conventions/ (created)
  mkdir      .brain/epics/ (created)
  mkdir      .brain/logs/ (created)
  mkdir      .brain/plans/ (created)
  mkdir      .brain/receipts/ (created)
  mkdir      .brain/specs/ (created)
  mkdir      .brain/tasks/ (created)
  gitignore  .gitignore (added .yard/, .brain/)

Done.
Next steps:
  1. Confirm the provider/auth settings in yard.yaml and optionally replace any builtin prompt marker with a file path override
  2. Confirm the provider block matches your auth setup
     (default is codex via the local Codex auth store).
  3. Run `yard index` to populate the code search index.
  4. Run `yard chain start --task "..."` to start your first chain.
```

## 7. The seeded `yard.yaml` shape

`templates/init/yard.yaml.example` is rewritten to contain:

- A header comment describing what the file is and how to substitute the placeholders
- `project_root: {{PROJECT_ROOT}}`
- `log_level: info`, `log_format: text`
- `routing.default` block — codex / gpt-5.5
- `providers.codex` block
- `index.include` / `index.exclude` blocks with a generic-but-reasonable file pattern set
- `brain.enabled: true`, `vault_path: .brain`, `log_brain_queries: true`
- `agent_roles:` with all 13 entries (see 3.2)
- `local_services:` block with the repo-owned llama.cpp Docker Compose stack enabled in `manual` mode
- `embedding:` block with the nomic-embed-code defaults

Each agent role entry has the shape:

```yaml
  <role-name>:
    system_prompt: builtin:<role-name>
    tools:
      - brain
      - <other tools as appropriate for the role>
    custom_tools:           # only on orchestrator
      - spawn_agent
      - chain_complete
    brain_write_paths:
      - "receipts/<role-name>/**"
      - "logs/<role-name>/**"
      - <other paths as appropriate for the role>
    brain_deny_paths:
      - "specs/**"
      - "architecture/**"
      - "conventions/**"
      - <other paths as appropriate for the role>
    max_turns: <role-appropriate value>
    max_tokens: <role-appropriate value>
```

The seeded local-service block is:

```yaml
local_services:
  enabled: true
  mode: manual
  provider: docker-compose
  compose_file: docker-compose.yml
  project_dir: ./ops/llm
  required_networks:
    - llm-net
  auto_create_networks: true
  startup_timeout_seconds: 180
  healthcheck_interval_seconds: 2
  services:
    qwen-coder:
      base_url: http://localhost:12434
      health_path: /health
      models_path: /v1/models
      required: true
    nomic-embed:
      base_url: http://localhost:12435
      health_path: /health
      models_path: /v1/models
      required: true
```

`manual` means `yard index` and `yard llm up` report exact remediation when services are unhealthy but do not auto-start containers. Operators can switch to `auto` to let the CLI create missing networks, run `docker compose up -d`, and wait for required service health.

Per-role tool/path/limit defaults are based on the role boundaries each prompt stub already implies (e.g., `correctness-auditor` gets `file:read` not `file`; `coder` gets `file`, `git`, `shell`, `search`; the auditors all get `brain_deny_paths` for `specs/`, `architecture/`, `conventions/`, `epics/`, `tasks/`, `plans/`). The implementation plan will enumerate the full per-role default block.

## 8. Idempotency / re-run behavior

`yard init` is safe to re-run against an already-initialized project. Each step has a defined no-op behavior:

| Step | First run | Re-run |
|---|---|---|
| `yard.yaml` | created from template | skipped if exists; printed as `(already exists, skipped)` |
| `.yard/` mkdir | created | skipped if exists |
| `yard.db` schema | created via `appdb.InitIfNeeded` | InitIfNeeded is already idempotent; printed as `(already initialized, skipped)` |
| `.yard/lancedb/{code,brain}/` | created | skipped if exists |
| `.brain/.obsidian/*.json` | created | each file skipped if it exists; the directory is not modified |
| `.brain/<section>/` | created | skipped if exists; `.gitkeep` not re-written |
| `.brain/notes/` | created | skipped if exists |
| `.gitignore` | patched (added missing entries) | each entry skipped if already present; printed as `(already has entries, skipped)` if all present |
| Project record in `yard.db` | inserted | upserted via `ON CONFLICT(id) DO UPDATE`, with `updated_at` refreshed |

A re-run never overwrites file content. It never deletes anything. It never modifies the project record's `created_at`. It is safe to run as part of a script that may have partially-completed earlier.

The idempotency is also the spec's only "validation" gate: if `yard init && yard init` produces any errors on the second run, that's a Phase 5b regression.

## 9. Acceptance criteria — "Phase 5b done"

Phase 5b is complete when **all** of the following are true:

1. `make build` builds the retained artifact set for the current no-legacy target state with the required FTS5 + lancedb cgo flags; `make all` is an alias for the same artifact set.
2. `make test` is green, including new tests in `internal/initializer/`.
3. `cmd/tidmouth/init.go` and `cmd/tidmouth/init_test.go` no longer exist; `tidmouth init` returns `unknown command` from cobra.
4. `internal/initializer/` exists and houses all the file/directory/database creation logic.
5. `templates/init/yard.yaml.example` contains all 13 `agent_roles` with stock `builtin:<role>` prompt selectors, embedded into the `yard` binary via `go:embed`.
6. Running `yard init` in an empty `/tmp/yard-init-smoke-<timestamp>/` directory produces the full file tree from section 6, exits 0, and prints the operator-facing report.
7. Running `yard init` again in the same directory exits 0 and prints `(already exists, skipped)` lines for everything; no file content is modified.
8. The smoke test in step 6 can be followed directly by `yard index --config yard.yaml` and `yard chain start --config yard.yaml --task "..."` succeeding against the freshly initialized project without any prompt-path substitution.
9. `docs/specs/16-yard-init.md` (this file) is updated to match anything that changed during implementation.
10. The Phase 5b commit stack is tagged `v0.5-yard-init`.

The smoke test in steps 6–8 is **end-to-end live**, not just unit tests. The Phase 3 verification this session proved that "tests pass" and "the binary actually works" are different gates; Phase 5b respects the same lesson.

## 10. Out of scope

The following are explicitly **not** in Phase 5b. Each may become a future spec.

- **Other `yard` subcommands.** `yard chain start`, `yard status`, `yard up`, `yard validate`, etc. are deferred. The new binary ships with exactly one command.
- **Auto-materializing built-in prompts onto disk.** Stock prompts are embedded and selected via `builtin:<role>` markers; `yard init` does not copy prompt files into the target project.
- **First-run wizard / interactive prompts.** Init is fully non-interactive.
- **`--force` / `--reset` flag.** Operators delete `.yard/` and `.brain/` themselves if they want to start over.
- **Provider auto-detection or credential validation at init time.** The seeded `yard.yaml` may point at a provider whose credentials don't exist; that's discovered on first real runtime use, not at init.
- **Updating `docs/specs/00-index.md`** to reference specs 10–16. The index is already stale (only references 01–09); fixing it is its own out-of-scope task.
- **`tidmouth init` deprecation alias.** Outright deletion only. No transitional period.
- **Brain section starter content.** No README files in `specs/`, `architecture/`, etc. Phase 4 prompts will tell agents what each section is for; embedding that as filesystem README content would be a second source of truth that drifts.
- **Multi-project / workspace init.** `yard init` initializes one project at a time, the current working directory.

## 11. Open questions / future work

These are intentionally left unresolved by this spec. Each is a future decision point, not a Phase 5b blocker.

- **Should `yard init` also offer optional first-run runtime validation?** Not in this phase; the bootstrap step stays non-interactive and deterministic.
- **How much of the internal engine contract should remain exposed in docs?** The current no-legacy direction keeps operator docs on `yard` while allowing `tidmouth` to remain internal for spawn-only use.
- **Should `templates/init/` ship example brain content** (e.g., a starter `specs/00-getting-started.md` that explains the brain layout)? Excluded from Phase 5b because brain content overlaps with Phase 4 prompt content; lock that in only after the prompts have stabilized.
- **Should `yard init` create a `.brain/_log.md`** the way tidmouth's brain MCP server does at runtime? Probably yes, but not a Phase 5b blocker — the brain MCP server already creates it on first connect, so init's job is just to make sure the parent directory exists.
- **Should there be a `yard init --template <name>` flag** that picks between minimal/full/etc. starter templates? Not in Phase 5b. One canonical template only.

## 12. References

- `cmd/tidmouth/init.go` — current init implementation, deleted as part of Phase 5b
- `templates/init/yard.yaml.example` — rewritten as part of Phase 5b
- `docs/specs/13_Headless_Run_Command.md` — defines the `yard.yaml` schema
- `docs/specs/14_Agent_Roles_and_Brain_Conventions.md` — defines the `agent_roles` config shape
- `docs/specs/15-chain-orchestrator.md` — defines the orchestrator's role config requirements
- `README.md` — tracks the current repo state and next-session starting point

---

**End of spec.** Implementation plan to follow via the writing-plans skill.
