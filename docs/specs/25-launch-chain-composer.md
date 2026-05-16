# 25 - Launch Chain Composer

**Status:** Implementation-ready draft
**Last Updated:** 2026-05-16
**Owner:** Mitchell

---

## Purpose

Replace the current form-heavy Launch workbench with a visual chain composer that matches Yard's runtime model: a work packet enters an ordered sequence of agent steps, each step can receive its own dossier, and the operator can preview the resulting run sheet before starting work.

This spec is intended to be deterministic enough to implement without further product discovery. It defines the first buildable version, the public launch contract changes, backend normalization rules, frontend behavior, and validation criteria.

---

## Current Baseline

The current Launch page at `/launch` can:

- load roles from `GET /api/roles`
- load launch mode metadata from `GET /api/chains/templates`
- load/save the current launch draft through `/api/launch/draft`
- load/save custom presets through `/api/launch/presets`
- attach backend-validated project files through `/api/project/validate-paths`
- preview through `POST /api/launch/preview`
- start through `POST /api/launch/start`
- send a `LaunchRequest` with `mode`, `role`, `roster`, `allowed_roles`, `source_task`, `source_specs`, and optional limits

The backend already supports sequential manual roster execution through `chainrun.Options.Roster []StepRequest`. The public API only exposes a plain `roster: string[]`, so there is no deterministic place for per-step notes or per-step source dossiers.

The current UI also foregrounds launch modes, raw JSON, and limit fields. Those are useful implementation details but should not be the normal operator model.

### Existing Runtime Constraints

The implementation must work with these existing constraints unless a slice explicitly changes them:

- `chain.Chain` currently stores `source_task` and `source_specs`, not step dossiers.
- manual roster execution needs the ordered roster again when resuming or continuing a chain.
- `chain_started` events already carry launch metadata and are the right place to preserve normalized launch shape for runtime audit/resume.
- `/api/chains/templates` currently returns built-in launch mode metadata, not user-authored chain templates.
- `/api/launch/presets` stores user-authored reusable launch shapes.

Therefore the composer work must persist structured steps in both draft/preset storage and chain launch metadata. A running chain cannot depend on the current draft remaining unchanged after start.

---

## Product Outcome

The Launch workbench becomes a composer with three visible ideas:

```text
Work Packet -> Gordon / planner -> Thomas / coder -> Percy / correctness-auditor
```

- The **work packet** is global task/source context inherited by every step.
- Each **agent node** is one ordered chain step.
- Each **node dossier** is material assigned specifically to that step.
- The **run sheet** is the primary preview.

The operator should understand the launch without reading request JSON.

---

## MVP Scope

The first implementation covers:

- a linear chain canvas for one-step and manual-roster launches
- a role palette populated from configured agent roles
- adding, duplicating, removing, and reordering agent nodes
- duplicate roles in the same chain
- global work packet task text and global project sources
- per-node dossier note and per-node project sources
- saved user chain templates that preserve shape and reusable notes, not task-specific sources
- run-sheet preview as the primary preview
- request JSON behind a developer/details disclosure
- removal of visible limit controls from the normal Launch page
- backend support for structured roster steps while preserving `roster: string[]` compatibility

This MVP must not implement graph execution, branching, joins, conditional routing, parallel execution, or automatic retry routing.

---

## Non-Goals

- No freeform node graph.
- No parallel branches.
- No joins.
- No conditional paths.
- No default built-in chain shapes inserted into an empty canvas.
- No task-specific sources saved into reusable templates by default.
- No normal-surface controls for `max_steps`, `max_resolver_loops`, `max_duration`, `token_budget`, `step_max_turns`, `step_max_tokens`, or `allow_approval_wait`.
- No redesign of the chain runner spawn contract beyond carrying per-step dossier data into the existing sequential model.
- No replacement for context assembly, brain retrieval, or tool access.

---

## Terms

