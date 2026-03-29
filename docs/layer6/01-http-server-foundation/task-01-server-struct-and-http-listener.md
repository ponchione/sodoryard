# Task 01: Server Struct and HTTP Listener

**Epic:** 01 — HTTP Server Foundation
**Status:** ⬚ Not started
**Dependencies:** Layer 0 Epic 01 (project scaffolding), Layer 0 Epic 03 (config)

---

## Description

Create the `internal/server/` package with a `Server` struct that wraps `http.Server` and provides the lifecycle for starting and stopping the HTTP listener. The server binds to a configurable `host:port` (default `localhost:3000`), enforces localhost-only binding for security, and supports graceful shutdown via context cancellation. The struct should accept handler registrations from other packages so that REST and WebSocket handlers can be mounted incrementally without modifying this package.

## Acceptance Criteria

- [ ] `internal/server/` package exists with a `Server` struct
- [ ] Constructor function (e.g., `New(cfg Config, logger *slog.Logger) *Server`) accepts server configuration and a logger
- [ ] Server configuration includes `host`, `port`, and `dev_mode` fields, sourced from the application config (Layer 0 Epic 03)
- [ ] Default bind address is `localhost:3000`
- [ ] Server binds to localhost only — rejects configuration that would bind to `0.0.0.0` or an external interface, or documents the constraint clearly
- [ ] `Start(ctx context.Context) error` method starts the HTTP listener in a goroutine and blocks until the context is cancelled or the server fails
- [ ] `Shutdown(ctx context.Context) error` method calls `http.Server.Shutdown` for graceful connection draining with a configurable timeout
- [ ] Server exposes a method for registering HTTP handlers (e.g., `Handle(pattern string, handler http.Handler)` or provides access to the underlying router) so other epics can mount routes
- [ ] `go build ./internal/server/...` compiles without errors
- [ ] `go vet ./internal/server/...` reports no issues
