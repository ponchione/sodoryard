# 20 - Command Center UI

**Status:** Active build spec
**Last Updated:** 2026-05-01
**Owner:** Mitchell

---

## Overview

The command center is the target shape for sodoryard's browser UI. Chat remains available, but the web app is no longer centered on a simple chat transcript. It is a personal desktop observatory and launch console for one operator: Mitchell.

The command center has two jobs:

1. **Observatory:** show what Yard knows, what it is doing, what it has done, what context it used, which agents ran, which docs/receipts exist, and where runtime health is weak.
2. **Launch console:** let the operator drop documents, assemble a work packet, choose the agents to run or delegate agent selection to Sir Topham, start chains, and control those chains while they execute.

This command center is served by `yard serve` inside the existing embedded React app and Go HTTP server. It is not a separate Knapford service, a separate binary, a deployed product, or a new container service.

The current implementation already has the base web shell:
- conversation list and chat routes
- WebSocket turn streaming
- inline tool-call rendering
- context inspector
- per-conversation metrics
- provider/project/settings panels
- project tree/file REST endpoints

The missing work is the command-center layer around those pieces: desktop observatory navigation, document intake, launch workbench, explicit agent selection, chain list/detail/control, project browser, global operational status, and buildable flows for starting and supervising work from the browser.

---

## Product Goal

The command center should answer five operator questions quickly:

1. **Can Yard work right now?**
   Provider/model/auth status, local service readiness, code index state, brain index state, and project identity must be visible without starting a chat turn.

2. **What is Yard doing?**
   Active chains, running steps, recent conversations, current turn state, recent tool activity, and runtime errors must be visible from one operational surface.

3. **What work packet am I launching?**
   The operator must be able to drop docs, classify them, save them into the brain/spec structure, attach them to a launch, and see exactly which task text, docs, constraints, and agents will be given to the runtime.

4. **Which agents are going to run?**
   The operator must be able to either choose a specific agent or ordered roster, constrain the available roles, or let Sir Topham decide the chain flow.

5. **Why did Yard do that?**
   Context reports, signal flow, tool details, retrieved files, receipts, token usage, and model selection must be inspectable from the UI.

---

## Non-Goals

- No mobile UI. The command center is a desktop-only personal tool. No hamburger nav, narrow breakpoint design, mobile-first layout, or touch optimization is required.
- No multi-user auth, tenancy, hosted mode, or remote collaboration.
- No product onboarding, mass-adoption polish, telemetry, accounts, billing, or public SaaS assumptions.
- No replacement IDE/editor. The project browser is for inspection and context, not full code editing.
- No separate Knapford runtime. Command-center work belongs in the existing `yard serve` server and `web/` app.
- No browser surface for arbitrary shell commands outside existing agent/tool flows.
- No generalized task tracker. Chains, conversations, receipts, and brain docs are the units of work.

---

## Information Architecture

The command center uses the existing SPA, but the app shell changes from "conversation sidebar plus chat" to "observatory navigation plus work surfaces."

Minimum supported viewport: **1440 x 900 desktop**. Layouts may assume a pointer, keyboard, persistent left navigation, dense tables, resizable split panes, and multi-column views. No UI requirement should be shaped around mobile constraints.

| Route | Purpose |
|---|---|
| `/` | Observatory: readiness, active work, recent work, warnings, quick launch |
| `/launch` | Opens the active launch draft or creates a new one |
| `/launch/:id` | Launch workbench: drop docs, define task, choose agents or Sir Topham, start work |
| `/docs` | Document intake and read-only brain/spec/receipt browser |
| `/agents` | Agent roster, role definitions, selectable launch profiles, recent agent activity |
| `/c/new` | Start a new conversation from chat input or launch context |
| `/c/:id` | Existing conversation view with metrics and context inspector |
| `/chains` | Chain list, filters, and start-chain entry point |
| `/chains/:id` | Chain detail: status, steps, event log, controls, receipts |
| `/project` | Project browser: tree, file preview, index/brain status |
| `/metrics` | Cross-conversation and chain/runtime metrics |
| `/settings` | Existing provider/model/project settings, expanded as needed |

The left navigation should expose these as stable top-level areas:
- Observatory
- Launch
- Docs
- Agents
- Chains
- Chat
- Project
- Metrics
- Settings

The conversation list can remain as a panel inside Chat or Observatory, but it is no longer the app's primary navigation model.

### App Shell

The command center uses one persistent shell for every route:

- left navigation rail is always visible
- top status strip is always visible except in full-screen modal dialogs
- route content occupies the remaining workspace
- route-level toolbars stay attached to the top of the route content
- long lists and logs scroll inside their panes, not the whole app shell
- destructive actions require confirmation
- background operations surface as compact status rows and route-level toasts

The top status strip shows the same fields everywhere:

| Field | Source | Interaction |
|---|---|---|
| Project | `/api/project` | Opens `/project` |
| Provider/model | `/api/config` and `/api/providers` | Opens `/settings` |
| Auth | `/api/auth/providers` | Opens `/settings` |
| Code index | `/api/project` or `/api/runtime/status` | Rebuild action when stale/error |
| Brain index | `/api/project` or `/api/runtime/status` | Rebuild action when stale/error |
| Active chains | chain list/readiness response | Opens `/chains` filtered to active |
| Operations | `/api/operations/:id` for known operations | Opens operations popover |

Status strip severity rules:
- `blocking`: auth unusable, selected provider/model unavailable, project config invalid, or selected role invalid
- `warning`: stale indexes, degraded optional local services, failed non-active operation, or paused/stuck chain
- `ok`: all known required readiness checks pass
- `unknown`: endpoint not loaded yet or feature not implemented in the current build slice

The shell must not hide the route when a readiness check fails. It should make the problem obvious and let the operator inspect or fix it.

### Active Launch Draft

The command center keeps a single active launch draft pointer in browser local storage as `active_launch_id`. The server remains the source of truth for launch contents.

Rules:
- `/launch` opens `active_launch_id` when it points to an existing non-terminal draft.
- If there is no valid active draft, `/launch` creates a new draft with `POST /api/launches`, stores its ID as `active_launch_id`, and routes to `/launch/:id`.
- `/launch/:id` loads that launch and sets `active_launch_id` when its status is `draft`.
- "New launch" always creates a fresh draft and routes to `/launch/:id`.
- Attach actions from `/docs`, `/agents`, `/project`, chat launch suggestions, and chain detail always target `active_launch_id`; if no active draft exists, they create one first.
- Terminal launches remain viewable but are not editable. Duplicating a terminal launch creates a new draft with the same normalized work packet, agent plan, execution policy, and preset reference.

Draft edit persistence:
- Launch form state is editable locally.
- Field changes mark the draft dirty immediately.
- The UI saves on blur, preset application, attach/detach actions, and before preflight/start.
- The UI may also debounce saves, but it must still show `saving`, `saved`, and `save_failed` states.
- Starting a launch is disabled while a save is in flight or failed.
- If saving fails, the local edits remain visible and the error must name the failed field or validation problem when available.

This keeps cross-route attach behavior deterministic without requiring the operator to manage multiple open launch workspaces.

---

## Screen Contracts

### Observatory

The observatory is a dense operational screen, not a marketing page. It should show:

- project name/root and current configured provider/model
- provider/auth health summary
- code index and brain index status
- local LLM service summary when configured
- active chains with status, current step, elapsed time, and controls
- recent chains with outcome and receipt links
- recent conversations with last activity and token usage
- recent document drops and whether they have been indexed
- agent roster summary: configured roles, disabled/missing roles, last agent activity
- runtime warnings: stale indexes, provider/auth problems, failed background operations, stuck chains
- quick actions: new chat, start chain, rebuild code index, rebuild brain index

The first version can compose this from existing endpoints plus chain endpoints. A later version may add a consolidated summary endpoint if frontend composition becomes noisy.

First-pass browser layout:
- persistent nav rail on the far left
- top status strip for project, provider/model, auth, code index, brain index, and local service state
- primary left/main column for active chains, current step, elapsed time, and controls
- secondary center/right column for launch drafts, queued/starting launches, and recent completed work
- lower or side warning panel for recent tool failures, stale-index warnings, failed operations, and stuck-chain signals
- compact recent chat module, not a full conversation sidebar

The root screen prioritizes "what is happening now" over historical metrics. Historical and aggregate analysis belongs on `/metrics`.

Observatory modules render in this order:

1. Runtime status strip summary
2. Active chains
3. Launch drafts and recently started launches
4. Runtime warnings and failed operations
5. Recent completed chains
6. Recent conversations
7. Agent roster health
8. Recent document intake

Active chains table:

| Column | Behavior |
|---|---|
| Chain | chain ID plus linked launch/preset when available |
| Mode | `sir_topham_decides`, `constrained_orchestration`, `manual_roster`, or `one_step_chain` |
| Status | running, pause_requested, paused, cancel_requested |
| Current step | sequence number, role/persona, and step status |
| Elapsed | wall-clock elapsed since `started_at` |
| Tokens | total known token count |
| Last event | latest normal-importance event display |
| Actions | Open, Pause/Resume, Cancel |

Active chains sort first by status priority (`running`, `pause_requested`, `paused`, `cancel_requested`), then most recent activity. Terminal chains never appear in the active table.

Launch drafts table:

| Column | Behavior |
|---|---|
| Launch | launch ID and preset name when present |
| Mode | selected launch mode |
| Work | task summary or primary spec paths |
| Agents | selected/ordered/allowed roles summary |
| Updated | last draft update time |
| State | dirty only if this browser has unsaved local edits; otherwise draft/starting/running |
| Actions | Open, Duplicate, Delete draft |

Runtime warnings are grouped by severity:
- blocking warnings at the top
- stale index and degraded service warnings next
- failed background operations next
- stuck or long-running chain warnings last

Empty states must be actionable:
- no active chains: show `Start Chain` and `New Chat`
- no launch drafts: show `New Launch`
- no warnings: show a compact all-clear row, not a blank panel
- no recent docs: show `Drop docs in /docs` link

Refresh cadence:
- active chains, active operations, and launch statuses poll every 2 seconds while visible
- runtime readiness polls every 15 seconds while visible
- recent conversations, terminal chains, docs, and agent roster health refresh on navigation or manual refresh
- polling pauses when the tab is hidden and resumes on visibility change

### Launch Workbench

The launch workbench is the primary place to start work. It is not just a modal in the chain list.

It has four fixed panels:

1. **Work packet:** task text, dropped docs, selected existing brain/spec docs, explicit files, constraints, and operator notes.
2. **Agent plan:** launch mode, selected agents, role order, allowed roles, required auditors, and model/provider override if supported.
3. **Preflight:** provider/model, auth state, code index state, brain index state, local service readiness, and estimated risk warnings.
4. **Launch output:** created chain ID, live status link, event preview, and receipt links once available.

