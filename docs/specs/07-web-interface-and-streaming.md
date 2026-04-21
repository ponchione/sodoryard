# 07 — Web Interface & Streaming Protocol

**Status:** Living spec — aligned with the current Layer 6 v0.1 contract
**Last Updated:** 2026-03-29
**Author:** Mitchell

---

## Overview

sodoryard's primary interface is a locally-served web application. `yard serve` starts the supported HTTP server and opens the browser. The frontend communicates with the Go backend via REST (CRUD operations) and WebSocket (real-time streaming).

This document covers the frontend stack, the backend HTTP/WebSocket server, the streaming message protocol, and the UI component architecture.

---

## Architecture

```
Browser (React app)
    ↕ WebSocket (streaming: tokens, tool events, status)
    ↕ REST (CRUD: conversations, config, project info)
Go HTTP Server
    → embed.FS (serves compiled frontend assets)
    → API handlers (REST endpoints)
    → WebSocket handler (streaming bridge to agent loop)
```

**Single binary:** The compiled React frontend is embedded in the Go binary via `embed.FS`. No separate frontend server in production. In development, Vite's dev server proxies API calls to the Go backend.

---

## Frontend Stack

**v0.1 decision:** React + TypeScript + Vite + Tailwind CSS + shadcn/ui

The UI needs to handle: WebSocket streaming, collapsible tool call blocks, syntax highlighting, file trees, diff views, and metrics panels. That favors a capable component ecosystem. shadcn/ui provides accessible, composable primitives, and the stack aligns with the current Layer 6 decomposition.

**Arguments for React:**
- Largest component ecosystem (syntax highlighters, diff viewers, tree views all exist)
- TypeScript support is first-class
- Developer familiarity
- shadcn/ui is React-native

**Arguments against:**
- Heavy for a single-user local app
- Build step adds complexity
- Overkill if the UI ends up being simpler than imagined

---

## WebSocket Streaming Protocol

This is the critical design piece. Every layer pushes events through this pipe.

### Event Types (Current shipped contract)

All server events are wrapped in a `ServerMessage` envelope:

```typescript
type ServerMessage = {
  type: string;
  timestamp: string;
  data: unknown;
};
```

Current event payloads follow the live backend/frontend contract:

```typescript
type AgentState =
  | "idle"
  | "assembling_context"
  | "waiting_for_llm"
  | "executing_tools"
  | "compressing";

// Server → Client events
type ServerEvent =
  | { type: "conversation_created"; data: { conversation_id: string } }
  | { type: "token"; data: { token: string } }
  | { type: "thinking_start"; data: {} }
  | { type: "thinking_delta"; data: { text: string } }
  | { type: "thinking_end"; data: {} }
  | { type: "tool_call_start"; data: { tool_call_id: string; tool_name: string; arguments: object } }
  | { type: "tool_call_output"; data: { tool_call_id: string; output: string } }
  | { type: "tool_call_end"; data: { tool_call_id: string; result: string; duration: number; success: boolean } }
  | { type: "turn_complete"; data: { turn_number: number; iteration_count: number; total_input_tokens: number; total_output_tokens: number; duration: number } }
  | { type: "turn_cancelled"; data: {} }
  | { type: "error"; data: { message: string; recoverable?: boolean; error_code?: string } }
  | { type: "context_debug"; data: ContextDebugInfo }
  | { type: "status"; data: { state: AgentState } };

// Client → Server events
type ClientEvent =
  | { type: "message"; content: string; conversation_id?: string; provider?: string; model?: string }
  | { type: "cancel" }
  | { type: "model_override"; provider?: string; model?: string };
```

### Protocol Notes
- Tool outputs stream incrementally via `tool_call_output` events.
- Multiple concurrent tool calls are represented as interleaved event streams keyed by `tool_call_id`.
- `context_debug` is emitted by the backend after context assembly; the frontend decides whether to render it.
- the context inspector also fetches stored reports from `GET /api/metrics/conversation/:id/context/:turn` and the ordered signal stream from `GET /api/metrics/conversation/:id/context/:turn/signals` when browsing history.
- if inspector report loading fails, the shipped UI now renders an explicit error state instead of silently looking empty.
- `conversation_created` is sent when a new conversation is created over WebSocket so the frontend can update routing and subsequent REST calls.
- Closing the WebSocket cancels any in-flight turn via shared context cancellation. This is distinct from an explicit client `cancel` message.

---

## REST API Endpoints (Current v0.1 contract)

```
GET    /api/conversations              List conversations
POST   /api/conversations              Create new conversation
GET    /api/conversations/:id          Get conversation metadata
GET    /api/conversations/:id/messages Get messages for conversation
DELETE /api/conversations/:id          Delete conversation
GET    /api/conversations/search?q=... Search conversations via FTS5

GET    /api/project                    Project metadata (id, name, root_path, language, last indexed info)
GET    /api/project/tree               File tree
GET    /api/project/file?path=...      Plain-text file contents plus path/language metadata

GET    /api/config                     Current UI-relevant runtime config
PUT    /api/config                     Update mutable runtime config (v0.1: default provider/model)
GET    /api/providers                  Configured providers and available models
GET    /api/auth/providers             Provider auth/status diagnostics for operator-facing surfaces

GET    /api/metrics/conversation/:id              Per-conversation token/tool/context metrics
GET    /api/metrics/conversation/:id/context/:turn ContextAssemblyReport for a specific turn
GET    /api/metrics/conversation/:id/context/:turn/signals Ordered signal-flow stream for a specific turn

WS     /api/ws                         WebSocket for streaming
```

---

## UI Components (Current shipped scope)

- **Conversation view:** Chat-style message thread with streaming token display
- **Tool call visualization:** Inline syntax-highlighted diffs, command output, search results
- **Context inspector (debug):** turn-by-turn context reports, ordered signal flow, retrieval results, token budget allocation, and explicit load errors when report fetches fail
- **Conversation sidebar:** Past conversations with search and delete controls
- **Settings page:** Model selection, provider config, tool permissions
- **Metrics/stats:** Token usage, cost per conversation, model breakdown

Notes:
- `/api/project/tree` and `/api/project/file` are exposed backend/operator endpoints today, but a dedicated file-browser/code-viewer route is not part of the current shipped UI.

### Compelling Visualizations (Web-Only)
- Live streaming diffs as the agent edits files
- Interactive file tree with "files the agent touched" highlighting
- Tool call timeline showing sequential execution
- RAG hit visualization — which code chunks were retrieved and why
- Cost breakdown charts per conversation or over time
- Side-by-side before/after code comparison

---

## Dependencies

- [[05-agent-loop]] — drives all streaming events
- [[08-data-model]] — conversations, messages, metrics
- [[03-provider-architecture]] — model selection, provider status

---

## Current v0.1 decisions

- WebSocket over SSE — bidirectional transport is required for `cancel` and `model_override`.
- SPA with client-side routing and one WebSocket connection per active conversation view.
- React + TypeScript + Vite + Tailwind + shadcn/ui for the initial frontend implementation.
- Vite dev server proxies API and WebSocket traffic to the Go backend during development.
- Offline/degraded provider behavior remains a runtime concern: the UI should still function when only cloud providers are available.
