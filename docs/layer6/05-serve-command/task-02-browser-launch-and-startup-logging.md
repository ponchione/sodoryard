# Task 02: Browser Launch and Startup Logging

**Epic:** 05 — Serve Command
**Status:** ⬚ Not started
**Dependencies:** Task 01

---

## Description

After the HTTP server starts successfully, open the default browser to the server URL and log structured startup information. Browser launch is controllable via a config flag (`open_browser: true/false`). Startup logging includes the server address, configured providers, project root, and whether dev mode is active. In dev mode, log a message indicating the Vite dev server should be started separately.

## Acceptance Criteria

- [ ] After the HTTP listener starts, open the default browser to `http://localhost:<port>`
- [ ] Browser launch uses `exec.Command("xdg-open", url)` on Linux, `exec.Command("open", url)` on macOS, or a cross-platform library like `browser.OpenURL`
- [ ] If the browser launch fails (e.g., no display server, headless environment), log the URL at `info` level: "Open your browser to http://localhost:3000" — do not exit or error
- [ ] Browser launch is controllable via config: `open_browser: true` (default) or `open_browser: false`
- [ ] CLI flag `--no-browser` overrides config to disable browser launch
- [ ] **Startup logging:** At `info` level, log:
  - Server address: "Server listening on http://localhost:3000"
  - Configured providers: "Providers: anthropic (claude-sonnet-4-20250514), codex (gpt-4o)" (list names and default models)
  - Project root: "Project: /home/user/myproject"
  - Dev mode: "Development mode enabled — start Vite dev server on port 5173" (if dev mode is active)
- [ ] In dev mode, the startup log clearly states which port the Go API is available on and which port the Vite dev server should use
- [ ] Startup logging uses structured `slog` fields from Layer 0 Epic 02
