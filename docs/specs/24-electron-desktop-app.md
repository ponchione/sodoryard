# 24 - Electron Desktop App

**Status:** Proposed active direction
**Last Updated:** 2026-05-12
**Owner:** Mitchell

---

## Overview

Yard should grow a native-feeling desktop operator interface built with Electron. The desktop app becomes the target graphical daily-driver surface for chat, project readiness, chain launch, chain monitoring, receipt review, project attachment, context inspection, tool details, metrics, and settings.

This direction intentionally reuses the existing Go runtime and React frontend investment. Electron should not become a second Yard runtime. The desktop app is a native shell around the same local backend services that power `yard serve`, the current web inspector, the CLI, and the TUI.

Target product split after the desktop app reaches parity:

| Surface | Target role |
|---|---|
| Yard Desktop | Primary graphical operator console and inspector |
| `yard` CLI | Scriptable commands, automation, fallback operation, project bootstrap |
| `yard serve` | Local browser/API server for development, remote inspection, and fallback browser use |
| TUI | Transitional fallback and diagnostic surface until retirement is explicitly approved |
| `tidmouth` | Internal engine subprocess contract only |

The near-term goal is not to delete the TUI. The goal is to specify and build a desktop app that can replace the TUI over time without breaking the existing CLI, runtime, or browser inspection workflows.

---

## Decision

Use Electron for the desktop GUI.

Electron is the pragmatic choice because Yard already has:

- React, TypeScript, Vite, Tailwind, and shadcn-style UI primitives under `web/`
- a Go HTTP/WebSocket backend served by `yard serve`
- embedded frontend production assets through `webfs/dist`
- shared operator service methods in `internal/operator`
- project-local state in `yard.yaml`, `.yard/`, and `.brain/`

The desktop app should reuse the React renderer rather than rewriting the UI in another toolkit. The Go backend remains authoritative for runtime state, persistence, chain execution, provider auth, context assembly, brain access, indexing, and tool execution.

The implementation should prefer a sidecar backend process at first:

```text
Electron main process
  -> selects project/config
  -> starts or connects to a local Yard backend
  -> owns native app lifecycle, menus, notifications, file dialogs, deep links

React renderer
  -> renders the operator console and inspector
  -> talks to Yard through REST and WebSocket APIs
  -> never mutates project state directly through Node filesystem APIs

Go backend
  -> runs the existing Yard runtime
  -> owns all database, chain, provider, auth, brain, and tool behavior
  -> exposes desktop-safe HTTP/WebSocket endpoints bound to localhost
```

An embedded library/runtime integration can be considered later, but the first implementation should keep the process boundary. That boundary preserves the current production model, keeps crash isolation simple, and lets `yard serve` remain the browser/API fallback.

### Alternatives Considered

| Alternative | Why not the first choice |
|---|---|
| Browser-only command center | Repeats the original browser-tab problem and misses native project/file/app lifecycle affordances |
| Keep TUI as primary | The TUI works, but rich layout, attachment flows, diffs, metrics, and document intake are naturally GUI-shaped |
| Tauri | Smaller runtime, but adds Rust-side shell work while Yard already has a Go backend and React app; Electron is lower-friction for this repo |
| Native toolkit | Higher platform-specific cost and would discard the existing React web inspector investment |
| Go desktop toolkit | Keeps one language but gives up the frontend ecosystem already in use for markdown, diffs, trees, routing, and charts |

This is not a permanent rejection of Tauri or another shell. It is a choice to optimize the first GUI path for reuse and implementation speed.

---

## Product Goals

1. **Make Yard approachable as a desktop app**
   Operators should be able to open a project, see readiness, chat, launch work, follow chains, inspect results, and adjust settings without managing browser tabs or terminal panes.

2. **Unify operation and inspection**
   The TUI split placed operation in the terminal and rich inspection in the browser. The desktop app should combine those into one coherent workspace: launch/control on the left side of the product, rich transcripts/context/diffs/metrics in the detail surfaces.

3. **Preserve local-first behavior**
   The app runs on the developer's machine, talks to local project state, and does not require a hosted service. Cloud providers remain optional provider backends, not Yard infrastructure.

4. **Keep Go as the source of truth**
   Electron should not duplicate Yard business logic. If a workflow changes chain state, provider config, auth state, launch drafts, brain docs, or project files, it goes through the Go backend.

5. **Improve file and document intake**
   The GUI should make it easy to attach project files, docs, brain notes, specs, and pasted context to launch packets. This is one of the areas where a desktop app can materially beat the TUI.

6. **Expose runtime state clearly**
   Provider/model, auth, index freshness, local service health, chain status, active process state, warnings, and failure remediation should be visible without searching logs.

7. **Support native desktop affordances**
   Use native file/folder pickers, app menus, notifications, tray/status behavior where useful, recent projects, deep links, and open-in-editor handoffs.

---

## Non-Goals

- No Electron-owned execution engine.
- No separate chain scheduler in TypeScript.
- No Electron-owned copy of Yard state tables.
- No direct renderer writes to `.yard/`, `.brain/`, `yard.yaml`, or project source files.
- No arbitrary browser terminal with unrestricted shell access.
- No hosted account system, tenancy, remote dashboard, or sync service.
- No mobile UI target.
- No forced removal of `yard serve` or the current React app.
- No TUI deletion in the same slice as introducing Electron.
- No exposure of `tidmouth` as a public desktop or CLI surface.

---

## Design Principles

### Backend Authority

The Go backend is authoritative. React may hold temporary UI state, optimistic display state, and form drafts, but durable state must live in the backend's existing stores or in backend-managed config.

Examples:

- launch drafts and presets use `internal/operator`
- chains use `internal/chain` and `.yard/yard.db`
- conversations use existing conversation persistence
- provider routing uses `yard.yaml` plus backend validation
- auth uses backend provider auth stores
- brain content is accessed through backend brain services

### Desktop As A Shell, Not A Fork

Electron owns the shell:

- app lifecycle
- project window lifecycle
- backend process supervision
- native menus
- native dialogs
- notifications
- deep links
- secure renderer bootstrapping

Electron does not own Yard runtime semantics.

### API First

Every desktop workflow should use a documented REST/WebSocket or local IPC contract. If the UI needs a feature that only exists in the TUI through direct Go calls, add the corresponding backend API by adapting `internal/operator`.

### One UI Codebase

The existing `web/` React app should become the shared renderer for:

- browser inspector through `yard serve`
- Electron renderer through packaged desktop builds
- frontend development through Vite

Desktop-only capabilities should enter through a narrow adapter layer, not through broad `if electron` branches across the app.

### Narrow Native Bridge

The renderer should not get broad Node access. Use `contextIsolation: true`, `nodeIntegration: false`, and a small preload bridge.

Allowed bridge examples:

- choose project directory
- choose files/directories for launch attachment
- open file in external editor
- reveal file in system file manager
- show native notification
- read app version/platform
- open external URL in default browser

Disallowed bridge examples:

- arbitrary filesystem read/write
- arbitrary shell command execution
- direct SQLite access
- direct provider credential access

---

## Target Architecture

```text
packages/desktop or desktop/
  Electron main process
  preload bridge
  renderer bootstrapping
  packaging config

web/
  shared React application
  desktop-aware environment adapter
  browser and Electron routes/components

cmd/yard
  existing CLI
  `yard serve` remains browser/API server
  target: desktop backend mode for sidecar startup

internal/server
  existing HTTP/WebSocket API
  target: desktop-safe operator endpoints

internal/operator
  shared service for runtime status, chain reads/control, launch drafts,
  launch presets, launch preview/start, receipts, roles, and raw chat
```

Process model:

```text
+---------------------------+
| Electron Main             |
|---------------------------|
| project selection         |
| backend spawn/connect     |
| app menu, tray, dialogs   |
| notifications             |
+-------------+-------------+
              |
       preload IPC bridge
              |
+-------------v-------------+       HTTP/WS        +----------------------+
| React Renderer            | <-------------------> | Yard Backend         |
|---------------------------|                       |----------------------|
| dashboard                 |                       | internal/server      |
| launch workbench          |                       | internal/operator    |
| chains and receipts       |                       | chainrun/runtime     |
| conversations             |                       | providers/tools      |
| context/diff/metrics      |                       | SQLite/LanceDB/brain |
+---------------------------+                       +----------------------+
```

---

## Backend Sidecar Contract

The first desktop implementation should package a `yard` binary and start a local backend sidecar for each open project window or project session.

### Startup

Target command shape:

```bash
yard serve \
  --config /path/to/project/yard.yaml \
  --host 127.0.0.1 \
  --port 0 \
  --desktop-session <opaque-session-id> \
  --desktop-origin <electron-origin-or-token>
```

If `--port 0` is not supported by the current server, add it. The backend should bind an available loopback port and emit a machine-readable readiness message.

Target readiness event on stdout:

```json
{"type":"yard_backend_ready","base_url":"http://127.0.0.1:49152","pid":12345,"project_root":"/path/to/project"}
```

The human-oriented `yard serve` output can remain for normal CLI use, but desktop mode needs a stable JSON readiness line so Electron does not scrape prose.

### Connection

The renderer talks to `base_url` through the existing REST/WebSocket client layer. The Electron main process passes the backend base URL to the renderer through preload at window creation time.

The app should support three backend modes:

| Mode | Behavior |
|---|---|
| managed | Electron starts and supervises the sidecar backend |
| attach | Electron connects to an already-running local `yard serve` selected by URL |
| dev | Vite dev server proxies to a developer-started backend |

Managed mode is the packaged default. Attach mode is useful for debugging and for operators who already run `yard serve`. Dev mode is for contributors.

### Shutdown

On app quit or project window close:

1. Ask the backend for active operations.
2. If no active chains or streaming turns are owned by this desktop session, request graceful shutdown.
3. If active work exists, warn the operator and offer:
   - keep backend running
   - cancel owned work and quit
   - return to app
4. If graceful shutdown times out, terminate the sidecar process.

The desktop app must not kill unrelated `yard` or `tidmouth` processes. It may only manage the child process it started.

### Health

Electron main monitors:

- child process exit
- `/api/health`
- backend version compatibility
- WebSocket connectivity
- project root/config identity

The renderer should distinguish:

- backend starting
- backend ready
- backend reconnecting
- backend crashed
- backend incompatible
- project config invalid

### Startup State Machine

Electron main should model backend startup explicitly:

```text
idle
  -> selecting_project
  -> validating_project
  -> spawning_backend
  -> waiting_for_ready
  -> probing_health
  -> ready
```

Failure states:

```text
invalid_project
missing_yard_binary
spawn_failed
ready_timeout
health_failed
version_incompatible
backend_exited
```

Each failure state should expose a user-facing remediation:

| State | Remediation |
|---|---|
| `invalid_project` | initialize project, choose another folder, or open config details |
| `missing_yard_binary` | show packaging/development setup error |
| `spawn_failed` | show command, exit code, stderr excerpt, and diagnostics action |
| `ready_timeout` | offer retry and show backend log excerpt |
| `health_failed` | offer retry, attach to backend logs, or quit backend |
| `version_incompatible` | show app/backend versions and require matching build |
| `backend_exited` | offer restart, keep logs, or close project |

### Version Compatibility

Add a desktop-readable capability endpoint:

```text
GET /api/desktop/capabilities
```

Target payload:

```json
{
  "yard_version": "0.1.0",
  "api_version": "desktop-v1",
  "project_root": "/path/to/project",
  "config_path": "/path/to/project/yard.yaml",
  "capabilities": [
    "runtime_status",
    "conversation_chat",
    "chains_read",
    "chains_control",
    "launch_preview",
    "launch_start",
    "launch_drafts",
    "launch_presets",
    "project_tree",
    "project_file_preview",
    "context_reports",
    "tool_details",
    "metrics"
  ]
}
```

The renderer should gate features based on capabilities instead of assuming every backend has every endpoint.

---

## Security Model

Yard is a local developer tool with powerful file and shell capabilities. The desktop app should treat itself as a privileged local control surface and minimize accidental expansion of that privilege.

### Network Binding

Managed desktop backends must bind to loopback only:

```text
127.0.0.1
```

No all-interfaces bind in desktop managed mode. Any remote access remains an explicit `yard serve --host 0.0.0.0 --allow-external` CLI workflow, not a desktop default.

### Session Token

Desktop managed mode should use a session token for API requests:

- Electron generates an unguessable token for the backend session.
- The backend accepts desktop API calls only with that token.
- The token is passed to the renderer through preload.
- The token is not written to project state.

Suggested header:

```text
X-Yard-Desktop-Session: <token>
```

The browser `yard serve` flow may continue without this in normal local mode, but managed desktop mode should opt into it.

### State Ownership

| State | Owner | Storage |
|---|---|---|
| project config | Go backend | `yard.yaml` |
| chain state | Go backend | `.yard/yard.db` |
| conversations | Go backend | `.yard/yard.db` |
| launch drafts | Go backend | `.yard/yard.db` through `internal/operator` |
| custom launch presets | Go backend | `.yard/yard.db` through `internal/operator` |
| brain docs | Go backend | `.brain/` |
| code/brain index state | Go backend | `.yard/` and LanceDB roots |
| provider credentials | Go backend | provider auth store |
| selected project recents | Electron main | OS app data |
| window size/position | Electron main | OS app data |
| sidebar/pane layout | Electron main or renderer | OS app data unless project-semantic |
| temporary form edits | React renderer | memory until saved through backend |
| session token | Electron main/backend | memory only |

The renderer may cache API responses for UI performance, but cache invalidation is driven by backend events, query refresh, or route transitions. Cached renderer state is never the durable source of truth.

### Renderer Isolation

Electron settings:

```typescript
webPreferences: {
  contextIsolation: true,
  nodeIntegration: false,
  sandbox: true,
  preload: "/path/to/preload.js"
}
```

Content security policy should prohibit remote script execution. The packaged app should load local renderer assets or a trusted development URL only in dev mode.

### External Links

External links opened from markdown, receipts, provider docs, or settings must go through `shell.openExternal` after URL validation. The renderer should not navigate the app window to arbitrary external origins.

### File Access

The renderer never receives direct filesystem primitives. File content reads and project tree reads go through backend endpoints with existing path safety checks. Native dialogs return selected paths, but backend validation decides whether those paths can be used in a launch packet.

### Shell Access

No generic terminal panel in v1. Agent tool output can be displayed. Operator-triggered commands must be explicit Yard operations exposed by backend APIs.

---

## Product Boundary

The desktop app absorbs the useful parts of both current UI directions:

| Existing idea | Desktop owner |
|---|---|
| TUI dashboard readiness | Desktop dashboard |
| TUI launch wizard | Desktop launch workbench |
| TUI chain list/control | Desktop chain monitor |
| TUI receipt browser | Desktop receipt/detail views |
| TUI project file attachment | Desktop native file/project picker plus project browser |
| Web conversation transcript | Desktop conversation view |
| Web context inspector | Desktop context inspector |
| Web tool details/diffs | Desktop tool and diff inspector |
| Web metrics | Desktop metrics views |
| Browser project tree | Desktop project browser |
| `yard serve` fallback | Retained as local browser/API mode |

The CLI remains the stable scriptable surface. Any desktop action that has a CLI equivalent can optionally show that command for learnability, but the desktop app should call APIs, not shell out to the CLI for core behavior.

---

## Project Lifecycle

### Recent Projects

Electron stores recent projects outside the Yard project:

```json
{
  "recent_projects": [
    {
      "root": "/home/user/source/project",
      "config_path": "/home/user/source/project/yard.yaml",
      "name": "project",
      "last_opened_at": "2026-05-12T12:00:00Z"
    }
  ]
}
```

This state is app convenience state. It should not be written to `.yard/yard.db`.

### Project Validation

When a project is selected, the app validates:

- directory exists
- directory is readable
- `yard.yaml` exists or initialization is possible
- config can be loaded by backend
- project root in backend response matches selected path
- required project-local state can be created or opened

If `yard.yaml` is missing, the desktop app should offer initialization. Initialization should be backend-owned, equivalent to `yard init`, and must be idempotent.

### Multiple Projects

The MVP may support one open project window. The architecture should avoid blocking future multiple windows:

- one window maps to one project session
- each managed project session has its own backend sidecar
- app-level recent project state is shared
- project runtime state remains project-local

Sharing one backend across multiple projects is deferred unless a later design proves it is worth the complexity.

---

## Information Architecture

The desktop app should use a workspace layout rather than browser-like pages as the primary mental model. Routes can still exist internally for deep links and history.

Minimum route/workspace set:

| Route | Workspace | Purpose |
|---|---|---|
| `/` | Dashboard | Project readiness, active work, recent chains/conversations/receipts |
| `/chat/:id?` | Chat | Raw provider/model chat and persisted conversation transcripts |
| `/launch` | Launch Workbench | Assemble work packet and start chain work |
| `/chains` | Chain Monitor | Active/recent chains, filters, controls, event summaries |
| `/chains/:id` | Chain Detail | Timeline, steps, events, receipts, files, metrics |
| `/receipts/:chainId/:step?` | Receipt Detail | Rendered receipt markdown and linked artifacts |
| `/project` | Project Browser | Tree, file preview, selected attachment set |
| `/context/:conversationId/:turn` | Context Inspector | Context report, signals, retrieval, budget |
| `/tools/:toolCallId` | Tool Detail | Structured tool result, command output, diff, metadata |
| `/metrics` | Metrics | Conversations, chains, provider/model, tool, context trends |
| `/settings` | Settings | Provider/model, auth, local services, config validation |

Deep links should support opening directly to a chain, receipt, conversation, file, or context report.

---

## Layout

Target shell:

```text
+--------------------------------------------------------------------------------+
| App menu / command palette / project switcher / provider:model / backend state  |
+--------------+-----------------------------------------------------------------+
| Navigation   | Main workspace                                                  |
|              |                                                                 |
| Dashboard    | Dashboard, launch, chat, chain detail, project browser, etc.    |
| Chat         |                                                                 |
| Launch       |                                                                 |
| Chains       |                                                                 |
| Project      |                                                                 |
| Metrics      |                                                                 |
| Settings     |                                                                 |
+--------------+-----------------------------------------------------------------+
| Status: project root, active chain count, index freshness, warnings             |
+--------------------------------------------------------------------------------+
```

Desktop should support resizable panes:

- primary nav rail
- main workspace
- optional right inspector panel
- optional bottom event/output drawer

The app should remember:

- last opened projects
- last active project
- window size and position
- sidebar collapsed state
- selected theme
- recently opened chain/conversation/receipt routes

Window state is app-level desktop state, not project runtime state. Store it in Electron app data, not `.yard/yard.db`, unless the state is meaningful to every UI surface.

### UI Density And Style

Yard Desktop is an operational tool, not a marketing app. The interface should be dense, quiet, and optimized for repeated technical work.

Rules:

- no landing page as the normal project screen
- no oversized hero sections after project selection
- avoid card-heavy decorative layouts
- use tables, split panes, trees, tabs, and inspectors for scan-heavy data
- keep cards for repeated items, modals, and bounded summaries
- use icons for common actions such as open, copy, search, refresh, pause, cancel, settings, and external open
- use tooltips for icon-only controls
- keep text labels short and specific
- show errors near the control or data they affect
- never hide runtime blockers behind only color or iconography
- preserve keyboard navigation for primary workflows

Primary desktop shortcuts should be conventional:

| Shortcut | Action |
|---|---|
| `CmdOrCtrl+O` | open project |
| `CmdOrCtrl+N` | new chat or launch, depending on active workspace |
| `CmdOrCtrl+K` | command palette |
| `CmdOrCtrl+F` | search/filter active view |
| `CmdOrCtrl+,` | settings |
| `Esc` | close modal, clear transient focus, or cancel search focus |

Destructive operations such as chain cancellation require confirmation and should state the exact chain ID.

---

## Primary Workflows

### 1. First Launch

When the user opens Yard Desktop:

1. Show recent projects if any exist.
2. Offer "Open Project" through a native directory picker.
3. Detect whether the selected directory has `yard.yaml`.
4. If no config exists, offer to initialize through backend-supported `yard init` semantics.
5. Start the managed backend for the selected project.
6. Load runtime status and route to the dashboard.

The app should not require the user to manually run `yard serve`.

### 2. Project Readiness

The dashboard answers:

- What project is open?
- Which provider/model is configured?
- Is auth valid?
- Are local LLM services configured, running, or unhealthy?
- Are code and brain indexes present and fresh?
- Are there active chains or running steps?
- Are there runtime warnings that block launch?
- What recent conversations/chains/receipts exist?

Actions:

- rebuild code index
- rebuild brain index
- open auth status/login flow
- open local service status/remediation
- start a launch
- resume/follow active work
- open settings

### 3. Raw Chat

The desktop chat view keeps the existing conversation model:

- persisted conversation list
- message composer
- streaming tokens
- thinking blocks
- tool call cards when tools are involved
- per-turn usage
- context report link
- provider/model override when supported
- cancellation

The raw desktop chat should match the current TUI raw chat contract: it calls the configured provider/model without one of the 13 role prompts, chain tools, or orchestration unless the operator explicitly starts chain work.

### 4. Launch Workbench

The launch workbench is the desktop replacement for the TUI launch screen.

Fields:

- task text
- source specs
- supporting brain docs
- explicit project files
- selected project directories
- pasted notes
- constraints
- operator notes
- launch mode
- selected role
- manual roster
- constrained allowed roles
- optional provider/model override when backend supports it
- preflight warning acceptance

Launch modes:

| Mode | Behavior |
|---|---|
| `one_step_chain` | Run one selected role against the work packet |
| `manual_roster` | Run ordered roles, each step receiving previous receipts |
| `sir_topham_decides` | Let the orchestrator choose the flow |
| `constrained_orchestration` | Let the orchestrator choose within selected allowed roles |

Desktop improvements over TUI:

- multi-select project tree
- drag/drop file attachment
- native file picker
- brain/doc picker with search
- visible work-packet preview
- role roster drag ordering
- preset cards or menu
- side-by-side preflight warnings and compiled packet
- saved launch drafts and custom presets through existing backend storage

Starting a launch calls the backend `StartChain` path. It does not shell out to `yard chain start`.

### 5. Chain Monitor

The chain monitor shows:

