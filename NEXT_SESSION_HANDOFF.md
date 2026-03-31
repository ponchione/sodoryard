Fresh-session handoff: Layer 6 Epic 06 complete, Epic 07 slice 1 complete

What was completed this session (2 commits, pushed to origin/main)
- `a046f7e` — `feat(web): add React scaffolding with Vite, Tailwind, shadcn/ui, and embed.FS integration (L6E06)`
- `0b8af83` — `feat(web): add WebSocket streaming chat — minimal conversation UI (L6E07 slice 1)`

Current state — what exists
- Layers 0-5: fully implemented (tools, agent loop, context assembly, providers, conversations)
- Layer 6 Epics 01, 02, 04, 05: complete (HTTP server, REST API, WebSocket, serve command)
- Layer 6 Epic 06: COMPLETE (React scaffolding)
- Layer 6 Epic 07: SLICE 1 COMPLETE (streaming chat plumbing)
- Layer 6 Epic 03: NOT started (REST API for project/config/metrics)
- Layer 6 Epics 08-10: NOT started
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
  🔶 Epic 07 — Conversation UI (slice 1 done: streaming chat)
  ⬚  Epic 08 — Sidebar & Navigation
  ⬚  Epic 09 — Context Inspector (debug panel)
  ⬚  Epic 10 — Settings & Metrics UI
```

Epic 07 — what's done vs remaining
DONE (slice 1 — streaming chat plumbing):
  - useWebSocket hook: connect /api/ws, auto-reconnect with exponential backoff
  - useConversation hook: reducer-based state machine for messages + streaming
  - ConversationPage: message bubbles, streaming text cursor, status indicator,
    error banner, cancel button, auto-scroll
  - ConversationListPage: input bar → navigates to /c/new with initial message
  - URL updates to /c/:id on conversation_created event
  - Enter to send, Shift+Enter for newline, disabled during turn

REMAINING (future slices):
  - Load existing conversation history via REST (GET /api/conversations/:id/messages)
  - Thinking blocks (collapsible, streaming thinking_delta events)
  - Tool call blocks (collapsible cards with tool name, args, output, duration)
  - Compressed/summary message rendering (greyed out, [compressed] indicator)
  - Markdown rendering (react-markdown + remark-gfm)
  - Syntax highlighting (code blocks in assistant responses + tool output)
  - Turn complete usage summary display

Important notes for next session
- shadcn/ui v4 uses @base-ui/react (NOT Radix). No `asChild` prop on Button
- `erasableSyntaxOnly` in tsconfig — no `public` constructor parameter properties
- .gitignore: `**/node_modules/` covers all nested node_modules (ts-analyzer too)
- .gitignore: `/lib/` and `/include/` root-anchored to avoid matching web/src/lib/
- .gitignore: `/sirtopham` root-anchored to avoid matching cmd/sirtopham/

Development workflow
- Two terminals: `make dev-backend` + `make dev-frontend`
- Or production: `make build && ./bin/sirtopham serve --config sirtopham.yaml`

Next steps — recommended order
1. Epic 07 slice 2: Thinking + tool call visualization
   - thinking_start/delta/end → collapsible "Thinking…" section
   - tool_call_start/output/end → collapsible cards
   - Multiple concurrent tool calls tracked by ID

2. Epic 07 slice 3: Markdown + syntax highlighting + load history
   - npm install react-markdown remark-gfm react-syntax-highlighter
   - GET /api/conversations/:id/messages on mount

3. Epic 08: Sidebar — wire conversation list from REST API

4. Epic 03: REST API for Project/Config/Metrics (independent, can parallel)

Validation commands
- `git log --oneline -10`
- `make test` (all packages green)
- `make build && ./bin/sirtopham serve --config sirtopham.yaml`
- `cd web && npx tsc --noEmit` (zero TS errors)
- `make dev-frontend` (Vite dev server on :5173)


What's left for Epic 07 (future slices):
     - Thinking blocks, tool call cards
     - Markdown rendering, syntax highlighting
     - Load existing conversation history from REST
     - Compressed message renderingRearearaerer