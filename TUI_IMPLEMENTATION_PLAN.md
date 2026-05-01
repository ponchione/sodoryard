# TUI Implementation Plan

Last updated: 2026-05-01

This is the concrete implementation plan for moving Yard to the new TUI-first operator direction.

Primary specs:

- `docs/specs/20-operator-console-tui.md`
- `docs/specs/21-web-inspector.md`
- `docs/specs/18-unified-yard-cli.md`
- `docs/specs/15-chain-orchestrator.md`

## Current Facts

- `yard serve` already serves the React app and API. Keep it. It now maps to the web-inspector role.
- `yard chain start` already delegates to `internal/chainrun.Start`.
- Chain status/logs/receipts/control currently live in `cmd/yard/chain_*.go`.
- `internal/chain.Store` already supports chains, steps, events, status transitions, events-since polling, and receipt path lookup through steps.
- `internal/spawn.SpawnAgentTool` already knows how to run a headless `tidmouth run` step, write step rows, stream step output events, parse receipts, and update metrics.
- `yard run` is still registered in `cmd/yard/main.go` and delegates to a direct headless session through `internal/cmdutil`. This conflicts with the target contract that public single-agent work should be represented as a one-step chain.
- `chainrun.Start` is currently orchestrator-focused. It always loads the `orchestrator` role and runs an orchestrator conversation. It does not yet implement `--role`, `one_step_chain`, or manual rosters.
- No Bubble Tea/Bubbles/Lip Gloss dependencies are in `go.mod` yet.
- `yard tui` is not implemented.

## Non-Negotiables

- Do not gut or delete the current React app. It is already useful as the web inspector.
- Do not create a second execution model for the TUI.
- Do not have the TUI shell out to Cobra commands for core Yard operations.
- Do not use local HTTP as the main TUI integration path. Prefer direct internal Go services.
- Do not churn `yard.yaml`, `.yard/`, or `.brain/` outside tests.
- Keep `tidmouth run` as the internal engine entrypoint until the spawn contract is deliberately redesigned.
- Prefer `make test` and `make build`. If running Go directly, use `-tags sqlite_fts5`.

## Target Architecture

```text
cmd/yard
  cobra command wiring only
  yard chain ... calls internal services
  yard tui calls internal/tui
  yard serve calls internal/server

internal/operator
  shared operator service/read model
  loads config/runtime
  exposes runtime status, chains, events, receipts, controls, launch/start calls

internal/chainrun
  shared execution runner
  supports orchestrator mode, one-step chain mode, and later manual roster mode

internal/tui
  Bubble Tea app
  depends on internal/operator
  renders dashboard/chains/receipts/launch/control flows

internal/server
  web inspector API and websocket
  may call internal/operator for shared chain/runtime views
```

## Recommended Order

1. Build the shared operator service/read model.
2. Move existing CLI chain read/control code onto that service where practical.
3. Add a read-only `yard tui` skeleton.
4. Fix the `yard run` / one-step chain mismatch.
5. Add TUI chain controls.
6. Add TUI launch wizard.
7. Add browser inspector chain/detail views only where they add value.

This order gives a useful TUI quickly without forcing the one-step-chain refactor to block all visible progress.

## Phase 1: Shared Operator Service

Goal: create the internal API that CLI, TUI, and later web handlers can share.

New package:

```text
internal/operator/
  service.go
  status.go
  chains.go
  receipts.go
  control.go
  service_test.go
  chains_test.go
  control_test.go
```

Suggested public shape:

```go
type Service struct {
    // unexported config/runtime/deps
}

type Options struct {
    ConfigPath string
    BuildRuntime func(context.Context, *config.Config) (*runtime.OrchestratorRuntime, error)
    ProcessSignaler func(pid int) error
}

func Open(ctx context.Context, opts Options) (*Service, error)
func (s *Service) Close()

type RuntimeStatus struct {
    ProjectRoot string
    ProjectName string
    Provider string
    Model string
    AuthSummary string
    CodeIndex RuntimeIndexStatus
    BrainIndex RuntimeIndexStatus
    ActiveChains int
    Warnings []RuntimeWarning
}

type ChainSummary struct {
    ID string
    Status string
    SourceTask string
    SourceSpecs []string
    TotalSteps int
    TotalTokens int
    StartedAt time.Time
    UpdatedAt time.Time
    CurrentStep *StepSummary
}

type ChainDetail struct {
    Chain chain.Chain
    Steps []chain.Step
    RecentEvents []chain.Event
}

func (s *Service) RuntimeStatus(ctx context.Context) (RuntimeStatus, error)
func (s *Service) ListChains(ctx context.Context, limit int) ([]ChainSummary, error)
func (s *Service) GetChainDetail(ctx context.Context, chainID string) (ChainDetail, error)
func (s *Service) ListEvents(ctx context.Context, chainID string) ([]chain.Event, error)
func (s *Service) ListEventsSince(ctx context.Context, chainID string, afterID int64) ([]chain.Event, error)
func (s *Service) ReadReceipt(ctx context.Context, chainID string, step string) (ReceiptView, error)
func (s *Service) PauseChain(ctx context.Context, chainID string) (ControlResult, error)
func (s *Service) ResumeChain(ctx context.Context, chainID string) (ControlResult, error)
func (s *Service) CancelChain(ctx context.Context, chainID string) (ControlResult, error)
```

