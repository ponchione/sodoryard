# No-legacy cleanup punchlist

Created: 2026-04-21

Goal: remove operator-facing legacy/backwards-compat code and duplicated CLI implementations without breaking the current `yard` functionality or the orchestrator's internal engine-spawn contract.

Grounding used:
- `docs/specs/18-unified-yard-cli.md`
- `docs/specs/17-yard-containerization.md`
- `docs/specs/16-yard-init.md`
- `docs/specs/15-chain-orchestrator.md`
- `docs/specs/13_Headless_Run_Command.md`
- `README.md`
- current `cmd/`, `internal/runtime/`, `internal/spawn/`, `Makefile`

## Executive summary

The current repo is halfway through a CLI consolidation: `yard` already implements the public surface, but `cmd/tidmouth` and `cmd/sirtopham` still retain large duplicated command trees, while docs/specs still explicitly preserve compatibility.

Under the new requirement — no legacy or backwards-compat code whatsoever — the cleanup should be executed in this order:

1. rewrite the specs/docs that still mandate compatibility
2. delete explicit compatibility code (`yard install`, placeholder substitutions, old path exclusions)
3. remove the public `sirtopham` CLI entirely
4. reduce `tidmouth` to only the internal engine contract still required by `spawn_agent`
5. finish de-duplicating command logic into shared internal packages
6. remove placeholder-only artifacts like `knapford` unless they become real

## Spec-grounded decisions

### 1) Spec 18 must be deliberately overturned/revised
`docs/specs/18-unified-yard-cli.md` currently says:
- `yard` is the only documented operator surface
- but `tidmouth` and `sirtopham` continue building and continue working unchanged
- and this is an acceptance criterion

That conflicts directly with the new no-legacy mandate.

Punchlist decision:
- revise Spec 18 first so the target state is explicit:
  - `yard` is the only operator-facing CLI
  - `sirtopham` is deleted unless execution uncovers a real non-duplicated internal need
  - `tidmouth` is internal-only and only retains the minimal subprocess contract still required by the orchestrator

### 2) Spec 16 is the precedent to follow
`docs/specs/16-yard-init.md` already established the repo's strongest anti-compat pattern:
- delete `tidmouth init`
- no alias
- no deprecation period

Punchlist decision:
- treat all remaining legacy CLI duplication the same way

### 3) Spec 17 currently preserves compatibility surface that should now be removed
`docs/specs/17-yard-containerization.md` still assumes:
- `yard install` exists for older placeholder configs
- operators may run `tidmouth run`
- operators may run `sirtopham chain`
- `knapford` remains a placeholder service in compose

Punchlist decision:
- revise Spec 17 before code deletion so containerization does not keep reintroducing compatibility requirements

### 4) Spec 13 means `tidmouth` cannot be deleted blindly
`docs/specs/13_Headless_Run_Command.md` defines the headless engine contract around `tidmouth run`, and the code matches that internal contract today:
- `internal/runtime/orchestrator.go` hardcodes `EngineBinary: "tidmouth"`
- `internal/spawn/spawn_agent.go` shells out to `tidmouth run` and `tidmouth index`

Punchlist decision:
- do not remove `tidmouth` outright in the first deletion pass
- first reduce it to the smallest internal surface needed by spawn (`run`, `index`, and only anything else proven necessary)

### 5) Spec 15 supports deleting duplicated public chain CLI code
`docs/specs/15-chain-orchestrator.md` defines the orchestrator behavior, not a requirement to preserve a separate public `sirtopham` binary.

Current code already runs `yard chain ...` in-process and no code path shells out to `sirtopham`.

Punchlist decision:
- `cmd/sirtopham/` is a clean deletion candidate after docs are updated

## Current code evidence that shapes safe deletions

### Duplicated command trees still present
High-confidence duplicate pairs in the current repo:
- `cmd/tidmouth/config.go` ↔ `cmd/yard/config_cmd.go`
- `cmd/tidmouth/serve.go` ↔ `cmd/yard/serve.go`
- `cmd/tidmouth/run.go` ↔ `cmd/yard/run.go`
- `cmd/tidmouth/llm.go` ↔ `cmd/yard/llm.go`
- `cmd/tidmouth/auth.go` ↔ `cmd/yard/auth.go`
- `cmd/sirtopham/chain.go` ↔ `cmd/yard/chain.go`

### `sirtopham` appears deletable as a binary
- no current runtime code shells out to `sirtopham`
- `yard chain start` already executes the chain path in-process
- `cmd/sirtopham/runtime.go` is only a thin wrapper around `internal/runtime`

### `tidmouth` is still part of a real internal contract
- `internal/runtime/orchestrator.go:202` uses `EngineBinary: "tidmouth"`
- `internal/spawn/spawn_agent.go:121` shells out to `tidmouth run ...`
- `internal/spawn/spawn_agent.go:200` shells out to `tidmouth index ...`

### Legacy guidance still leaks from the new CLI
`cmd/yard/init.go` still tells users to run:
- `tidmouth index`
- `sirtopham chain --task "..."`

### Explicit compatibility code still exists
- `cmd/yard/install.go`
- `internal/initializer/install.go`
- `internal/tool/search_text.go` old state-dir exclusions for `.sirtopham` / `.sodoryard`
- `internal/config/config.go` still contains a transitional `DefaultConfigFilename(projectRoot string)` helper/comment

