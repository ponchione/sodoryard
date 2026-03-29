# Layer 6: Web Interface & Streaming — Epic Overview

**Build Phase:** 6 (Web Interface)
**Architecture Doc:** [[07-web-interface-and-streaming]]
**Layer Dependencies:** [[layer-0-overview]] (Epics 01-06), [[layer-2-overview]] (Epic 07), [[layer-3-overview]] (Epics 01, 06-07), [[layer-4-overview]] (Epic 01), [[layer-5-overview]] (Epics 01-02, 05-06)
**Layer Consumers:** None — this is the top of the stack. The user interacts here.
**Last Updated:** 2026-03-29

---

## Summary

Layer 6 is the web interface — the Go HTTP/WebSocket backend server and the React frontend that together make sirtopham usable. It bridges the agent loop (Layer 5) and the browser. This is the first time all layers are instantiated together: the `sirtopham serve` command is the composition root that wires config, database, providers, tools, context assembly, and the agent loop into a running server.

The backend serves three purposes: static file server for the compiled React frontend (via `embed.FS`), REST API for CRUD operations, and WebSocket endpoint that bridges agent loop events to the browser. The frontend is a SPA: React + TypeScript + Vite + Tailwind + shadcn/ui, compiled and embedded in the Go binary.

v0.1 scope is functional, not polished. Per [[01-project-vision-and-principles]] §What Success Looks Like v0.1: "Index a Go project, open a browser, have a multi-turn conversation where the agent reads files, runs commands, and edits code — with RAG-assembled context visible in a debug panel."

---

## Dependency Graph

```
Layer 0-5 (available)
         │
    Epic 01 (HTTP Server Foundation)
    ┌────┼──────────────┐
    │    │              │
  Epic 02  Epic 03    Epic 04
  (REST    (REST       (WebSocket
  Convos)  Proj/Cfg)   Handler)
    │       │           │
    └───────┴─────┬─────┘
                  │
            Epic 05 (Serve Command)
                  │
            Epic 06 (React Scaffolding)
                  │
         ┌────────┼──────────┐
         │        │          │
       Epic 07  Epic 08    Epic 10
       (Chat    (Sidebar   (Settings
       UI)      & Nav)     & Metrics)
         │        │
         └───┬────┘
             │
           Epic 09
           (Context Inspector)
```

### Parallelism

- **Parallel track A:** Epics 02, 03, 04 — all three REST/WebSocket handlers can be built simultaneously after Epic 01.
- **Sequential gate:** Epic 05 (Serve Command) requires all backend epics (01-04) to compose the full server.
- **Parallel track B:** Epics 07, 08, 10 — all three frontend features can be built simultaneously after Epic 06.
- **Sequential gate:** Epic 09 (Context Inspector) requires Epic 07 (lives within the conversation view) and benefits from Epic 08 (navigation to select conversations).

### Implementation Priority

Recommended build order for the fastest path to a working product:

1. **Epic 01** (HTTP Server) — foundation for everything
2. **Epics 02 + 04** (REST Conversations + WebSocket) — the two endpoints needed for chat
3. **Epic 05** (Serve Command) — get a running backend you can test with curl/wscat
4. **Epic 06** (React Scaffolding) — get the frontend building and served
5. **Epic 07** (Chat UI) — first working conversation in the browser
6. **Epic 08** (Sidebar) — conversation management
7. **Epic 09** (Context Inspector) — the critical debug panel
8. **Epic 03 + 10** (Project/Config/Metrics REST + Settings/Metrics UI) — rounding out

---

## Cross-Layer Dependencies

| Foundation Epic | What Layer 6 Uses |
| --- | --- |
| Layer 0 Epic 01 | Package layout (`internal/server/`, `web/`) |
| Layer 0 Epic 02 | Structured logging for HTTP requests, WebSocket events, errors |
| Layer 0 Epic 03 | Config loading — server config (port, host, dev mode), provider list for settings |
| Layer 0 Epic 04 | SQLite connection manager — for metrics queries |
| Layer 0 Epic 05 | UUIDv7 — for new conversation creation |
| Layer 0 Epic 06 | Schema & sqlc — queries for conversations, messages, sub_calls, tool_executions, context_reports, messages_fts |
| Layer 2 Epic 07 | Provider router — list providers/models for settings, model override |
| Layer 4 Epic 01 | Tool registry — tool schemas for optional UI display |
| Layer 5 Epic 01 | Event types, EventSink interface, AgentState enum |
| Layer 5 Epic 02 | Conversation manager — Create, Get, List, Delete, SetTitle, FTS5 search, message reconstruction |
| Layer 3 Epic 06 | Context assembly — ContextAssemblyReport consumed by context inspector |
| Layer 5 Epic 06 | Agent loop — RunTurn, Cancel, Subscribe, Unsubscribe — the primary interface Layer 6 calls |

---

## Architecture Doc References

- [[07-web-interface-and-streaming]] — Primary. Entire document defines Layer 6.
- [[05-agent-loop]] — "Streaming to the Web UI" section. All event types with fields. EventSink interface. Agent loop's public API (RunTurn, Cancel, Subscribe, Unsubscribe).
- [[06-context-assembly]] — ContextAssemblyReport data for the context inspector debug panel.
- [[08-data-model]] — All tables queried by REST endpoints. Key query patterns in §Key Query Patterns.
- [[03-provider-architecture]] — Provider/model list for settings panel. Model override mechanism.
- [[01-project-vision-and-principles]] — §Design Principles (web-first), §What Success Looks Like (v0.1 criteria).
- [[02-tech-stack-decisions]] — §Frontend (React + TypeScript + Vite, tentative), §Build System (Makefile integration).
- [[09-project-brain]] — Brain document display in context inspector, "Open in Obsidian" links (v0.3).

---

## Resolved Design Questions

The following questions from [[07-web-interface-and-streaming]] §Open Questions are resolved by constraints in other architecture docs:

- **SSE vs WebSocket:** WebSocket. Confirmed by [[05-agent-loop]] §Streaming to the Web UI — bidirectional protocol needed for cancel and model_override client→server events.
- **SPA vs multi-route:** SPA with client-side routing. Single WebSocket connection per active conversation.
- **Frontend stack:** React + TypeScript + Vite + Tailwind + shadcn/ui. Per [[02-tech-stack-decisions]] §Frontend and the arguments in [[07-web-interface-and-streaming]] §Frontend Stack — the UI requirements (streaming, syntax highlighting, diff views, collapsible blocks, charts) favor React's component ecosystem.
- **Tool output streaming:** Tool call events include `tool_call_output` for incremental output per [[05-agent-loop]] §Streaming to the Web UI. Long-running shell commands stream stdout in real-time.
- **Concurrent tool calls:** Events carry tool call IDs per [[05-agent-loop]] — the UI matches starts to ends and displays concurrent executions correctly.
- **Context debug opt-in:** The `context_debug` event is always emitted by the backend. The frontend displays it when the debug panel is open and ignores it otherwise. No server-side opt-in logic.

---

## Status Legend

- ⬚ Not started
- 🔨 In progress
- ✅ Complete
