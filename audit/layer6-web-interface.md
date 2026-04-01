# Layer 6 Audit: Web Interface & Streaming

## Scope

Layer 6 is the web interface — the Go HTTP/WebSocket backend server and the React
frontend that together make sirtopham usable in a browser. It is the composition
root where all layers are instantiated and wired together.

## Audit Result

**Audited:** 2026-04-01 | **Result:** Clean — 2 code defects fixed, 4 informational items deferred.

All 10 epics (full checklist) pass. Two issues found during audit were fixed in the same session:
1. `SearchResult` TypeScript type fields (`conversation_id`, `rank`) did not match Go JSON tags (`id`, `updated_at`). Fixed in `web/src/types/api.ts`.
2. Settings page used `useState()` for an API side-effect instead of `useEffect()`. Fixed in `web/src/pages/settings.tsx`.

Frontend builds without errors. TypeScript type-checks cleanly. All Go tests pass (race detector clean).

---

## Spec References

- `docs/specs/07-web-interface-and-streaming.md` — Full architecture
- `docs/layer6/layer-6-overview.md` — Epic index (10 epics)
- `docs/layer6/01-http-server-foundation/` through `10-settings-metrics/` — Task-level specs

## Packages to Audit

| Package | Src | Test | Purpose |
|---------|-----|------|---------|
| `internal/server` | 9 | 3 | HTTP server, REST API, WebSocket |
| `cmd/sirtopham` | 2 | 0 | CLI + composition root (serve.go) |
| `webfs` | 1 | 0 | go:embed for frontend dist |
| `web/src/` | 32 files | — | React + TypeScript frontend |

Backend source files in `internal/server/`:
- `server.go` — HTTP server setup, route registration
- `middleware.go` — CORS, request logging, recovery
- `static.go` — Serves embedded React frontend
- `conversations.go` — REST API for conversations (CRUD, messages, search)
- `project.go` — REST API for project info, file tree
- `configapi.go` — REST API for config and providers
- `metrics.go` — REST API for per-conversation metrics and context reports
- `websocket.go` — WebSocket handler bridging agent loop events to browser

## Test Commands

```bash
CGO_ENABLED=1 CGO_LDFLAGS="-L$(pwd)/lib/linux_amd64 -llancedb_go -lm -ldl -lpthread" \
  LD_LIBRARY_PATH="$(pwd)/lib/linux_amd64" \
  go test -tags 'sqlite_fts5' ./internal/server/...
```

## Audit Checklist

### Epic 01: HTTP Server Foundation
- [x] `server.go` — `Server` struct with `Start()` and `Shutdown()`
- [x] Graceful shutdown on SIGTERM/SIGINT
- [x] Route registration for all endpoints
- [x] `middleware.go`:
  - CORS headers for dev mode (Vite proxy)
  - Request logging with duration
  - Panic recovery
- [x] `static.go` — serves `webfs/dist/` via `embed.FS`
  - SPA fallback: non-API paths serve index.html
  - Content-Type headers set correctly

### Epic 02: REST API — Conversations
- [x] `conversations.go` endpoints:
  - `POST /api/conversations` — create conversation
  - `GET /api/conversations` — list conversations (with pagination)
  - `GET /api/conversations/:id` — get conversation with messages
  - `DELETE /api/conversations/:id` — delete conversation
  - `GET /api/conversations/search?q=` — FTS5 search across messages
- [x] Messages ordered by sequence number
- [x] FTS5 search returns snippets with highlight markers
- [x] JSON response format consistent across endpoints

### Epic 03: REST API — Project, Config, Metrics
- [x] `project.go` endpoints:
  - `GET /api/project` — project info (name, root path)
  - `GET /api/project/files` — file tree
- [x] `configapi.go` endpoints:
  - `GET /api/config` — current config (sanitized — no API keys)
  - `GET /api/providers` — provider list with status
- [x] `metrics.go` endpoints:
  - `GET /api/conversations/:id/metrics` — per-conversation token usage
  - `GET /api/conversations/:id/context-reports` — context assembly reports

