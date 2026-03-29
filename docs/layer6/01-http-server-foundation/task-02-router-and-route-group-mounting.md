# Task 02: Router and Route Group Mounting

**Epic:** 01 — HTTP Server Foundation
**Status:** ⬚ Not started
**Dependencies:** Task 01

---

## Description

Set up the router framework and define the route group structure. Choose between Go 1.22+ stdlib `http.ServeMux` (with method-based pattern matching) or a third-party router like `chi`. Configure the route groups so that REST handlers mount under `/api/*` and the WebSocket endpoint is reserved at `/api/ws`. Include a health check endpoint at `GET /api/health` that returns `{"status": "ok"}` with a 200 status code — this serves as both a smoke test and a verification point for the serve command.

## Acceptance Criteria

- [ ] Router framework chosen and integrated into the `Server` struct
- [ ] `/api/*` route group is mountable — REST handlers from Epics 02 and 03 can register under this prefix
- [ ] `/api/ws` path is reserved for the WebSocket handler (Epic 04)
- [ ] `GET /api/health` endpoint exists and returns HTTP 200 with JSON body `{"status": "ok"}` and `Content-Type: application/json`
- [ ] Route registration API is clean: other packages can call a method on `Server` to add routes without importing router internals
- [ ] Non-API routes (paths not starting with `/api/`) are handled by the static file server or SPA fallback (wired in Task 04)
- [ ] 404 responses for unmatched API routes return JSON `{"error": "not found"}` rather than the default Go plaintext 404