The route header shows:
- launch ID
- draft/starting/running/terminal status
- dirty/saving/saved/save_failed state
- applied preset name when present
- linked chain ID once started
- actions: New Launch, Duplicate, Save, Start Chain, Open Chain, Delete Draft

Primary start action rules:
- The button text is `Start Chain`.
- It is disabled when the draft is saving, save_failed, already starting/running/terminal, or has blocking preflight issues.
- Pressing it saves the current draft if needed, runs preflight, compiles the work packet, starts the chain, and routes the launch output panel to the linked chain.
- After start, the work packet, agent plan, execution policy, and compiled packet are read-only on the launch record.

Workbench panel contracts:

| Panel | Required controls | Required states |
|---|---|---|
| Work packet | task text, source spec picker, supporting doc picker, explicit file picker, constraints list, operator notes, drop zone | empty, dirty, saved, validation error, unsaved dropped docs |
| Agent plan | mode selector, preset picker, selected role, ordered roles, allowed roles, required roles, provider/model override | invalid role, missing role, preset disabled, mode-specific missing fields |
| Preflight | readiness checklist, warnings, blocking errors, last run timestamp, rerun action | not_run, running, passed_with_warnings, blocked |
| Launch output | linked chain, current status, current step, live preview, receipt links, open chain action | not_started, starting, running, terminal, start_failed |

The launch workbench never starts work from unsaved document content. Dropped documents must be saved to the brain first, then attached by path.

Mode selector behavior:

| Mode | Visible role controls | Hidden/ignored controls |
|---|---|---|
| `sir_topham_decides` | none by default; optional provider/model override | selected role, ordered roles, allowed roles, required roles |
| `constrained_orchestration` | allowed roles, required roles, provider/model override | selected role, ordered roles |
| `manual_roster` | ordered roles, provider/model override, continue/reindex policy | selected role, allowed roles, required roles |
| `one_step_chain` | selected role, provider/model override | ordered roles, allowed roles, required roles |

Mode changes preserve fields in the draft JSON but only validate fields relevant to the selected mode. This lets the operator switch modes without losing a partially assembled roster.

The launch workbench supports these launch modes:

| Mode | Meaning | Backend path |
|---|---|---|
| `sir_topham_decides` | Sir Topham receives the work packet and chooses the chain flow. | `internal/chainrun.Start` with orchestrator role |
| `constrained_orchestration` | Sir Topham chooses the flow but may only use the operator-selected role set and required checkpoints. | `internal/chainrun.Start` plus enforced role constraints in the spawn path |
| `manual_roster` | The operator chooses an ordered list of agents to run. Each agent gets the work packet and previous receipts in order. | New chain runner mode built on existing role/runtime primitives |
| `one_step_chain` | The operator runs one selected agent against the work packet. | One-step chain using the selected role |

All command-center launches create chains. A single-agent launch is a one-step chain, not a separate run target. This keeps observability, receipts, pause/cancel behavior, live streaming, metrics, and history on the same chain detail surface. Normal interactive chat remains conversation-based, but autonomous harness work started from the command center uses chains.

The first frozen command-center pass includes all four modes above. If implementation is staged internally, the pass is not acceptance-complete until `sir_topham_decides`, `constrained_orchestration`, `manual_roster`, and `one_step_chain` all create normal chain records and read back through the same chain detail surface.

Completed spec documents are valid primary launch inputs. A launch may be started from one or more saved spec docs without separate task text. In that case the work packet's `source_specs` are passed through to the chain runner the same way `yard chain start --specs` does. Optional task text becomes an operator instruction layered on top of the specs, such as "decompose these into epics and tasks."

The launch workbench should include a decomposition-oriented preset that selects the decomposition roles from the configured roster. The default version is a manual roster of `epic-decomposer` followed by `task-decomposer`. The operator may still let Sir Topham decide instead.

### Launch Presets

Presets are reusable launch templates. They populate the launch draft; they do not start work by themselves. After applying a preset, the operator can edit mode, roles, constraints, docs, provider/model, and notes before starting.

Built-in presets:

| Preset | Mode | Agent plan |
|---|---|---|
| `Sir Topham Decides` | `sir_topham_decides` | no explicit roles |
| `Spec Decomposition` | `manual_roster` | `epic-decomposer -> task-decomposer` |
| `Plan Only` | `one_step_chain` | `planner` |
| `Code Then Correctness` | `manual_roster` | `coder -> correctness-auditor` |
| `Audit Only` | `one_step_chain` | selected auditor role |
| `Full Audit` | `manual_roster` | `correctness-auditor -> quality-auditor -> security-auditor -> integration-auditor -> performance-auditor` |
| `Docs Review` | `one_step_chain` | `docs-arbiter` |

Preset rules:
- built-in presets are immutable, but can be duplicated into custom presets
- presets reference role config keys, not persona names
- applying a preset validates every referenced role against the current `yard.yaml`
- if a role is missing or invalid, the preset remains visible but disabled with the validation problem
- `Audit Only` is a parameterized preset: the UI asks which auditor role to use
- custom presets can be created, edited, disabled, duplicated, and deleted from the UI
- custom presets may reference any current or future role key from `yard.yaml`
- custom presets should be stored in SQLite, not written back to `yard.yaml`

The command center must not hard-code the initial 13 roles as the universe of valid agents. The built-in presets start with those roles because they are the shipped defaults, but the roster and custom presets must work with additional configured roles.

Preset picker behavior:
- show built-ins first, then enabled custom presets, then disabled custom presets when "show disabled" is on
- disabled presets are visible in management views and hidden from compact launch pickers unless explicitly expanded
- applying a preset to a draft replaces `mode`, relevant `agent_plan` fields, `execution_policy` defaults, and any empty work-packet defaults
- applying a preset must not remove existing task text, source specs, supporting docs, explicit files, or operator notes unless the operator confirms replacement
- applying a preset clears mode-specific validation errors after recalculating them
- parameterized presets open an inline parameter dialog before applying

Preset editor fields:

| Field | Behavior |
|---|---|
| Name | required, unique among custom presets |
| Description | optional |
| Enabled | controls normal picker visibility |
| Mode | one `LaunchMode` value |
| Agent plan | role pickers appropriate to the selected mode |
| Work-packet defaults | optional constraints and notes; no hidden document content |
| Execution policy | continue-on-non-success and reindex policy |

Built-in preset IDs are stable strings prefixed with `builtin:`. Custom preset IDs are generated by the server. Existing launches keep `preset_id` as historical provenance even if the custom preset is later edited or deleted.

### Document Intake

The document intake flow accepts local text documents from drag/drop, file picker, or paste.

Supported initial document types:
- `.md`
- `.txt`

Unsupported files are rejected before upload with an explicit message. Binary documents, PDFs, images, and directories are out of scope until a parser/importer is specified.

Maximum initial size is **1 MiB per document**, matching the existing `/api/project/file` read ceiling. Oversized documents are rejected before save with a clear error.

Each accepted document enters an intake table with:
- source file name
- detected title
- target brain path
- document kind: `spec`, `task`, `plan`, `architecture`, `convention`, `note`, or `reference`
- tags
- primary launch input checkbox for specs/tasks/plans
- include-in-launch checkbox
- save status

Intake queue states:

| State | Meaning | Allowed actions |
|---|---|---|
| `ready` | File text loaded and draft metadata generated. | edit metadata, save, remove |
| `invalid` | File type, size, path, title, or content validation failed. | edit metadata when possible, remove |
| `saving` | Save request in flight. | none |
| `saved` | Document exists in the brain vault. | attach as primary/supporting, open in Docs, remove from queue |
| `save_failed` | Server rejected or failed the save. | edit metadata, retry, remove |
| `indexing` | Save succeeded and brain-index operation is running. | attach, open in Docs |
| `indexed` | Save succeeded and requested brain-index operation completed. | attach, open in Docs |

Intake table columns:

| Column | Behavior |
|---|---|
| Source | original file name or `pasted text` |
| Title | editable detected title |
| Kind | select with the supported document kinds |
| Target path | editable brain-relative `.md` path |
| Tags | editable comma/token input |
| Launch use | none, primary spec, supporting doc |
| Status | queue state plus validation/save error when present |
| Actions | Save, Save and Index, Attach, Open, Remove |

Pasted text behavior:
- default source name is `pasted-text.md`
- title detection runs against pasted content
- default kind is `note` unless the first heading or operator selection says otherwise
- pasted text follows the same size, path, frontmatter, and collision rules as dropped files

Saving a dropped document writes markdown into the configured brain vault. The operator can edit the generated target path before saving. The server must sanitize paths, require brain-relative paths, reject traversal, and create parent directories only under the configured vault.

The UI defaults to standard folders, but the operator may type a custom brain-relative path. Custom paths are allowed if they pass the same validation rules. This keeps the brain flexible while still making the common spec/task/plan paths fast.

Default target paths:

| Kind | Path pattern |
|---|---|
| `spec` | `specs/<slug>.md` |
| `task` | `tasks/<slug>.md` |
| `plan` | `plans/<slug>.md` |
| `architecture` | `architecture/<slug>.md` |
| `convention` | `conventions/<slug>.md` |
| `note` | `notes/<slug>.md` |
| `reference` | `references/<slug>.md` |

Title detection order:
1. frontmatter `title`
2. first markdown `# H1`
3. source filename without extension

Slug generation:
- start from detected title
- lowercase
- trim leading/trailing whitespace
- replace whitespace runs with `-`
- strip characters outside `a-z`, `0-9`, `-`, and `_`
- collapse repeated `-`
- if the result is empty, use `untitled`

Saved intake docs should get deterministic frontmatter. If the document already has frontmatter, preserve it and merge missing command-center fields. If a field already exists, do not overwrite it during the initial save.

```yaml
---
title: <title>
kind: <kind>
created_by: command_center
created_at: <RFC3339 timestamp>
tags: [<tags>]
---
```

For `.txt` files, save as markdown by wrapping the original text under a `# <title>` heading unless the generated body already starts with a markdown heading. For `.md` files, preserve the original body content exactly aside from frontmatter insertion/merge.

Existing file collisions are rejected by default. A later revision may add explicit overwrite/update behavior, but the initial implementation should not risk replacing existing brain docs from a drag/drop flow.

Collision handling:
- the client may preflight collisions when listing existing docs is available, but the server is authoritative
- a collision returns `409` with code `path_exists`
- the UI keeps the intake row in `save_failed` and focuses the target path field
- suggested alternate paths may be shown by appending `-2`, `-3`, etc., but the operator must choose the final path

