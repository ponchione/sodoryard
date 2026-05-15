# 24 - Electron Desktop App

**Status:** Electron MVP stop point reached; future parity work deferred
**Last Updated:** 2026-05-14
**Owner:** Mitchell

---

## Overview

Yard should grow a native-feeling desktop operator interface built with Electron. The desktop app becomes the target graphical daily-driver surface for chat, project readiness, chain launch, chain monitoring, receipt review, project attachment, context inspection, tool details, metrics, and settings.

This direction intentionally reuses the existing Go runtime and React frontend investment. Electron should not become a second Yard runtime. The desktop app is a native shell around the same local backend services that power `yard serve`, the current web inspector, and the CLI.

Target product split:

| Surface | Target role |
|---|---|
| Yard Desktop | Primary graphical operator console and inspector |
| `yard` CLI | Scriptable commands, automation, fallback operation, project bootstrap |
| `yard serve` | Local browser/API server for development, remote inspection, and fallback browser use |
| `tidmouth` | Internal engine subprocess contract only |

The terminal UI has been retired. Bare `yard` is a help entrypoint for the CLI command tree, and graphical operation belongs in Yard Desktop or the browser fallback served by `yard serve`.

---

## Decision

Use Electron for the desktop GUI.

Electron is the pragmatic choice because Yard already has:

- React, TypeScript, Vite, Tailwind, and shadcn-style UI primitives under `web/`
- a Go HTTP/WebSocket backend served by `yard serve`
- embedded frontend production assets through `webfs/dist`
- shared operator service methods in `internal/operator`
- Shunter-backed project memory under `.yard/shunter/project-memory`
- Shunter v1.1.0 TypeScript client/runtime, protocol v2, generated binding support, and parameterized declared reads

The desktop app should reuse the React renderer rather than rewriting the UI in another toolkit. The Go backend remains authoritative for runtime state, persistence, chain execution, provider auth, context assembly, brain access, indexing, and tool execution.

The implementation should prefer a sidecar backend process at first:

```text
Electron main process
  -> selects project/config
  -> starts or connects to a local Yard backend
  -> owns native app lifecycle, menus, notifications, file dialogs, deep links

React renderer
  -> renders the operator console and inspector
  -> talks to Yard through command REST/WS APIs
  -> can subscribe to Shunter project memory through generated TypeScript bindings
  -> never mutates project state directly through Node filesystem APIs

Go backend
  -> runs the existing Yard runtime
  -> owns all database, chain, provider, auth, brain, and tool behavior
  -> exposes desktop-safe HTTP/WebSocket and Shunter protocol endpoints bound to localhost
```

An embedded library/runtime integration can be considered later, but the first implementation should keep the process boundary. That boundary preserves the current production model, keeps crash isolation simple, and lets `yard serve` remain the browser/API fallback.

### Shunter TypeScript SDK Impact

Shunter v1.1.0 changes the frontend plan. Yard no longer needs to invent a custom WebSocket stream for every Shunter-backed state table before the desktop app can feel live. Shunter now ships:

- `@shunter/client` as a TypeScript runtime package
- `createShunterClient(...)` for Shunter WebSocket lifecycle, token propagation, reconnect, reducer calls, declared queries, declared views, table subscriptions, and managed subscription handles
- generated TypeScript bindings that import shared runtime types from `@shunter/client`
- generated table row interfaces, table-name-to-row maps, schema-aware BSATN row decoders, reducer helper surfaces, declared-query helpers, declared-view helpers, table subscription helpers, contract metadata, runtime import overrides, and typed declared-read parameter helpers
- protocol v2 for BSATN-encoded declared query/view parameters, while no-parameter reads remain compatible with protocol v1

Yard should use that SDK for project-memory reads and live updates where the data already lives in the Shunter module. The desktop contract should split into two planes:

| Plane | Owner | Intended use |
|---|---|---|
| Yard app API | Go backend / `internal/operator` | commands, auth, provider/model config, launch preview/start, chain control, file validation, indexing, diagnostics, computed summaries |
| Shunter project-memory protocol | Go backend mounting the Shunter runtime | live Shunter table/view reads, chain/event/conversation/update subscriptions, generated row types, contract compatibility checks |

This does not make Electron a Yard runtime. The renderer may consume Shunter's typed client contract, but the Go backend still owns the Shunter runtime, opens the project-memory data directory, validates auth, starts chains, runs providers, writes receipts, indexes, and executes tools.

Initial desktop usage should be read-oriented:

- generate a Yard project-memory TypeScript binding from `internal/projectmemory.NewModule()`
- use Shunter subscriptions for live state updates and cache invalidation
- use Shunter declared query/view parameters for selected-project or selected-chain reads instead of constructing raw SQL strings in the renderer
- keep launch start, pause/resume/cancel, index rebuilds, provider auth, settings mutation, file reads, and editor/reveal validation behind Yard backend APIs
- avoid renderer-initiated reducer calls unless a later slice explicitly marks a reducer as desktop-safe and documents the UX and authorization semantics

The generated binding should be build output, not hand-maintained TypeScript. A practical layout is:

```text
web/src/generated/yard-project-memory.ts
web/src/lib/project-memory/
  client.ts
  subscriptions.ts
  selectors.ts
```

