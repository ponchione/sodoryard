# Task 03: Graceful Shutdown

**Epic:** 05 — Serve Command
**Status:** ⬚ Not started
**Dependencies:** Task 01

---

## Description

Implement graceful shutdown for the serve command. SIGINT (Ctrl+C) and SIGTERM trigger an ordered shutdown: the HTTP server stops accepting new connections and drains active requests, in-flight agent loop turns are cancelled cleanly, and the SQLite database connection is closed. The shutdown has a timeout to prevent hanging indefinitely. Exit code is 0 for a clean shutdown.

## Acceptance Criteria

- [ ] SIGINT and SIGTERM are caught via `signal.Notify` on a buffered channel
- [ ] On signal receipt, log "Shutting down..." at `info` level
- [ ] **HTTP server shutdown:** Call `http.Server.Shutdown(ctx)` with a configurable timeout (default 10 seconds) to drain active connections
- [ ] Active WebSocket connections receive a close frame before the server shuts down (if the WebSocket library supports graceful close)
- [ ] **Agent loop cancellation:** Cancel the context passed to any in-flight `RunTurn` calls. The agent loop's cancellation semantics (from Layer 5 Epic 06) ensure partially completed work is persisted
- [ ] **Database close:** Close the SQLite database connection after the HTTP server has stopped and all in-flight operations have completed
- [ ] Shutdown sequence is ordered: HTTP server stop → agent loop cancellation → database close
- [ ] If the shutdown timeout expires before connections drain, force-close remaining connections and log a warning
- [ ] A second SIGINT/SIGTERM during shutdown triggers an immediate forced exit (log "Forced shutdown" and `os.Exit(1)`)
- [ ] Clean shutdown exits with status 0
- [ ] Log "Server stopped" at `info` level after successful shutdown