Saving docs marks the brain index stale. The operator can rebuild the brain index from the command center after saving, but saving does not automatically run indexing unless an explicit "save and index" action is selected. "Save and index" performs the writes first, then starts a brain-index background operation.

Completed specs are treated as durable source documents, not transient uploads. When a dropped document is classified as `spec`, the command center should:
- default its target path to `specs/<slug>.md`
- offer "Use as primary spec" after save
- place the saved path in `work_packet.source_specs` when selected as a primary spec
- place the saved path in `work_packet.brain_paths` only when selected as supporting context rather than primary work definition
- preserve existing headings/body content without attempting to rewrite the spec

This supports the decomposition-agent workflow: drop a completed spec, save it as a brain spec, choose the decomposition preset or manual `epic-decomposer -> task-decomposer` roster, and start the launch.

Attach behavior:
- attaching as primary spec adds the saved path to `active_launch_id.work_packet.source_specs`
- attaching as supporting doc adds the saved path to `active_launch_id.work_packet.brain_paths`
- a document path must not appear in both `source_specs` and `brain_paths`; changing the attach type moves it
- duplicate attaches are ignored and surfaced as already attached, not as errors
- attaching creates an active launch draft first when needed
- attach actions save the target launch draft immediately
- removing a queued intake row never deletes the saved brain file

### Docs Browser

The docs route is a read-only observability surface after intake. It is for finding and attaching brain documents, specs, tasks, plans, conventions, notes, and receipts. It is not a general markdown editor in the first command-center build.

Docs browser capabilities:
- list saved brain docs by kind, tag, path, and title
- show saved document content in read-only markdown/source view
- show brain index status and stale reason
- attach a document to the current launch as a primary spec or supporting brain path
- deep-link to receipts from chains
- deep-link to docs from context inspector brain hits
- show path, title, kind, tags, created/updated metadata when available

Editing saved brain docs is out of scope for this pass. If a saved document needs correction, the operator can edit it directly in the vault/Obsidian or use a later explicit update action.

Docs browser layout:
- left pane: filters and document list
- center pane: selected document rendered as markdown with source/raw toggle
- right pane: metadata, index state, backlinks/links when available, and launch attach actions

Document list columns:

| Column | Behavior |
|---|---|
| Title | title from metadata/frontmatter/path fallback |
| Kind | normalized document kind |
| Path | brain-relative path |
| Tags | compact tag list |
| Updated | updated timestamp when known |
| Index | clean/stale/missing/chunk count when available |
| Actions | Open, Attach Primary, Attach Supporting |

Default filters:
- kind: all
- tag: all
- text query: empty
- include receipts: off unless opened from a chain/receipt link

Docs browser attach rules:
- `spec`, `task`, and `plan` can be attached as primary specs
- any saved doc can be attached as supporting context
- receipts cannot be primary specs, but can be supporting context
- attaching from Docs uses the active launch draft rules above
- opening `/docs?path=<brain-relative-path>` selects the document and expands filters as needed
- opening `/docs?kind=spec` prefilters the list to specs

Read-only document view states:
- loading
- loaded
- missing document
- invalid path rejected
- unsupported binary/non-text file
- render failed, with source view still available when content loaded

### Chat

The existing chat view remains the single-agent interactive workspace. It is useful for exploratory questions and direct interactive work, but it is no longer the default first screen.

Command-center work should preserve:

- streaming token display
- tool-call cards
- context inspector
- per-conversation metrics
- cancellation
- conversation history reload

New command-center affordances may link from chat to:
- files the agent read or touched
- related chain or receipt
- project browser at a specific file
- metrics for the current turn

Chat must also support simple harness commands in natural language. The operator can type things like "Spawn the performance audit agent for X" or "Run Percy against this receipt." The chat surface should not hide that behind the launch workbench.

There are two valid outcomes for these commands:

1. If the request is simple and maps to one selected role plus a clear task, the UI/backend may create a `one_step_chain` launch draft, show a compact confirmation, and start it after confirmation.
2. If the request implies multiple roles, attached docs, decomposition, or ambiguous scope, the UI should create a launch draft and route the operator to `/launch` with the inferred packet filled in.

Normal chat remains normal chat. The command detection should be opt-in by intent phrases such as "spawn", "run the <agent> agent", "audit", "decompose this spec", or explicit role/persona names. The fallback for ambiguous text is to send a regular chat message.

Chat command detection must never silently run work. It returns a compact "launch suggestion" that the operator can start, edit in `/launch`, or dismiss and send as normal chat. This keeps simple chat cheap and predictable.

### Chains

The chains area is the largest new surface.

The list route shows:
- active chains first
- status, task/spec source, started time, elapsed time, current step, total steps, token total
- controls for active or paused chains
- filters for running, paused, completed, failed, cancelled

Chain list table:

| Column | Behavior |
|---|---|
| Chain | chain ID, linked launch ID when present, and source mode |
| Work | first non-empty value of task summary, primary spec paths, or compiled packet title |
| Status | chain status with terminal/non-terminal styling |
| Current/last step | active step for non-terminal chains; last completed/failed step for terminal chains |
| Roles | compact ordered role list when known |
| Started | absolute timestamp plus relative age |
| Duration | running elapsed time or terminal duration |
| Tokens | total chain tokens |
| Outcome | summary/verdict for terminal chains |
| Actions | Open, Pause/Resume, Cancel, Receipt |

Default filters:
- status: active plus the 20 most recent terminal chains
- mode: all
- role: all
- text: empty

Available filters:
- status set
- launch mode
- role/persona
- source spec path
- text search over chain ID, source task, summary, and receipt paths when indexed
- date range once the list grows enough to need it

Default sort is active chains first, then newest `started_at`. The list route must not hide paused or cancel-requested chains behind terminal history.

The detail route shows:
- chain status and limits
- source task/specs
- active execution metadata when available
- ordered steps with role, task, status, verdict, duration, tokens, receipt path
- event log with normal/debug filtering matching the CLI semantics
- live agent stream for running steps
- controls: pause, resume, cancel
- receipt viewer for orchestrator and step receipts

Chain detail query params:

| Param | Meaning |
|---|---|
| `step=<step-id>` | Select a step in the timeline and live stream. |
| `receipt=<brain-relative-path>` | Open the receipt panel to a specific receipt. |
| `events=debug` | Show debug-level event rows by default. |
| `follow=1` | Keep live stream/event panes pinned to newest entries. |

If a query param references a missing step or receipt, the route shows the chain normally and displays a non-blocking warning.

Starting a chain from the UI must use the same internal runner as `yard chain start`. The browser handler must not shell out to the Cobra command. CLI-only concerns such as printing the chain ID and writing watch output stay in `cmd/yard`; chain creation, resume handling, active-execution registration, and finalization stay in `internal/chainrun`.

Chain detail must make agent participation explicit. Every step row shows:
- persona/role display name
- config role key
- task assigned to that role
- why it was selected when that information is available
- provider/model used
- status, verdict, tokens, duration, receipt path

For Sir Topham-decided chains, the event stream should distinguish orchestrator decisions from spawned-agent output.

First-pass chain detail layout:
- **Header:** chain ID, status, launch mode, source task/specs, elapsed time, token total, and controls for pause/resume/cancel/open launch/open receipt
- **Step timeline:** primary supervision surface, one row per step with role/persona, task summary, status, verdict, tokens, duration, and receipt link; current step is highlighted
- **Live agent stream:** visible-by-default stream for the running step, showing status transitions, tool calls, model waits, meaningful agent output, and current receipt/progress hints
- **Event log:** normal/debug verbosity toggle matching CLI semantics, event-type and step filters, and follow/live polling toggle
- **Receipt panel:** orchestrator receipt and selected step receipt rendered as markdown, with raw toggle
- **Agent activity panel:** current role, model/provider, process/execution metadata; raw subprocess stdout/stderr remains debug-only, but meaningful agent activity is visible in the live stream
- **Manual roster handoff panel:** compiled work packet, prior receipt paths, and prior verdict summaries when the chain came from `manual_roster`

The step timeline is the primary chain detail surface. Raw logs are secondary/debug.

Live output requirement:
- the operator must be able to watch active agents work from the browser
- live output should be scoped to the selected running step by default, with an option to show all active chain activity
- default live output includes agent status, role/persona, provider/model, important stdout/stderr lines, and receipt/progress messages; tool-call start/end and tool result summaries are included when the runner persists structured tool events
- raw subprocess chatter, full debug lines, and noisy process output are hidden unless debug mode is enabled
- if token-level assistant streaming is available for a browser-started chain step, show it; if a spawned headless step only exposes line/event output, show that persisted event stream instead
- the UI should make it clear whether it is showing token streaming, tool/event streaming, or subprocess output

Chain detail refresh rules:
- while the chain is non-terminal, poll detail every 2 seconds
- while the chain is non-terminal and the tab is visible, poll `agent-stream` every 1 second
- event log polling can use the same `after_id` cursor as agent stream
- when a chain reaches a terminal state, perform one final refresh, then stop polling
- manual refresh remains available for terminal chains

Chain control button rules:

| Chain status | Visible controls |
|---|---|
| `running` | Pause, Cancel |
| `pause_requested` | Cancel |
| `paused` | Resume, Cancel |
| `cancel_requested` | none; show pending cancellation state |
| terminal statuses | Duplicate Launch when linked, Open Receipt |

Pause, resume, and cancel actions use confirmation dialogs. Confirmation text should name the chain ID and current active step when known.

Cross-linking rules:
- tool-call file paths link to `/project?file=<project-relative-path>`
- source specs link to `/docs?path=<brain-relative-path>`
- supporting brain docs link to `/docs?path=<brain-relative-path>`
- receipts link first to `/chains/:id?receipt=<receipt-path>` so the chain context is preserved
- context inspector brain hits link to `/docs?path=<brain-relative-path>`
- context inspector explicit files and RAG hits link to `/project?file=<project-relative-path>`
- launch IDs link to `/launch/:id` once a launch-detail route exists; before that, they link to the chain

### Agents

The agents route is a roster and launch-planning surface for configured roles.

The initial roster contains the 13 shipped roles, but the UI must treat `yard.yaml.agent_roles` as the source of truth. Additional roles added to config should appear automatically without frontend code changes.

It shows every role from `yard.yaml`:
- config key
- persona name
- purpose
- tool groups/custom tools
- max turns and timeout
- configured system prompt source
- provider/model override if present
- last chain/step using that role
- availability problems, such as missing prompt or invalid tool config

From this route the operator can:
- start a one-step chain launch
- add a role to the launch workbench roster
- create a constrained role set for Sir Topham
- inspect the role prompt source
- create or update custom launch presets using selected roles

Editing role prompts/config is out of scope for the first build. This route is initially for selection, observability, and preset creation.