| Term | Meaning |
|---|---|
| Work packet | Global launch input: task text plus global source paths. |
| Source | A backend-validated project-relative file/spec path carried in `source_specs` or per-step `sources`. |
| Agent node | A visual step in the linear chain. |
| Node dossier | A step-local note plus step-local source paths. |
| Run sheet | Preview of ordered steps, what each receives, and what each produces. |
| Dispatcher | The orchestrator role shown as a dispatch surface, not a normal deterministic chain step in the MVP UI. |
| Saved template | A reusable user-saved chain shape: ordered roles plus reusable notes. |

---

## Resolved Product Decisions

- The normal UI infers mode from the composed surface.
- An empty canvas is invalid and cannot be started.
- One worker node maps to `one_step_chain`.
- Two or more ordered worker nodes map to `manual_roster`.
- The one-step launch is just a chain with one node.
- A launch requires a real global work packet: task text or at least one global source. Dossier-only launches are invalid.
- Manual roster is the primary explicit-control model.
- Chains are linear only.
- Per-agent dossiers are required in the MVP.
- Global sources are shown once at the work packet; connectors and run sheet show inheritance.
- Agent nodes show both persona/name when available and role key. Role key is always visible.
- The UI term is **Dossier**.
- Saved templates store ordered roles and reusable node notes only.
- Saved templates do not store visual layout metadata; layout is derived from order.
- Saved templates do not store task text, global sources, per-node source attachments, preview state, runtime state, or limits.
- Limit fields remain backend/CLI/advanced-debug concerns and are not visible in the normal Launch page.
- Approval wait is not a normal launch option. It remains available through CLI or a future advanced/debug surface if needed.

---

## Interaction Model

### Initial State

When `/launch` opens:

- Start from the saved draft if one exists; otherwise start from an empty composer.
- Merge query parameter `source_spec` values into the work packet's global sources after draft load.
- Query parameter sources must not replace draft task text, nodes, node dossiers, or existing global sources.
- Query parameter source order is appended in URL order after existing global sources, with duplicates removed.
- Do not insert a default role node.
- Disable Preview and Start until the request is valid.

Valid launch input requires:

- a non-empty global work packet: task text or at least one global source
- at least one runnable node, or a valid dispatch surface

Per-node dossier sources do not make an otherwise empty work packet valid. The node dossier is guidance for a step, not the launch's primary work request.

### Layout

The normal desktop/web layout uses three regions:

```text
Role Palette       Composer Canvas                         Inspector
------------       ---------------                         ---------
Gordon             Work Packet -> Gordon -> Thomas          Selected node
Thomas                           -> Percy                   Role details
Percy                                                       Dossier note
James                                                       Dossier sources
Rosie                                                       Run-sheet slice
Victor
```

The layout may collapse responsively, but the information architecture must remain the same:

- role palette
- work packet and chain canvas
- selected item inspector
- run-sheet preview

### Work Packet

The work packet contains:

- task text
- global source paths

Global source paths use the existing `source_specs` wire field for compatibility. The UI should label them as **Sources** or **Project Sources**, not only **Source Specs**, because current launch attachments can be normal project files.

Global sources are inherited by every step. They are displayed at the start of the canvas, not repeated visually on every node.

The backend still accepts legacy aliases `task` and `specs` on HTTP payloads. The frontend should emit `source_task` and `source_specs`.

### Role Palette

The role palette:

- loads configured roles from `GET /api/roles`
- sorts roles in the backend-provided order, or alphabetically if no order metadata exists
- allows dragging or clicking a role to add a node
- does not deduplicate roles when adding to the canvas
- shows role key for every role
- shows persona/icon if that metadata becomes available

For MVP, the dispatcher/orchestrator should not appear as a normal worker in the linear chain palette. If the UI supports orchestrated modes in this pass, show them in a separate dispatch affordance so the operator does not confuse deterministic roster execution with orchestrator-selected execution.

The built-in launch metadata returned by `GET /api/chains/templates` may still be used for labels, schema, and receipt metadata. It should not be rendered as the primary "choose a template first" workflow.

### Chain Canvas

The chain canvas:

