# Next session handoff

Objective
- Continue keeping the repo on the no-legacy/current-truth contract now that the public `sirtopham` CLI, placeholder `knapford` surfaces, and stale live packaging/install residue are gone.
- Treat Phases 0-5 cleanup as checkpointed work: `yard` is the only public CLI, `tidmouth` is internal-only, live packaging/install surfaces are trimmed, and the current-truth doc set is being kept clean instead of preserving stale migration notes.
- Start the next session from the current README/specs/handoff, not from historical planning artifacts.

Read first
1. `AGENTS.md`
2. `README.md`
3. `docs/specs/13_Headless_Run_Command.md`
4. `docs/specs/17-yard-containerization.md`
5. `docs/specs/18-unified-yard-cli.md`
6. `NEXT_SESSION_HANDOFF.md`

Current state
- `cmd/sirtopham/` has been removed from the working tree.
- `yard install` / initializer install compatibility code has been removed from the working tree.
- `cmd/tidmouth/` has been slimmed to internal-only commands:
  - kept: `run`, `index`, `index brain`
  - removed: `serve`, `auth`, `doctor`, `config`, `llm`, `brain-serve`
- Tidmouth help text now frames it as an internal engine binary.
- Phase 4 has two helper extractions landed on disk now:
  - `internal/headless/` for shared headless-run helper behavior
  - `internal/cmdutil/llm.go` for shared yard local-services command behavior
- One more Phase 4 extraction is now landed on disk:
  - `internal/cmdutil/provider_diag.go` for shared provider-auth/doctor report collection + formatting
  - `internal/cmdutil/config_report.go` for shared `yard config` summary output
- `cmd/tidmouth` and `cmd/yard` headless helpers delegate to `internal/headless`.
- `cmd/yard/llm.go` now delegates local-services command behavior to `internal/cmdutil`, and `cmd/yard/auth.go` / `cmd/yard/config_cmd.go` now use that shared package for provider diagnostics and config-summary formatting too.
- The first contradiction-sweep pass is done for the main operator-facing docs/specs:
  - `README.md`
  - `docs/specs/15-chain-orchestrator.md`
  - `docs/specs/16-yard-init.md`
  - `docs/specs/18-unified-yard-cli.md`
- Those docs no longer tell operators to use `yard install`, `sirtopham chain`, or removed `tidmouth` public subcommands.
- A second contradiction-sweep pass is now done for remaining live internal operator-facing remediation/help strings in code:
  - `internal/localservices/manager.go`
  - `internal/localservices/docker.go`
- Local-services remediation now points to `yard llm ...`, not removed `tidmouth llm ...` commands.
- A third contradiction-sweep pass is now done for still-live current-truth specs outside the earlier main-doc sweep:
  - `docs/specs/04-code-intelligence-and-rag.md`
  - `docs/specs/07-web-interface-and-streaming.md`
  - `docs/specs/08-data-model.md`
  - `docs/specs/10_Tool_System.md`
  - `docs/specs/14_Agent_Roles_and_Brain_Conventions.md`
- Those specs now use `yard serve` / `yard init` / `yard index` for operator-facing flows, and use `tidmouth index` only where the spec is explicitly describing the current internal orchestrator subprocess contract.
- The helper-only command-test collapse pass is now a little further along:
  - shared receipt/read-task/progress-format coverage now lives in `internal/headless/headless_test.go`
  - deleted `cmd/yard/run_helpers_test.go`
  - deleted `cmd/tidmouth/receipt_test.go`
  - removed duplicated helper-forwarding tests from `cmd/tidmouth/run_test.go`
  - dropped the remaining non-command `LoadRoleSystemPrompt(...)` coverage from `cmd/tidmouth/run_test.go` because `internal/runtime/helpers_test.go` already owns that helper contract
  - moved `ResolveModelContextLimit(...)` coverage out of `cmd/tidmouth/run_test.go` into `internal/runtime/helpers_test.go`, so the runtime helper owns its own contract
- `cmd/tidmouth/run_test.go` now looks limited to command-specific `runHeadless(...)` behavior coverage, while shared helper behavior is owned by `internal/headless` / `internal/runtime`.
- Phase 5 Knapford live-surface cleanup is now landed:
  - deleted `cmd/knapford/main.go`
  - removed the `knapford` target from `Makefile all`
  - removed the placeholder `knapford` service from `docker-compose.yaml`
  - removed placeholder `knapford` build/copy steps from `Dockerfile`
  - removed live README/spec/script references that still treated `knapford` as a shipped artifact
- immediate post-Knapford packaging/install cleanup is now landed too:
  - removed `sirtopham` build/copy steps from `Dockerfile`
  - removed `sirtopham` from `scripts/install-user-bin.sh`
  - updated `scripts/test_install_user_bin.sh` to expect only the retained shipped binaries (`tidmouth`, `yard`)
  - taught `Makefile` to delete stale retired binaries from `bin/` before `make all` / `make build`

