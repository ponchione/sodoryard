# Task 01: CLI Command and Initialization Sequence

**Epic:** 05 — Serve Command
**Status:** ⬚ Not started
**Dependencies:** Layer 0 Epics 01, 03, 04; Layer 2 Epic 07; Layer 4 Epic 01; Layer 3 Epic 06; Layer 5 Epic 06; Epic 01 (HTTP Server Foundation)

---

## Description

Implement the `sirtopham serve` cobra subcommand with the full initialization sequence that wires all layers together. The command loads configuration, opens the SQLite database (WAL mode), initializes the provider router, tool registry/executor, context assembly pipeline, and agent loop, then constructs the HTTP server with all REST and WebSocket handlers mounted and starts the listener. Each initialization step depends on the prior; if any step fails, the command exits with a clear error identifying the failing component.

## Acceptance Criteria

- [ ] `sirtopham serve` command exists as a cobra subcommand (using the existing CLI framework from Layer 0 Epic 01)
- [ ] Initialization sequence executes in order: config → SQLite DB → provider router → tool registry/executor → context assembly → agent loop → HTTP server → start listener
- [ ] Config loaded from `sirtopham.yaml` (or default path) via the config package from Layer 0 Epic 03
- [ ] SQLite database opened with WAL mode and appropriate pragmas via Layer 0 Epic 04
- [ ] Provider router initialized and discovers configured provider credentials via Layer 2 Epic 07
- [ ] Tool registry and executor initialized via Layer 4 Epic 01
- [ ] Context assembly pipeline initialized via Layer 3 Epic 06
- [ ] Agent loop initialized with provider router, tool executor, and context assembler via Layer 5 Epic 06
- [ ] HTTP server constructed with all REST (Epics 02, 03) and WebSocket (Epic 04) handlers registered
- [ ] If any initialization step fails, the command logs a clear error identifying the failing component and step, then exits with status 1
- [ ] The server does NOT partially start — all components must initialize successfully before the listener starts
- [ ] The command checks for project registration in the `projects` table and prints a helpful error if missing: "Project not initialized. Run `sirtopham init` first."
- [ ] Manual dependency injection — no DI framework. Components are constructed and passed to their dependents via constructors