- is a single ordered row/list of agent nodes
- preserves duplicate roles as separate nodes
- assigns each node a local UI id so duplicates can be edited independently
- reorders nodes by drag or keyboard-accessible controls
- supports duplicate, remove, and insert-between actions
- shows source/dossier counts per node
- shows no branch, join, or conditional affordance

Node order is execution order. The request sent to the backend is the canonical source of execution order.

### Node Inspector

Clicking a node opens the inspector for that node.

The inspector shows:

- role key
- persona/name if available
- short role purpose if available
- dossier note textarea
- dossier source list
- controls to attach/remove backend-validated project paths
- preview of the step's effective inputs

Dossier note text is task-specific in a launch draft. When saving a reusable template, the operator is saving the note as a reusable process note.

### Preview

Preview generates a run sheet. The run sheet is primary.

Raw request JSON stays available behind a collapsed details/debug disclosure.

Preview must not start work. Start may call preview internally for validation, but the UI should still treat preview as read-only.

### Start

Start sends the same normalized request represented by the current composer state.

After a successful start:

- navigate to `/chains/{chain_id}`
- keep the current launch draft intact unless existing draft behavior intentionally clears it
- show backend validation errors inline if start fails

---

## Mode Mapping

The composer sends deterministic launch modes from canvas state:

| Composer state | Launch mode | Request fields |
|---|---|---|
| no nodes | invalid | no preview/start |
| one worker node | `one_step_chain` | `role`, `steps[0]` |
| two or more worker nodes | `manual_roster` | `roster`, `steps` |
| dispatcher only | `sir_topham_decides` | `role: "orchestrator"` |
| dispatcher plus allowed-role pool | `constrained_orchestration` | `role: "orchestrator"`, `allowed_roles` |

Rules:

- The linear composer may not mix dispatcher mode and ordered worker nodes in the MVP.
- `roster` remains a compatibility mirror of `steps[].role` for manual roster requests.
- `role` remains a compatibility mirror of the single step role for one-step requests.
- `steps` is authoritative when present.
- A request with both `steps` and `roster` is valid only when `roster` exactly equals `steps.map(step => step.role)` after normalization. Otherwise validation fails.
- A request with mode `one_step_chain` and `steps.length != 1` fails validation.
- A request with mode `manual_roster` and `steps.length == 0` fails validation unless legacy `roster` is present.
- Duplicate roles are valid in `steps` and `roster` for manual roster.

---

## Node Dossiers

A node dossier contains step-specific operator assertions:

- a freeform note
- project source paths

Dossiers are additive:

- They do not replace the global work packet.
- They do not make an empty global work packet valid.
- They do not restrict normal context assembly.
- They do not restrict tool access.
- They do not prevent the agent from searching or reading other files allowed by its role tools.

Dossiers are priority signals:

- global work packet sources and node dossier sources should be considered assigned material
- assigned material should win over opportunistic retrieval when context budgeting forces a choice
- if context assembly cannot yet prioritize dossier sources directly, the runtime must still include the source paths in the step prompt so the agent can read them explicitly

Duplicate handling:

- duplicate paths inside one source list are removed, preserving first occurrence
- a path may appear globally and in a node dossier
- the step's effective source list is global sources first, then node sources not already present

---

## Public Contract

### Existing Fields Kept

The launch API keeps these request fields for compatibility:

```ts
type LaunchMode =
  | "one_step_chain"
  | "manual_roster"
  | "sir_topham_decides"
  | "constrained_orchestration";

interface LaunchRequest {
  template_id?: string;
  mode?: LaunchMode;
  role?: string;
  allowed_roles?: string[];
  roster?: string[];
  source_task?: string;
  source_specs?: string[];

  // Accepted by backend/CLI, not shown in normal Launch UI.
  max_steps?: number;
  max_resolver_loops?: number;
  max_duration?: string;
  token_budget?: number;
  step_max_turns?: number;
  step_max_tokens?: number;
  allow_approval_wait?: boolean;
}
```

### New Structured Step Fields

Add structured roster steps:

