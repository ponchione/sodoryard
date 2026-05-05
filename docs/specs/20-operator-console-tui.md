# 20 - Operator Console TUI

**Status:** Active build spec
**Last Updated:** 2026-05-03
**Owner:** Mitchell

---

## Overview

The operator console is the target daily-driver interface for Yard. This spec describes target behavior. The initial implementation is available, and the intended launch command is:

```bash
yard
```

The console is not a replacement for the `yard` CLI command tree. The CLI remains the scriptable, composable surface for init, index, auth, doctor, config, brain, llm, serve, and chain commands. The TUI is the keyboard-driven operational surface for the same local runtime: raw provider/model chat, project readiness, role selection, chain launch, chain control, live event following, receipt browsing, and operational metrics. It intentionally does not become a project file browser or code review surface; operators use their IDE for that.

The browser app remains available through `yard serve`, but its target role is the web inspector described in [[21-web-inspector]]. The product split is:

- **TUI:** normal daily operation in the terminal.
- **Web inspector:** rich layout and visual inspection when terminal panes are not enough.
- **CLI:** scripts, automation, one-shot commands, and fallback operation.

All three surfaces should converge on shared internal runtime services. The TUI must not shell out to `yard chain start`, `yard index`, or other Cobra commands for core behavior. Cobra commands, the TUI, and HTTP handlers should call the same internal packages.

Implementation status as of 2026-05-03:

- Landed: bare `yard` starts the TUI, raw chat calls the configured provider/model without one of the 13 role prompts or chain tools, shared `internal/operator` reads and controls, dashboard readiness metadata, chain/detail views, chain and receipt filtering, receipt summaries/content, live event follow, pause/cancel, receipt open through `$PAGER`/`$EDITOR`, web-inspector target handoffs, built-in and custom launch presets, persistent current launch drafts, launch role-list add/remove/clear controls, and launch preview/start for `one_step_chain`, `manual_roster`, `constrained_orchestration`, and `sir_topham_decides`.
- Daily-driver final touches landed: actionable runtime readiness in the TUI, in-console pause/resume/cancel controls, and read-only browser inspector routes for chains and metrics.

---

## TUI Primer

A TUI is a full-screen terminal application. Good reference shapes are `lazygit`, `k9s`, `tig`, `btop`, and `htop`: persistent panes, keyboard shortcuts, tables, filters, logs, and modal forms inside the terminal.

For Yard, the selected stack is:

- **Bubble Tea:** app event loop. A Bubble Tea app has a model, receives messages, updates state, returns commands for async work, and renders a string view for each frame.
- **Bubbles:** reusable controls such as list, table, text input, text area, spinner, progress, paginator, and viewport.
- **Lip Gloss:** terminal styling, borders, spacing, colors, and layout.

The basic mental model:

| Term | Meaning in Yard |
|---|---|
| Model | Current app state: active route, selected chain, loaded status, form fields, errors |
| Msg | An input or event: keypress, timer tick, chain event, API response, resize |
| Update | Pure-ish state transition that handles a Msg and may start async work |
| Cmd | Async action: load runtime status, start chain, poll events, open editor |
| View | Function that renders the current model into terminal text |

This architecture makes it natural to keep the UI responsive while chains run, status refreshes, and event logs stream.

---

## Product Goals

The operator console should answer five questions quickly:

1. **Can Yard work right now?**
   Provider/model/auth state, config validity, local service readiness, code index state, brain index state, and project identity must be visible from the dashboard.

2. **What is Yard doing?**
   Active chains, running steps, recent events, current operation status, and failures should be visible without opening a browser.

3. **What work am I launching?**
   The operator should be able to write or load a task, reference source specs/docs by path, choose launch mode, choose roles, and preview the compiled work packet.

4. **Which agents are going to run?**
   The operator should be able to choose one role, an ordered manual roster, constrained orchestration, or Sir Topham-managed orchestration.

5. **What happened?**
   Receipts, event logs, step outcomes, changed files, token totals, and follow-up actions should be inspectable from the terminal, with escape hatches into `$EDITOR` or the web inspector.

---

## Non-Goals

- No attempt to duplicate every rich browser visualization.
- No arbitrary shell console inside the TUI outside existing Yard operations and agent tool output.
- No full code editor. Use `$EDITOR` for editing files and brain docs.
- No project file browser or code review surface. The normal dogfood posture is an IDE side-by-side with the TUI.
- No mobile or touch design.
- No remote multi-user dashboard, tenancy, accounts, or hosted assumptions.
- No separate runtime service, Knapford process, or container.
- No public `yard run` execution model. Single-agent autonomous work remains a one-step chain.

---

## Shell Layout

Minimum comfortable terminal size: **120 columns x 36 rows**.

The app should degrade at narrower sizes by hiding secondary panes and showing a compact warning, but the target environment is a normal developer terminal, not mobile.