- Remaining `sirtopham` mentions in current specs are intentional historical/problem-statement or no-legacy-negation references unless a future session takes a broader archive/doc-cleanup pass.
- Stale migration/implementation-plan markdown is being removed rather than treated as archival guidance.
- The working tree is intentionally dirty with these cleanup slices until they are committed.

Validated on current tree
- checkpoint validation before commit:
  - `make test` ✅
  - `make build` ✅
- earlier wider validation from the previous slice still stands:
  - `./bin/tidmouth --help` still shows only `run` and `index` ✅
  - targeted string searches across the edited docs for `yard install`, `sirtopham chain`, `tidmouth serve`, `tidmouth config`, `tidmouth llm`, `tidmouth auth` ✅
  - targeted string searches across live `.go` files under `cmd/` and `internal/` for the same removed public-command phrases ✅
  - targeted current-spec sweep after the latest doc edits:
    - `docs/specs/04-code-intelligence-and-rag.md`, `07-web-interface-and-streaming.md`, `08-data-model.md`, `10_Tool_System.md`, and `14_Agent_Roles_and_Brain_Conventions.md` no longer contain operator-facing `sirtopham serve` / `sirtopham init` / `sirtopham index` guidance ✅
    - remaining matches in `docs/specs/13`, `15`, and `17` are intentional internal-contract or explicit no-legacy-negation references ✅
- focused validation that was rerun during the helper-test collapse slice:
  - `go test ./internal/headless` ✅
  - `CGO_ENABLED=1 CGO_LDFLAGS="-L$PWD/lib/linux_amd64 -llancedb_go -lm -ldl -lpthread" LD_LIBRARY_PATH="$PWD/lib/linux_amd64" go test ./internal/runtime` ✅
  - `CGO_ENABLED=1 CGO_LDFLAGS="-L$PWD/lib/linux_amd64 -llancedb_go -lm -ldl -lpthread" LD_LIBRARY_PATH="$PWD/lib/linux_amd64" go test ./internal/cmdutil` ✅
  - `CGO_ENABLED=1 CGO_LDFLAGS="-L$PWD/lib/linux_amd64 -llancedb_go -lm -ldl -lpthread" LD_LIBRARY_PATH="$PWD/lib/linux_amd64" go test -tags sqlite_fts5 ./cmd/tidmouth ./cmd/yard` ✅
- focused validation for the Phase 5 Knapford live-surface cleanup:
  - `bash ./scripts/test_install_user_bin.sh` ✅
  - `docker compose config` ✅
  - `make all` ✅
  - `make build` ✅
  - `make test` ✅
  - targeted live-file search across `Makefile`, `Dockerfile`, `docker-compose.yaml`, `README.md`, `scripts/*.sh`, and `docs/specs/*.md` for `knapford` now returns no matches ✅
- focused validation for the immediate post-Knapford packaging/install cleanup:
  - `bash ./scripts/test_install_user_bin.sh` ✅
  - `docker compose config` ✅
  - `make build` ✅
  - `make all` ✅
  - `make test` ✅
  - targeted live-file search across `Dockerfile`, `README.md`, `scripts/*.sh`, `docs/specs/*.md`, `Makefile`, and `docker-compose.yaml` for `sirtopham` now returns no matches ✅
  - `make all` / `make build` now remove stale retired binaries from `bin/`, leaving only `tidmouth` and `yard` in the built artifact directory ✅

Immediate next checkpoint
- The no-legacy/live-surface cleanup is substantially complete.
- The active doc set should now stay small and current-truth: README, current specs, and this handoff.
- If another cleanup slice is needed, prefer narrow current-truth fixes over reviving historical migration/implementation plans.

Best next slice
1. Audit the remaining current specs for any operator-facing guidance that still diverges from the live `yard` / `tidmouth` / container/runtime contract.
2. Patch only the still-live current-truth docs that are actually misleading.
3. Avoid recreating broad historical planning backlogs unless a new real implementation effort needs them.

Files likely in next slice
- `README.md`
- `docs/specs/*.md`
- `NEXT_SESSION_HANDOFF.md`
- optionally `AGENTS.md` if workflow guidance drifts again

Do not change
- Do not redesign the orchestrator spawn contract.
- Do not remove `tidmouth run` or `tidmouth index` unless `internal/spawn/spawn_agent.go` and `internal/runtime/orchestrator.go` are changed together with passing validation.
- Do not reopen already-closed Phase 0-4 cleanup just to chase stylistic consistency.

Useful commands
```bash
make test
make build
./bin/tidmouth --help
./bin/yard --help
rtk go list ./...
```

Before handing off again
- Update this file with the exact Phase 4 checkpoint reached.
- Record any failing validation command verbatim.
- Note whether broader suites were intentionally skipped because the slice was docs-only or test-only.
- Note the next unresolved narrow sub-step, not a broad theme.