```ts
interface LaunchRosterStep {
  role: string;
  note?: string;
  sources?: string[];
}

interface LaunchRequest {
  steps?: LaunchRosterStep[];
}
```

`steps` represents a concrete launch instance. It is ordered. Array position is the step sequence.

For MVP, do not include UI-only node ids in the request. The backend does not need them because the ordered array is canonical. The frontend can use local ids to edit duplicate roles.

### Preview Response

Extend preview to include run-sheet data:

```ts
interface LaunchPreviewStep {
  sequence: number;
  role: string;
  note?: string;
  global_sources: string[];
  dossier_sources: string[];
  effective_sources: string[];
  prior_receipt_count: number;
  receives: string[];
  produces: string;
  brief_markdown: string;
}

interface LaunchPreview {
  mode: LaunchMode;
  template: LaunchTemplate;
  role?: string;
  allowed_roles?: string[];
  roster?: string[];
  source_task?: string;
  source_specs: string[];
  steps?: LaunchPreviewStep[];
  summary: string;
  compiled_task: string;
  work_packet_markdown: string;
  run_sheet_markdown: string;
  warnings: RuntimeWarning[];
}
```

Compatibility:

- Keep `compiled_task`.
- Keep `work_packet_markdown`.
- Keep HTTP input aliases `task` and `specs`, but normalize responses to `source_task` and `source_specs`.
- Existing clients that ignore `steps` and `run_sheet_markdown` continue to work.
- New composer UI displays `run_sheet_markdown` first.

### Normalized Response Invariants

Every successful preview/start response should reflect the normalized launch shape:

- `mode` is set.
- `role` is set for one-step and orchestrated modes.
- `roster` is set for manual roster mode and equals `steps.map(step => step.role)`.
- `allowed_roles` is set only for constrained orchestration.
- `steps` is set for one-step and manual roster modes.
- `source_task`/`source_specs` are returned on preview/start and draft/preset reads, not legacy `task`/`specs` aliases.
- source arrays preserve normalized order.

### Legacy Roster Upgrade

When the backend receives legacy `roster: string[]` without `steps`, normalize it to:

```ts
steps = roster.map((role) => ({ role }))
```

When the backend receives one-step `role` without `steps`, normalize it to:

```ts
steps = [{ role }]
```

The normalized response should include `steps`.

---

## Backend Normalization

Normalization happens before validation, preview, draft save, preset save, and start.

Rules:

1. Trim `template_id`, `mode`, `role`, `source_task`, every role string, every note, and every source path.
2. Drop empty role strings.
3. Drop empty note strings.
4. Drop empty source paths.
5. Deduplicate `allowed_roles` case-insensitively, preserving first occurrence.
6. Do not deduplicate `steps` or `roster`; repeated roles are intentional.
7. Deduplicate `source_specs` and each step's `sources` exactly after trimming, preserving first occurrence.
8. Resolve all roles through `cfg.ResolveAgentRole`.
9. Reject unknown roles with the step index in the error.
10. If `steps` is present and `roster` is present, require normalized `roster == steps.map(role)`.
11. If `steps` is present and `role` is present for one-step, require normalized `role == steps[0].role`.
12. Infer missing mode from structured state using the mode mapping table.
13. Require a global work packet: task text or at least one global source.
14. Preserve backend defaults for safety limits, but do not require the UI to send limits.

Validation error examples:

```text
manual roster requires at least one step
one-step launch requires exactly one step
launch roster does not match structured steps
resolve launch step 2 role: agent role "missing" not found in config
one of task or global sources is required
```

---

## Runtime Compilation

The runtime compiles each step into a deterministic step brief.

Effective step input order:

```text
role prompt
+ global work packet
+ this node's dossier
+ prior receipts
+ normal context assembly and brain retrieval
+ runtime tool access
```

The prompt text for a structured manual roster step should follow this shape:

```text
You are running manual roster step {sequence} for role {role} in chain {chain_id}.

Original work packet:
{task text or "No task text was provided."}
Global sources:
{global source list or "None."}

This step's dossier:
Note:
{step note or "None."}
Assigned sources:
{node source list or "None."}

Receipt history:
{previous receipt paths or "No previous receipt paths are available yet."}

Complete only the work appropriate for this roster step and produce the required receipt.
```

One-step mode with a structured step uses the same work packet and dossier sections, without receipt history.

Current `chainrun.StepRequest` must grow enough structure to carry the dossier:

```go
type StepRequest struct {
    Role          string
    TaskContext   string
    Note          string
    Sources       []string
    ReindexBefore bool
}
```

Manual roster maps `LaunchRequest.Steps` directly to `chainrun.Options.Roster`.

One-step mode must carry the single structured step's note and sources into `buildOneStepTask`. The implementation can add a `Step StepRequest`, `OneStep StepRequest`, or equivalent fields to `chainrun.Options`; behavior is what matters:

- `steps[0].role` selects the role.
- `steps[0].note` appears in the one-step brief.
- `steps[0].sources` appears in the one-step brief.
- existing legacy one-step launches without `steps` behave as if `steps = [{ role }]`.

`TaskContext` remains deterministic:

```text
manual_roster:{chain_id}:{sequence padded to 3}:{role}
```

The dossier note and sources should not be folded into `TaskContext`.

### Running Chain Metadata

Starting a chain must record the normalized launch shape in the `chain_started` event payload:

```json
{
  "mode": "manual_roster",
  "role": "planner,coder",
  "roster": ["planner", "coder"],
  "steps": [
    { "role": "planner", "note": "Plan only.", "sources": ["docs/specs/25-launch-chain-composer.md"] },
    { "role": "coder", "note": "Implement narrowly.", "sources": ["web/src/pages/launch.tsx"] }
  ],
  "task": "ship composer",
  "specs": ["README.md"]
}
```

Resume/continue logic for manual roster must recover structured steps from persisted chain metadata. If an older chain has no structured `steps`, fall back to legacy roster behavior.

---

## Run Sheet Compilation

Preview compiles a run sheet from the normalized request.

Manual roster example:

```text
Run sheet

Work packet
- Task: redesign Launch as a chain composer
- Global sources:
  - docs/specs/24-electron-desktop-app.md
  - docs/specs/25-launch-chain-composer.md

1. Gordon / planner
   Receives:
   - global work packet
   - Gordon dossier: 1 source, note present
   - prior receipts: none
   Produces: planner receipt

2. Thomas / coder
   Receives:
   - global work packet
   - Thomas dossier: 2 sources, note present
   - prior receipts: planner receipt
   Produces: coder receipt

3. Percy / correctness-auditor
   Receives:
   - global work packet
   - Percy dossier: 1 source
   - prior receipts: planner and coder receipts
   Produces: correctness-auditor receipt
```

The run sheet must be deterministic for the same normalized request:

- same step order
- same source order
- same role names after config resolution
- same summary strings
- no timestamps
- no generated ids

---

## Persistence

### Drafts

The current draft should preserve a concrete launch instance:

- mode
- role compatibility field
- roster compatibility field
- allowed roles
- task text
- global sources
- structured steps with notes and sources
- existing backend-only limit fields if they were already present from CLI/API clients

Add storage for structured steps:

- SQLite `launches.steps` as JSON text
- Shunter `launches.steps_json` as string

Existing `launches.roster` / `roster_json` remains for compatibility.

Draft step JSON shape is exactly the public `LaunchRosterStep[]` after normalization. Store `[]` instead of `null` for no steps.

### Saved Templates / Presets

The existing custom preset endpoints can continue to back saved composer templates.

When saving a template from the composer, normalize it as task-neutral:

- keep mode
- keep role compatibility field
- keep roster compatibility field
- keep allowed roles for constrained templates
- keep structured steps with `role` and reusable `note`
- drop `source_task`
- drop `source_specs`
- drop every `steps[*].sources`
- drop all limit fields

Add storage for structured template steps:

- SQLite `launch_presets.steps` as JSON text
- Shunter `launch_presets.steps_json` as string