Default full-screen layout:

```text
+ Yard: project / provider:model / auth / indexes / active chains ------------+
| Nav        | Main workspace                                      | Detail   |
| Dashboard  | Tables, forms, chain list, receipt list, etc.        | Logs,    |
| Launch     |                                                       | preview, |
| Chains     |                                                       | help     |
| Receipts   |                                                       |          |
| Agents     |                                                       |          |
| Settings   |                                                       |          |
+------------+-------------------------------------------------------+----------+
| ? help  / search  enter open  tab focus  esc back  q quit                     |
+------------------------------------------------------------------------------+
```

Navigation rules:

- Left nav stays visible unless the terminal is too narrow.
- Top status bar is always visible.
- Bottom key-hint bar reflects the focused pane.
- Long content scrolls inside panes, not through terminal scrollback.
- Forms should show dirty/saving/error state near the field that needs attention.
- Destructive actions require confirmation.
- When an action has a CLI equivalent, the TUI can display that equivalent for learnability.

---

## Primary Screens

### Dashboard

Purpose: "Can Yard work right now, and what is happening?"

Shows:

- project root/name
- provider/model pair
- auth status
- code index status
- brain index status
- local LLM service state when configured
- active chains
- active background operations
- recent terminal chains
- recent receipts
- runtime warnings

Actions:

- start launch wizard
- rebuild code index
- rebuild brain index
- open active chain
- open recent receipt
- open settings
- open web inspector for selected item

### Launch

Purpose: assemble and start chain work.

Launch modes:

| Mode | Behavior |
|---|---|
| `one_step_chain` | Run one selected role against the work packet |
| `manual_roster` | Run an ordered role list, each step receiving previous receipts |
| `sir_topham_decides` | Let the orchestrator choose the flow |
| `constrained_orchestration` | Let the orchestrator choose within selected allowed roles |

Built-in presets are available in the launch screen and preserve the current task/spec draft while changing mode and role selection. Custom presets can be saved from the current launch role/mode shape with `B` and are included in the `b` preset cycle. The current launch draft can be saved and loaded across sessions through shared operator state.

Fields:

- task text
- source spec paths from the brain/docs tree
- supporting brain docs
- constraints
- operator notes
- launch mode
- selected role or roster
- optional provider/model override when supported
- preflight acceptance when warnings exist

MVP behavior:

- The current launch draft lives in memory until saved. `s` saves the current draft and `L` loads the saved draft.
- Manual roster and constrained orchestration role lists can be adjusted in place. `n` appends the next role, `-` removes the last entry, and `ctrl+u` clears the active role list.
- Starting compiles a deterministic work packet and calls the same internal chain start path used by `yard chain start`.
- Persistent current drafts and custom presets are stored in Shunter project memory through `internal/operator`. Broader launch history remains future work.

### Chains

Purpose: list, follow, and control chains.

Views:

- active chains first
- recent terminal chains
- filters by status, role, mode, and text
- selected-chain detail pane
- event log pane

Actions:

- follow live events
- pause/resume
- cancel
- open receipt
- duplicate work packet into a new launch when launch records exist
- open richer browser detail if available

### Receipts

Purpose: read durable agent outputs.

Shows:

- receipt path
- chain ID and step number
- role/persona
- status/verdict
- summary
- changed files
- follow-ups
- linked event log

Actions:

- open in `$PAGER`
- open in `$EDITOR`
- copy path
- filter/search receipts
- open associated chain

### Agents

Purpose: inspect configured roles and choose them for launches.

Shows:

- config key
- persona name
- enabled/disabled status
- prompt source
- recent activity
- suitable launch modes

Actions:

- add role to current launch
- start one-step launch with selected role
- open prompt file in `$EDITOR` when it is file-backed

### Settings

Purpose: inspect and validate runtime configuration.

Shows:

- selected provider/model
- configured providers
- auth diagnostics
- local service settings
- index roots
- Shunter project-memory location and brain backend
- command equivalents for common checks

Most config editing can remain file/editor-based. The TUI should validate and explain, not become a full settings editor in the first pass.

---

## Keybindings

Default bindings:

| Key | Action |
|---|---|
| `q` | Quit, or close modal when one is open |
| `?` | Toggle contextual help |
| `tab` / `shift+tab` | Move focus between panes |
| `enter` | Open selected item or confirm focused action |
| `esc` | Back, close modal, or clear focus |
| `/` | Search/filter current list |
| `backspace` / `ctrl+u` | Edit or clear the active filter while filtering |
| `r` | Refresh focused screen |
| `n` | New launch |
| `f` | Follow selected chain |
| `p` | Pause/resume selected chain when supported |
| `x` | Cancel selected chain after confirmation |
| `e` | Open selected file/receipt in `$EDITOR` |
| `o` | Open selected receipt in `$PAGER` when on receipts |
| `w` | Show web inspector target for selected chain/receipt without starting `yard serve` |