Agents route layout:
- left pane: role filters and roster table
- center pane: selected role detail
- right pane: launch actions and preset builder shortcuts

Role filters:
- availability: all, available, unavailable
- category: all, orchestrator, decomposer, planner, coder, auditor, resolver, docs, custom
- tool group
- provider/model override present
- text query over role key, persona, purpose, tool group, and prompt source

Role table columns:

| Column | Behavior |
|---|---|
| Role | config key and persona display name |
| Category | derived categories such as coder/auditor/custom |
| Purpose | short purpose/description |
| Tools | compact tool group/custom tool summary |
| Limits | max turns and timeout |
| Provider/model | override or default |
| Availability | available or problem count |
| Last activity | most recent chain step or conversation when available |
| Actions | Select, Add to Roster, Constrain, Start One-Step |

Role detail shows:
- full role key, persona, purpose, categories
- system prompt source and whether it resolves
- tool groups and custom tools with validation state
- brain write/deny paths when available from config
- max turns, max tokens, timeout
- provider/model override and whether it is currently available
- recent chain steps using this role
- validation problems with stable codes and suggested fix text when available

Role validation rules:
- role key must be non-empty and unique
- prompt source must be `builtin:<role>` or a readable project-relative prompt path
- every tool group must be known to the role registry
- every custom tool must be available in the execution environment where the role can run
- max turns, token limits, and timeout must be positive when set
- provider/model overrides must reference configured providers/models; unavailable auth is a readiness warning but still appears in role problems when it would block launch

Role categories are response metadata, not frontend hard-coding. The server derives categories from built-in metadata where available and may infer broad categories from role key suffixes such as `-auditor`. Custom roles with no recognized category still appear under `custom` and remain selectable.

Launch actions from role rows:
- `Start One-Step` creates or updates the active launch draft with `mode: "one_step_chain"` and `agent_plan.selected_role`
- `Add to Roster` appends the role to `agent_plan.ordered_roles` and switches to `manual_roster` unless the current mode is already manual roster
- `Constrain` adds the role to `agent_plan.allowed_roles` and switches to `constrained_orchestration` unless already constrained
- role actions save the target draft immediately and route to `/launch/:id` only when the operator asks to review the draft
- unavailable roles cannot be added to a launch; they can still be inspected

### Agent Builder

Agent extensibility is a product target. The command center should eventually let the operator create and edit custom agents from the UI, but this lands after the roster/preset foundation.

Agent Builder target behavior:
- create a new role key
- choose or write persona display name and purpose
- choose tool groups and custom tools from validated options
- set max turns and timeout
- choose a prompt source: builtin marker, existing file path, or new prompt file
- write role config into `yard.yaml` only after explicit confirmation
- create prompt files under the configured prompt/agents directory when requested
- run the same validation used by `/api/agents` before saving

Until Agent Builder lands, custom agents can still be added by editing `yard.yaml`; the command center must discover and use them dynamically.

### Project

The project route turns existing project endpoints into a usable browser:

- project metadata and index state
- file tree from `/api/project/tree`
- file preview from `/api/project/file?path=...`
- direct links from tool calls, context reports, and chain receipts to files
- optional "agent touched" or "retrieved by context" highlighting once that data is available

This is read-only. Code editing stays outside scope.

Project browser capabilities:
- browse the repository tree
- preview source files
- attach a file to the current launch as an explicit project file
- deep-link from tool-call file paths to `/project?file=<path>`
- deep-link from context inspector explicit files/RAG hits to `/project?file=<path>`
- show basic file metadata returned by the existing file endpoint

The project route must not write source files, run commands, or act as an IDE.

### Metrics

The metrics route is read-only and observational. It is useful, but it is not required for the first command-center loop of intake -> launch -> supervise -> inspect receipts.

First pass should favor dense sortable tables over charts. Add charts only after the data shape proves useful.

The metrics route aggregates what is currently only per-conversation:

- token usage by conversation, chain, provider, and model
- tool usage counts and failure rates
- context assembly quality summaries
- cache hit rate when available
- latency by provider/model
- launch and chain duration summaries

First-pass modules:
- token usage by provider/model
- token usage by chain/conversation
- tool calls and failures
- context hit/quality summaries
- launch/chain duration summaries

Metrics rows should link back to the relevant chain, launch, conversation, turn, receipt, or context report. Metrics must not become a separate analysis island.

No cost accounting is required unless the existing backend data is already sufficient. Do not invent per-token pricing config for this route in the first pass.

The first implementation may start with per-conversation metrics already exposed by `/api/metrics/conversation/:id`, then add backend aggregation endpoints as the route becomes useful.

### Settings

Settings remains the place for provider/model/project configuration and diagnostics. It should also become the place to verify the runtime pair the command center will use before starting chains or smoke tests.

---

## API Contract

### Existing APIs Used Directly

The command center should reuse these existing endpoints:

```
GET /api/project
GET /api/project/tree
GET /api/project/file?path=...
GET /api/config
PUT /api/config
GET /api/providers
GET /api/auth/providers
GET /api/conversations
GET /api/conversations/search?q=...
GET /api/conversations/:id/messages
GET /api/metrics/conversation/:id
GET /api/metrics/conversation/:id/context/:turn
GET /api/metrics/conversation/:id/context/:turn/signals
WS  /api/ws
```

### API Error Shape

Existing endpoints may keep returning `{"error": "<message>"}`. New command-center endpoints should return the compatible extended shape below so the UI can distinguish validation errors from invalid state transitions and missing resources.

```typescript
type APIErrorResponse = {
  error: string;
  code?: string;
  recoverable?: boolean;
  field_errors?: {
    field: string;
    message: string;
    code?: string;
  }[];
  details?: unknown;
};
```

HTTP status conventions for new endpoints:

| Status | Use |
|---|---|
| `400` | malformed JSON, invalid query params, invalid path syntax |
| `401` | provider auth required when an endpoint performs a provider-dependent action |
| `404` | missing chain, launch, document, preset, role, receipt, or operation |
| `409` | valid request but invalid current state, such as editing a non-draft launch or resuming a non-paused chain |
| `413` | document payload exceeds size limit |
| `422` | semantic validation failed, such as missing required launch fields or invalid role selection |
| `500` | unexpected server/runtime failure |
| `503` | required runtime dependency unavailable, such as selected provider/model or required local service |

Stable error codes should be short snake_case strings. Initial command-center codes:
- `invalid_request`
- `not_found`
- `invalid_state`
- `validation_failed`
- `preflight_blocked`
- `provider_unavailable`
- `auth_unavailable`
- `role_unavailable`
- `document_too_large`
- `path_rejected`
- `path_exists`
- `operation_failed`

### New Chain APIs

Add chain HTTP handlers under `internal/server/` backed by `internal/chain.Store` and `internal/chainrun`. Chain APIs are read/control APIs plus low-level event/receipt access. The launch workbench starts new work through `/api/launches`.

```
GET  /api/chains?status=<csv>&mode=<mode>&role=<role>&q=<query>&limit=<n>
GET  /api/chains/:id
GET  /api/chains/:id/steps
GET  /api/chains/:id/events?after_id=<id>&verbosity=normal|debug
GET  /api/chains/:id/agent-stream?after_id=<id>&step_id=<step-id>
GET  /api/chains/:id/receipt
GET  /api/chains/:id/steps/:step_id/receipt
POST /api/chains/:id/pause
POST /api/chains/:id/resume
POST /api/chains/:id/cancel
```

Initial event delivery can use polling against `/api/chains/:id/events` and `/api/chains/:id/agent-stream`. A later slice may multiplex chain events over WebSocket, but polling is acceptable for the first usable command-center implementation because chain state is already persisted in SQLite.

The `events` endpoint is the complete audit/event log. The `agent-stream` endpoint is a presentation-oriented feed for live supervision; it filters and normalizes events into what the operator wants to watch while an agent is working.

`GET /api/chains` defaults to active chains plus recent terminal chains when no query params are supplied. `status` accepts comma-separated chain statuses. `mode` uses `LaunchMode` values. `role` filters chains that contain at least one step for that role. `q` searches chain ID, source task, source spec paths, summary, and receipt paths where supported.

Chain endpoint response rules:
- `GET /api/chains` returns `{ chains: ChainSummary[] }`; `limit` defaults to 50 and is capped at 200.
- `GET /api/chains/:id` returns `ChainDetail`.
- `GET /api/chains/:id/steps` returns `{ steps: ChainStepSummary[] }` ordered by `sequence_num`.
- `GET /api/chains/:id/events` returns `{ chain_id, after_id, next_after_id, events: ChainEvent[] }`.
- `GET /api/chains/:id/receipt` returns the orchestrator receipt for the chain.
- `GET /api/chains/:id/steps/:step_id/receipt` returns the receipt for a concrete step ID, not a role name or sequence number.
- `POST /api/chains/:id/pause`, `resume`, and `cancel` return `{ chain: ChainDetail, event?: ChainEvent }` after the requested state transition is durable.
- Invalid chain state transitions return `409` with `code: "invalid_state"`; missing chains, steps, or receipts return `404`.

### Launch APIs

Launches are first-class records. A launch captures the original work packet and agent plan before it becomes a chain. UI-started work always has a launch record. CLI-started chains may not.

A launch can remain in `draft` while the operator drops docs, writes the task, chooses agents, and checks preflight. Starting the launch is a separate action.

```
GET    /api/launches
POST   /api/launches
POST   /api/launches/infer
GET    /api/launches/:id
PUT    /api/launches/:id
DELETE /api/launches/:id
POST   /api/launches/:id/duplicate
POST   /api/launches/:id/preflight
POST   /api/launches/:id/start
```

Endpoint behavior:

| Endpoint | Behavior |
|---|---|
| `GET /api/launches` | List recent launches, newest first. Supports `status` and `limit` query params. |
| `POST /api/launches` | Create a `draft` launch. Does not start a chain. Defaults `mode` to `sir_topham_decides`. |
| `POST /api/launches/infer` | Convert a natural-language harness command into a proposed draft. Does not start work. |
| `GET /api/launches/:id` | Return the launch record and linked chain summary when present. |
| `PUT /api/launches/:id` | Update a `draft` launch. Returns `409` for non-draft launches. |
| `DELETE /api/launches/:id` | Delete a `draft` launch. Returns `409` for non-draft launches. |
| `POST /api/launches/:id/duplicate` | Create a new `draft` from any existing launch, copying normalized packet, plan, policy, and preset reference. |
| `POST /api/launches/:id/preflight` | Run preflight for the saved draft and update `preflight_json`. Does not start work. |
| `POST /api/launches/:id/start` | Run preflight, create the chain, and update launch status. |