Applying a saved template:

- populates the canvas steps and notes
- does not start work
- does not attach old task-specific sources
- does not overwrite current work packet sources unless the user explicitly confirms replacing the current draft

In the UI, call user-authored presets **Saved Templates**. In Go/HTTP code, keeping the existing `LaunchPreset` naming is acceptable to avoid churn.

---

## API Surface

No endpoint path changes are required for the MVP.

Use existing endpoints:

```text
GET    /api/roles
GET    /api/chains/templates
GET    /api/launch/draft
PUT    /api/launch/draft
GET    /api/launch/presets
POST   /api/launch/presets
POST   /api/launch/preview
POST   /api/launch/start
POST   /api/project/validate-paths
```

Request decoding should accept both shapes:

```json
{
  "mode": "manual_roster",
  "roster": ["planner", "coder"],
  "source_task": "ship composer"
}
```

```json
{
  "mode": "manual_roster",
  "steps": [
    { "role": "planner", "note": "Plan only.", "sources": ["docs/specs/25-launch-chain-composer.md"] },
    { "role": "coder", "note": "Implement narrowly.", "sources": ["web/src/pages/launch.tsx"] }
  ],
  "source_task": "ship composer",
  "source_specs": ["README.md"]
}
```

Response encoding should include both compatibility and structured fields when possible.

All newly added JSON fields should use `omitempty` in server responses where existing clients expect absent empty fields, except persisted draft/preset reads may return empty arrays when that is the established generated binding shape.

---

## Frontend Requirements

### State Shape

The Launch page should model nodes explicitly:

```ts
interface ComposerNode {
  id: string; // UI-only
  role: string;
  note: string;
  sources: string[];
}

interface ComposerState {
  sourceTask: string;
  sourceSpecs: string[];
  nodes: ComposerNode[];
  allowedRoles: string[];
  dispatchMode: "none" | "free" | "constrained";
}
```

Build the API request from state at preview/start time. Do not store request JSON as the primary state.

Canvas-derived request rules:

- `dispatchMode: "none"` with one node emits `mode: "one_step_chain"`, `role`, `roster: undefined`, and `steps`.
- `dispatchMode: "none"` with multiple nodes emits `mode: "manual_roster"`, `roster`, and `steps`.
- `dispatchMode: "free"` emits `mode: "sir_topham_decides"` and no `steps`.
- `dispatchMode: "constrained"` emits `mode: "constrained_orchestration"`, `allowed_roles`, and no `steps`.
- normal composer requests do not include limit fields.

### UI Behavior

- Add role creates a new node even if the same role already exists.
- Duplicate node copies role, note, and sources.
- Remove node only removes that node.
- Reorder preserves each node's note and sources.
- Selecting a removed node moves selection to the next node, previous node, or work packet.
- Drag/drop source attachment to work packet adds a global source.
- Drag/drop source attachment to a node adds a dossier source.
- Query-string `source_spec` values attach to the work packet only.
- The normal page does not render the old Mode section.
- The normal page does not render the old Limits section.
- The normal page does not render built-in launch modes as user templates.
- Request JSON appears only in a collapsed details/debug section.

### Accessibility

The composer must be usable without pointer drag:

- role palette has buttons to add roles
- each node has move left/up and move right/down controls
- each node has duplicate and remove controls
- inspector fields have labels
- Preview and Start buttons expose disabled reasons through nearby validation text

### Warnings

Keep readiness warnings for real runtime issues.

Remove or suppress the warning:

```text
single-step {role} launch has no per-step turn/token caps
```

from the normal Launch preview because uncapped normal launches are expected. The backend may keep the warning for CLI/debug callers only if there is a way to distinguish surfaces; otherwise remove it globally.

---

## Implementation Plan

### Slice 1: Contract And Backend Normalization

- Add `LaunchRosterStep` to `internal/operator`.
- Add `steps` to operator/server/web launch request and response types.
- Normalize legacy `role`/`roster` into structured steps.
- Validate conflicts between `steps`, `role`, and `roster`.
- Preserve duplicate roles in ordered steps.
- Add focused operator and server tests.