The build should fail if the generated binding is stale relative to the exported Shunter contract. The desktop capabilities endpoint should report the project-memory module name, schema/contract version, Shunter runtime version, negotiated protocol support, and whether the Shunter protocol endpoint is available.

### Shunter v1.1.0 Baseline

Spec 24 implementation should start by moving Yard's Shunter dependency set to the stable `v1.1.0` tag, not by depending on a sibling checkout at branch head. If the Shunter source tree has already moved on to `v1.1.1-dev` or later development commits, desktop work should still pin to the latest stable tag until a newer tag is intentionally adopted.

Required Yard-side updates before desktop UI work depends on Shunter state:

1. Bump `github.com/ponchione/shunter` from `v1.0.1` to `v1.1.0`.
2. Replace `third_party/shunter-client` with the built TypeScript client package from the Shunter `v1.1.0` tag, preserving the package name `@shunter/client`.
3. Run `npm install` in `web/` so `web/package-lock.json` records the vendored client package version `1.1.0`.
4. Regenerate `web/src/generated/yard-project-memory.ts` and the adjacent contract artifact with Shunter `v1.1.0` codegen.
5. Confirm generated `shunterProtocol` defaults to `v2.bsatn.shunter` and still lists `v1.bsatn.shunter` for no-parameter compatibility.
6. Add parameterized declared reads to `internal/projectmemory.NewModule()` for chain-scoped desktop data, starting with selected-chain events.

The immediate declared-read surface needed to replace the current raw SQL helper is:

```go
mod.Query(shunter.QueryDeclaration{
    Name: "chain_events",
    SQL:  "SELECT * FROM events WHERE chain_id = :chain_id ORDER BY sequence DESC LIMIT 500",
    ReadModel: shunter.ReadModelMetadata{
        Tables: []string{"events"},
        Tags:   []string{"chains", "events", "operator-ui", "desktop"},
    },
}, shunter.WithQueryParameters(shunter.ProductSchema{
    Columns: []shunter.ProductColumn{
        {Name: "chain_id", Type: "string"},
    },
}))

mod.View(shunter.ViewDeclaration{
    Name: "live_chain_events",
    SQL:  "SELECT * FROM events WHERE chain_id = :chain_id ORDER BY sequence DESC LIMIT 500",
    ReadModel: shunter.ReadModelMetadata{
        Tables: []string{"events"},
        Tags:   []string{"chains", "events", "operator-ui", "desktop"},
    },
}, shunter.WithViewParameters(shunter.ProductSchema{
    Columns: []shunter.ProductColumn{
        {Name: "chain_id", Type: "string"},
    },
}))
```

The generated binding should then expose typed helpers shaped like `queryChainEventsDecoded(client.runDeclaredQuery, { chainId })` and `subscribeLiveChainEvents(client.subscribeDeclaredView, { chainId }, options)`. Once those helpers exist, remove the renderer-side `chainEventsSQL(...)` raw SQL construction from `web/src/lib/project-memory/client.ts`.

Follow-on declared reads should be added only when a desktop view needs them. Likely next candidates are `chain_steps`, `chain_receipt_documents`, `conversation_messages`, `context_reports_by_conversation`, and `tool_executions_by_conversation`. Keep each read narrow, named, permissionable, and generated.

### Shunter SDK Packaging Decision

The intended long-term dependency shape is a normal npm package whose version matches the Shunter release tag without the leading `v`:

```json
{
  "dependencies": {
    "@shunter/client": "1.1.0"
  }
}
```

As of Shunter v1.1.0, public npm publishing is still not part of the v1 contract. The SDK is checked into the Shunter source tree under `typescript/client`, its `package.json` is marked `private`, and generated bindings still import from `@shunter/client`.

Yard's least-churn temporary path is:

1. Vendor the exact `typescript/client` directory from the pinned Shunter release into the Yard repo.
2. Install it as a local dependency that still resolves as `@shunter/client`.
3. Keep generated project-memory bindings importing `@shunter/client`.
4. Replace the local dependency with the published npm package once Shunter publishes it.

Proposed temporary layout:

```text
third_party/shunter-client/
  package.json
  src/index.ts
  tsconfig.json
  ...
```

`web/package.json` dependency:

```json
{
  "dependencies": {
    "@shunter/client": "file:../third_party/shunter-client"
  }
}
```

The vendored SDK must be copied from the pinned Shunter tag, not from an operator's Go module cache path. If the source-TS package export causes Vite or TypeScript trouble after installation from `file:`, add a small local build step for the vendored package rather than changing generated binding imports across the app.

Shunter v1.1.0 already provides the pieces Yard was waiting on for a workable desktop contract:

- typed declared query/view parameters across Go declarations, runtime calls, protocol v2, TypeScript runtime, and generated helpers
- codegen runtime import override, defaulting to `@shunter/client`
- generated contract metadata from TypeScript bindings alongside `shunterProtocol`

The remaining packaging gap is public npm distribution. Until Shunter publishes `@shunter/client`, Yard should keep vendoring or packing the private local package from the pinned Shunter tag. Yard may still write adjacent metadata for `generated_binding_hash`, generated-from Shunter tag, and build provenance, but basic contract format/version, module name/version, and protocol metadata should come from generated `shunterContract` and `shunterProtocol`.