`POST /api/launches/:id/preflight` returns `409` for non-draft launches. Any draft edit makes the previous preflight stale. The response uses the same `LaunchPreflight` shape as start.

`POST /api/launches/:id/start` returns as soon as the chain record exists and execution has been launched or queued by the server process. It must not keep the HTTP request open for the full chain duration. The current `internal/chainrun.Start` call is foreground/blocking, but it invokes `Options.OnChainID` immediately after the chain record is created; the HTTP handler should run `Start` in a server-owned goroutine and return after receiving that callback or a setup error.

Start success and failure are separate response paths. A successful start returns only after a durable `chain_id` exists. Preflight failures, validation failures, provider/auth failures, and setup failures before chain creation return `APIErrorResponse` with the appropriate `4xx`/`5xx` status and no `LaunchStartResponse`. If execution fails after the chain ID was returned, the launch and chain are updated asynchronously and the UI discovers that state through polling.

### Launch Inference

`POST /api/launches/infer` powers chat-to-launch commands. The first implementation should be deterministic/rule-based, using configured role keys, persona names, and known role purpose labels. It should not call an LLM.

```typescript
type LaunchInferRequest = {
  text: string;
  conversation_id?: string;
  current_file?: string;
  selected_brain_paths?: string[];
};

type LaunchInferResponse = {
  kind: "no_launch" | "suggest_launch";
  confidence: "low" | "medium" | "high";
  reason: string;
  draft?: LaunchDraftRequest;
  ambiguous_roles?: AgentRoleSummary[];
};
```

Inference rules:
- explicit "spawn", "run", "start", "kick off", or "launch" plus a role key/persona/purpose should suggest a launch
- "performance audit" maps to `performance-auditor`
- "security audit" maps to `security-auditor`
- "quality audit" maps to `quality-auditor`
- "correctness audit" maps to `correctness-auditor`
- "integration audit" maps to `integration-auditor`
- "decompose spec" maps to manual roster `epic-decomposer -> task-decomposer` when source specs are present or selected
- Sir Topham/Hatt/orchestrator phrasing maps to `sir_topham_decides`
- if multiple roles match equally, return `suggest_launch` with `confidence: "low"` and `ambiguous_roles`
- if no launch intent is found, return `no_launch` and the chat UI sends the message normally

The chat UI behavior is:
- `no_launch`: send normal chat
- `suggest_launch` high confidence: show inline confirmation with Start, Edit in Launch, and Send as Chat actions
- `suggest_launch` low/medium confidence: show Edit in Launch and Send as Chat actions, with Start disabled until reviewed

### Preset APIs

Preset APIs expose built-in presets and UI-managed custom presets through one surface.

```
GET    /api/launch-presets
POST   /api/launch-presets
GET    /api/launch-presets/:id
PUT    /api/launch-presets/:id
DELETE /api/launch-presets/:id
POST   /api/launch-presets/:id/duplicate
POST   /api/launch-presets/:id/apply
```

Endpoint behavior:

| Endpoint | Behavior |
|---|---|
| `GET /api/launch-presets` | Return built-in and custom presets, including validation state. |
| `POST /api/launch-presets` | Create a custom preset. |
| `GET /api/launch-presets/:id` | Return one preset. |
| `PUT /api/launch-presets/:id` | Update a custom preset. Returns `409` for built-ins. |
| `DELETE /api/launch-presets/:id` | Delete a custom preset. Returns `409` for built-ins. |
| `POST /api/launch-presets/:id/duplicate` | Copy any preset into a new custom preset. |
| `POST /api/launch-presets/:id/apply` | Create or update a draft launch from the preset. Does not start work. |

```typescript
type LaunchPreset = {
  id: string;
  name: string;
  description?: string;
  builtin: boolean;
  enabled: boolean;
  mode: LaunchMode;
  agent_plan_template: AgentPlan;
  work_packet_defaults: Partial<WorkPacket>;
  execution_policy_defaults?: LaunchExecutionPolicy;
  parameters?: LaunchPresetParameter[];
  validation: {
    available: boolean;
    problems: string[];
  };
  created_at?: string;
  updated_at?: string;
};

type LaunchPresetParameter = {
  key: string;
  label: string;
  type: "role";
  role_filter?: "auditor" | "any";
  required: boolean;
};

type LaunchPresetApplyRequest = {
  launch_id?: string;
  parameters?: Record<string, string>;
};

type LaunchPresetApplyResponse = {
  launch: LaunchRecord;
};
```

`Audit Only` uses a `role` parameter with `role_filter: "auditor"`. The server resolves that parameter to `agent_plan.selected_role` when applying the preset.

Custom preset creation/editing should use role pickers populated from `/api/agents`, so adding new roles to `yard.yaml` automatically makes them selectable.

Preset validation response rules:
- `available` is false when mode is invalid, required parameters are missing, referenced roles are missing/unavailable, or the preset would fail mode-specific launch validation
- `problems` should name role keys and fields, not only say "invalid preset"
- built-ins are validated against the current config on every response
- custom presets are stored even when invalid so they can be repaired in the UI

### Launch Record

```typescript
type LaunchStatus =
  | "draft"
  | "starting"
  | "running"
  | "completed"
  | "failed"
  | "cancelled";

type LaunchRecord = {
  id: string;
  mode: LaunchMode;
  status: LaunchStatus;
  work_packet: WorkPacket;
  agent_plan: AgentPlan;
  execution_policy: LaunchExecutionPolicy;
  preset_id?: string;
  compiled_packet_markdown?: string;
  preflight?: LaunchPreflight;
  preflight_stale: boolean;
  chain_id?: string;
  created_at: string;
  updated_at: string;
  started_at?: string;
  completed_at?: string;
  error?: string;
};
```

Launch status rules:
- `draft`: editable; no chain exists.
- `starting`: start request accepted, preflight passed, chain creation in progress.
- `running`: chain exists and is non-terminal.
- `completed`: chain reached a terminal successful or partial state.
- `failed`: preflight failed, chain creation failed, or chain failed.
- `cancelled`: linked chain was cancelled.

Launch status is derived from the chain when a chain exists, but the launch keeps its own status column so the observatory can show launch failures that occur before a chain ID exists.

### Launch Draft Request

```typescript
type LaunchMode =
  | "sir_topham_decides"
  | "constrained_orchestration"
  | "manual_roster"
  | "one_step_chain";

type LaunchDraftRequest = {
  mode?: LaunchMode;
  work_packet: WorkPacket;
  agent_plan: AgentPlan;
  execution_policy?: LaunchExecutionPolicy;
  preset_id?: string;
  max_steps?: number;
  max_resolver_loops?: number;
  max_duration_seconds?: number;
  token_budget?: number;
};

type WorkPacket = {
  task?: string;
  source_specs: string[];
  brain_paths: string[];
  explicit_files: string[];
  operator_notes?: string;
  constraints?: string[];
};

type AgentPlan = {
  selected_role?: string;
  ordered_roles?: string[];
  allowed_roles?: string[];
  required_roles?: string[];
  provider?: string;
  model?: string;
};

type LaunchExecutionPolicy = {
  continue_on_non_success?: boolean;
  reindex_policy?: "on_code_change" | "never" | "between_steps";
};
```

Before start, the work packet must have either `task` or at least one `source_specs` entry. A draft may temporarily have neither while the operator is assembling it. `source_specs` and `brain_paths` must reference saved brain documents. Unsaved dropped docs cannot be launched; they must be saved through the document intake API first.

Field semantics:
- `task`: operator-authored task/instruction. Required only when `source_specs` is empty.
- `source_specs`: saved brain spec paths that define the primary work. These map to `chainrun.Options.SourceSpecs` for orchestrated launches.
- `brain_paths`: saved brain docs used as supporting context, not as the primary work definition.
- `explicit_files`: project-relative code/file paths the operator wants called out in the packet.
- `operator_notes`: free-form notes for the launch. These are included in the compiled work packet but are not saved as separate brain docs.
- `constraints`: explicit limits or instructions, such as "do not edit generated files" or "performance audit only."

Execution policy defaults:
- `continue_on_non_success`: `false`
- `reindex_policy`: `"on_code_change"`

UI start forms must show the resolved provider/model before submission. `sir_topham_decides` is the default mode for new drafts.

Mode-specific validation:

| Mode | Required fields | Behavior |
|---|---|---|
| `sir_topham_decides` | task or source specs | Start an orchestrated chain with no role constraints. |
| `constrained_orchestration` | task or source specs, non-empty `agent_plan.allowed_roles` or `required_roles` | Start an orchestrated chain and enforce role constraints in the spawn/finalization path. |
| `manual_roster` | task or source specs, non-empty `agent_plan.ordered_roles` | Start a deterministic chain where the server runs roles in the specified order. |
| `one_step_chain` | task or source specs, `agent_plan.selected_role` | Start a one-step chain from the browser. |

### Launch Start Response

```typescript
type LaunchStartResponse = {
  launch_id: string;
  chain_id: string;
  mode: LaunchMode;
  status: "starting" | "running";
  preflight: LaunchPreflight;
};
```

### Launch Preflight

```typescript
type LaunchPreflight = {
  status: "pass" | "warn" | "block";
  checks: LaunchPreflightCheck[];
};

type LaunchPreflightCheck = {
  key: string;
  severity: "info" | "warning" | "blocking";
  message: string;
  detail?: string;
};
```

Preflight blocks launch only for hard failures:
- neither task text nor source specs are present
- selected provider/model unavailable
- auth missing or unusable for the selected provider
- selected agent role missing or invalid
- selected custom/tool group unavailable
- referenced brain document path missing
- referenced explicit file missing or outside project root

Preflight warns but does not block for:
- stale code index
- stale or never-built brain index
- local embedding/model service degraded when not required for the selected runtime path
- high max-step/token/duration settings
- no auditors selected in manual roster mode

The start endpoint returns `409` if the launch is not in `draft`, and `422` if preflight blocks launch.

### Launch Persistence

Launch records live in SQLite alongside conversations and chains. The launch record is the source of truth for the operator-authored work packet. Runtime chains may copy selected fields into `chains.source_task`, step tasks, receipts, or conversations, but those are derived execution artifacts.

```sql
CREATE TABLE IF NOT EXISTS launches (
    id                  TEXT PRIMARY KEY,
    mode                TEXT NOT NULL DEFAULT 'sir_topham_decides',
    status              TEXT NOT NULL DEFAULT 'draft',

    work_packet_json    TEXT NOT NULL,
    agent_plan_json     TEXT NOT NULL,
    execution_policy_json TEXT NOT NULL DEFAULT '{}',
    compiled_packet_md  TEXT,
    preflight_json      TEXT,

    chain_id            TEXT,
    preset_id           TEXT,

    error_message       TEXT,
    created_at          TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at          TEXT NOT NULL DEFAULT (datetime('now')),
    started_at          TEXT,
    completed_at        TEXT
);

CREATE INDEX IF NOT EXISTS idx_launches_status ON launches(status);
CREATE INDEX IF NOT EXISTS idx_launches_created ON launches(created_at);
CREATE INDEX IF NOT EXISTS idx_launches_chain ON launches(chain_id);
```

