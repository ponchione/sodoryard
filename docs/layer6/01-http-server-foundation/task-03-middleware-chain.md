# Task 03: Middleware Chain (Logging, CORS, Panic Recovery)

**Epic:** 01 — HTTP Server Foundation
**Status:** ⬚ Not started
**Dependencies:** Task 02, Layer 0 Epic 02 (structured logging)

---

## Description

Implement the middleware chain applied to all HTTP requests. Three middleware components are required: request logging (structured log entry with method, path, status code, and duration using the Layer 0 logging package), panic recovery (catch panics in handlers, log the stack trace, return a 500 response), and CORS (enabled only in dev mode, permissive for the Vite dev server origin at `localhost:5173`). The CORS middleware must also allow WebSocket upgrade requests from the dev server origin.

## Acceptance Criteria

- [ ] **Request logging middleware:** Logs every HTTP request with structured fields: method, path, HTTP status code, response duration in milliseconds. Uses `slog` from Layer 0 Epic 02
- [ ] Log level is `info` for successful requests (2xx/3xx), `warn` for client errors (4xx), `error` for server errors (5xx)
- [ ] **Panic recovery middleware:** Catches any panic in a downstream handler, logs the panic value and stack trace at `error` level, and returns an HTTP 500 response with JSON body `{"error": "internal server error"}`
- [ ] Panic recovery does not crash the server — other requests continue to be served after a panic in one handler
- [ ] **CORS middleware:** Active only when `dev_mode` is true in config. Sets `Access-Control-Allow-Origin` to `http://localhost:5173` (Vite dev server). Allows methods GET, POST, PUT, DELETE, OPTIONS. Allows headers `Content-Type`, `Authorization`. Allows credentials
- [ ] CORS middleware handles preflight `OPTIONS` requests and returns 204 with appropriate headers
- [ ] CORS middleware allows WebSocket upgrade requests (does not interfere with the `Upgrade: websocket` header)
- [ ] In production mode (dev_mode=false), no CORS headers are set — the frontend is served from the same origin via `embed.FS`
- [ ] Middleware is applied in the correct order: panic recovery (outermost) → request logging → CORS → handler
