# Wide shallow Go audit — 2026-04-21

Scope: broad maintainability/code-smell audit of the Go codebase.

Baseline checked:
- README.md, go.mod, Makefile
- Go package inventory via `rtk go list ./...`
- Go tests via `make test`
- Vet via `go vet ./...` and `go vet -tags sqlite_fts5 ./...`
- Structural hotspot scan across 399 `.go` files

Quick repo signals:
- ~53.6k Go LOC across 390 non-node_modules Go files (`pygount` summary)
- Tests pass with repo-native path: `make test`
- Plain `go vet ./...` fails, but `go vet -tags sqlite_fts5 ./...` passes

## Highest-signal findings

### 1) CLI transition debt is still duplicated across three binaries
The repo says the operator-facing surface is `yard`, but a large amount of command logic still exists in near-identical legacy binaries:
- `cmd/tidmouth/main.go`
- `cmd/sirtopham/main.go`
- `cmd/yard/main.go`

Concrete near-duplicates:
- `cmd/tidmouth/config.go` vs `cmd/yard/config_cmd.go` are functionally the same (51 lines each)
- `cmd/tidmouth/run.go` vs `cmd/yard/run.go` are highly similar headless-run implementations
- `cmd/sirtopham/chain.go` and `cmd/sirtopham/logs.go` duplicate large chunks now embedded in `cmd/yard/chain.go`
- `cmd/tidmouth/index.go:99-157` and `cmd/yard/brain.go:60-118` duplicate the brain-index rebuild flow

Why it matters:
- Every behavior fix now risks triple maintenance and skew between CLIs
- The repo has both shared runtime extraction and retained copy/paste command logic, which is the worst of both worlds
- Test files are duplicated too (`cmd/sirtopham/chain_test.go` 651 LOC, `cmd/yard/chain_test.go` 822 LOC)

Suggested direction:
- Move command behavior into shared non-`main` packages and make binaries thin wiring only
- Or formally demote/remove legacy binaries if `yard` is truly the only supported operator surface

### 2) `internal/agent/loop.go` is a monolithic orchestration hotspot
- File: `internal/agent/loop.go` (~1315 LOC)
- Function: `RunTurn` at `internal/agent/loop.go:295` spans ~574 lines

Observed responsibilities inside one function:
- request validation
- cancellation wiring
- persistence of user messages
- context assembly
- model/provider override resolution
- iteration orchestration
- prompt construction
- preflight compression
- streaming/retry
- emergency compression fallback
- assistant serialization
- final persistence/cancellation/error handling

Why it matters:
- Change amplification hotspot: almost any turn-lifecycle change lands here
- Harder to reason about invariants around cancellation/persistence/partial state
- High risk of regressions because control flow is deeply nested and mixes policy with mechanics

Suggested direction:
- Extract explicit phases (`prepareTurn`, `runIteration`, `persistFinalResponse`, `handleStreamFailure`, etc.)
- Keep `RunTurn` as a readable coordinator rather than the implementation body

### 3) `cmd/yard/chain.go` is doing too much and absorbing old `sirtopham` code
- File: `cmd/yard/chain.go` (~851 LOC, 35 funcs)
- `yardRunChain` at `cmd/yard/chain.go:119` is ~116 lines
- The same file also contains logs/status/receipt/pause/resume/cancel/watch formatting logic (`cmd/yard/chain.go:543+`, `737+`)

This file is effectively:
- chain start/resume execution
- watch/poll loop behavior
- event formatting and noise suppression
- control status transitions
- signal-to-process logic
- multiple subcommand constructors

Why it matters:
- One file now owns both orchestration execution and CLI presentation concerns
- It already duplicated major parts of `cmd/sirtopham/chain.go` and `cmd/sirtopham/logs.go`
- Makes future chain work harder to isolate

Suggested direction:
- Split execution/control/watch/rendering into package-level helpers outside `main`
- Use one shared chain CLI implementation instead of separate `yard` and `sirtopham` trees

### 4) Transitional/deprecated API crumbs are still checked in
Examples:
- `internal/config/config.go:479-487` — `DefaultConfigFilename(projectRoot string)` keeps an intentionally unused parameter and a comment saying the function may be deleted later
- SearchText still carries legacy state-dir exclusions for `.sirtopham` and `.sodoryard` in `internal/tool/search_text.go:28-41`
- `cmd/yard/install.go` and initializer logic are still centered on legacy `{{SODORYARD_AGENTS_DIR}}` placeholder substitution