Rules:
- `mode` must be one of the `LaunchMode` values.
- `status` must be one of the `LaunchStatus` values.
- `work_packet_json` and `agent_plan_json` are stored exactly as accepted after validation and normalization.
- `execution_policy_json` stores normalized launch execution policy with defaults applied.
- `compiled_packet_md` is null while the launch is a draft and is written at start after preflight succeeds.
- `preflight_json` is written by explicit preflight checks and overwritten by start preflight.
- `preflight_stale` is a response-level derived flag: true when `preflight_json` is absent or older than the current `updated_at`.
- `chain_id` is null until start creates a chain.
- `preset_id` records the preset that populated the launch when applicable; editing the draft does not mutate the preset.
- updating a draft replaces the JSON blobs and refreshes `updated_at`.
- starting a launch writes `compiled_packet_md`, `preflight_json`, `started_at`, `chain_id`, and status.
- terminal chain completion writes `completed_at` and maps the chain state to launch status.

Optional later normalization may split document attachments into a separate `launch_documents` table, but the first implementation keeps document paths in `work_packet_json` because launches are personal/local and queried mostly as whole records.

### Preset Persistence

Custom presets live in SQLite. Built-in presets are generated in code and returned through the same API with `builtin: true`; they do not need database rows.

```sql
CREATE TABLE IF NOT EXISTS launch_presets (
    id                       TEXT PRIMARY KEY,
    name                     TEXT NOT NULL,
    description              TEXT,
    mode                     TEXT NOT NULL,
    agent_plan_template_json TEXT NOT NULL,
    work_packet_defaults_json TEXT NOT NULL DEFAULT '{}',
    execution_policy_json    TEXT NOT NULL DEFAULT '{}',
    enabled                  INTEGER NOT NULL DEFAULT 1,
    created_at               TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at               TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_launch_presets_enabled ON launch_presets(enabled);
CREATE UNIQUE INDEX IF NOT EXISTS idx_launch_presets_name ON launch_presets(name);
```

Preset validation:
- `mode` must be one of the `LaunchMode` values.
- referenced roles may be missing at rest, but the API response must surface validation problems against current role config.
- disabled presets remain stored but are hidden from normal launch-preset pickers unless "show disabled" is enabled.
- deleting a custom preset does not affect existing launches that were created from it.

### Work Packet Compilation

Starting a launch compiles the accepted launch draft into deterministic markdown. This compiled packet is stored on the launch record and is the exact operator-authored input artifact the runtime receives, along with any structured `SourceSpecs` field the current runner already supports.

The compiled packet has this fixed shape:

```markdown
# Yard Launch Work Packet

## Launch
- Launch ID: <launch-id>
- Mode: <mode>
- Chain: <chain-id or pending>
- Created At: <timestamp>

## Task
<task text, or "No separate task text; primary source specs define the work.">

## Primary Specs
- <brain path>

## Supporting Brain Documents
- <brain path>

## Explicit Project Files
- <project-relative path>

## Operator Notes
<operator notes or "None.">

## Constraints
- <constraint>

## Agent Plan
- Selected role: <role or none>
- Ordered roles: <roles or none>
- Allowed roles: <roles or none>
- Required roles: <roles or none>
- Provider/model override: <provider/model or none>

## Preflight Warnings
- <warning message>
```

Rules:
- section order is fixed
- empty lists render as `- None`
- paths are rendered exactly as validated, relative to brain vault or project root as appropriate
- preflight blocking errors never appear in a started packet because blocked launches do not start
- preflight warnings are included so the receipt trail shows what the operator accepted
- the compiled packet must not include unsaved dropped document content

Runtime use:
- `sir_topham_decides`: pass `source_specs` into `chainrun.Options.SourceSpecs`; use the compiled packet as the orchestrator task/body so Sir Topham sees notes, constraints, selected docs, and preflight warnings.
- `constrained_orchestration`: same as `sir_topham_decides`, with allowed/required role constraints explicitly included in the packet and enforced by the chain runner.
- `manual_roster`: each selected role receives the compiled packet plus prior receipt paths and verdict summaries.
- `one_step_chain`: create a one-step chain; the selected role receives the compiled packet as its task message.

This makes completed spec-file launches deterministic: the saved spec path is a primary spec, the compiled packet records that path, and decomposition agents receive a clear instruction surface without requiring the UI to rewrite the spec.

### Chain Runner Contract

The command center should not own separate execution logic. Browser launches and CLI launches converge on an internal chain runner contract; Cobra commands only parse flags and render terminal output.

Target internal options shape:

```go
type StartOptions struct {
    LaunchID            string
    LaunchMode          string // sir_topham_decides, constrained_orchestration, manual_roster, one_step_chain
    ChainID             string // optional caller-provided ID
    Task                string
    SourceSpecs         []string
    CompiledPacket      string
    SelectedRole        string
    OrderedRoles        []string
    AllowedRoles        []string
    RequiredRoles       []string
    ExecutionPolicy     LaunchExecutionPolicy
    MaxSteps            int
    MaxResolverLoops    int
    MaxDurationSeconds  int
    TokenBudget         int
    Provider            string
    Model               string
    OnChainID           func(string)
}
```

The concrete Go type can differ, but the semantics are fixed:

1. `Start` validates the launch mode, source docs, role references, limits, provider/model override, and execution policy before creating the chain.
2. `Start` creates a `chains` row before any agent process starts. The row records `launch_id` when present, `launch_mode`, `source_task`, `source_specs`, and limits.
3. `OnChainID` is called exactly once after the chain row and first event are durable. If chain creation fails, it is not called.
4. All launch modes write normal chain, step, event, receipt, metric, and final status records. The UI never has to branch into a run-specific read model.
5. `Start` may block until terminal completion for CLI use. Server handlers wrap it in a background execution and return after `OnChainID`.

Mode execution:

| Mode | Runner behavior |
|---|---|
| `sir_topham_decides` | Run the existing orchestrator-managed chain flow. The compiled packet is the orchestrator task body. |
| `constrained_orchestration` | Same as `sir_topham_decides`, but `spawn_agent` rejects disallowed roles and finalization verifies required roles. |
| `manual_roster` | Run a deterministic ordered set of direct chain steps with no orchestrator agent. |
| `one_step_chain` | Run the same direct-step path as `manual_roster` with exactly one selected role and one step. |

Constrained orchestration enforcement:
- `allowed_roles` is an allow-list at the chain-runner/spawn layer, not only prompt text. When non-empty, `spawn_agent` rejects any role outside the list, logs a normal-importance chain event, and returns a tool error to the orchestrator.
- `required_roles` records checkpoints that must appear as completed steps before a constrained chain can finish successfully. If the orchestrator calls `chain_complete` without satisfying them, the runner must either reject successful finalization and ask for the missing roles or finalize as `partial` with a clear event and summary.
- `required_roles` must also be a subset of `allowed_roles` when both are present; otherwise launch validation returns `422`.

Direct-step runner semantics:

1. Create each step record with the next sequence number and a deterministic receipt path: `receipts/{role}/{chain-id}-step-{NNN}.md`.
2. Build the step task from the compiled packet, current-role instruction, prior receipt paths, and prior verdict summaries.
3. Execute the role through the internal chain-step engine.
4. Persist meaningful step output as chain events so `/api/chains/:id/agent-stream` can render live progress.
5. Parse the receipt frontmatter and store verdict, token usage, turn count, duration, and receipt path on the step.
6. Apply pause/cancel requests between steps. Cancelling an active step requests termination through the same process-control path as CLI chain cancellation.

Verdict mapping for direct-step modes:

| Receipt verdict | Step status | Continue by default | Chain effect |
|---|---|---|---|
| `completed` | `completed` | yes | success |
| `completed_with_concerns` | `completed` | yes | terminal `partial` if no later hard failure |
| `completed_no_receipt` | `completed` | yes | terminal `partial` if no later hard failure |
| `fix_required` | `completed` | no | terminal `partial` unless `continue_on_non_success` is true |
| `blocked` | `completed` | no | terminal `partial` unless `continue_on_non_success` is true |
| `escalate` | `completed` | no | terminal `partial` unless `continue_on_non_success` is true |
| `safety_limit` | `failed` | no | terminal `failed` unless `continue_on_non_success` is true |
| missing/invalid receipt after fallback attempt | `failed` | no | terminal `failed` |

For `one_step_chain`, `continue_on_non_success` has no practical effect because there is no next step.

### Manual Roster Semantics

`manual_roster` is intentionally deterministic and does not use Sir Topham for sequencing.

Execution is strictly sequential:

1. Create a normal chain record with `source_task`, `source_specs`, attached docs, and `launch_mode: "manual_roster"`.
2. For each role in `ordered_roles`, create a step in order.
3. Before starting a step, build that step's task from:
   - the compiled work packet
   - prior step receipt paths
   - prior step verdict summaries
   - an explicit instruction naming the current role and what that role is expected to do
4. Run the selected role through the existing step execution primitives.
5. Parse the role receipt and update the step verdict/metrics.
6. If the verdict mapping says not to continue, stop the roster unless `execution_policy.continue_on_non_success` is explicitly true.
7. Mark the chain `completed`, `partial`, or `failed` from the final step outcomes.

Receipts are the handoff mechanism. Manual roster agents should not receive hidden conversation history from prior steps. If a prior step matters, it must be visible through receipt paths and verdict summaries in the next step task.

Reindex policy:
- default `on_code_change`: reindex only after a step reports code changes or emits a receipt/verdict that requires fresh retrieval before the next role
- `never`: do not reindex between manual-roster steps
- `between_steps`: reindex between every completed step before starting the next one

For `on_code_change`, the runner must use a machine-checkable signal rather than receipt prose alone. The initial implementation may snapshot `git status --porcelain` before and after each direct step, compare indexed file hashes when available, or consume structured changed-file metadata if the step engine records it. Receipt summaries may explain why reindexing happened, but they are not the authoritative change detector.

The default is intentionally conservative. Manual roster should not pay indexing cost between every step unless the operator asks for it.

This mode exists because sometimes the operator knows the desired sequence better than the orchestrator.

### Chain Summary Response