Implementation notes:

- Start with the data already available through `rtpkg.BuildOrchestratorRuntime`.
- Move or mirror `openYardChainRuntime` from `cmd/yard/chain_readonly.go`.
- Move control logic from `cmd/yard/chain_control.go` into `internal/operator/control.go`.
- Keep process interruption injectable so tests do not signal real PIDs.
- Reuse `chain.NextControlStatus`, `chain.LatestActiveExecution`, and `chain.LatestActiveStepProcess`.
- For `RuntimeStatus`, do not block Phase 1 on perfect provider/auth/index detail. A minimal status with project root, default provider/model, and active chain count is acceptable. Expand later.
- Keep event formatting out of `internal/operator`. Return structured events. CLI/TUI can render differently.

CLI migration:

- Update `cmd/yard/chain_readonly.go` to call `internal/operator` for list/detail/events/receipt reads.
- Update `cmd/yard/chain_control.go` to call `internal/operator` for pause/resume/cancel.
- Keep `cmd/yard/chain_render.go` as CLI formatting only.
- Keep tests in `cmd/yard` focused on CLI output. Move behavioral tests to `internal/operator`.

Acceptance:

- Existing `yard chain status`, `yard chain logs`, `yard chain receipt`, `yard chain pause`, `yard chain resume`, and `yard chain cancel` behavior remains compatible.
- `internal/operator` tests cover read-only chain detail, event cursor reads, receipt path resolution, and control-state transitions.
- No Bubble Tea dependency is introduced in this phase.

Verification:

```bash
make test
make build
```

## Phase 2: Read-Only TUI Skeleton

Goal: add `yard tui` and a working full-screen read-only console.

Dependencies:

- Add stable Charm dependencies to `go.mod`:
  - `github.com/charmbracelet/bubbletea`
  - `github.com/charmbracelet/bubbles`
  - `github.com/charmbracelet/lipgloss`
- Pin exact versions selected by `go get` and commit `go.sum`.

New files:

```text
cmd/yard/tui.go
internal/tui/app.go
internal/tui/model.go
internal/tui/messages.go
internal/tui/keys.go
internal/tui/styles.go
internal/tui/screens/dashboard.go
internal/tui/screens/chains.go
internal/tui/screens/receipts.go
internal/tui/screens/help.go
internal/tui/model_test.go
internal/tui/render_test.go
```

Command:

```go
func newYardTUICmd(configPath *string) *cobra.Command
```

Register it in `cmd/yard/main.go`:

```go
newYardTUICmd(&configPath)
```

First screens:

- Dashboard:
  - project root/name
  - default provider/model
  - code/brain index status when available
  - active chain count
  - recent chains
- Chains:
  - active chains first
  - recent terminal chains
  - selected chain detail
  - recent events pane
- Receipts:
  - list steps with receipt paths
  - read selected receipt content in a viewport

Initial keybindings:

```text
q      quit
?      help
tab    next pane
enter  open selected item
esc    back
/      filter current list
r      refresh
j/k    move selection
up/down move selection
```

Implementation notes:

- `internal/tui` should depend on `internal/operator`, not `cmd/yard`.
- Start with polling refresh. Do not design a custom event bus yet.
- Do not include launch or controls in Phase 2.
- Keep rendering dense and terminal-native. Avoid large decorative boxes.
- Tests should exercise model update behavior and a few stable render fragments. Do not overfit to every whitespace column.

Acceptance:

- `yard tui` starts without `yard serve`.
- It can show chain summaries and chain detail from `.yard/yard.db`.
- It can display receipt content.
- It exits cleanly on `q` and handles terminal resize.
- It does not start chains or mutate state.