Reconnect and resubscribe are useful, but desktop views should treat reconnect as a cache boundary. After reconnect, rehydrate from replayed initial rows or explicitly re-fetch REST snapshots; do not assume continuous deltas across the disconnected interval.

### Alternatives Considered

| Alternative | Why not the first choice |
|---|---|
| Browser-only command center | Repeats the original browser-tab problem and misses native project/file/app lifecycle affordances |
| Terminal UI as primary | Rich layout, attachment flows, diffs, metrics, and document intake are naturally GUI-shaped |
| Tauri | Smaller runtime, but adds Rust-side shell work while Yard already has a Go backend and React app; Electron is lower-friction for this repo |
| Native toolkit | Higher platform-specific cost and would discard the existing React web inspector investment |
| Go desktop toolkit | Keeps one language but gives up the frontend ecosystem already in use for markdown, diffs, trees, routing, and charts |

This is not a permanent rejection of Tauri or another shell. It is a choice to optimize the first GUI path for reuse and implementation speed.

---

## Product Goals

1. **Make Yard approachable as a desktop app**
   Operators should be able to open a project, see readiness, chat, launch work, follow chains, inspect results, and adjust settings without managing browser tabs or terminal panes.

2. **Unify operation and inspection**
   The previous terminal/browser split placed operation in one surface and rich inspection in another. The desktop app should combine those into one coherent workspace: launch/control on the left side of the product, rich transcripts/context/diffs/metrics in the detail surfaces.

3. **Preserve local-first behavior**
   The app runs on the developer's machine, talks to local project state, and does not require a hosted service. Cloud providers remain optional provider backends, not Yard infrastructure.

4. **Keep Go as the source of truth**
   Electron should not duplicate Yard business logic. If a workflow changes chain state, provider config, auth state, launch drafts, brain docs, or project files, it goes through the Go backend.

5. **Improve file and document intake**
   The GUI should make it easy to attach project files, docs, brain notes, specs, and pasted context to launch packets.

6. **Expose runtime state clearly**
   Provider/model, auth, index freshness, local service health, chain status, active process state, warnings, and failure remediation should be visible without searching logs.

7. **Support native desktop affordances**
   Use native file/folder pickers, app menus, notifications, tray/status behavior where useful, recent projects, deep links, and open-in-editor handoffs.

---

## Non-Goals

- No Electron-owned execution engine.
- No separate chain scheduler in TypeScript.
- No Electron-owned copy of Yard state tables.
- No hand-maintained TypeScript mirror of the Shunter project-memory schema.
- No direct renderer writes to `.yard/`, `yard.yaml`, project source files, or legacy `.brain/` state.
- No renderer-initiated Shunter reducers for runtime control in the MVP.
- No arbitrary browser terminal with unrestricted shell access.
- No hosted account system, tenancy, remote dashboard, or sync service.
- No mobile UI target.
- No forced removal of `yard serve` or the current React app.
- No exposure of `tidmouth` as a public desktop or CLI surface.

---

## Design Principles

### Backend Authority

The Go backend is authoritative. React may hold temporary UI state, optimistic display state, and form drafts, but durable state must live in the backend's existing stores or in backend-managed config.

Examples:

- launch drafts and presets use `internal/operator`
- chains use `internal/chain` over Shunter project memory
- conversations use existing conversation persistence
- provider routing uses `yard.yaml` plus backend validation
- auth uses backend provider auth stores
- brain content is accessed through backend brain services
- generated TypeScript reads/subscriptions are consumers of Shunter state, not independent state ownership

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

Every desktop workflow should use a documented REST/WebSocket, Shunter protocol, or local IPC contract. If the UI needs a command workflow that only exists in CLI code, add the corresponding backend API by adapting `internal/operator`. If the UI needs live Shunter-backed state, prefer generated project-memory bindings and Shunter subscriptions over a one-off Yard WebSocket envelope.

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
- direct Shunter data-directory access

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
  generated Shunter project-memory bindings

cmd/yard
  existing CLI
  `yard serve` remains browser/API server
  target: desktop backend mode for sidecar startup

internal/server
  existing HTTP/WebSocket API
  target: desktop-safe operator endpoints and Shunter protocol mount

internal/operator
  shared service for runtime status, chain reads/control, launch drafts,
  launch presets, launch preview/start, receipts, roles, and raw chat