```typescript
type ChainSummary = {
  id: string;
  launch_id?: string;
  launch_mode: LaunchMode;
  preset_id?: string;
  status: "running" | "pause_requested" | "paused" | "cancel_requested" | "completed" | "partial" | "failed" | "cancelled";
  source_task?: string;
  source_specs?: string[];
  summary?: string;
  roles?: string[];
  total_steps: number;
  total_tokens: number;
  total_duration_secs: number;
  started_at: string;
  completed_at?: string;
  current_step?: ChainStepSummary;
  last_event?: ChainEvent;
};
```

### Chain Detail Response

```typescript
type ChainDetail = ChainSummary & {
  limits: {
    max_steps: number;
    max_resolver_loops: number;
    max_duration_secs: number;
    token_budget: number;
  };
  active_execution?: {
    execution_id: string;
    orchestrator_pid?: number;
  };
};
```

### Chain Step Response

```typescript
type ChainStepSummary = {
  id: string;
  sequence_num: number;
  role: string;
  task: string;
  task_context?: string;
  status: "pending" | "running" | "completed" | "failed";
  verdict?: string;
  receipt_path?: string;
  tokens_used?: number;
  turns_used?: number;
  duration_secs?: number;
  started_at?: string;
  completed_at?: string;
};
```

### Chain Event Response

```typescript
type ChainEvent = {
  id: number;
  chain_id: string;
  step_id?: string;
  event_type: string;
  event_data?: unknown;
  created_at: string;
  display?: string;
};
```

`display` is optional server-rendered text using the same normal/debug filtering semantics as `yard chain logs`. The client should still keep `event_type` and `event_data` for richer rendering.

```typescript
type ChainReceiptResponse = {
  chain_id: string;
  step_id?: string;
  path: string;
  content: string;
  raw_frontmatter?: Record<string, unknown>;
};
```

Receipt responses read brain-relative paths only. The server derives the receipt path from the chain or step record and must not accept arbitrary filesystem paths through these chain receipt endpoints.

### Agent Stream Response

```typescript
type AgentStreamResponse = {
  chain_id: string;
  step_id?: string;
  after_id: number;
  next_after_id: number;
  stream_source: "events" | "token_stream" | "subprocess";
  entries: AgentStreamEntry[];
};

type AgentStreamEntry = {
  id: number;
  chain_id: string;
  step_id?: string;
  role?: string;
  persona?: string;
  provider?: string;
  model?: string;
  kind:
    | "status"
    | "token"
    | "thinking"
    | "tool_start"
    | "tool_output"
    | "tool_end"
    | "stdout"
    | "stderr"
    | "receipt"
    | "decision";
  text: string;
  importance: "normal" | "debug";
  source: "events" | "token_stream" | "subprocess";
  created_at: string;
};
```

The first implementation may derive this stream from persisted chain events, including `step_output`, process lifecycle events, step start/completion events, and any structured agent events available from the runner. Entries with `importance: "debug"` are hidden unless debug mode is enabled.

Minimum first-pass stream contract:
- every running step must produce `status` entries for step start, process start when available, process exit, step completion/failure, and receipt path discovery
- meaningful stdout/stderr lines from the step process become `stdout` or `stderr` entries; noisy process chatter is marked `debug`
- structured tool events become `tool_start`, `tool_output`, and `tool_end` entries when the headless runner exposes them
- assistant text/token events become `token` entries only when the runner has a real token stream; absence of token streaming is not a failure when `stream_source` is `events` or `subprocess`
- orchestrator decisions become `decision` entries for Sir Topham-managed chains when available from tool calls or chain events
- the stream endpoint must never fabricate agent output; when a detail is unavailable, it returns the available lifecycle/event entries and labels the source accurately

Browser-started single-agent launches are one-step chains, so they use the same chain detail and agent stream shape as every other launch.

### Document APIs

The document APIs support drag/drop intake and read-only brain/spec browsing.

```
GET  /api/brain/documents?kind=<kind>&tag=<tag>&q=<query>&include_receipts=<bool>&limit=<n>
GET  /api/brain/document?path=<brain-relative-path>
POST /api/brain/documents
```

The browser reads dropped files with the File API and sends text content as JSON. The server does not need multipart upload for the initial implementation.

```typescript
type BrainDocumentKind =
  | "spec"
  | "task"
  | "plan"
  | "architecture"
  | "convention"
  | "note"
  | "reference"
  | "receipt";

type BrainDocumentWriteRequest = {
  documents: BrainDocumentDraft[];
  index_after_save?: boolean;
};

type BrainDocumentDraft = {
  source_name?: string;
  path: string;
  title: string;
  kind: Exclude<BrainDocumentKind, "receipt">;
  tags: string[];
  content: string;
  original_extension?: ".md" | ".txt";
};

type BrainDocumentListResponse = {
  documents: SavedBrainDocument[];
  brain_index_status: "stale" | "clean" | "never_indexed";
};

type BrainDocumentDetailResponse = {
  document: SavedBrainDocument & {
    content: string;
    raw_frontmatter?: Record<string, unknown>;
    line_count: number;
    backlinks?: string[];
    outgoing_links?: string[];
  };
};

type BrainDocumentWriteResponse = {
  documents: SavedBrainDocument[];
  brain_index_status: "stale" | "clean" | "never_indexed";
  operation_id?: string;
};

type SavedBrainDocument = {
  id: string;
  path: string;
  title: string;
  kind: BrainDocumentKind;
  tags: string[];
  created_at: string;
  updated_at?: string;
  index_status?: "clean" | "stale" | "missing";
  chunk_count?: number;
};
```

List endpoint behavior:
- saved brain documents must be visible immediately after save, even before `yard brain index` or a background brain-index operation runs
- the server treats the vault filesystem as the source of truth for existence and merges `brain_documents` metadata when it is available and fresh enough
- `kind` filters by one document kind. Omitted means all non-receipt documents by default.
- `include_receipts=true` includes receipt documents in all-kind lists.
- `kind=receipt` returns receipt documents even if `include_receipts` is omitted.
- `tag` filters by a single tag.
- `q` searches title, path, tags, and indexed body text when available; when the brain index is stale or missing, title/path/tag search still works from parsed vault files.
- `limit` defaults to 100 and is capped at 500.

Detail endpoint behavior:
- returns raw markdown content exactly as saved
- parses frontmatter into `raw_frontmatter` when valid
- rejects absolute/traversal paths with `400` and `path_rejected`
- returns `404` and `not_found` when the path is valid but missing

Server rules:
- `path` is always brain-relative.
- absolute paths, traversal, empty paths, and paths outside the configured vault are rejected.
- only `.md` target paths are accepted for saved brain documents in the initial implementation.
- each document content payload must be at most 1 MiB.
- the server may create parent directories inside the vault.
- existing files require an explicit overwrite flag in a later API revision; initial implementation rejects collisions.
- title/frontmatter normalization follows the Document Intake rules above.
- successful writes mark the brain index stale.
- if `index_after_save` is true, the server writes all documents first and then launches a brain-index background operation, returning `operation_id`.

### Agent APIs

The agents route reads configured roles and prompt metadata.

```
GET /api/agents?category=<category>&available=<bool>&q=<query>
GET /api/agents/:role
```

```typescript
type AgentRoleSummary = {
  key: string;
  persona?: string;
  purpose?: string;
  categories: string[];
  system_prompt_source: string;
  tool_groups: string[];
  custom_tools: string[];
  brain_write_paths?: string[];
  brain_deny_paths?: string[];
  max_turns?: number;
  max_tokens?: number;
  timeout_seconds?: number;
  provider?: string;
  model?: string;
  available: boolean;
  problems: AgentRoleProblem[];
  last_activity?: {
    kind: "chain_step" | "conversation";
    id: string;
    at: string;
    status?: string;
  };
};

type AgentRoleProblem = {
  code: string;
  message: string;
  field?: string;
  blocking: boolean;
};
```

Role summaries come from `yard.yaml`, embedded/builtin prompt metadata where available, and recent chain history. The first implementation can omit `last_activity` if no query exists yet, but `available` and `problems` must be real validation, not placeholder text.

`GET /api/agents` may return all roles and let the client filter locally in the first implementation. Query parameters are part of the target contract once role counts grow.

Initial role problem codes:
- `missing_prompt`
- `invalid_prompt_source`
- `unknown_tool_group`
- `custom_tool_unavailable`
- `invalid_limit`
- `provider_unavailable`
- `model_unavailable`
- `auth_unavailable`

### New Readiness and Operation APIs

Observatory and project screens can initially compose existing endpoints. Once quick actions land, add operation endpoints instead of shelling out to Cobra commands from HTTP handlers.

```
GET  /api/runtime/status
POST /api/index/code
POST /api/index/brain
GET  /api/operations/:id
```

`GET /api/runtime/status` should aggregate the readiness facts the observatory needs: project, configured provider/model, provider/auth health, code index state, brain index state, local-service status when configured, configured agent-role validity, active chains, and recent failed operations.

Index rebuild endpoints should launch background operations and return an operation ID. `GET /api/operations/:id` reports `pending`, `running`, `completed`, or `failed`, plus timestamps and an optional error message. These operations should call internal runtime/indexing packages directly; they should not execute the `yard` CLI.

Runtime operation output is intentionally summarized. The command center should show operation state, timestamps, final summary, and failure details, but it should not expose an arbitrary terminal stream for indexing/local-service operations. Live streaming is required for agent/chain work, not for maintenance operations.

```typescript
type OperationStartResponse = {
  operation_id: string;
  operation: OperationRecord;
};

type OperationRecord = {
  id: string;
  kind: "code_index" | "brain_index";
  status: "pending" | "running" | "completed" | "failed";
  created_at: string;
  started_at?: string;
  completed_at?: string;
  summary?: string;
  error?: string;
};
```

`POST /api/index/code` and `POST /api/index/brain` return `202` with `OperationStartResponse` after the operation record is durable. If the operation cannot be created because config is invalid or a required service is unavailable, the endpoint returns `APIErrorResponse` with `422` or `503`. Operations are persisted in SQLite so the observatory can still show recent failures after route navigation or reload.

Initial operation persistence:

```sql
CREATE TABLE IF NOT EXISTS background_operations (
    id            TEXT PRIMARY KEY,
    kind          TEXT NOT NULL,
    status        TEXT NOT NULL DEFAULT 'pending',
    summary       TEXT,
    error_message TEXT,
    created_at    TEXT NOT NULL DEFAULT (datetime('now')),
    started_at    TEXT,
    completed_at  TEXT
);

CREATE INDEX IF NOT EXISTS idx_background_operations_status ON background_operations(status);
CREATE INDEX IF NOT EXISTS idx_background_operations_created ON background_operations(created_at);
```

### Metrics APIs

Use existing per-conversation metrics endpoints first. Add aggregate metrics endpoints only when the route needs them.