Verification:

```bash
make test
make build
```

Manual smoke:

```bash
yard chain status
yard tui
```

## Phase 3: One-Step Chain Contract

Goal: make public single-agent work a real one-step chain, then remove or hide the old public `yard run` path.

Current mismatch:

- `cmd/yard/run.go` still exposes `yard run`.
- `cmd/yard/chain.go` does not expose a role flag.
- `internal/chainrun.Start` always runs the orchestrator role.
- `internal/spawn.SpawnAgentTool` has the step execution logic we need, but it is wrapped as a tool with unexported input types.

Recommended refactor:

1. Add launch mode and role fields to `chainrun.Options`.

```go
type Mode string

const (
    ModeOrchestrator Mode = "sir_topham_decides"
    ModeOneStep      Mode = "one_step_chain"
    ModeManualRoster Mode = "manual_roster"
)

type StepRequest struct {
    Role string
    Task string
    TaskContext string
    ReindexBefore bool
}

type Options struct {
    ChainID string
    Mode Mode
    Role string
    Roster []StepRequest
    SourceSpecs []string
    SourceTask string
    // existing fields...
}
```

2. Export a reusable step runner from `internal/spawn`.

Preferred shape:

```go
type AgentStepInput struct {
    Role string
    Task string
    TaskContext string
    ReindexBefore bool
}

type AgentStepResult struct {
    StepID string
    Sequence int
    ReceiptPath string
    Verdict receipt.Verdict
    Status string
    TokensUsed int
    TurnsUsed int
    DurationSecs int
    ExitCode int
}

func (t *SpawnAgentTool) RunStep(ctx context.Context, in AgentStepInput) (AgentStepResult, string, error)
```

Then make `SpawnAgentTool.Execute` call `RunStep` and wrap the result as a tool result. This avoids duplicating subprocess, receipt, event, and metrics behavior.

3. Branch inside `chainrun.Start`.

```text
orchestrator mode:
  existing behavior

one_step_chain mode:
  create or resume chain
  register active execution
  run exactly one spawn step for opts.Role and compiled task
  finalize chain status from receipt verdict / step result

manual_roster mode:
  not required in this phase
```

4. Add `--role` to `yard chain start`.

Rules:

- No `--role`: default to orchestrator mode.
- `--role <role>`: use one-step chain mode.
- Validate role through config role resolution before running.
- Persona aliases should work because config already has `ResolveAgentRole`.

5. Reconcile `yard run`.

Preferred target:

- Remove `newYardRunCmd` from `cmd/yard/main.go`.
- Keep `internal/cmdutil` and `tidmouth run` untouched because spawn still needs `tidmouth run`.
- Update tests that expected public `yard run`.

Temporary fallback if removal is too disruptive:

- Hide `yard run` from help.
- Make it print guidance and delegate to one-step chain only if flags can be mapped safely.
- Document it as compatibility-only and remove in the next slice.

Status mapping:

- Completed/success receipt verdicts should close the chain as `completed`.
- Non-success but receipt-producing verdicts should close as `partial` unless the runner has a stronger failure reason.
- Infrastructure failure, missing receipt after fallback, timeout, and safety-limit failure should close as `failed`.
- Cancel/pause requests should preserve existing pause/cancel semantics.

Acceptance:

- `yard chain start --role coder --task "..."` creates one chain with one step and one receipt.
- The step row has the selected role, not `orchestrator`.
- The chain has terminal status after the step completes.
- `yard chain status`, `yard chain logs`, and `yard chain receipt` work for the one-step chain.
- `yard run` is no longer advertised as a public command, or it delegates to the one-step chain path with tests documenting that behavior.

Verification:

```bash
make test
make build
```

Targeted tests:

```bash
go test -tags sqlite_fts5 ./internal/chainrun ./internal/spawn ./cmd/yard
```

## Phase 4: TUI Chain Controls

Goal: let the TUI control running chains through `internal/operator`.

Add TUI actions:

- follow selected chain
- pause selected chain
- resume selected chain
- cancel selected chain
- open receipt in `$PAGER`
- open receipt/file in `$EDITOR`

Implementation notes:

- Reuse `operator.PauseChain`, `operator.ResumeChain`, and `operator.CancelChain`.
- Add confirmation modal for cancel.
- If pause/cancel writes `pause_requested` or `cancel_requested`, TUI should show requested state immediately.
- Follow mode can poll `ListEventsSince` once per second.
- The TUI should not render raw noisy step output by default. Reuse the suppression logic concept from `cmd/yard/chain_render.go`, but do not import CLI package code.

