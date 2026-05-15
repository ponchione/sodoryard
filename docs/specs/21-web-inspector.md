# 21 - Web Inspector

**Status:** Active companion spec
**Last Updated:** 2026-05-03
**Owner:** Mitchell

---

## Overview

The web inspector is the retained browser surface served by `yard serve`. It is not a separate runtime. Daily graphical operation belongs in Yard Desktop, specified by [[24-electron-desktop-app]], while the browser exists for development, fallback access, and views that benefit from direct local HTTP access.

The current React app already provides:

- conversation list and chat routes
- WebSocket turn streaming
- inline tool-call rendering
- context inspector
- per-conversation metrics
- read-only chain list/detail routes
- chain-level metrics route
- provider/project/settings panels
- project metadata endpoints

The target is to sharpen that app into an inspector rather than growing it into a second full command center.

---

## Product Boundary

The split is intentional:

| Surface | Primary responsibility |
|---|---|
| Yard Desktop | Start, monitor, control, and inspect work in the primary graphical shell |
| `yard serve` | Browser fallback for transcripts, context, tools, diffs, files, and metrics |
| `yard <subcommand>` CLI | Scriptable one-shot commands and automation |

The browser can include chain views and convenience actions, but normal graphical operation should stay aligned with Desktop and must use the same backend APIs.

---

## Web Inspector Goals

1. **Rich transcript browsing**
   Render conversation history, streaming tokens, markdown, code blocks, thinking blocks, and tool calls more comfortably than terminal panes can.

2. **Context inspection**
   Show context reports, analyzer signals, retrieval results, budget allocation, explicit files, brain hits, and context-debug events in a visual layout.

3. **Tool details**
   Render structured tool metadata, command output, file diffs, search results, and normalized tool result details.

4. **Visual comparison**
   Support side-by-side or inline diff views, touched-file navigation, and readable code previews.

5. **Metrics and quality analysis**
   Show token usage, latency, provider/model breakdowns, tool counts, context quality signals, and chain-level summaries where available.

6. **Optional document intake**
   Browser document drop can remain if it proves materially better than editor-based flows. It should feed the same launch/work-packet model used by Desktop, not create a separate execution path.

---

## Non-Goals

- No second execution runtime competing with Desktop.
- No browser-only chain execution path.
- No mobile UI.
- No hosted/multi-user/product onboarding assumptions.
- No arbitrary shell terminal in the browser.
- No full code editor.
- No separate Knapford service, binary, or container.

---

## Information Architecture

Minimum useful routes:

| Route | Purpose |
|---|---|
| `/` | Inspector home: recent conversations, recent chains, runtime warnings, links into detail views |
| `/c/new` | Existing new conversation flow |
| `/c/:id` | Existing conversation transcript with tool details and context inspector |
| `/chains` | Read-oriented chain list with filters and links to receipts/events |
| `/chains/:id` | Chain detail, event log, step summaries, receipts, and agent output |
| `/metrics` | Conversation, chain, provider/model, tool, and context metrics |
| `/settings` | Existing provider/model/project settings and diagnostics |

Routes should favor inspection and drilldown. Launch and control actions may appear where useful, but the canonical primary paths are Desktop and the CLI.

---

## Rich Views

### Conversation View

Keep and improve the current chat route:

- streaming response rendering
- markdown/code block rendering
- thinking block display
- inline tool call cards
- turn usage badges
- per-turn context report link
- search and history navigation

### Context Inspector

The context inspector remains a browser-strength view:

- ordered signal flow
- analyzer needs
- semantic queries
- explicit files and symbols
- code retrieval hits
- brain retrieval hits
- graph/context relationships
- budget allocation
- final injected context summary

### Tool Detail Inspector

The browser should render rich tool details from [[19-tool-result-details]]:

- command output with truncation controls
- file read/write summaries
- diffs and changed-file lists
- search result groups
- error state and duration
- structured metadata when present

### Chain Detail

The browser chain detail route is useful for inspection:

- step timeline
- event log with filtering
- receipt links and rendered markdown
- role/persona per step
- tool output summaries
- token/model summaries
- links back to conversations or source specs when available

Desktop remains the preferred graphical place to follow and control running chains.

### Project Browser

Use existing project endpoints:

- tree navigation
- text file preview
- language/line count metadata
- touched-file highlighting when chain/tool data supports it
- links to open files in editor from terminal workflows when possible

### Metrics

Browser metrics should focus on views that are awkward in terminal tables:

- token trends
- provider/model comparisons
- tool execution breakdowns
- context quality summaries
- chain duration and outcome summaries

---

## API Contract

The web inspector reuses the existing HTTP/WebSocket contract in [[07-web-interface-and-streaming]] and should share internal services with Desktop where behavior overlaps.

Current reused endpoints:

```text
GET    /api/conversations
POST   /api/conversations
GET    /api/conversations/:id
GET    /api/conversations/:id/messages
DELETE /api/conversations/:id
GET    /api/conversations/search?q=...

GET    /api/health
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

Current chain/readiness endpoints mirror the service methods used by Desktop:

```text
GET    /api/runtime/status
GET    /api/chains
GET    /api/chains/:id
GET    /api/chains/:id/events
GET    /api/chains/:id/receipts
GET    /api/chains/:id/receipt?step=...
POST   /api/chains/:id/approvals/:approval_id/approve
POST   /api/chains/:id/approvals/:approval_id/deny
```

Approval controls exist on chain detail because they are tied to inspecting an approval-required timeline. Pause/resume/cancel endpoints may exist for parity, but their presence does not make the browser the primary control surface.

---

## Implementation Guidance

- Keep React route work focused on inspection surfaces already hard or awkward in terminal panes.
- Avoid browser-only launch state. Persistent current launch drafts and custom presets now live in shared operator storage; future browser launch work should use that backend path instead of React-owned state.
- Do not introduce a second execution path. Browser actions that start or control work must call the same internal chain runner/service path as CLI and Desktop.
- Prefer read-only browser additions before write/control additions.
- Preserve the embedded frontend production model through `web/dist` and `webfs/dist`.

---

## Migration From Browser Command Center

The previous browser-first command-center spec has been superseded. Useful ideas survive, but ownership changes:

| Former command-center idea | New owner |
|---|---|
| Runtime readiness dashboard | Desktop primary, web optional summary |
| Launch workbench | Desktop primary |
| Chain list/control | Desktop primary, web inspector detail |
| Context inspector | Web inspector |
| Tool details/diffs | Web inspector |
| Metrics/charts | Web inspector |
| Project browser | IDE; browser keeps project metadata/settings only |
| Document intake | Deferred/shared; browser only if it remains materially better |

The old route-heavy browser plan should not be implemented wholesale.

---

## Acceptance Criteria

1. `yard serve` continues to serve the embedded React app and HTTP/WebSocket API.
2. Existing conversation, streaming, tool-call, settings, and context-inspector behavior remains intact.
3. New browser work improves rich inspection rather than creating a divergent command center.
4. Browser chain views, if added, read from the same chain store/service model as Desktop.
5. Browser launch/control actions, if added, call the same internal chain runner path as `yard chain start` and Desktop.
6. The browser is optional for normal chain launch and supervision.

---

## Open Questions

- Which chain detail views are rich enough to justify browser work beyond the desktop routes?
- Should document drag/drop survive as a first-class browser feature or remain desktop/editor-centered?
- How much browser metrics work is useful before chain execution is stable enough to generate meaningful data?