internal/projectmemory
  Shunter module and runtime that owns project state, contract export,
  TypeScript codegen source, and protocol serving when enabled
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
| context/diff/metrics      |                       | Shunter/LanceDB      |
+---------------------------+                       +----------------------+
        |
        | Shunter protocol over backend-mounted WS
        v
  generated project-memory bindings
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
{
  "type": "yard_backend_ready",
  "base_url": "http://127.0.0.1:49152",
  "pid": 12345,
  "project_root": "/path/to/project",
  "project_memory": {
    "backend": "shunter",
    "module": "yard_project_memory",
    "schema_version": 12,
    "subscribe_url": "ws://127.0.0.1:49152/api/project-memory/subscribe"
  }
}
```

The human-oriented `yard serve` output can remain for normal CLI use, but desktop mode needs a stable JSON readiness line so Electron does not scrape prose.

### Connection

The renderer talks to `base_url` through the existing Yard REST/WebSocket client layer. It also uses the backend-advertised Shunter project-memory `subscribe_url` with generated TypeScript bindings when that capability is present. The Electron main process passes the backend base URL, desktop session token, Shunter protocol URL, and Shunter protocol token to the renderer through preload at window creation time.

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
- Shunter project-memory protocol connectivity when enabled
- project root/config identity

The renderer should distinguish:

- backend starting
- backend ready
- backend reconnecting
- project-memory reconnecting
- backend crashed
- backend incompatible
- project-memory contract incompatible
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
    "project_memory_protocol",
    "project_memory_subscriptions",
    "project_memory_contract",
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
  ],
  "project_memory": {
    "backend": "shunter",
    "module": "yard_project_memory",
    "schema_version": 12,
    "contract_version": 1,
    "shunter_version": "v1.1.0",
    "default_subprotocol": "v2.bsatn.shunter",
    "supported_subprotocols": ["v2.bsatn.shunter", "v1.bsatn.shunter"],
    "subscribe_url": "ws://127.0.0.1:49152/api/project-memory/subscribe",
    "generated_binding_hash": "sha256:..."
  }
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

Shunter protocol authentication should be explicit too. Browser WebSockets cannot reliably set arbitrary authorization headers, and `@shunter/client` supports the server's `token` query parameter. In managed desktop mode:

- Electron asks the backend for a short-lived Shunter project-memory protocol token.
- The token is scoped to the selected project session and mounted project-memory route.
- The renderer passes it through `createShunterClient({ token })`.
- The token is memory-only and must not be written to project state or Electron app data.
- The Shunter protocol endpoint stays on loopback and should run in strict-auth mode for desktop-managed sessions.

Do not reuse provider credentials or long-lived auth material as the Shunter protocol token.

### State Ownership

| State | Owner | Storage |
|---|---|---|
| project config | Go backend | `yard.yaml` |
| chain state | Go backend | Shunter project memory |
| conversations | Go backend | Shunter project memory |
| launch drafts | Go backend | Shunter project memory through `internal/operator` |
| custom launch presets | Go backend | Shunter project memory through `internal/operator` |
| brain docs | Go backend | Shunter project memory documents |
| code/brain index state | Go backend | Shunter project memory plus derived LanceDB roots |
| provider credentials | Go backend | provider auth store |
| selected project recents | Electron main | OS app data |
| window size/position | Electron main | OS app data |
| sidebar/pane layout | Electron main or renderer | OS app data unless project-semantic |
| temporary form edits | React renderer | memory until saved through backend |
| desktop session token | Electron main/backend | memory only |
| Shunter protocol token | Electron main/backend/Shunter client | memory only |

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

The desktop app absorbs the useful parts of the previous UI directions:

| Existing idea | Desktop owner |
|---|---|
| Runtime readiness dashboard | Desktop dashboard |
| Launch wizard | Desktop launch workbench |
| Chain list/control | Desktop chain monitor |
| Receipt browser | Desktop receipt/detail views |
| Project file attachment | Desktop native file/project picker plus project browser |
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

This state is app convenience state. It should not be written to project-local Yard state.

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

Window state is app-level desktop state, not project runtime state. Store it in Electron app data, not Shunter project memory or other project-local Yard state, unless the state is meaningful to every UI surface.

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
- open provider credential status/remediation
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

The raw desktop chat calls the configured provider/model without one of the 13 role prompts, chain tools, or orchestration unless the operator explicitly starts chain work.

### 4. Launch Workbench

The launch workbench is the desktop surface for assembling and starting chain work.

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

Desktop workflow strengths:

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

The chain detail view should be richer than the current browser inspector:

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
- provider credential status
- provider credential remediation flows where backend supports them
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
- provider credential status
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

The desktop app should reuse existing endpoints from [[07-web-interface-and-streaming]], add missing command endpoints by adapting `internal/operator`, and expose Shunter project-memory protocol access for live state.

Use this rule of thumb:

| Need | Preferred contract |
|---|---|
| start work, preview launch, pause/resume/cancel, auth, settings, index rebuilds, file reads, diagnostics | Yard REST/WS API |
| chain rows, step rows, event rows, launch/preset rows, conversation/message rows, context/tool/subcall rows | Shunter generated bindings and subscriptions |
| computed summaries, permission-checked file content, markdown receipt rendering, context reports with derived search data | Yard REST API |

The Yard API should remain usable without Electron. The Shunter protocol mount should be an optional capability so older browser-only or CLI flows keep working.

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
POST   /api/auth/providers/:provider/refresh

GET    /api/metrics/conversation/:id
GET    /api/metrics/conversation/:id/context/:turn
GET    /api/metrics/conversation/:id/context/:turn/signals

WS     /api/ws
```

### Required Desktop/Operator Endpoints

Runtime:

```text
GET    /api/desktop/capabilities
GET    /api/project-memory/contract
POST   /api/project-memory/token
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

### Shunter Project-Memory Protocol

Managed desktop mode should mount the already-open project-memory Shunter runtime under a backend-owned prefix, for example:

```text
WS     /api/project-memory/subscribe
GET    /api/project-memory/contract
POST   /api/project-memory/token
```

`/api/project-memory/subscribe` is the Shunter protocol endpoint mounted from `Runtime.HTTPHandler()` under the prefix, so the SDK sees the normal `/subscribe` path after prefix stripping. Desktop builds must use Shunter v1.1.0 or newer so generated clients can negotiate `v2.bsatn.shunter` for parameterized declared reads while retaining `v1.bsatn.shunter` compatibility for no-parameter reads. `/api/project-memory/contract` returns the exported `yard_project_memory` module contract plus hash/version metadata. `/api/project-memory/token` mints a short-lived token for the current desktop session.

Renderer setup:

```typescript
import { createShunterClient } from "@shunter/client";
import { shunterContract, shunterProtocol } from "../generated/yard-project-memory";

const client = createShunterClient({
  url: platform.projectMemory.subscribeUrl,
  protocol: shunterProtocol,
  contract: shunterContract,
  token: platform.projectMemory.token,
  reconnect: { enabled: true, resubscribe: true },
});
```

The renderer should verify generated-binding compatibility against the runtime contract metadata before enabling Shunter-backed views. A mismatch should show a backend/app version error and fall back to REST snapshots where available.

Do not expose the Shunter data directory to Electron. Do not start a second Shunter runtime from the renderer or Electron main process. The backend owns the single local runtime and mounts protocol traffic from that runtime.

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

Desktop also needs chain/event updates. With Shunter v1.1.0, the preferred path is:

- use the generated project-memory binding for chain, step, event, launch, conversation, message, tool execution, subcall, and context-report table row types
- subscribe to Shunter tables/views for live row deltas
- use REST snapshots for initial projected summaries and after reconnect
- add parameterized declared Shunter queries/views when the renderer needs narrow, server-owned projections instead of whole-table subscriptions; selected-chain event reads should use `chain_events` and `live_chain_events` rather than renderer-built raw SQL

A custom Yard chain WebSocket should only be added if Shunter table/view subscriptions cannot express a required UI behavior. Fallback options:

1. Add chain events to the existing Yard WebSocket envelope.
2. Add a dedicated Yard chain WebSocket:

```text
WS /api/chains/:id/ws
```

Any non-Shunter chain event contract must support:

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

When using Shunter subscriptions, the renderer should treat subscription updates as cache invalidation plus row deltas, not as the only source of truth for command outcomes. Mutating Yard operations still return command results through REST, and the UI reconciles those results with subsequent Shunter updates.

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
  desktopSessionToken?: string;
  projectMemory?: {
    subscribeUrl: string;
    token: string;
    module: string;
    schemaVersion: number;
    contractVersion: number;
    bindingHash?: string;
  };
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
- Shunter project-memory client wrapper and generated binding integration
- small UI affordances that only render when a desktop capability exists

It should not leak into every page/component.

### Data Fetching

The renderer should centralize backend calls in typed API modules:

```text
web/src/lib/
  api.ts
  project-memory-api.ts
  runtime-api.ts
  launch-api.ts
  chains-api.ts
  project-api.ts
  diagnostics-api.ts
web/src/generated/
  yard-project-memory.ts
```

Fetch behavior:

- include desktop session header when present
- pass the Shunter protocol token only to `createShunterClient`, not to normal REST calls unless the backend explicitly asks for it
- use abort controllers for route changes and cancelled operations
- surface structured backend errors
- retry health/capability checks with backoff during startup
- avoid retrying mutating operations unless the request is explicitly idempotent
- keep Shunter subscription updates separate from REST list refreshes and command results
- subscribe with bounded reconnect and resubscribe enabled for live state views

Project-memory data flow:

1. Load capabilities and project-memory contract metadata through REST.
2. Compare runtime contract metadata with the generated `shunterContract` metadata and generated binding hash.
3. Connect `@shunter/client` to the mounted `/api/project-memory/subscribe` endpoint.
4. Hydrate views from REST snapshots or Shunter initial rows.
5. Apply Shunter row deltas to local query caches.
6. Re-fetch REST snapshots after reconnect or command completion when a computed summary may have changed.

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
- generated Yard project-memory TypeScript binding
- platform-specific `yard` sidecar binary
- any required runtime dynamic libraries already required by Yard, including LanceDB library handling
- license/about metadata

### Platform Targets

Initial target:

- Linux x64 unpacked directory built directly from the source checkout
- user-local Linux `.desktop` launcher that points at the checkout's unpacked package

Later targets:

- macOS arm64/x64 `.dmg` or `.zip`
- Windows x64 installer or portable build

Cross-platform packaging should not block the Linux source-first workflow.

### Build Commands

Target Makefile additions after implementation:

```bash
make bootstrap
make doctor-dev
make desktop-dev
make desktop-build
make desktop-package
make desktop-install-user
```

Suggested behavior:

| Command | Behavior |
|---|---|
| `make bootstrap` | checks source-build prerequisites, installs web/desktop npm dependencies, and verifies generated bindings |
| `make doctor-dev` | reports source-development health without changing project-local runtime state |
| `make desktop-dev` | runs Go backend and Vite/Electron dev loop |
| `make desktop-build` | builds Go sidecar, React renderer, Electron bundles |
| `make desktop-package` | creates a local unpacked package for current platform under `desktop/out/` |
| `make desktop-install-user` | rebuilds the unpacked package and installs/refreshes the user-local Linux launcher and icon |

The existing `make build` should not be changed to require Electron packaging unless explicitly decided later. Node/Electron packaging can be heavier than normal Go builds.

### Candidate Dependencies

Prefer conservative dependencies:

| Need | Candidate |
|---|---|
| Electron shell | `electron` |
| packaging | `electron-builder` or Electron Forge |
| main/preload TypeScript build | `tsup`, `vite`, or `esbuild` |
| Shunter TypeScript runtime | temporary `file:../third_party/shunter-client` dependency resolving as `@shunter/client` from pinned Shunter `v1.1.0`; later `@shunter/client@1.1.0+` from npm if public publishing becomes part of the Shunter contract |
| Shunter project-memory binding | generated from `internal/projectmemory.NewModule()` via Shunter contract/codegen |
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
make bootstrap
make desktop-dev
```