```
GET /api/metrics/summary
GET /api/metrics/tokens?group_by=provider|model|chain|conversation
GET /api/metrics/tools
GET /api/metrics/context
GET /api/metrics/durations
```

Aggregate metrics responses should include stable IDs for linking back to source records. If a metric cannot be traced to a chain, launch, conversation, turn, receipt, or operation, the UI should label it as unlinked rather than fabricating navigation.

---

## Backend Implementation Notes

- Chain HTTP handlers live in `internal/server`, not `cmd/yard`.
- Launch HTTP handlers live in `internal/server`, not `cmd/yard`.
- `sir_topham_decides` and `constrained_orchestration` delegate to `internal/chainrun.Start`.
- Because `chainrun.Start` runs the chain to completion, the HTTP launch/resume handler needs a small async wrapper around it. The wrapper should bridge `Options.OnChainID` back to the request and record/log any later execution error after the response has returned.
- `manual_roster` needs a new deterministic chain runner that reuses the same runtime primitives as orchestrated chains but does not run the orchestrator agent. It must create normal chain/step/event records so the chain detail UI works unchanged.
- `one_step_chain` creates a one-step chain using the same step execution machinery as manual roster. There is no separate browser run target.
- Chain read/control delegates to `internal/chain.Store` and the same transition validation used by CLI commands.
- Long-running chain execution should run under a server-owned background context, not the request context that returned the chain ID.
- Pausing and cancelling must persist state transitions before signalling active processes, matching CLI behavior.
- Receipt reads should use the existing brain/receipt path conventions and must reject path traversal or arbitrary file access.
- Document writes should go through the same vault/backend semantics as brain tools where practical, and must mark the derived brain index stale on success.
- Agent role summaries should be derived from real config/prompt validation. Missing prompts, unknown tool groups, or unavailable custom tools must appear as role problems.
- Preset validation should reuse agent role validation. Missing preset roles must disable that preset until fixed or edited.
- Custom preset writes should not mutate `yard.yaml`; they only write `launch_presets`.
- Agent Builder writes to `yard.yaml` and prompt files only after explicit confirmation and only after passing validation.
- The API should return structured errors with stable messages for validation failures, missing chains, invalid state transitions, and unavailable runtime dependencies.

---

## Frontend Implementation Notes

- Keep the existing chat components; do not rewrite chat as part of command-center scaffolding.
- Add command-center routes incrementally and keep each route useful with partial data.
- Prefer dense desktop operational layouts over large hero sections or marketing copy.
- Do not build mobile-specific navigation or responsive compromises. The desktop view is the product.
- Use the existing `api` helper and hook style for new resources.
- Introduce chain-specific hooks:
  - `useChains`
  - `useChainDetail`
  - `useChainEvents`
  - `useChainActions`
- Introduce launch/document/agent hooks:
  - `useActiveLaunch`
  - `useLaunchWorkbench`
  - `useLaunchPresets`
  - `useLaunchPreflight`
  - `useBrainDocuments`
  - `useDocumentIntake`
  - `useAgents`
- Introduce shell/readiness hooks:
  - `useRuntimeStatus`
  - `useOperations`
- Poll chain detail/events on active chains. Stop polling terminal chains unless the user manually refreshes.
- Use confirmations for pause, resume, cancel, and index rebuild actions.
- Use stable split panes, tables, fixed toolbars, keyboard-friendly controls, and persistent side navigation.

---

## Build Phases

### Phase A - Desktop Shell and Observatory

- Replace the root chat-only landing screen with the observatory.
- Add top-level navigation for Observatory, Launch, Docs, Agents, Chains, Chat, Project, Metrics, Settings.
- Add `/launch/:id` routing and persistent app shell with top status strip.
- Populate readiness cards from existing project/config/provider endpoints.
- Implement active-chain and runtime-readiness polling cadences.
- Keep new chat creation reachable from the observatory and Chat route.

### Phase B - Document Intake and Agent Roster

- Add document write/read/list endpoints.
- Add `/docs` route with drag/drop intake, target path editing, save status, and brain index stale state.
- Ensure completed spec files can be saved as `specs/<slug>.md` and selected as primary launch specs.
- Add `/agents` route backed by real role config validation.
- Add `/agents` roster filters, role detail, and launch actions.
- Add built-in launch presets, custom preset CRUD, duplicate/apply behavior, and preset validation.
- Add launch persistence endpoints for creating/updating/deleting drafts.
- Add active launch draft behavior using `active_launch_id`.
- Add launch duplicate and draft preflight endpoints.
- Add launch workbench skeleton with preset picker, work packet, agent-plan, and preflight panels.

### Phase C - Chain Read Model

- Add chain REST read endpoints.
- Add chains list and chain detail routes.
- Render steps, event log, status, and receipts read-only.
- Poll event and live-agent-stream updates for active chains.

### Phase D - First-Pass Chain Launch and Control

- Add `POST /api/launches/:id/start` for `sir_topham_decides`, `constrained_orchestration`, `manual_roster`, and `one_step_chain`.
- Compile and store deterministic work packet markdown before runtime start.
- Add the async server wrapper around foreground `chainrun.Start`.
- Enforce constrained-orchestration `allowed_roles` and `required_roles` in the runner/spawn/finalization path.
- Add deterministic direct-step execution for `one_step_chain` and `manual_roster`.
- Add `POST /api/launches/infer` for chat-to-launch suggestions.
- Add pause/resume/cancel endpoints and UI controls.
- Start a chain from the command center and watch it progress without using the CLI.
- Confirm the UI and CLI see the same chain state.
- Normalize the minimum first-pass runner/agent events into the live agent stream.

### Phase E - Agent Selection Refinements

- Make chain detail show role/persona/model and selection rationale where available.
- Add richer stream rendering for structured token/tool events beyond the minimum persisted-event stream.
- Add workflow polish for editing inferred launch drafts from chat and chain-detail duplicate flows.

### Phase E2 - Agent Builder

- Add UI for creating custom agent roles after preset/roster flows are stable.
- Save role config to `yard.yaml` only after explicit confirmation.
- Create prompt files when requested.
- Reuse `/api/agents` validation before and after saving.

### Phase F - Project Browser

- Add `/project` route backed by existing project tree/file endpoints.
- Link tool calls, context reports, and receipts to file previews where possible.
- Keep source files read-only and support attaching file paths to launch drafts.

### Phase F2 - Docs Browser

- Add read-only saved-doc browser for brain docs, specs, tasks, plans, notes, conventions, and receipts.
- Link context inspector brain hits and source specs to `/docs`.
- Support attaching saved docs to launch drafts as primary specs or supporting brain docs.
- Do not add saved-doc editing in this pass.

### Phase G - Metrics Expansion

- Add `/metrics` route.
- Start with dense read-only tables, not charts.
- Start with conversation-level metrics and add chain/global aggregations as backend endpoints land.
- Link metric rows back to source chains, launches, conversations, turns, receipts, or context reports.

### Phase H - Runtime Operations

- Add runtime readiness aggregation if frontend composition becomes noisy.
- Add code-index and brain-index rebuild actions as background operations.
- Surface operation status in observatory/project panels.

---

## Acceptance Criteria

### First-Pass Acceptance (Phases A-D)

1. `/` is a desktop observatory showing project readiness, provider/model status, index state, active chains, recent work, dropped docs, agent-role status, and runtime warnings.
2. Existing chat workflows still work from `/c/new` and `/c/:id`; context inspector remains available for conversations.
3. `/docs` accepts dropped `.md` and `.txt` files, rejects unsupported/oversized/path-invalid documents with structured errors, saves valid docs into the brain vault, and marks the brain index stale.
4. Newly saved brain docs are visible and attachable immediately before brain indexing runs.
5. Completed spec files can be dropped, saved as primary specs, and used to launch decomposition agents without requiring separate task text.
6. `/agents` shows configured roles from real config validation, role categories, prompt/tool readiness, and launch actions.
7. The UI reads roles dynamically from `yard.yaml`; it does not hard-code the initial 13 as the valid universe.
8. Built-in presets are visible and disabled with validation problems when referenced roles are missing.
9. Custom launch presets can be created, edited, duplicated, disabled, deleted, validated, and applied from the UI without mutating `yard.yaml`.
10. `/launch` opens or creates an active draft, `/launch/:id` renders that draft, and cross-route attach actions target `active_launch_id`.
11. Draft launches can be saved, duplicated, deleted, preflighted, and started with explicit dirty/saving/save_failed states; Start performs save-before-start when the draft is dirty.
12. Starting a launch compiles deterministic work packet markdown and stores it on the launch record before runtime start.
13. `/launch/:id` can start `sir_topham_decides`, `constrained_orchestration`, `manual_roster`, and `one_step_chain` launches from task text, saved specs, or both.
14. Constrained orchestration rejects disallowed spawned roles and verifies required roles before successful finalization.
15. One-step browser launches create one-step chains; there is no separate browser run record or run detail model.
16. Chat remains normal chat by default, but launch-intent phrases can create reviewed launch suggestions for one-step chain or decomposition work.
17. Chain list and detail routes show normal chain, step, event, receipt, status, role/persona, task, token, and duration records for CLI-started and UI-started chains.
18. Chain detail includes a visible live agent stream for active steps using persisted events, with raw process output available only in debug mode.
19. Pause, resume, and cancel from the UI follow the same state-transition rules as the CLI and return structured chain-control responses.
20. Chain receipts are readable from the UI through brain-relative receipt paths derived from chain/step records.
21. Runtime maintenance operations show summarized status/output; agent and chain work exposes live activity streams.
22. The command center does not introduce a separate binary, service, container, deployment model, mobile layout, multi-user behavior, public `yard run`, or separate browser run target.
23. `make test` and `make build` remain the final validation gates for backend-integrated slices; frontend slices also run the relevant `web/` test/build commands.

### Deferred Target Acceptance

1. Project tree and file preview are available from `/project`, remain read-only, and support attaching explicit file paths to launch drafts.
2. `/docs` expands into the full read-only saved-doc browser for brain docs, specs, tasks, plans, notes, conventions, and receipts.
3. `/metrics` is read-only and links aggregate rows back to source chains, launches, conversations, turns, receipts, or context reports where possible.
4. Agent Builder can create custom role config and prompt files only after explicit confirmation and validation.
5. Runtime operation APIs support code/brain index rebuilds as summarized background operations.

---

## Dependencies

- [[07-web-interface-and-streaming]] - current web stack and streaming contract
- [[15-chain-orchestrator]] - chain state, events, controls, receipts
- [[18-unified-yard-cli]] - shared internal chain runner and CLI/runtime split
- [[08-data-model]] - SQLite persistence and metrics tables
- [[19-tool-result-details]] - structured tool rendering