Potential shared formatter:

- If CLI and TUI need the same event summarization, move event formatting helpers to `internal/operator/eventview` or `internal/chainview`.
- Keep CLI-specific tabular output in `cmd/yard`.

Acceptance:

- TUI can follow a running chain and append new events.
- TUI can request pause/cancel and the CLI sees the updated status.
- TUI can resume a paused chain or surface the command needed if resume still requires foreground execution semantics.
- Controls are not available for terminal chains.

Verification:

```bash
make test
make build
```

Manual smoke:

```bash
yard chain start --watch=false --task "small real task"
yard tui
yard chain status <chain-id>
```

## Phase 5: TUI Launch Wizard

Goal: start new work from the TUI.

Prerequisite:

- Phase 3 one-step chain contract should be complete.

Launch modes to implement first:

1. `one_step_chain`
2. `sir_topham_decides`

Defer:

- `manual_roster`
- `constrained_orchestration`
- persistent launch drafts
- custom presets
- browser document drop integration

New operator service methods:

```go
type LaunchRequest struct {
    Mode chainrun.Mode
    Role string
    SourceTask string
    SourceSpecs []string
    ExplicitFiles []string
    Constraints []string
    OperatorNotes string
    MaxSteps int
    MaxResolverLoops int
    MaxDuration time.Duration
    TokenBudget int
    Watch bool
}

type LaunchPreview struct {
    Mode string
    Summary string
    CompiledTask string
    Warnings []RuntimeWarning
}

func (s *Service) ValidateLaunch(ctx context.Context, req LaunchRequest) (LaunchPreview, error)
func (s *Service) StartChain(ctx context.Context, req LaunchRequest, hooks ChainStartHooks) (StartResult, error)
```

TUI flow:

- New launch screen.
- Select mode.
- If one-step, select role.
- Enter task text.
- Optionally add specs by path.
- Show preflight summary.
- Confirm start.
- Route to chain follow view.

Acceptance:

- TUI can start a one-step chain.
- TUI can start an orchestrator chain.
- It validates missing task/spec inputs before starting.
- It validates missing/unknown role before one-step start.
- It shows chain ID immediately after creation.

Verification:

```bash
make test
make build
```

## Phase 6: Manual Roster Mode

Goal: support an explicit ordered set of roles without an orchestrator.

Prerequisite:

- Reusable step runner from Phase 3.

Implementation:

- Add `ModeManualRoster` branch in `chainrun.Start`.
- Execute `opts.Roster` in order.
- Each step task should include:
  - original work packet
  - previous receipt paths
  - current role/step context
- Stop policy:
  - default stop on infrastructure failure, safety limit, blocked/escalate/fix_required unless an explicit `ContinueOnNonSuccess` option is added later
  - successful or concern-only receipts proceed to next role
- After all steps, close chain as `completed` or `partial` based on verdicts.

Acceptance:

- A roster of two roles creates two ordered step rows.
- Step 2 receives the receipt path from step 1 in its task context.
- Chain status is terminal after the roster finishes.
- Pause/cancel requests stop scheduling before the next step.

## Phase 7: Web Inspector Follow-Up

Goal: keep the browser useful without rebuilding the old command center.

Do after TUI Phase 2 or later, not before.

Useful additions:

- read-only `/chains`
- read-only `/chains/:id`
- receipt markdown rendering
- event log filtering
- links from TUI "open in web" when `yard serve` is already running

Avoid:

- browser launch workbench before TUI launch works
- duplicated browser state for drafts
- browser-only execution paths

Backend:

- Add HTTP handlers backed by `internal/operator`:
  - `GET /api/chains`
  - `GET /api/chains/:id`
  - `GET /api/chains/:id/events`
  - `GET /api/chains/:id/receipts`

Frontend:

- Keep routes inspection-oriented.
- Reuse existing layout/components where possible.
- Do not reintroduce the old command-center app shell unless there is a specific need.

## Parallel Work Split

These tasks can be assigned to separate agents with minimal conflict if their write scopes stay separate.

### Agent A: Operator Read Model

Write scope:

- `internal/operator/**`
- tests under `internal/operator/**`

Deliver:

- `Service`
- runtime status minimum
- chain list/detail
- events-since
- receipt read

Do not edit:

- `cmd/yard`
- `internal/tui`
- `internal/chainrun`