Bindings should be visible in the bottom hint bar and discoverable through `?`.

---

## Shared Runtime Contract

The TUI needs a shared internal operator service layer. This does not need to be a network service.

Target shape:

- `cmd/yard` owns Cobra command wiring only.
- `internal/runtime` continues to build providers, stores, brain backends, and context assembly.
- `internal/chainrun` remains the shared chain start/resume runner.
- A small internal operator-facing service package may be introduced for runtime status, chain summaries, receipt loading, launch compilation, and background operations.
- HTTP handlers and TUI screens should call the same service methods where their behavior overlaps.

The TUI should prefer direct internal calls over local HTTP calls. Local HTTP is acceptable only for web-specific behavior.

Status data needed by the TUI:

- project metadata
- provider/model/auth state
- code index status
- brain index status
- local service readiness
- active chain summaries
- active operation summaries
- recent terminal chains
- recent receipts

Chain data needed by the TUI:

- chain list and filters
- chain detail
- step list
- event log
- live/follow event stream or efficient polling cursor
- pause/resume/cancel controls
- receipt lookup

Launch data needed by the TUI:

- configured roles
- role aliases/personas
- built-in presets
- draft validation
- deterministic work-packet compilation
- chain start result with chain ID

---

## Web Inspector Integration

The TUI can offer "open in web" actions, but those actions should be convenience escapes, not required paths.

Useful web handoffs:

- conversation transcript
- context inspector for a turn
- chain detail with rich event/tool layout
- receipt rendered as markdown
- side-by-side diff
- metrics charts
- provider/project settings metadata

If `yard serve` is not running, an open-in-web action may either:

- show the equivalent command to start it, or
- start the local server if a later implementation deliberately supports that behavior.

The first implementation should keep this simple and avoid hidden long-running server side effects.

Implemented first pass: the TUI shows the `yard serve` command and target web-inspector URL for the selected chain or receipt. It does not detect, start, or supervise the web server.

---

## Implementation Phases

### Phase A - Read-Only Console

- Make bare `yard` start the terminal console behind the normal build.
- Add a raw chat screen backed by the configured provider/model, with no agent role prompt, no tools, and no chain runner.
- Show dashboard with project, provider/model, auth, index state, and active chains.
- Show chains list/detail from existing stores.
- Show receipts list/detail.
- Support refresh and basic navigation.
- Status: landed.

### Phase B - Chain Control

- Follow chain events.
- Pause/resume/cancel active chains.
- Show step status and latest event.
- Open receipts/files in `$EDITOR` or `$PAGER`.
- Status: landed. Follow, pause, resume, cancel, step/event display, and receipt open are present.

### Phase C - Launch Wizard

- Start one-step chains.
- Start orchestrated chains from task text or source specs.
- Add manual roster mode when the runner supports it.
- Validate preflight warnings before start.
- Preview compiled work packet.
- Status: landed for one-step, manual-roster, constrained orchestration, and orchestrated launch preview/start. Constrained orchestration is an orchestrator-managed run with an allowed-role list, not a second scheduler.

### Phase D - Operator Polish

- Built-in presets.
- Search/filter across chains and receipts.
- Runtime readiness polish.
- Role roster actions.
- Open-in-web handoffs.
- Focused rendering tests for key screens.
- Status: chain/receipt filtering, built-in/custom launch presets, persistent current launch drafts, launch role-list add/remove/clear controls, and notice-only web-inspector target handoffs are landed. Runtime readiness polish and fuller browser inspector chain/metrics parity remain.

---

## Acceptance Criteria

1. `yard` starts a full-screen terminal app without requiring `yard serve`.
2. The chat screen can send a direct raw message to the configured provider/model and persist the transcript as a conversation.
3. The dashboard shows project, provider/model, auth, code index, brain index, and active-chain state.
4. The chains screen lists active and recent terminal chains.
5. The operator can follow a running chain and see new events without restarting the app.
6. The operator can pause/resume/cancel a chain when the chain runner supports those controls.
7. The operator can read receipts and open them in `$EDITOR` or `$PAGER`.
8. The launch wizard can start at least a one-step chain through the same internal path as `yard chain start --role`.
9. The TUI works without shelling out to Cobra commands for core Yard operations.
10. The web UI remains optional for richer inspection and is not required for normal chain operation.

---

## Open Questions

- Should a later TUI handoff detect an already-running `yard serve` and open directly, or keep the current notice-only command/URL display?
- What is the minimum useful event-follow contract for chains: store polling cursor, channel subscription, or both?
- Which screens deserve snapshot/golden tests versus ordinary model update tests?
- Should document intake be terminal-native first, browser-native first, or deferred until chain launch is stable?