Under the hood:

1. Build or locate the `yard` backend binary.
2. Verify the vendored `@shunter/client` copy matches the pinned Shunter release.
3. Generate or verify the Yard project-memory TypeScript binding.
4. Start `yard serve` in desktop/dev mode or connect to a configured backend.
5. Mount the Shunter project-memory protocol endpoint when available.
6. Start Vite.
7. Start Electron pointing at the Vite URL.
8. Pass backend base URL, desktop session token, Shunter subscribe URL, and Shunter token through preload.

### Packaged Smoke Test

Target flow:

```bash
make desktop-install-user
desktop/out/Yard-linux-x64/yard-desktop
```

Smoke test should verify:

- app launches
- project can be selected
- backend starts
- health endpoint passes
- project-memory contract compatibility passes when Shunter protocol is enabled
- dashboard renders
- app quits cleanly

---

## Migration Plan

Implementation progress as of 2026-05-14:

- Shunter v1.1.0 is pinned, the local `@shunter/client` package is vendored, generated project-memory bindings include protocol v2 metadata, and selected-chain events are exposed through generated `chain_events` / `live_chain_events` helpers.
- Backend desktop API groundwork required by the MVP is implemented: capabilities, project-memory contract/token/protocol endpoints, runtime status, local-service controls, launch draft/preset/preview/start endpoints, chain snapshots/events/receipts/control endpoints, approvals, metrics, and roles are available through HTTP.
- The Electron shell MVP lives in `desktop/`: `make desktop-dev` opens Electron against the existing React app, starts or attaches to a local Yard backend, shows startup/failure states, persists recent project/window state, and exposes a minimal preload bridge with backend and project-memory metadata.
- The active desktop distribution model is source-first. `make bootstrap` prepares the checkout, `make doctor-dev` reports source-development readiness, and `make desktop-install-user` rebuilds the local unpacked package plus a user-local Linux app-menu/taskbar launcher.
- `make desktop-package` creates a local unpacked package under `desktop/out/` containing Electron, the desktop main/preload build, the `yard` sidecar, embedded web assets through the sidecar, a copied web-dist provenance directory, and LanceDB libraries.
- The first desktop product route is `/dashboard`, which Electron opens by default. It combines runtime readiness, recent chains, recent conversations, and Shunter Project Memory activity using existing REST and SDK paths.
- Dashboard readiness actions show detailed local-service health from `/api/runtime/local-services` and can call local-service start, stop, and logs endpoints.
- The chain monitor route at `/chains` lists recent chains active-first with Shunter Project Memory event activity, text/status/role filtering, last-event, duration, and receipt indicators.
- The launch workbench route is implemented at `/launch`. It loads roles, templates, the current draft, and custom presets, assembles launch requests, calls `/api/launch/preview`, starts chains through `/api/launch/start`, and routes to the started chain detail.
- Chain detail includes desktop pause/resume/cancel controls with exact chain-id confirmation before cancellation, links timeline entries with conversation/turn metadata to a desktop context-report route, filters recent events by severity/type, links step receipt paths to receipt detail routes, opens or reveals guardrail changed-file manifests through backend validation, and renders receipt previews with frontmatter, follow-up sections, and backend-validated changed-file open/reveal actions. Receipts have a desktop detail route for frontmatter, body, chain event review with linked event anchors, receipt path copy, follow-up sections, backend-validated source-spec and changed-file open/reveal actions, and step links back to chain detail anchors.
- Launch attachments are implemented for the MVP: `/launch` can browse project files from `/api/project/tree`, validate selected paths through `/api/project/validate-paths`, include accepted paths in launch `source_specs`, and receive handoffs from `/project`.
- The project browser route is implemented at `/project`. It loads the backend-safe project tree, filters files, previews file contents through `/api/project/file`, validates selected launch attachments through `/api/project/validate-paths`, can add project-relative files selected from an Electron native file dialog, opens or reveals backend-validated files through native desktop actions, and hands attachments to `/launch` through `source_spec` query parameters.
- The settings route now shows project status, desktop app/backend version and launch-mode metadata, backend-validated runtime routing controls for the default provider/model, read-only fallback and agent settings, provider model metadata, provider credential status/remediation with backend refresh for refresh-token providers plus source/store details, and diagnostics export through `/api/diagnostics/export`.
- The 2026-05-14 MVP/polish stop point is reached. Do not keep inventing small polish slices unless they fix a concrete bug or regression in the implemented desktop workflows.
- Native notifications, deep links, launch history or duplicate launch packets, standalone metrics workspace expansion, command palette/menu depth, tool/diff inspector expansion, and broad desktop parity remain future-phase work.

