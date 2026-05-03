# Next session handoff

You are resuming work in `/home/gernsback/source/sodoryard`.

Objective
- Keep work narrow, behavior-preserving, and well-validated.
- Prefer current-truth docs (`README.md`, specs, this handoff) over historical planning artifacts.
- Do not reopen runtime/provider/UI/broad architecture work unless the user explicitly asks.

Read first
1. `AGENTS.md`
2. `RTK.md`
3. `README.md`
4. `NEXT_SESSION_HANDOFF.md`
5. `docs/specs/20-operator-console-tui.md`
6. `TUI_IMPLEMENTATION_PLAN.md`

Current repo truth
- The active UI direction is TUI-first: bare `yard` is the daily-driver operator console, and `yard serve` remains the optional web inspector/API surface.
- Bare `yard` is wired through `cmd/yard/tui.go`; the Bubble Tea app lives in `internal/tui`.
- Shared operator services live in `internal/operator`; the TUI calls them directly and does not require or start `yard serve`.
- `tidmouth` remains internal engine plumbing. Do not expose it as an operator-facing surface.
- Landed TUI features include raw provider/model chat without an agent role prompt, readiness metadata, recent chains and chain detail, step/event display, live event follow, pause/cancel, receipt summaries/content, receipt opening through `$PAGER`/`$EDITOR`, launch preview/start for `one_step_chain`, `manual_roster`, `constrained_orchestration`, and `sir_topham_decides`, chain/receipt filtering, web-inspector target handoffs, built-in and custom launch presets, persistent current launch drafts, and launch role-list add/remove/clear controls.
- Resume is still a foreground command handoff: the TUI shows `yard chain resume <chain-id>` rather than continuing runner execution inside the TUI.
- Remaining TUI-first product gaps are project tree file attachment and fuller browser inspector parity.

Most recent landed slices
- Implemented TUI search/filter for chains and receipts inside `internal/tui`.
- `/` starts filter editing on the chains and receipts screens.
- `esc` exits filter editing and keeps the current query; `ctrl+u` clears it; backspace edits it; an empty query means no filtering.
- Chain filtering matches loaded chain summary data: chain ID, status, source task, source specs, and current step role/status/verdict/receipt path where available.
- Receipt filtering matches loaded receipt summary data: label, step, path, and the visible loaded receipt content without broad receipt reads.
- Filter changes clamp cursors safely and refresh the currently selected chain/detail/receipt so selection stays coherent.
- Help/footer and render output now show the filter keys and active filter state.
- Implemented notice-only web-inspector handoffs for selected chains and receipts.
- `w` shows the `yard serve` command and target URL. It does not detect, start, or supervise the web server.
- The TUI target base URL comes from the configured `server.host` / `server.port` when available and falls back to `http://localhost:8090`.
- Implemented constrained orchestration through `internal/operator` and `internal/chainrun`.
- The TUI launch mode cycle now includes `constrained_orchestration`; `n` adds allowed roles for that mode.
- Constrained orchestration reuses the existing orchestrator execution path and injects the allowed-role list into the orchestrator task packet. It does not add a second scheduler or durable launch table.
- Implemented built-in TUI launch presets. `b` cycles presets generated from configured roles; presets preserve the current task/spec draft and only change mode/role selection.
- Implemented persistent current launch drafts. The launch screen saves with `s` and loads with `L`; drafts are stored through `internal/operator` in the `.yard/yard.db` `launches` table and do not add a second execution path.
- Implemented custom TUI launch presets. `B` saves the current role/mode shape as a durable preset in `launch_presets`; `b` cycles built-in and custom presets while preserving task/spec draft text.
- Implemented richer launch role-list controls. `n` adds a manual roster or constrained allowed-role entry, `-` removes the last entry, and `ctrl+u` clears the active role list.

Validation completed for the landed slice
- Focused TUI package:
  - `rtk env CGO_ENABLED=1 CGO_LDFLAGS='-L/home/gernsback/source/sodoryard/lib/linux_amd64 -llancedb_go -lm -ldl -lpthread' LD_LIBRARY_PATH='/home/gernsback/source/sodoryard/lib/linux_amd64' go test -tags sqlite_fts5 ./internal/tui` ✅
- Focused TUI plus CLI command wiring:
  - `rtk env CGO_ENABLED=1 CGO_LDFLAGS='-L/home/gernsback/source/sodoryard/lib/linux_amd64 -llancedb_go -lm -ldl -lpthread' LD_LIBRARY_PATH='/home/gernsback/source/sodoryard/lib/linux_amd64' go test -tags sqlite_fts5 ./internal/tui ./cmd/yard` ✅
- Focused constrained launch support:
  - `rtk env CGO_ENABLED=1 CGO_LDFLAGS='-L/home/gernsback/source/sodoryard/lib/linux_amd64 -llancedb_go -lm -ldl -lpthread' LD_LIBRARY_PATH='/home/gernsback/source/sodoryard/lib/linux_amd64' go test -tags sqlite_fts5 ./internal/chainrun ./internal/operator ./internal/tui` ✅
- Focused built-in preset support:
  - `rtk env CGO_ENABLED=1 CGO_LDFLAGS='-L/home/gernsback/source/sodoryard/lib/linux_amd64 -llancedb_go -lm -ldl -lpthread' LD_LIBRARY_PATH='/home/gernsback/source/sodoryard/lib/linux_amd64' go test -tags sqlite_fts5 ./internal/tui` ✅
- Full project validation should be rerun after any follow-up slice:
  - `rtk make test`
  - `rtk make build`

Recommended next order
1. Project tree file attachment or browser inspector parity.

Do not change by default
- Do not add search behavior to `cmd/yard`.
- Do not shell out from the TUI to Cobra commands for core behavior.
- Do not add database tables for TUI filter/search.
- Do not secretly start `yard serve` from the TUI unless a future slice explicitly designs that behavior.
- Do not churn `yard.yaml`, `.yard/`, or `.brain/` unless the task explicitly requires it.
- Do not create new standing plan docs unless specifically needed.

Recommended workflow for the next agent
1. Inspect repo state with `rtk git status --short --branch`.
2. Read the files listed above before deciding scope.
3. Keep new TUI behavior in `internal/tui` unless the feature genuinely belongs in shared runtime packages.
4. Prefer focused model/render tests in `internal/tui/model_test.go` and `internal/tui/render_test.go`.
5. Finish with `rtk make test` and `rtk make build`.