## Punchlist

## Phase 0 — Reset the written contract

1. Patch `docs/specs/18-unified-yard-cli.md`
   - Remove “legacy binaries continue working unchanged” as a goal/acceptance criterion.
   - State the no-legacy end state explicitly.
   - Clarify whether `tidmouth` remains internal-only.

2. Patch `docs/specs/17-yard-containerization.md`
   - Remove `yard install` compatibility flow.
   - Replace operator examples that use `tidmouth run`, `sirtopham chain`, and placeholder-driven config migration with `yard` equivalents.
   - Decide whether `knapford` stays in scope at all.

3. Patch `docs/specs/13_Headless_Run_Command.md`
   - Fix stale implementation references (`cmd/sirtopham/run.go`).
   - Re-express the headless engine as an internal contract if `tidmouth` is no longer public.

4. Patch `README.md`
   - Remove operator-facing legacy instructions.
   - Keep internal binary notes only if those binaries survive the cleanup.

## Phase 1 — Delete compatibility-only code

1. Delete `cmd/yard/install.go`
2. Remove `newInstallCmd()` from `cmd/yard/main.go`
3. Delete `internal/initializer/install.go`
4. Delete/update install-related tests
5. Remove `SODORYARD_AGENTS_DIR` references from docs/comments/tests
6. Remove compatibility-only path exclusions:
   - `internal/tool/search_text.go`
   - `internal/tool/list_directory.go`
   - related tests
7. Delete or simplify `internal/config/config.go: DefaultConfigFilename(projectRoot string)` if it no longer has a real use

## Phase 2 — Remove `cmd/sirtopham/`

1. Delete all files under `cmd/sirtopham/`
2. Delete `cmd/sirtopham` tests
3. Remove `sirtopham` target from `Makefile` unless a real build-time/runtime dependency is found
4. Remove `sirtopham` references from:
   - `README.md`
   - `ops/llm/README.md`
   - spec docs
   - command help text
   - comments referencing `cmd/sirtopham`

## Phase 3 — Slim `cmd/tidmouth/` to internal engine-only responsibilities

Keep only what the orchestrator/internal runtime still needs.

Likely keep initially:
- `run`
- `index`

Likely delete after verification:
- `serve.go`
- `auth.go`
- `config.go`
- `llm.go`
- `brain_serve.go` (unless some internal path still needs it)

Required follow-up:
- slim `cmd/tidmouth/main.go`
- update help text to internal-only framing
- remove duplicated tests for deleted subcommands

## Phase 4 — Finish internal de-duplication

1. Extract shared non-Cobra behavior into internal packages for:
   - run/headless execution
   - chain operations
   - config output
   - auth/doctor reporting
   - llm management
2. Make `cmd/yard/*.go` thin adapters only
3. Make the remaining `cmd/tidmouth` wrappers call the same shared internal code
4. Collapse duplicate tests to shared package-level tests where possible

## Phase 5 — Remove placeholder surfaces

1. Decide whether `cmd/knapford/` stays.
2. If it is still only a placeholder, remove it from:
   - `Makefile all`
   - README
   - specs
   - future container compose assumptions
3. If it stays, define a real contract and stop calling it placeholder

## Phase 6 — Contradiction sweep

Search and remove/update all remaining references to:
- `yard install`
- `tidmouth serve`
- `tidmouth config`
- `tidmouth llm`
- `tidmouth auth`
- `sirtopham chain`
- `.sirtopham` / `.sodoryard` compatibility state
- placeholder-only Knapford statements

## Files that are very likely to change

Docs/specs:
- `README.md`
- `docs/specs/13_Headless_Run_Command.md`
- `docs/specs/17-yard-containerization.md`
- `docs/specs/18-unified-yard-cli.md`
- `ops/llm/README.md`

Public CLI:
- `cmd/yard/main.go`
- `cmd/yard/init.go`
- `cmd/yard/install.go` (delete)

Legacy/internal CLI:
- `cmd/sirtopham/*` (delete)
- `cmd/tidmouth/main.go`
- selected `cmd/tidmouth/*.go`

Internal packages:
- `internal/runtime/orchestrator.go`
- `internal/spawn/spawn_agent.go`
- `internal/config/config.go`
- `internal/initializer/install.go` (delete)
- `internal/tool/search_text.go`
- `internal/tool/list_directory.go`
- any new shared internal command packages created during extraction

## Validation after each phase

- `make test`
- `go vet -tags sqlite_fts5 ./...`
- `rtk go list ./...`
- targeted string searches for deleted legacy terms
- targeted smoke for:
  - `go run -tags sqlite_fts5 ./cmd/yard --help`
  - `go run -tags sqlite_fts5 ./cmd/yard chain --help`
  - internal `tidmouth run` / `tidmouth index` if tidmouth is still retained for spawn

## Final acceptance criteria

- `yard` is the only operator-facing CLI in code, docs, and help text
- no compatibility-only command (`yard install`) remains
- no `cmd/sirtopham/` tree remains
- `tidmouth` is either fully gone or reduced to a clearly internal engine contract only
- no compatibility handling for `.sirtopham` / `.sodoryard` remains
- no placeholder-only artifacts are built by default
- tests and tagged vet pass