### Phase 0: Spec And Alignment

- Add this spec.
- Keep [[21-web-inspector]] as the browser/API implementation reference.
- Treat the retired terminal UI docs as historical only; active operator workflow belongs in this spec.

Acceptance:

- New spec defines product boundary, architecture, backend contract, and phased implementation.
- Spec accounts for Shunter v1.1.0 TypeScript SDK, protocol v2 parameterized declared reads, temporary local `@shunter/client` packaging, and generated project-memory bindings.
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
- pass through project-memory capability metadata when available
- no new product routes required

Acceptance:

- `make desktop-dev` opens the app against a local project.
- Existing conversation/settings/project endpoints work from Electron.
- If the backend advertises Shunter project-memory protocol, the renderer can connect and receive at least a basic table initial snapshot or subscription acknowledgement.
- Closing the app cleans up only its managed backend process.

Suggested implementation order:

1. Add `desktop/package.json` and minimal Electron main/preload.
2. Add backend process supervisor with attach/dev fallback.
3. Add app data storage for recent project and window state.
4. Add renderer platform adapter.
5. Update API client to support explicit backend base URL.
6. Add project-memory capability plumbing without making product routes depend on it.
7. Add `make desktop-dev`.

### Phase 2: Backend Desktop API Parity

Expose missing operator APIs through `internal/server` using `internal/operator`.

Scope:

- Shunter Go module bumped to `v1.1.0`
- Shunter project-memory protocol mount and token endpoint
- vendored `@shunter/client` package copied from the pinned Shunter `v1.1.0` release
- generated Yard project-memory TypeScript binding and stale-check command
- parameterized declared query/view surfaces for selected-chain events
- runtime status endpoint
- roles endpoint
- launch draft/preset endpoints
- launch preview/start endpoints
- chain list/detail/event/receipt endpoints
- pause/resume/cancel endpoints
- desktop capabilities endpoint
- desktop session token for managed mode

Acceptance:

- Every operator service method needed by desktop has an HTTP equivalent.
- Shunter-backed state needed by the UI has either a generated binding subscription path or a REST snapshot path.
- `web/package.json` resolves `@shunter/client` through the vendored package until npm publishing is available.
- A verification check fails when the vendored SDK does not match pinned Shunter `v1.1.0`.
- Contract/codegen tests fail when `yard_project_memory` generated TypeScript is stale.
- Generated Shunter protocol metadata advertises `v2.bsatn.shunter`, and the project-memory SDK smoke covers at least one parameterized declared query or view.
- Chain detail no longer builds raw SQL strings for Shunter event reads after `chain_events` and `live_chain_events` generated helpers exist.
- API tests cover launch preview/start, chain reads, receipts, and controls.
- Existing browser routes keep working.

Suggested implementation order:

1. Bump the Go Shunter dependency to `v1.1.0`.
2. Vendor `typescript/client` from the pinned Shunter `v1.1.0` release and wire `web/package.json` to it as `@shunter/client`.
3. Add `chain_events` and `live_chain_events` parameterized declared reads to `internal/projectmemory.NewModule()`.
4. Regenerate Yard project-memory TypeScript bindings and stale-check automation.
5. Update the project-memory SDK wrapper to use generated parameterized helpers and delete the manual `chainEventsSQL(...)` helper.
6. Add adjacent generated provenance metadata only for values not already emitted by Shunter, such as generated binding hash and generated-from tag.
7. Add `/api/desktop/capabilities`.
8. Mount Shunter project-memory `/subscribe`, expose `/api/project-memory/contract`, and add `/api/project-memory/token`.
9. Add `/api/runtime/status` from `internal/operator.RuntimeStatus`.
10. Add roles and launch draft/preset endpoints.
11. Add launch preview/start endpoints.
12. Add chain list/detail/events/receipts endpoints for computed or fallback snapshots.
13. Add pause/resume/cancel endpoints.
14. Use Shunter subscriptions for chain event liveness unless a custom Yard stream proves necessary.
15. Add focused API, SDK smoke, and contract/codegen tests.

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
- Shunter-backed live state subscriptions for lists, detail views, and activity indicators

Acceptance:

- The app can start every supported launch mode.
- The app can pause/cancel/resume where backend supports it.
- The app can read receipts and follow live chain events through Shunter subscriptions or an explicitly documented fallback stream.
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

### Phase 5: Terminal UI Retirement

