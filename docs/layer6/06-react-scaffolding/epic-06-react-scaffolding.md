# Layer 6, Epic 06: React Scaffolding & Build Integration

**Layer:** 6 — Web Interface & Streaming
**Status:** ⬚ Not started
**Dependencies:** [[layer-6-epic-01-http-server-foundation]] (embed.FS serving), [[layer-6-epic-05-serve-command]] (running backend to develop against)

---

## Description

Set up the React frontend project and integrate its build into the Go binary. This includes initializing the Vite + React + TypeScript project in `web/`, installing and configuring Tailwind CSS and shadcn/ui, setting up the Vite dev server proxy to route `/api/*` and `/api/ws` to the Go backend, integrating the frontend build into the Makefile (`make build` compiles frontend then embeds it), and creating the app shell — basic layout, client-side routing, and theme configuration.

This epic produces an empty but buildable frontend that is served correctly by the Go backend in both development (Vite proxy) and production (`embed.FS`) modes. No functional UI components — just the skeleton that subsequent epics fill in.

---

## Definition of Done

- [ ] `web/` directory exists at project root with a Vite + React + TypeScript project
- [ ] `npm install` (or equivalent) installs all dependencies. `package.json` includes: react, react-dom, react-router-dom, typescript, vite, tailwindcss, @tailwindcss/vite (or postcss plugin), and shadcn/ui dependencies
- [ ] Tailwind CSS configured and working — utility classes render correctly
- [ ] shadcn/ui initialized — at minimum the base theme, `cn()` utility, and a few core components installed (Button, Card, Input, ScrollArea, Separator) to verify the setup works. More components added by subsequent epics as needed
- [ ] **Vite dev server proxy:** `vite.config.ts` proxies `/api` to `http://localhost:3000` (or whatever the Go server port is). WebSocket proxy configured for `/api/ws` — Vite passes through the upgrade correctly
- [ ] **Makefile integration:** `make build` runs `cd web && npm run build` (producing `web/dist/`), then runs `go build` which embeds `web/dist/` via `embed.FS`. If the frontend build fails, `make build` fails
- [ ] **`embed.FS` wiring:** The Go server from [[layer-6-epic-01-http-server-foundation]] serves `web/dist/` contents in production mode. Verified: `make build && ./sirtopham serve` opens the browser and renders the React app
- [ ] **App shell:** Basic layout with a sidebar placeholder (left) and main content area (right). Client-side routing set up with react-router-dom: `/` (conversation list), `/c/:id` (conversation view). Routes render placeholder components
- [ ] **Theme:** Dark theme as default (consistent with coding tool aesthetics). CSS variables for colors so shadcn/ui components theme correctly. Light theme is a nice-to-have, not required for v0.1
- [ ] **TypeScript types for WebSocket events:** Type definitions matching the server→client and client→server event types from [[05-agent-loop]] §Streaming to the Web UI. These types are the contract between frontend and backend
- [ ] **API client utility:** A thin fetch wrapper for REST calls (`api.get('/api/conversations')`, `api.post('/api/conversations', body)`) with error handling and JSON parsing. Not a full SDK — just a convenience wrapper
- [ ] `make dev` or equivalent starts both the Go backend and Vite dev server (or documents the two-terminal workflow)
- [ ] Frontend compiles with zero TypeScript errors and zero ESLint errors

---

## Architecture References

- [[02-tech-stack-decisions]] — §Frontend: "React + TypeScript + Vite", "Tailwind CSS + shadcn/ui", "embed.FS in Go means the compiled frontend ships inside the binary"
- [[02-tech-stack-decisions]] — §Build System: "Makefile for builds. `make build`, `make test`, `make dev`"
- [[07-web-interface-and-streaming]] — §Architecture: "In development, Vite's dev server proxies API calls to the Go backend"
- [[07-web-interface-and-streaming]] — §Frontend Stack: arguments for React + TypeScript ecosystem
- [[05-agent-loop]] — §Streaming to the Web UI: event type definitions that become TypeScript types

---

## Notes for the Implementing Agent

The `web/` directory should be at the project root, not inside `internal/`. The `embed.FS` directive in Go references it: `//go:embed web/dist/*` (or `all:web/dist` to include dot-files).

For shadcn/ui: use the CLI to initialize (`npx shadcn-ui@latest init`) and add components (`npx shadcn-ui@latest add button card input`). shadcn/ui copies component source into the project (typically `web/src/components/ui/`) — these are owned source files, not node_modules dependencies.

The Vite proxy config for WebSocket needs the `ws: true` option:
```typescript
server: {
  proxy: {
    '/api/ws': { target: 'http://localhost:3000', ws: true },
    '/api': { target: 'http://localhost:3000' }
  }
}
```

For client-side routing with `react-router-dom`, the Go server's SPA fallback ([[layer-6-epic-01-http-server-foundation]]) is critical — all non-API, non-static-file requests must serve `index.html` so the React router handles the path.

The TypeScript event types should be defined in a shared types file (`web/src/types/events.ts` or similar) and imported by every component that touches the WebSocket. They are the single source of truth for the frontend's understanding of the protocol.

Consider using `react-router-dom` v6+ with data routers (loader functions) for the conversation list and conversation detail routes. This gives a clean pattern for loading data before rendering.