### Agent B: CLI Migration To Operator Service

Write scope:

- `cmd/yard/chain_readonly.go`
- `cmd/yard/chain_control.go`
- related `cmd/yard/*test.go`

Deliver:

- CLI behavior preserved while using `internal/operator`.
- CLI formatting remains in `cmd/yard`.

Depends on:

- Agent A base service.

### Agent C: TUI Skeleton

Write scope:

- `cmd/yard/tui.go`
- `internal/tui/**`
- `go.mod`
- `go.sum`

Deliver:

- read-only `yard tui`
- dashboard/chains/receipts
- model/render tests

Depends on:

- Agent A base service.

### Agent D: One-Step Chain

Write scope:

- `internal/chainrun/**`
- `internal/spawn/**`
- `cmd/yard/chain.go`
- `cmd/yard/run.go`
- tests in those packages

Deliver:

- `yard chain start --role ...`
- one-step chain mode
- public `yard run` removed/hidden/delegated

Do not edit:

- TUI files
- web frontend

### Agent E: TUI Controls

Write scope:

- `internal/tui/**`
- maybe shared event view package if needed

Deliver:

- follow/pause/resume/cancel/open receipt

Depends on:

- Agent A
- preferably Agent C

### Agent F: Web Inspector Chain Views

Write scope:

- `internal/server/**`
- `web/src/**`

Deliver:

- read-only chain inspection routes/endpoints

Depends on:

- Agent A

Avoid:

- launch workbench
- browser-first command-center shell

## Database Considerations

Current `chains` schema does not include `launch_id` or `launch_mode`, even though newer specs mention those concepts. Do not block Phase 1 or Phase 2 on this.

When Phase 3 or later needs durable mode:

- Add `launch_mode TEXT NOT NULL DEFAULT 'sir_topham_decides'` to `chains`.
- Add compatibility upgrade for existing dev DBs.
- Update `internal/db/schema.sql`, `internal/db/init.go`, sqlc queries, generated code, and data model tests.
- Add `launch_id TEXT` only when persistent launch records are actually implemented.

Do not create `launches`, `launch_presets`, or `background_operations` tables until persistent launch drafts are being implemented.

## Test Strategy

Use focused tests at each layer:

- `internal/operator`: behavior tests with temp SQLite DB and fake process signaler.
- `internal/chainrun`: mode branching, one-step execution, roster execution, status mapping.
- `internal/spawn`: exported step runner still matches tool behavior.
- `cmd/yard`: CLI flag parsing and output compatibility.
- `internal/tui`: Bubble Tea model update tests and stable render fragment tests.
- `internal/server`: only if chain inspector endpoints are added.
- `web`: only if chain inspector routes are added.

Preferred commands:

```bash
make test
make build
```

Targeted commands during development:

```bash
go test -tags sqlite_fts5 ./internal/operator
go test -tags sqlite_fts5 ./internal/chainrun ./internal/spawn
go test -tags sqlite_fts5 ./cmd/yard
go test -tags sqlite_fts5 ./internal/tui
```

## First Slice Checklist

Start here.

1. Create `internal/operator`.
2. Implement:
   - `Open`
   - `Close`
   - `RuntimeStatus` minimum
   - `ListChains`
   - `GetChainDetail`
   - `ListEventsSince`
   - `ReadReceipt`
3. Add tests for those methods.
4. Do not add Bubble Tea yet.
5. Do not touch `yard run` yet.
6. Run `make test`.

Definition of done for first slice:

- A follow-on TUI can list chains and read events/receipts using `internal/operator` without importing `cmd/yard`.
- Existing CLI behavior is unchanged.
- Tests pass.

## Risks

- One-step chain can accidentally duplicate `SpawnAgentTool` logic. Avoid this by extracting an exported step runner from `internal/spawn`.
- TUI can become a parallel command runner. Avoid this by routing all behavior through `internal/operator`.
- Runtime status can grow too large. Start minimal and expand after the dashboard exists.
- Browser work can drift back into command-center scope. Keep browser additions read-oriented until the TUI is useful.
- Removing `yard run` can break tests and habits. Handle it in the one-step-chain slice with explicit tests and README/spec alignment.

## Final Target

The target state is:

```bash
yard tui
# daily driver

yard chain start --role coder --task "..."
# scriptable one-step chain

yard chain start --task "..."
# scriptable orchestrator chain

yard serve
# optional web inspector
```

No public workflow should require `yard run`.
