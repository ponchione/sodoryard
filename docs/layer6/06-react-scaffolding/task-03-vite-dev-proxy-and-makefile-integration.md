# Task 03: Vite Dev Server Proxy and Makefile Integration

**Epic:** 06 — React Scaffolding
**Status:** ⬚ Not started
**Dependencies:** Task 01, Epic 01 (HTTP Server Foundation — embed.FS)

---

## Description

Configure the Vite dev server to proxy API requests to the Go backend and integrate the frontend build into the project's Makefile. The proxy ensures that during development, `/api/*` requests and WebSocket connections at `/api/ws` are forwarded from the Vite dev server (port 5173) to the Go backend (port 3000). The Makefile integration ensures `make build` compiles the frontend first, then builds the Go binary with embedded assets.

## Acceptance Criteria

- [ ] `vite.config.ts` proxy configuration routes `/api` requests to `http://localhost:3000`
- [ ] WebSocket proxy configured for `/api/ws` with `ws: true` — Vite correctly passes through the WebSocket upgrade handshake
- [ ] Proxy order matters: `/api/ws` rule appears before `/api` to prevent the general rule from intercepting WebSocket upgrades
- [ ] In development: Vite serves frontend assets directly, API calls proxy to the Go backend — verified by starting both servers and making a REST call from the browser
- [ ] **Makefile `build` target:** Runs `cd web && npm run build` to produce `web/dist/`, then runs `go build` which embeds `web/dist/` via `embed.FS`. If the frontend build fails, the entire `make build` fails
- [ ] **Makefile `dev` target:** Either starts both the Go backend and Vite dev server (e.g., using `concurrently` or background processes), or documents the two-terminal workflow clearly
- [ ] The `//go:embed` directive in the Go server correctly references `web/dist/` — verified by running `make build && ./sirtopham serve` and confirming the React app is served at `http://localhost:3000`
- [ ] `make clean` removes `web/dist/` and the Go binary
- [ ] Frontend build artifacts (`web/dist/`) are not committed to git — `.gitignore` updated if not already done in Task 01
