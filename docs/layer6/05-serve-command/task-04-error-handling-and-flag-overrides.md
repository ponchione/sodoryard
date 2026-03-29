# Task 04: Error Handling and Flag Overrides

**Epic:** 05 — Serve Command
**Status:** ⬚ Not started
**Dependencies:** Task 01

---

## Description

Implement comprehensive error handling for initialization failures and CLI flag overrides for server configuration. Each initialization step that can fail must produce a clear, actionable error message identifying both the failing component and the likely cause. CLI flags (`--port`, `--host`, `--dev`) override their corresponding config file values, giving the user control at invocation time without editing config.

## Acceptance Criteria

- [ ] **CLI flags:** `--port <int>` overrides the server port from config (default 3000)
- [ ] `--host <string>` overrides the server host from config (default `localhost`)
- [ ] `--dev` flag enables dev mode, overriding the `dev_mode` config value
- [ ] Flag values take precedence over config file values — config file values are the fallback
- [ ] **Config errors:** Missing config file logs "Config file not found at <path>. Using defaults." at `warn` level and continues with defaults
- [ ] Invalid config file (malformed YAML) exits with status 1 and error: "Failed to parse config file: <parse error details>"
- [ ] **Database errors:** SQLite open failure exits with: "Failed to open database at <path>: <error>. Check file permissions and disk space."
- [ ] **Provider errors:** If no providers are configured (no API keys found), exit with: "No providers configured. Set ANTHROPIC_API_KEY or configure providers in sirtopham.yaml."
- [ ] If a configured provider fails to initialize (bad API key format, unreachable endpoint), log a warning but continue if at least one provider is available. Exit only if ALL providers fail
- [ ] **Project not initialized:** If the `projects` table has no entry for the current directory, exit with: "Project not initialized. Run `sirtopham init` in your project directory first."
- [ ] **Port conflict:** If the configured port is already in use, exit with: "Port <port> is already in use. Use --port to specify a different port."
- [ ] All error messages are logged at `error` level via structured logging and also printed to stderr for CLI visibility
- [ ] A running server can be verified with: `curl http://localhost:<port>/api/health` returning `{"status": "ok"}`
