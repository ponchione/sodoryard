# Layer 6, Epic 05: Serve Command (Composition Root)

**Layer:** 6 — Web Interface & Streaming
**Status:** ⬚ Not started
**Dependencies:** [[layer-6-epic-01-http-server-foundation]], [[layer-6-epic-02-rest-api-conversations]], [[layer-6-epic-03-rest-api-project-config-metrics]], [[layer-6-epic-04-websocket-handler]], Layer 0 Epic 03 (config), Layer 0 Epic 04 (SQLite), Layer 2 Epic 07 (provider router), Layer 4 Epic 01 (tool registry & executor), Layer 3 Epic 06 (context assembly), Layer 5 Epic 06 (agent loop)

---

## Description

Implement the `sirtopham serve` CLI command — the composition root that instantiates every layer and wires them into a running server. This is the first time all layers are instantiated together. The command loads configuration, opens the SQLite database, initializes the provider router, tool registry/executor, context assembler, and agent loop, constructs the HTTP server with all REST and WebSocket handlers mounted, starts listening, and opens the browser (or prints the URL).

This epic also includes graceful shutdown: SIGINT/SIGTERM triggers context cancellation, the HTTP server drains active connections, in-flight turns complete or are cancelled, and the database connection is closed cleanly.

---

## Definition of Done

- [ ] `sirtopham serve` command exists (cobra, or a simple subcommand pattern — agent's choice for CLI framework)
- [ ] **Initialization sequence:**
  1. Load config from `sirtopham.yaml` (or default path) via Layer 0 Epic 03
  2. Open SQLite database via Layer 0 Epic 04 (WAL mode, pragmas)
  3. Initialize provider router via Layer 2 Epic 07 (discovers credentials, sets up configured providers)
  4. Initialize tool registry and executor via Layer 4 Epic 01
  5. Initialize context assembly pipeline via Layer 3 Epic 06
  6. Initialize agent loop via Layer 5 Epic 06 (receives provider router, tool executor, context assembler)
  7. Construct HTTP server via [[layer-6-epic-01-http-server-foundation]] with all handlers registered
  8. Start HTTP listener
- [ ] **Browser launch:** After the server starts, open the default browser to `http://localhost:<port>` (using `exec.Command("xdg-open", url)` on Linux, or `browser.OpenURL`). If opening fails, log the URL for the user to open manually. Controllable via config flag (`open_browser: true/false`)
- [ ] **Startup logging:** Log the server address, configured providers, project root, and whether dev mode is active
- [ ] **Graceful shutdown:** SIGINT/SIGTERM caught via `signal.Notify`. Triggers:
  - HTTP server `Shutdown(ctx)` with a timeout (e.g., 10 seconds)
  - Agent loop cancellation (in-flight turns are cancelled cleanly per [[05-agent-loop]] §Cancellation)
  - SQLite database close
  - Clean exit with status 0
- [ ] **Error handling:** If any initialization step fails (bad config, DB open failure, missing credentials), log a clear error message identifying the failing component and exit with status 1. Do NOT partially start the server
- [ ] The command accepts flags: `--port`, `--host`, `--dev` (overrides config values)
- [ ] In dev mode (`--dev` or `dev_mode: true` in config), log a message indicating the Vite dev server should be started separately and which port the API is available on
- [ ] A running `sirtopham serve` can be verified by: `curl http://localhost:3000/api/health` returning `{"status": "ok"}`

---

## Architecture References

- [[07-web-interface-and-streaming]] — §Overview: "`sirtopham serve` starts an HTTP server and opens the browser"
- [[01-project-vision-and-principles]] — §Design Principles: "Single binary", "Web-first interface. The CLI exists only for non-interactive operations (init, index, config, serve)"
- [[05-agent-loop]] — §Cancellation: graceful cancellation semantics when the server shuts down
- [[02-tech-stack-decisions]] — §Build System: Makefile. The serve command should be reachable via `make serve` or `./sirtopham serve`

---

## Notes for the Implementing Agent

This is a wiring epic, not a logic epic. Each component being instantiated already exists and has its own constructor/initialization. The serve command's job is to call them in the right order, pass the right dependencies, and handle the startup/shutdown lifecycle.

The dependency injection is manual — no DI framework. Construct each component, pass it to the next. The agent loop needs the provider router, tool executor, and context assembler. The HTTP server needs the agent loop, conversation manager, and sqlc queries. The serve command constructs all of these and passes them through.

For the CLI framework: if sirtopham already has a `main.go` with cobra or a simple subcommand setup from Layer 0 Epic 01, use that. If not, cobra is standard but a simple `os.Args` switch works fine for a personal tool with ~4 subcommands (serve, init, index, config).

The initialization order matters: config → DB → providers → tools → context assembly → agent loop → server. Each step depends on the prior. If any step fails, exit immediately with a clear error — don't try to continue with a degraded setup.

The project must be initialized (indexed) before serve works meaningfully. The serve command should check for the project registration in the `projects` table and print a helpful error if missing: "Project not initialized. Run `sirtopham init` first."