### Epic 04: WebSocket Handler
- [x] `websocket.go` — WebSocket endpoint at `/api/ws`
- [x] Accepts conversation ID as query param or in initial message
- [x] Bridges agent loop events to the browser:
  - Receives user message → starts agent turn
  - Streams events: content_delta, tool_call, tool_result, turn_complete, error
- [x] JSON serialization of events over WebSocket
- [x] Connection lifecycle: upgrade, message loop, clean close
- [x] Handles client disconnect gracefully (cancels agent turn)

### Epic 05: Serve Command (Composition Root)
- [x] `cmd/sirtopham/serve.go` — `sirtopham serve` command
- [x] Wires ALL layers together in order:
  1. Config loading
  2. SQLite database + schema init
  3. Provider construction and router registration
  4. Tool registry: file, git, shell, search, brain tools
  5. Executor with recording
  6. Conversation manager
  7. Context assembler
  8. Agent loop construction
  9. HTTP server start
- [x] Brain tools wired: ObsidianClient created when `brain.enabled=true`
  - Defaults to `http://localhost:27124`
  - Passes client and brain config to `RegisterBrainTools`
- [x] Browser launch (if configured)
- [x] Graceful shutdown sequence
- [x] Port configurable via config

### Epic 06: React Scaffolding
- [x] `web/` — Vite + React + TypeScript project
- [x] `web/vite.config.ts` — dev proxy for `/api/*` to Go backend
- [x] Tailwind CSS configured
- [x] shadcn/ui components: button, card, input, scroll-area, separator
- [x] `Makefile` targets: `frontend-deps`, `frontend-build`, `dev-frontend`
- [x] `webfs/embed.go` — `go:embed dist/*`

### Epic 07: Conversation UI
- [x] `web/src/pages/conversation.tsx` — main chat page
- [x] `web/src/hooks/use-websocket.ts` — WebSocket connection management
- [x] `web/src/hooks/use-conversation.ts` — conversation data fetching
- [x] Message rendering:
  - User messages, assistant text, thinking blocks
  - `tool-call-card.tsx` — expandable tool call display
  - `markdown-content.tsx` — markdown rendering in messages
  - `turn-usage-badge.tsx` — token usage per turn
- [x] Streaming display: text appears as content_delta events arrive
- [x] Message input with send button and cancel capability
- [x] Loading state while waiting for response
- [x] `conversation-metrics.tsx` — inline metrics display

### Epic 08: Sidebar Navigation
- [x] `web/src/components/layout/sidebar.tsx` — conversation list sidebar
- [x] `web/src/components/layout/root-layout.tsx` — app shell with sidebar
- [x] `web/src/hooks/use-conversation-list.ts` — fetches conversation list
- [x] Conversation list: title, updated_at, selection state
- [x] Create new conversation button
- [x] Client-side routing: `/`, `/c/:id`, `/settings`

### Epic 09: Context Inspector
- [x] `web/src/components/inspector/context-inspector.tsx` — debug panel
- [x] Shows per-turn context assembly details:
  - Signals extracted
  - Semantic queries generated
  - RAG results with scores
  - Graph results
  - File results
- [x] `budget-bar.tsx` — visual budget breakdown
- [x] `collapsible-section.tsx` — expandable sections
- [x] `web/src/hooks/use-context-report.ts` — fetches context reports
- [x] Quality metrics and latency display

### Epic 10: Settings & Metrics
- [x] `web/src/pages/settings.tsx` — settings panel
- [x] `web/src/hooks/use-providers.ts` — provider list and status
- [x] `web/src/hooks/use-conversation-metrics.ts` — aggregate metrics
- [x] Provider list with health/status indicators
- [x] Default model selection display
- [x] Per-conversation metrics: total tokens, turns, tool calls
- [x] Context quality metrics and project info

### Cross-cutting
- [x] Frontend builds without errors: `cd web && npm run build`
- [x] TypeScript types (`web/src/types/`) match Go API response shapes
- [x] `web/src/lib/api.ts` — API client uses correct endpoint paths
- [x] `web/src/types/events.ts` — WebSocket event types match Go event types
- [x] No console errors in browser during normal operation
- [x] Dev mode works: `make dev-backend` + `make dev-frontend` with Vite proxy
