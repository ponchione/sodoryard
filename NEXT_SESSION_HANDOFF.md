Fresh-session handoff: Layer 6 Epics 03, 07, 08 complete

What was completed this session (3 commits, pushed to origin/main)
- `6af791c` — `feat(web): thinking blocks, tool call cards, markdown rendering, history loading (L6E07 slices 2-3)`
- `80b81c7` — `feat(web): sidebar with conversation list, navigation, and mobile responsive layout (L6E08)`
- `584beb2` — `feat(api): REST endpoints for project, config, providers, and metrics (L6E03)`

Current state — what exists
- Layers 0-5: fully implemented (tools, agent loop, context assembly, providers, conversations)
- Layer 6 Epics 01-08: ALL COMPLETE
- Layer 6 Epic 09: NOT started (Context Inspector debug panel)
- Layer 6 Epic 10: NOT started (Settings & Metrics UI)
- `make build` compiles frontend (Vite) → copies dist/ → builds Go binary with embed.FS
- `make test` — all packages pass
- Zero TypeScript errors

Layer 6 status map
```
  ✅ Epic 01 — HTTP Server Foundation
  ✅ Epic 02 — REST API: Conversations (6 endpoints)
  ✅ Epic 03 — REST API: Project, Config & Metrics (8 endpoints)
  ✅ Epic 04 — WebSocket Handler
  ✅ Epic 05 — Serve Command (composition root)
  ✅ Epic 06 — React Scaffolding (Vite + React + TS + Tailwind + shadcn/ui)
  ✅ Epic 07 — Conversation UI (all 3 slices complete)
  ✅ Epic 08 — Sidebar & Navigation
  ⬚  Epic 09 — Context Inspector (debug panel)
  ⬚  Epic 10 — Settings & Metrics UI
```

Epic 03 — REST API endpoints (NEW this session)
  GET /api/project           — project root_path, name, detected language
  GET /api/project/tree      — nested JSON file tree (?depth=1-10)
  GET /api/project/file      — file content (?path=relative/path)
  GET /api/config            — default/fallback provider+model, agent settings
  PUT /api/config            — runtime override default provider/model
  GET /api/providers         — provider list with models and status
  GET /api/metrics/conversation/:id — token usage, cache hit, tool usage, context quality
  GET /api/metrics/conversation/:id/context/:turn — full context assembly report

Epic 08 — Sidebar & Navigation (NEW this session)
  - useConversationList hook: fetch, delete, refresh
  - Real conversation list with titles, relative timestamps
  - Active conversation highlight (from URL path)
  - Delete button (hover reveal, navigates home if active)
  - Mobile responsive: hamburger menu, overlay sidebar with backdrop
  - Close on conversation selection (mobile)

File map (new backend files)
  internal/server/project.go   — project info, file tree, file content
  internal/server/configapi.go — config CRUD, provider listing
  internal/server/metrics.go   — per-conversation metrics, context reports

File map (frontend)
  web/src/hooks/use-conversation.ts       — block-based reducer, all event types
  web/src/hooks/use-conversation-list.ts  — REST conversation list
  web/src/hooks/use-websocket.ts          — WebSocket connection management
  web/src/pages/conversation.tsx          — main chat page with block rendering
  web/src/pages/conversation-list.tsx     — conversation list / home page
  web/src/components/layout/sidebar.tsx   — sidebar with nav, conv list, mobile
  web/src/components/layout/root-layout.tsx — sidebar state, hamburger
  web/src/components/chat/
    thinking-block.tsx                    — collapsible thinking section
    tool-call-card.tsx                    — collapsible tool call card
    turn-usage-badge.tsx                  — token/duration usage pill
    markdown-content.tsx                  — markdown + syntax highlighting
  web/src/lib/history.ts                  — REST MessageView[] → ChatMessage[]
  web/src/lib/api.ts                      — fetch wrapper

Important notes for next session
- shadcn/ui v4 uses @base-ui/react (NOT Radix). No `asChild` prop on Button
- `erasableSyntaxOnly` in tsconfig — no `public` constructor parameter properties
- PUT /api/config is runtime-only, NOT persisted to sirtopham.yaml
- react-syntax-highlighter bundle is ~1MB — consider lazy import if bundle size matters
- `make test` not `go test ./...` — Makefile has CGo linker flags for lancedb

Development workflow
- Two terminals: `make dev-backend` + `make dev-frontend`
- Or production: `make build && ./bin/sirtopham serve --config sirtopham.yaml`

Next steps — recommended order
1. Epic 09: Context Inspector (debug panel)
   - Tab/drawer in conversation view for context_debug events
   - Token budget visualization (budget_total, budget_used)
   - Needs, signals, RAG/brain/graph results display
   - Per-turn context report from GET /api/metrics/conversation/:id/context/:turn

2. Epic 10: Settings & Metrics UI
   - Settings page: model/provider selector (GET/PUT /api/config)
   - Per-conversation metrics dashboard (GET /api/metrics/conversation/:id)
   - Project info display (GET /api/project)

Validation commands
- `git log --oneline -10`
- `make test` (all packages green)
- `make build && ./bin/sirtopham serve --config sirtopham.yaml`
- `cd web && npx tsc --noEmit` (zero TS errors)
- `make dev-frontend` (Vite dev server on :5173)