- active chains first
- recent chains
- status filters
- text search
- role/mode filters
- active step
- last event
- token totals
- duration
- receipt availability
- warning/error state

Actions:

- follow live events
- pause
- resume
- cancel
- duplicate launch packet when launch records support it
- open chain detail
- open receipts
- reveal changed files

Controls call backend APIs that adapt `internal/operator` control methods.

### 6. Chain Detail

The chain detail view should be richer than both TUI and current browser inspector:

- timeline of orchestrator and step events
- step cards with role/persona/status/verdict
- active process state
- event log with severity/type filters
- receipt rendering
- changed file list
- tool output summaries
- token/model usage
- links to context reports and conversations when available
- follow-up actions from receipts

The view should update live while the chain runs.

### 7. Receipt Review

Receipt detail renders markdown receipts with:

- frontmatter summary
- role/persona
- verdict/status
- changed files
- follow-ups
- linked chain events
- copy path
- open in editor
- reveal in file manager
- open source specs/files

Receipt files still live in project-local brain/state locations as defined by existing chain behavior. Desktop only renders and navigates them.

### 8. Project Browser And Attachments

The desktop project browser should use backend-safe project APIs and native dialogs.

Capabilities:

- tree navigation
- file preview
- search/filter
- selected attachment set
- recent/touched file highlighting
- launch attachment actions
- open in external editor
- reveal in file manager

File previews should respect backend limits for size, path traversal, binary files, and project root containment.

### 9. Context Inspector

The context inspector keeps the existing browser strength:

- analyzer needs
- semantic queries
- explicit files
- explicit symbols
- brain hits
- code retrieval hits
- graph relationships
- token budget allocation
- final injected context summary
- context-debug event history

Desktop should make this easier to reach from:

- chat turns
- chain steps
- tool calls
- launch preview when future context simulation exists

### 10. Tool And Diff Inspector

Tool detail views should render structured tool metadata from [[19-tool-result-details]]:

- shell command output
- file read/write summaries
- unified diff
- changed-file list
- search result groups
- duration
- success/error state
- truncation state
- model-visible vs UI-only result distinction

Diffs should support:

- side-by-side view
- inline view
- file list navigation
- copy path
- open in editor
- reveal in file manager

### 11. Settings

Settings should cover:

- project identity and config path
- provider routing default/fallback
- provider auth status
- provider login flows where backend supports them
- available models
- local LLM service status
- index configuration and status
- server/backend status
- app version
- backend version
- diagnostics export

Mutable settings must go through backend validation. If the backend only permits a subset of config mutation, the UI should show unsupported fields read-only.

---

## Native Integrations

### App Menu

Target menu groups:

- File
  - Open Project
  - Recent Projects
  - Close Project
  - New Chat
  - New Launch
  - Quit
- Edit
  - standard OS edit roles
- View
  - Dashboard
  - Chat
  - Launch
  - Chains
  - Project
  - Metrics
  - Toggle Developer Tools
- Chain
  - Start From Current Launch
  - Pause Selected Chain
  - Resume Selected Chain
  - Cancel Selected Chain
  - Open Receipt
- Tools
  - Rebuild Code Index
  - Rebuild Brain Index
  - Auth Status
  - Local LLM Status
- Help
  - Documentation
  - Diagnostics
  - About Yard

Menu commands should route to renderer actions or backend APIs through a small command bus.

### Command Palette

Desktop should include a command palette for common actions:

- open project
- switch project
- new chat
- new launch
- start launch
- open chain by ID
- open receipt
- rebuild indexes
- provider auth status
- local service status
- open settings

### Notifications

Native notifications:

- chain completed
- chain failed
- human attention required
- auth expired
- indexing completed/failed
- local service unhealthy

Notifications should deep-link back into the relevant chain, receipt, settings page, or diagnostics view.

### File Associations And Deep Links

Potential custom protocol:

```text
yard://project/open?path=/path/to/project
yard://chain/<chain-id>
yard://conversation/<conversation-id>
yard://receipt/<chain-id>/<step>
```

Deep links should validate project identity and never execute arbitrary commands.

### External Editor

Desktop should support "open in editor" through:

- configured editor command from settings or environment
- OS default app fallback
- backend path validation before open

The app should not silently edit files itself unless the workflow is explicitly a Yard backend operation.

---

## API Requirements

The desktop app should reuse existing endpoints from [[07-web-interface-and-streaming]] and add missing operator endpoints by adapting `internal/operator`.

### Existing Endpoint Groups

```text
GET    /api/health

GET    /api/conversations
POST   /api/conversations
GET    /api/conversations/:id
GET    /api/conversations/:id/messages
DELETE /api/conversations/:id
GET    /api/conversations/search?q=...

GET    /api/project
GET    /api/project/tree
GET    /api/project/file?path=...

GET    /api/config
PUT    /api/config
GET    /api/providers
GET    /api/auth/providers

GET    /api/metrics/conversation/:id
GET    /api/metrics/conversation/:id/context/:turn
GET    /api/metrics/conversation/:id/context/:turn/signals

WS     /api/ws
```

### Required Desktop/Operator Endpoints

Runtime:

```text
GET    /api/desktop/capabilities
GET    /api/runtime/status
POST   /api/runtime/index/code
POST   /api/runtime/index/brain
GET    /api/runtime/local-services
POST   /api/runtime/local-services/up
POST   /api/runtime/local-services/down
GET    /api/runtime/local-services/logs?tail=...
```

Roles:

```text
GET    /api/roles
```

Launch:

```text
GET    /api/launch/draft
PUT    /api/launch/draft
GET    /api/launch/presets
POST   /api/launch/presets
POST   /api/launch/preview
POST   /api/launch/start
```

Chains:

```text
GET    /api/chains
GET    /api/chains/:id
GET    /api/chains/:id/events
GET    /api/chains/:id/events?after=<event-id>
GET    /api/chains/:id/receipts
GET    /api/chains/:id/receipts/:step
POST   /api/chains/:id/pause
POST   /api/chains/:id/resume
POST   /api/chains/:id/cancel
```

Files:

```text
POST   /api/project/validate-paths
POST   /api/project/editor/open
POST   /api/project/reveal
```

Diagnostics:

```text
GET    /api/diagnostics
POST   /api/diagnostics/export
```

The file/editor endpoints can be implemented either in the backend or through the Electron main bridge, but project path validation should remain backend-owned.

### Payload Sketches

The exact Go/TypeScript names can follow implementation conventions, but the desktop contract should preserve these shapes.

Runtime status:

```typescript
type RuntimeStatus = {
  project_root: string;
  project_name: string;
  provider: string;
  model: string;
  auth_status: string;
  code_index: RuntimeIndexStatus;
  brain_index: RuntimeIndexStatus;
  local_services_status: string;
  active_chains: number;
  warnings: RuntimeWarning[];
};

type RuntimeIndexStatus = {
  status: "unknown" | "missing" | "stale" | "ready" | "disabled" | string;
  last_indexed_at?: string;
  stale_since?: string;
  stale_reason?: string;
};
```

Launch request:

```typescript
type LaunchMode =
  | "one_step_chain"
  | "manual_roster"
  | "sir_topham_decides"
  | "constrained_orchestration";

type LaunchRequest = {
  source_task?: string;
  source_specs?: string[];
  brain_docs?: string[];
  project_files?: string[];
  project_dirs?: string[];
  constraints?: string;
  operator_notes?: string;
  mode: LaunchMode;
  role?: string;
  roster?: string[];
  allowed_roles?: string[];
  provider?: string;
  model?: string;
  accepted_warnings?: string[];
};
```

Launch preview:

```typescript
type LaunchPreview = {
  mode: LaunchMode;
  role?: string;
  roster?: string[];
  allowed_roles?: string[];
  summary: string;
  work_packet_markdown: string;
  warnings: LaunchWarning[];
};
```

Chain summary:

```typescript
type ChainSummary = {
  id: string;
  status: string;
  source_task: string;
  source_specs: string[];
  total_steps: number;
  total_tokens: number;
  started_at: string;
  updated_at: string;
  current_step?: StepSummary;
};

type StepSummary = {
  sequence_num: number;
  role: string;
  status: string;
  verdict?: string;
  receipt_path?: string;
};
```

Control result:

```typescript
type ControlResult = {
  chain_id: string;
  status?: string;
  message: string;
};
```

Receipt view:

```typescript
type ReceiptView = {
  chain_id: string;
  step: string;
  path: string;
  content: string;
  summary?: string;
  changed_files?: string[];
};
```

Path validation:

```typescript
type ValidatePathsRequest = {
  paths: string[];
  purpose: "launch_attachment" | "open_editor" | "reveal";
};

type ValidatePathsResponse = {
  accepted: string[];
  rejected: Array<{ path: string; reason: string }>;
};
```

### WebSocket Requirements

Existing conversation WebSocket remains:

```text
WS /api/ws
```

Desktop also needs chain/event updates. Options:

1. Add chain events to the existing WebSocket envelope.
2. Add a dedicated chain WebSocket:

```text
WS /api/chains/:id/ws
```

Either is acceptable. The chosen contract must support:

- event replay from cursor
- live chain events
- chain status changes
- step status changes
- receipt availability notifications
- cancellation/pause/resume acknowledgements
- backend reconnect with cursor

Suggested chain event envelope:

```typescript
type ChainServerMessage = {
  type:
    | "chain_snapshot"
    | "chain_event"
    | "chain_status"
    | "step_status"
    | "receipt_available"
    | "control_result"
    | "error";
  timestamp: string;
  chain_id: string;
  cursor?: number;
  data: unknown;
};
```

The renderer should reconnect with the last seen cursor and fetch a REST snapshot after reconnect so missed state transitions are not lost.

---

## Renderer Architecture

The React app should gain a small platform adapter:

```text
web/src/platform/
  index.ts
  browser.ts
  desktop.ts
  types.ts
```

Target adapter shape:

```typescript
type YardPlatform = {
  kind: "browser" | "desktop";
  backendBaseUrl: string;
  openExternal(url: string): Promise<void>;
  chooseProjectDirectory?(): Promise<string | null>;
  chooseAttachmentPaths?(): Promise<string[]>;
  openInEditor?(path: string, line?: number): Promise<void>;
  revealPath?(path: string): Promise<void>;
  notify?(notification: YardNotification): Promise<void>;
  getAppInfo?(): Promise<YardAppInfo>;
};
```

The existing API client in `web/src/lib/api.ts` should use the adapter's `backendBaseUrl` rather than assuming same-origin for every runtime. Browser mode can continue using same-origin defaults.

Desktop-specific code should be concentrated in:

- Electron main process
- preload bridge
- platform adapter
- small UI affordances that only render when a desktop capability exists

It should not leak into every page/component.

### Data Fetching

The renderer should centralize backend calls in typed API modules:

```text
web/src/lib/
  api.ts
  runtime-api.ts
  launch-api.ts
  chains-api.ts
  project-api.ts
  diagnostics-api.ts
```

Fetch behavior:

- include desktop session header when present
- use abort controllers for route changes and cancelled operations
- surface structured backend errors
- retry health/capability checks with backoff during startup
- avoid retrying mutating operations unless the request is explicitly idempotent
- keep chain event live updates separate from REST list refreshes

### Error Presentation

Every API error should map to:

- short user-facing title
- technical detail panel
- retry action when safe
- copy diagnostics action
- relevant logs or request ID when available

The UI should not silently fall back to stale data for launch/control operations. Stale data is acceptable for read-only lists only when clearly labeled.

---

## Packaging

Target package layout can be decided during implementation, but the spec prefers keeping the desktop shell adjacent to, not inside, the existing web app:

```text
desktop/
  package.json
  electron-builder.yml or forge config
  src/
    main.ts
    preload.ts
    backend.ts
    menu.ts
    projects.ts
    notifications.ts
```

Alternative if a monorepo workspace is introduced:

```text
package.json
web/package.json
desktop/package.json
```

### Build Artifacts

Packaged app contains:

- Electron main/preload bundles
- built React renderer assets
- platform-specific `yard` sidecar binary
- any required runtime dynamic libraries already required by Yard, including LanceDB library handling
- license/about metadata

### Platform Targets

Initial target:

- Linux x64 AppImage or unpacked directory for local development

Later targets:

- macOS arm64/x64 `.dmg` or `.zip`
- Windows x64 installer or portable build

Cross-platform packaging should not block the Linux MVP.

### Build Commands

Target Makefile additions after implementation:

```bash
make desktop-dev
make desktop-build
make desktop-package
```

Suggested behavior:

| Command | Behavior |
|---|---|
| `make desktop-dev` | runs Go backend and Vite/Electron dev loop |
| `make desktop-build` | builds Go sidecar, React renderer, Electron bundles |
| `make desktop-package` | creates distributable package for current platform |

The existing `make build` should not be changed to require Electron packaging unless explicitly decided later. Node/Electron packaging can be heavier than normal Go builds.

### Candidate Dependencies

Prefer conservative dependencies:

| Need | Candidate |
|---|---|
| Electron shell | `electron` |
| packaging | `electron-builder` or Electron Forge |
| main/preload TypeScript build | `tsup`, `vite`, or `esbuild` |
| e2e | Playwright Electron support |
| IPC validation | handwritten narrow schemas or a small schema library |

Do not add a desktop state framework unless the React app already needs one. Prefer existing React Router and local hooks until state complexity proves otherwise.

---

## Development Modes

### Browser Development

Existing flow remains:

```bash
make dev-backend
make dev-frontend
```

### Desktop Development

Target flow:

```bash
make desktop-dev
```

Under the hood:

1. Build or locate the `yard` backend binary.
2. Start `yard serve` in desktop/dev mode or connect to a configured backend.
3. Start Vite.
4. Start Electron pointing at the Vite URL.
5. Pass backend base URL and session token through preload.

### Packaged Smoke Test

Target flow:

```bash
make desktop-package
./dist/Yard*.AppImage
```

Smoke test should verify:

- app launches
- project can be selected
- backend starts
- health endpoint passes
- dashboard renders
- app quits cleanly

---

## Migration Plan

### Phase 0: Spec And Alignment

- Add this spec.
- Keep [[20-operator-console-tui]] and [[21-web-inspector]] as current implementation references.
- Do not delete or rewrite TUI docs until the desktop implementation exists.

Acceptance:

- New spec defines product boundary, architecture, backend contract, and phased implementation.
- Existing docs can still explain current behavior.

### Phase 1: Desktop Shell MVP

Build the Electron shell around the existing web app.

Scope:

- create `desktop/`
- package/start Electron in dev mode
- start/connect to local Yard backend
- load existing React app
- show backend starting/ready/crashed states
- expose minimal preload bridge
- no new product routes required

Acceptance:

- `make desktop-dev` opens the app against a local project.
- Existing conversation/settings/project endpoints work from Electron.
- Closing the app cleans up only its managed backend process.

Suggested implementation order:

1. Add `desktop/package.json` and minimal Electron main/preload.
2. Add backend process supervisor with attach/dev fallback.
3. Add app data storage for recent project and window state.
4. Add renderer platform adapter.
5. Update API client to support explicit backend base URL.
6. Add `make desktop-dev`.

### Phase 2: Backend Desktop API Parity

Expose missing operator APIs through `internal/server` using `internal/operator`.

Scope:

- runtime status endpoint
- roles endpoint
- launch draft/preset endpoints
- launch preview/start endpoints
- chain list/detail/event/receipt endpoints
- pause/resume/cancel endpoints
- desktop capabilities endpoint
- desktop session token for managed mode

Acceptance:

- Every TUI operator service method needed by desktop has an HTTP equivalent.
- API tests cover launch preview/start, chain reads, receipts, and controls.
- Existing browser routes keep working.

Suggested implementation order:

1. Add `/api/desktop/capabilities`.
2. Add `/api/runtime/status` from `internal/operator.RuntimeStatus`.
3. Add roles and launch draft/preset endpoints.
4. Add launch preview/start endpoints.
5. Add chain list/detail/events/receipts endpoints.
6. Add pause/resume/cancel endpoints.
7. Add chain event streaming.
8. Add focused API tests.

### Phase 3: Desktop Operator UI

Promote the React app from inspector-only to full operator console.

Scope:

- dashboard
- launch workbench
- chain monitor
- chain detail
- receipt detail
- project attachment picker
- role/preset controls
- runtime readiness actions

Acceptance:

- The app can start every launch mode currently supported by TUI.
- The app can pause/cancel/resume where backend supports it.
- The app can read receipts and follow live chain events.
- The app can attach project files/specs/docs to launch requests.

Suggested implementation order:

1. Dashboard route using runtime/chains/conversations.
2. Launch workbench with preview only.
3. Launch start and started-chain navigation.
4. Chain monitor and chain detail.
5. Receipt rendering.
6. Project browser attachment set.
7. Settings/readiness actions.

### Phase 4: Rich Inspection And Native Polish

