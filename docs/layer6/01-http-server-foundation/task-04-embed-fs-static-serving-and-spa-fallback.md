# Task 04: embed.FS Static File Serving and SPA Fallback

**Epic:** 01 — HTTP Server Foundation
**Status:** ⬚ Not started
**Dependencies:** Task 02

---

## Description

Configure `embed.FS` to serve the compiled React frontend from `web/dist/` in production mode, and implement SPA fallback routing so that client-side routes (e.g., `/c/:id`) serve `index.html` instead of returning 404. In development mode, when `web/dist/` does not exist, the server should handle this gracefully by serving only API routes and logging a warning that the Vite dev server should be used for frontend assets.

## Acceptance Criteria

- [ ] `//go:embed` directive configured to embed `web/dist/` contents into the Go binary (e.g., `//go:embed all:web/dist` or `//go:embed web/dist/*`)
- [ ] In production mode, non-API requests that match a file in `web/dist/` serve that file with the correct `Content-Type` (JS, CSS, HTML, images, fonts)
- [ ] Static files are served with appropriate cache headers: hashed asset filenames (Vite default) get long-lived `Cache-Control`, `index.html` gets `no-cache` (ensures the browser always fetches the latest entry point)
- [ ] **SPA fallback:** Non-API requests that do NOT match a static file serve `web/dist/index.html` — this enables React Router's client-side routing (e.g., `/c/some-uuid` loads the SPA which handles the route)
- [ ] SPA fallback does NOT apply to `/api/*` routes — API 404s remain API 404s
- [ ] In dev mode, if `web/dist/` does not exist (or is empty), the server logs a warning at startup: "Frontend assets not found. Start the Vite dev server for frontend development." and serves only API routes
- [ ] Dev mode flag from config controls whether `embed.FS` serving is active
- [ ] The `embed.FS` file server strips the `web/dist/` prefix so files are served at the root path (e.g., `web/dist/index.html` is served at `/index.html`, `web/dist/assets/main.js` at `/assets/main.js`)