The terminal UI has been removed. Bare `yard` now prints CLI help, and the supported operator surfaces are Yard Desktop, `yard serve`, and scriptable `yard` subcommands.

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
- `internal/projectmemory` contract export and protocol serving
- `cmd/yard` serve flags
- chain control endpoints
- launch endpoints
- desktop capabilities/session behavior
- Shunter protocol token minting and route mounting

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
- `@shunter/client` resolution from the vendored `file:` dependency
- generated project-memory binding typecheck
- Shunter client connection state, reconnect, and subscription cache updates
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
- Shunter subscribe URL/token handoff
- menu command routing

### End-To-End

Use Playwright with Electron support or an equivalent harness.

Critical e2e flows:

1. Launch app.
2. Open project.
3. Start managed backend.
4. Dashboard loads runtime status.
5. Project-memory contract compatibility passes when advertised.
6. Shunter live connection reaches connected state or a clear capability-disabled state.
7. Open existing conversation.
8. Create raw chat turn against a mocked or test provider where available.
9. Preview launch.
10. Start a one-step chain against a test/dry-run backend path if available.
11. Open chain detail.
12. Render receipt.
13. Quit and confirm backend cleanup behavior.

Canvas/pixel checks are only required for future 3D/canvas-heavy views. Normal desktop UI should use DOM assertions and screenshots for layout regressions.

---

## Acceptance Criteria

Desktop MVP:

1. Electron app launches from a repo command.
2. Electron can open a project directory.
3. Electron starts a managed `yard` backend on loopback with an ephemeral port.
4. The backend emits a machine-readable readiness event.
5. The renderer receives backend base URL through preload.
6. The renderer receives Shunter project-memory protocol metadata through preload when the backend advertises it.
7. Existing conversation/settings/project views work in Electron.
8. Renderer runs with context isolation and without Node integration.
9. Closing the window does not kill unrelated Yard processes.

Desktop operator parity:

1. Runtime readiness is visible in the dashboard.
2. All current launch modes can be previewed and started.
3. Launch drafts and custom presets persist through backend storage.
4. Chains can be listed, filtered, opened, followed through live state, paused, resumed, and cancelled where supported.
5. Receipts render with links to chain steps and files.
6. Project files can be selected as launch attachments through both project browser and native dialog.
7. Context reports, tool details, diffs, and metrics are reachable from chat and chain views.
8. App notifications link to completed/failed chains and required attention.

Architecture:

1. Electron does not duplicate chain execution, provider routing, auth, indexing, brain, or database logic.
2. Durable project state remains backend-owned.
3. Shunter project-memory TypeScript is generated from the backend module contract, not hand-maintained.
4. Desktop-specific TypeScript is isolated to the Electron shell, preload bridge, platform adapter, and project-memory client wrapper.
5. Existing `yard serve` browser mode remains functional.
6. Existing CLI commands remain functional.
7. Retired terminal UI code stays removed; active surfaces are Desktop, browser/API, and CLI.

Validation:

1. `make test` passes after backend/API work.
2. `make build` passes after backend/API work.
3. Vendored Shunter client verification passes after Shunter dependency changes.
4. Project-memory contract/codegen stale checks pass after Shunter module changes.
5. `make projectmemory-sdk-smoke` covers generated Shunter `v1.1.0` helpers, including at least one parameterized declared read over the mounted runtime.
6. `npm run build` passes for renderer changes.
7. Desktop package smoke test passes on the initial target platform.

---

## Open Questions

1. Should the packaged app support multiple project windows concurrently in v1, or only one project window at a time?
2. Should managed desktop mode start one backend per project window, or one backend process that can switch projects?
3. Should desktop mode be a flag on `yard serve`, or a new hidden/internal command such as `yard desktop-backend`?
4. How should provider credential refresh flows that currently assume terminal interaction be represented in desktop?
5. Should the desktop app include a tray/status item for long-running chains after all windows close?
6. Should `yard://` deep links be implemented in the MVP or deferred until chain notifications are useful?
7. Should AppImage/installer work stay out of scope while the project is in active source-first development?
8. Should `make build` eventually include desktop assets, or should Electron packaging stay behind explicit desktop commands?
9. Should any future lightweight terminal helper exist, or should the CLI remain strictly command-oriented?
10. Should browser `yard serve` gain the same operator routes as desktop immediately, or should some routes be hidden behind capabilities until the product boundary is settled?
11. Should the renderer ever call a whitelisted subset of Shunter reducers directly, or should all mutations stay behind Yard REST commands permanently?
12. After `chain_events` and `live_chain_events`, which project-memory declared queries/views should be added next so desktop can keep moving from broad table subscriptions to narrow generated projections?

---

## Dependencies

- [[07-web-interface-and-streaming]] - existing REST/WebSocket and React browser contract
- [[18-unified-yard-cli]] - `yard` as the operator-facing CLI and `tidmouth` as internal engine
- [[21-web-inspector]] - current rich inspection route/API target
- [[08-data-model]] - shared project persistence and operator state
- [[15-chain-orchestrator]] - chain execution, control, event, and receipt model
- [[19-tool-result-details]] - structured tool metadata for desktop inspectors
- Shunter v1.1.0 `@shunter/client` and TypeScript codegen - generated project-memory bindings, protocol v2 parameterized declared reads, and live subscription runtime
