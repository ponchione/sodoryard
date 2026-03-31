Fresh-session handoff: Layer 6 Epic 06 complete — frontend scaffolding done

What was completed this session (1 commit, pushed to origin/main)
- `a046f7e` — `feat(web): add React scaffolding with Vite, Tailwind, shadcn/ui, and embed.FS integration (L6E06)`

Current state — what exists
- Layers 0-5: fully implemented (tools, agent loop, context assembly, providers, conversations)
- Layer 6 Epics 01, 02, 04, 05: complete (HTTP server, REST API, WebSocket, serve command)
- Layer 6 Epic 06: COMPLETE (React scaffolding)
- Layer 6 Epic 03: NOT started (REST API for project/config/metrics)
- Layer 6 Epics 07-10: NOT started (React UI components)
- `make build` compiles frontend (Vite) → copies dist/ → builds Go binary with embed.FS
- `make test` — all packages pass
- Zero TypeScript errors

Layer 6 status map
```
  ✅ Epic 01 — HTTP Server Foundation
  ✅ Epic 02 — REST API: Conversations (6 endpoints)
  ⬚  Epic 03 — REST API: Project, Config & Metrics
  ✅ Epic 04 — WebSocket Handler
  ✅ Epic 05 — Serve Command (composition root)
  ✅ Epic 06 — React Scaffolding (Vite + React + TS + Tailwind + shadcn/ui)
  ⬚  Epic 07 — Conversation UI (chat interface)
  ⬚  Epic 08 — Sidebar & Navigation
  ⬚  Epic 09 — Context Inspector (debug panel)
  ⬚  Epic 10 — Settings & Metrics UI
```

Epic 06 — what was built
- `web/` — Vite + React + TypeScript project
  - Tailwind CSS v4 with @tailwindcss/vite plugin
  - shadcn/ui v4 (uses @base-ui/react, NOT radix — no asChild prop)
  - Components: Button, Card, Input, ScrollArea, Separator
  - Dark theme default (class="dark" on <html>)
  - React Router v6 with data router: / (conversation list), /c/:id (conversation)
  - App shell: sidebar (w-64) + main content area (flex-1)
  - Vite dev proxy: /api/ws → ws://localhost:8090, /api → http://localhost:8090
- `web/src/types/events.ts` — TypeScript types for all WebSocket events (mirrors Go agent/events.go)
- `web/src/types/api.ts` — TypeScript types for REST API responses
- `web/src/lib/api.ts` — fetch wrapper (api.get, api.post, api.put, api.delete)
- `webfs/embed.go` — go:embed all:dist for production embedding
- `cmd/sirtopham/serve.go` — wires webfs.FS() into server.Config.FrontendFS in prod mode
- Makefile: frontend-build copies web/dist/ → webfs/dist/ before go build

Important notes for next session
- shadcn/ui v4 uses @base-ui/react (NOT Radix). No `asChild` prop on Button — use plain Link/anchor instead
- `erasableSyntaxOnly` is set in tsconfig — cannot use `public` constructor parameter properties
- web/node_modules/ contains a Go package (flatted) — harmless `[no test files]` in `make test`
- .gitignore uses `/lib/` and `/include/` (root-anchored) to avoid matching web/src/lib/

Development workflow
- Two terminals: `make dev-backend` + `make dev-frontend`
- Or production: `make build && ./bin/sirtopham serve --config sirtopham.yaml`

Dependency graph for remaining work
```
  Epic 03 (REST API: Project/Config/Metrics) — independent, can start now
       │
  Epic 07 (Chat UI) ← needs 06 (done)
  Epic 08 (Sidebar) ← needs 06 (done)
  Epic 10 (Settings) ← needs 06 (done)
       │
  ┌────┼──────────┐
  │    │           │
Epic 07  Epic 08  Epic 10     ← parallel, all unblocked
(Chat)  (Sidebar) (Settings)
  │       │
  └──┬────┘
     │
  Epic 09 (Context Inspector) ← needs 07 + 08
```

Next steps — recommended order
1. Epic 07: Conversation UI (first working chat in browser) — HIGH PRIORITY
   - Message list, user input, streaming token display via WebSocket
   - Tool call visualization (collapsible blocks)
   - Thinking indicator
   - Read: docs/layer6/07-conversation-ui/epic-07-conversation-ui.md

2. Epic 08: Sidebar & Navigation
   - Wire conversation list from REST API
   - Active conversation highlighting
   - Read: docs/layer6/08-sidebar-navigation/epic-08-sidebar-navigation.md

3. Epic 03: REST API for Project/Config/Metrics (can parallel with frontend)
   - Read: docs/layer6/03-rest-api-project-config-metrics/epic-03-rest-api-project-config-metrics.md

Validation commands
- `git log --oneline -10`
- `make test` (all packages green)
- `make build && ./bin/sirtopham serve --config sirtopham.yaml`
- `cd web && npx tsc --noEmit` (zero TS errors)
- `make dev-frontend` (Vite dev server on :5173)
