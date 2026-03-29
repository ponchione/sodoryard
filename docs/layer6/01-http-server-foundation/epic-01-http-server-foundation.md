# Layer 6, Epic 01: HTTP Server Foundation

**Layer:** 6 — Web Interface & Streaming
**Status:** ⬚ Not started
**Dependencies:** Layer 0 Epic 01 (project scaffolding), Layer 0 Epic 02 (logging), Layer 0 Epic 03 (config)

---

## Description

Set up the Go HTTP server that serves as the backbone for all of Layer 6. This includes the HTTP listener bound to localhost, a router for mounting REST and WebSocket handlers, middleware (request logging, CORS for dev mode, panic recovery), and the `embed.FS` static file server for serving the compiled React frontend in production mode. In development mode, the server serves only the API — the Vite dev server handles frontend assets and proxies API calls back.

This is the skeleton that every subsequent Layer 6 epic builds on. No REST handlers, no WebSocket logic — just the server, the router, and the static file serving infrastructure.

---

## Definition of Done

- [ ] `internal/server/` package exists
- [ ] HTTP server starts and listens on configurable `host:port` from config (default `localhost:3000`)
- [ ] Server binds to localhost only — no external access (per [[01-project-vision-and-principles]]: "Not a multi-user system")
- [ ] Router framework chosen and set up (stdlib `http.ServeMux` with Go 1.22+ pattern matching, or chi — agent's choice based on needs)
- [ ] Route groups mountable: `/api/*` for REST handlers, `/api/ws` reserved for WebSocket
- [ ] Middleware chain: request logging (method, path, status, duration via [[layer-0-overview]] logging), panic recovery, CORS (dev mode only — permissive for `localhost:5173` Vite dev server)
- [ ] `embed.FS` configured to serve compiled frontend from `web/dist/` in production mode. Falls back gracefully if `web/dist/` doesn't exist yet (dev mode)
- [ ] SPA fallback routing: non-API requests that don't match a static file serve `index.html` (enables client-side routing)
- [ ] Dev mode flag from config controls CORS behavior and whether `embed.FS` is active
- [ ] Health check endpoint: `GET /api/health` returns 200 with `{"status": "ok"}`
- [ ] Server supports graceful shutdown via context cancellation (for SIGINT/SIGTERM handling in the serve command)
- [ ] Server compiles and starts without any other Layer 6 epics — handlers can be registered incrementally

---

## Architecture References

- [[07-web-interface-and-streaming]] — §Architecture: "Go HTTP Server → embed.FS (serves compiled frontend assets) → API handlers → WebSocket handler"
- [[02-tech-stack-decisions]] — §Frontend: "embed.FS in Go means the compiled frontend ships inside the binary. No separate frontend server in production."
- [[01-project-vision-and-principles]] — §Design Principles: "Single binary. The Go binary embeds the frontend."
- [[01-project-vision-and-principles]] — §What sirtopham Is Not: "Not a multi-user system. Single developer, single machine. No auth, no tenancy."

---

## Notes for the Implementing Agent

The `embed.FS` setup needs care. In production, `//go:embed web/dist/*` pulls the compiled frontend into the binary. In development, the `web/dist/` directory may not exist — the server should handle this gracefully (serve API only, log a warning). The Vite dev server (started separately during development) proxies `/api/*` to the Go backend.

The server struct should accept handler registrations so that Epics 02, 03, and 04 can mount their routes without modifying this package. A pattern like `server.HandleFunc("GET /api/conversations", handler)` or `server.Mount("/api", apiRouter)` keeps the dependency direction clean.

CORS middleware in dev mode must allow WebSocket upgrades from the Vite dev server origin (`http://localhost:5173` or whatever port Vite uses).
