Fresh-session handoff: Layer 6 backend — Epics 01, 02, 04 complete

What was completed this session (4 commits, not yet pushed)
- `fc17d80` — `feat(server): add HTTP server foundation with middleware and static serving (L6E01)`
- `bc3aae0` — `feat(server): add REST API for conversations (L6E02)`
- `35ba0ff` — `feat(server): add WebSocket handler for agent event streaming (L6E04)`
- `b6fffcb` — `docs: add Layer 6 backend implementation plan`

Current state
- Layer 6 Epic 01 (HTTP Server Foundation) — COMPLETE
- Layer 6 Epic 02 (REST API Conversations) — COMPLETE
- Layer 6 Epic 04 (WebSocket Handler) — COMPLETE
- Layer 6 Epic 05 (Serve Command) — NOT STARTED (4 tasks remain)
- 28 tests in internal/server/ (all pass with -race)
- All pushed to origin/main: NO — push needed

What's implemented in internal/server/
- server.go: Server struct, Start/Shutdown, ListenAddr (blocks on ready chan), HandleFunc/Handle
- middleware.go: requestLogger, panicRecovery, CORS (dev mode), statusWriter with Hijacker
- static.go: staticHandler with SPA fallback via embed.FS
- api.go: writeJSON, writeError, decodeJSON helpers
- conversations.go: ConversationService interface, ConversationHandler with 6 REST endpoints
- websocket.go: AgentService interface, WebSocketHandler with upgrade/read/write loops

REST API endpoints
- GET    /api/health                       → 200 {"status":"ok"}
- GET    /api/conversations                → paginated list
- POST   /api/conversations                → create (optional title/model/provider)
- GET    /api/conversations/{id}           → get single (404 if not found)
- GET    /api/conversations/{id}/messages  → all messages with compression flags
- DELETE /api/conversations/{id}           → delete with cascade (204)
- GET    /api/conversations/search?q=      → FTS5 full-text search with snippets
- WS     /api/ws                           → WebSocket for agent event streaming

Also added to conversation.Manager
- GetMessages(ctx, conversationID) → []MessageView (includes is_compressed, is_summary)
- Search(ctx, query) → []SearchResult
- ListAllMessages sqlc query in internal/db/query/conversation.sql

Key design decisions
- Go stdlib net/http with Go 1.22+ pattern matching (no framework)
- nhooyr.io/websocket v1.8.17 for WebSocket
- Server.ListenAddr() blocks on a `ready` channel — race-free for tests
- statusWriter implements http.Hijacker for WebSocket upgrade through middleware chain
- ConversationService and AgentService are narrow interfaces in server package — testable with mocks
- ChannelSink reused from agent.NewChannelSink (not duplicated)
- One-turn-at-a-time enforced via atomic.Bool in WebSocket read loop

Pitfalls discovered
- nhooyr/websocket v1.8.17 uses direct http.Hijacker type assertion, not Unwrap()
  → statusWriter must implement Hijacker explicitly
- Polling ListenAddr in tests causes data races with Start goroutine
  → Use a ready channel that blocks until listener is bound

What is NOT implemented
- Epic 05: Serve Command — wires all layers into `sirtopham serve`
- Epic 06-10: React frontend (out of scope for this plan)
- Agent loop batch dispatch refactor (independent, lower priority)
- Obsidian Client & Brain Tools (v0.2 scope)

Next natural slice
a. Epic 05: Serve Command (composition root) — 4 tasks:
   1. Wire init sequence: config → DB → providers → tools → context → agent loop → server
   2. Graceful shutdown with signal handling (SIGINT/SIGTERM)
   3. Startup logging + browser launch
   4. Flag overrides (--port, --host, --dev)
   See: docs/plans/2026-03-31-layer6-backend.md (Tasks 5.1-5.4)
   Key files: cmd/sirtopham/main.go, internal/server/, internal/agent/loop.go

Validation state
- `make test` green (28 packages, 28 server tests)
- `go test -race ./internal/server/... -count=1` clean

Suggested commands
- `git log --oneline -10`
- `git push origin main`
- `make test`