Unify the current web inspector strengths with desktop-native affordances.

Scope:

- context inspector integration from chat and chain steps
- tool detail and diff inspector
- metrics views
- notifications
- command palette
- app menus
- recent projects
- open-in-editor/reveal-in-file-manager
- diagnostics export

Acceptance:

- A completed chain can be inspected end to end without leaving the desktop app.
- Tool outputs, diffs, context reports, and receipts are navigable from one workspace.
- Native notifications deep-link into relevant app views.

Suggested implementation order:

1. Connect existing context inspector to desktop routes.
2. Add tool detail route and diff view.
3. Add metrics workspace.
4. Add command palette.
5. Add native menus.
6. Add notifications.
7. Add diagnostics export.

### Phase 5: TUI Retirement Decision

Only after desktop parity is real, decide the TUI's future.

Options:

| Option | Behavior |
|---|---|
| keep | TUI remains supported for terminal-first users |
| freeze | TUI remains but receives only bug fixes |
| demote | bare `yard` opens a launcher/help message and TUI moves to `yard tui` |
| remove | TUI code and docs are removed after a compatibility window |

This decision should be made in a separate spec/update after real desktop usage. It should not be bundled into the Electron MVP.

---

## Testing Strategy

### Go Backend

Use existing preferences:

```bash
make test
make build
```

Focused tests:

- `internal/operator`
- `internal/server`
- `cmd/yard` serve flags
- chain control endpoints
- launch endpoints
- desktop capabilities/session behavior

### React Renderer

Use existing frontend tests plus new route/component tests:

```bash
cd web
npm run lint
npm test
npm run build
```

If the repo does not yet define `npm test`, add a script around Vitest before relying on it in CI.

Focus areas:

- API client base URL handling
- platform adapter browser/desktop behavior
- launch form validation
- chain list filtering
- live event rendering
- receipt rendering
- project attachment selection

### Electron Main Process

Add tests where practical for:

- backend command construction
- readiness JSON parsing
- process cleanup
- recent project persistence
- preload API shape
- menu command routing

### End-To-End

Use Playwright with Electron support or an equivalent harness.

Critical e2e flows:

1. Launch app.
2. Open project.
3. Start managed backend.
4. Dashboard loads runtime status.
5. Open existing conversation.
6. Create raw chat turn against a mocked or test provider where available.
7. Preview launch.
8. Start a one-step chain against a test/dry-run backend path if available.
9. Open chain detail.
10. Render receipt.
11. Quit and confirm backend cleanup behavior.

Canvas/pixel checks are only required for future 3D/canvas-heavy views. Normal desktop UI should use DOM assertions and screenshots for layout regressions.

---

## Acceptance Criteria

Desktop MVP:

1. Electron app launches from a repo command.
2. Electron can open a project directory.
3. Electron starts a managed `yard` backend on loopback with an ephemeral port.
4. The backend emits a machine-readable readiness event.
5. The renderer receives backend base URL through preload.
6. Existing conversation/settings/project views work in Electron.
7. Renderer runs with context isolation and without Node integration.
8. Closing the window does not kill unrelated Yard processes.

Desktop operator parity:

1. Runtime readiness is visible in the dashboard.
2. All current launch modes can be previewed and started.
3. Launch drafts and custom presets persist through backend storage.
4. Chains can be listed, filtered, opened, followed, paused, resumed, and cancelled where supported.
5. Receipts render with links to chain steps and files.
6. Project files can be selected as launch attachments through both project browser and native dialog.
7. Context reports, tool details, diffs, and metrics are reachable from chat and chain views.
8. App notifications link to completed/failed chains and required attention.

Architecture:

1. Electron does not duplicate chain execution, provider routing, auth, indexing, brain, or database logic.
2. Durable project state remains backend-owned.
3. Desktop-specific TypeScript is isolated to the Electron shell, preload bridge, and platform adapter.
4. Existing `yard serve` browser mode remains functional.
5. Existing CLI commands remain functional.
6. TUI remains available until a separate retirement decision.

Validation:

1. `make test` passes after backend/API work.
2. `make build` passes after backend/API work.
3. `npm run build` passes for renderer changes.
4. Desktop package smoke test passes on the initial target platform.

---

## Open Questions

1. Should the packaged app support multiple project windows concurrently in v1, or only one project window at a time?
2. Should managed desktop mode start one backend per project window, or one backend process that can switch projects?
3. Should desktop mode be a flag on `yard serve`, or a new hidden/internal command such as `yard desktop-backend`?
4. How should provider login flows that currently assume terminal interaction be represented in desktop?
5. Should the desktop app include a tray/status item for long-running chains after all windows close?
6. Should `yard://` deep links be implemented in the MVP or deferred until chain notifications are useful?
7. What is the first supported packaged platform: Linux AppImage, unpacked Linux directory, or something else?
8. Should `make build` eventually include desktop assets, or should Electron packaging stay behind explicit desktop commands?
9. What is the right compatibility promise for the TUI once desktop reaches launch/control parity?
10. Should browser `yard serve` gain the same operator routes as desktop immediately, or should some routes be hidden behind capabilities until the product boundary is settled?

---

## Dependencies

- [[07-web-interface-and-streaming]] - existing REST/WebSocket and React browser contract
- [[18-unified-yard-cli]] - `yard` as the operator-facing CLI and `tidmouth` as internal engine
- [[20-operator-console-tui]] - current operator feature set to reach or replace
- [[21-web-inspector]] - current rich inspection route/API target
- [[08-data-model]] - shared project persistence and operator state
- [[15-chain-orchestrator]] - chain execution, control, event, and receipt model
- [[19-tool-result-details]] - structured tool metadata for desktop inspectors