### Slice 2: Runtime Briefs And Run Sheet

- Carry step notes/sources into `chainrun.StepRequest`.
- Include dossier sections in one-step and manual-roster task compilation.
- Add `run_sheet_markdown` and preview step data.
- Update tests for `buildOneStepTask`, `buildManualRosterTask`, and `ValidateLaunch`.

### Slice 3: Persistence

- Add SQLite columns for `steps` on drafts and presets.
- Add Shunter fields `steps_json` for launches and launch presets.
- Round-trip drafts with task-specific node sources.
- Round-trip presets with roles and notes only.
- Update project-memory generated bindings if needed.

### Slice 4: Composer UI

- Replace the Mode/Roles form sections with palette plus linear canvas.
- Add work packet source management.
- Add node inspector with dossier note/sources.
- Build request from composer state.
- Display run sheet as primary preview.
- Move JSON to collapsed details.
- Remove visible limits.
- Keep existing project attachment validation.

### Slice 5: Verification

- Update frontend tests for one-step, manual roster, duplicate roles, node dossiers, run-sheet preview, project-browser handoff, and start navigation.
- Run `make test`.
- Run `make build`.
- Optionally run `make desktop-build` or `make desktop-dev` for visual validation if frontend changes are substantial.

---

## Acceptance Criteria

### Backend

- `POST /api/launch/preview` accepts legacy `roster: ["planner", "coder"]`.
- `POST /api/launch/preview` accepts structured `steps`.
- Structured `steps` preview returns normalized `steps`, `roster`, and `run_sheet_markdown`.
- Duplicate roles in `steps` are preserved.
- Unknown role errors identify the failing step index.
- `steps`/`roster` mismatch fails validation.
- One-step with exactly one structured step maps to `one_step_chain`.
- Manual roster with multiple structured steps maps to `manual_roster`.
- A launch with only per-node dossier sources and no task/global source fails validation.
- Per-step note and sources appear in the compiled task for that step.
- Prior receipt paths still appear in manual roster steps after the first step.
- The `chain_started` event includes normalized structured steps for new structured launches.
- Manual roster resume preserves future step notes and sources.
- Existing legacy clients still pass current launch preview/start tests.

### Persistence

- A draft with task text, global sources, duplicate role nodes, per-node notes, and per-node sources round-trips exactly after normalization.
- A saved template drops task text, global sources, per-node sources, and limits.
- A saved template keeps ordered roles, duplicate roles, and per-node notes.
- Shunter-backed project memory and SQLite fallback stores have matching behavior.

### Frontend

- Empty composer cannot preview/start and explains why.
- Dossier-only composer cannot preview/start and explains that the work packet needs a task or global source.
- Adding the same role twice creates two distinct nodes.
- Reordering nodes updates the run sheet and request order.
- Per-node notes and sources stay attached to the correct node after reorder.
- Project browser handoff query parameters merge into the global work packet without replacing draft content.
- Node dossier attachments call backend path validation before entering state.
- Preview shows run sheet by default.
- Raw JSON is hidden in a details/debug disclosure.
- The normal UI does not show Limits controls.
- Built-in launch mode metadata is not presented as user-saved templates.
- The normal UI does not show a forced built-in default chain.
- Start navigates to the returned chain detail route.

### Docs And Regression

- `docs/specs/00-index.md` lists spec 25 as draft/implementation-ready.
- `README.md` does not need a status update until the implementation lands.
- `make test` passes.
- `make build` passes.

---

## Future Work

These are intentionally out of scope for the first implementation:

- graph execution with branches and joins
- visualizing live node run state in the launch composer itself
- launch history and duplicate-from-previous-run workflows
- reusable pinned template sources
- richer role metadata from backend prompt frontmatter
- advanced/debug launch limits UI
- exact approval-wait controls in Launch
- context assembly hard-prioritization of dossier files beyond prompt inclusion