Why it matters:
- Transitional compatibility is reasonable, but these paths should be clearly bounded and periodically retired
- Otherwise old rename-era compatibility code becomes permanent surface area

Suggested direction:
- Tag rename-compat code with a real retirement issue/doc target
- Delete APIs that are now test-only or migration-only once callers are gone

### 5) The module surface is polluted by vendored Go code under `web/node_modules`
`rtk go list ./...` and `make test` both traverse:
- `web/node_modules/flatted/golang/pkg/flatted`

Evidence:
- `web/node_modules/flatted/golang/pkg/flatted/flatted.go`
- Test output includes `github.com/ponchione/sodoryard/web/node_modules/flatted/golang/pkg/flatted`

Why it matters:
- Node dependency content should not appear as first-class Go packages in repo health checks
- Pollutes package inventory, test output, and tool scans
- Creates accidental maintenance/supply-chain surface inside Go tooling

Suggested direction:
- Exclude this subtree from Go package traversal, or keep generated/vendor/frontend deps outside the Go module root
- At minimum, stop treating `web/node_modules/**` as Go packages seen by `go list ./...`

### 6) Default build still ships placeholder/legacy binaries
Makefile still builds all of these by default:
- `tidmouth`
- `sirtopham`
- `knapford`
- `yard`

And `cmd/knapford/main.go:6-13` is explicitly a placeholder binary.

Why it matters:
- Default build output advertises artifacts that are not equally real/maintained
- Increases perceived surface area and maintenance burden
- Conflicts with README language that `yard` is the operator-facing surface

Suggested direction:
- Remove placeholder binaries from `all`, or gate them behind explicit dev targets
- Decide whether `tidmouth`/`sirtopham` are internal implementation binaries or public tools

### 7) Vet only passes with repo-specific tags; the default vet path is broken
Observed:
- `go vet ./...` fails because untagged tests reference helpers defined in `//go:build sqlite_fts5` test files
- `go vet -tags sqlite_fts5 ./...` passes

Examples:
- `internal/chain/control_test.go` references helper from `internal/chain/state_test.go` (tagged)
- `cmd/sirtopham/chain_test.go` references helper from `cmd/sirtopham/chain_control_sqlite_test.go` (tagged)
- `cmd/yard/chain_test.go` references helper from `cmd/yard/chain_control_sqlite_test.go` (tagged)

Why it matters:
- Repo-native paths work, but default Go tooling gives misleading failure noise
- Makes audit/lint ergonomics worse for anyone not remembering the special tag

Suggested direction:
- Put dependent test files behind the same tag, or move shared helpers into untagged helpers that do not require sqlite build constraints

## Lower-priority smells

### 8) `internal/config/config.go` concentrates a lot of policy/defaulting/validation
- ~949 LOC, 36 funcs
- `Default()` alone is a long configuration literal at `internal/config/config.go:220-339`

This is not broken, but it is becoming a god file for:
- schema types
- defaults
- rename compatibility
- path derivation
- validation helpers
- env override behavior

Suggested direction:
- Split into `types.go`, `defaults.go`, `paths.go`, `validate.go`, `env.go`

### 9) There are tiny doc-only/dead-looking packages that add surface with almost no value
- `internal/tools/doc.go` defines a package with no other files/usages

This is harmless, but it is classic repo lint clutter and reads like abandoned naming churn (`tool` vs `tools`).

## Healthy signals

- `make test` is green across the repo
- `go vet -tags sqlite_fts5 ./...` is green
- A lot of duplication has already been partially factored into `internal/runtime/*`
- Test coverage footprint is broad; most major packages have tests
- README architecture and package layout are reasonably clear

## Suggested cleanup order

1. Collapse legacy CLI duplication (`tidmouth`/`sirtopham` vs `yard`)
2. Break up `internal/agent/loop.go`
3. Split `cmd/yard/chain.go` into execution/control/render/watch layers
4. Remove rename-era compatibility crumbs that are now test-only or legacy-only
5. Stop Go tooling from traversing `web/node_modules/**`
6. Make default `go vet` behavior clean without requiring insider knowledge
7. Remove placeholder/default-build artifacts like `knapford` if not actually active

## Bottom line
The repo does not look unhealthy in correctness terms — the native test path is green — but it is carrying a noticeable amount of transition debt. The biggest maintainability drag is not low-level Go idioms; it is architectural duplication from the multi-binary-to-`yard` transition plus a few oversized orchestration hotspots.