# Build Phase 1: Foundation (Layer 0) — Epic Index

**Phase:** Build Phase 1
**Layer:** Layer 0 — Config, SQLite, Structured Logging
**Scope:** Testable foundation: config parsing, database operations, project scaffolding

---

## Epics

| #   | Epic                        | Status | Dependencies          |
| --- | --------------------------- | ------ | --------------------- |
| 01  | [[01-project-scaffolding]]  | ⬚      | None                  |
| 02  | [[02-structured-logging]]   | ⬚      | Epic 01               |
| 03  | [[03-configuration]]        | ⬚      | Epic 01               |
| 04  | [[04-sqlite-connection]]    | ⬚      | Epic 01, Epic 03      |
| 05  | [[05-uuidv7]]              | ⬚      | Epic 01               |
| 06  | [[06-schema-and-sqlc]]      | ⬚      | Epic 04, Epic 05      |

## Status Legend

- ⬚ Not started
- 🔨 In progress
- ✅ Complete

---

## Dependency Graph

```
Epic 01: Project Scaffolding
  ├──→ Epic 02: Structured Logging        ─┐
  ├──→ Epic 03: Configuration Loading     ─┤──→ Epic 04: SQLite Connection ──→ Epic 06: Schema & sqlc
  └──→ Epic 05: UUIDv7 Generation         ─────────────────────────────────┘
```

## Parallelism

- **After Epic 01:** Epics 02, 03, and 05 are independent and can execute in parallel.
- **After Epic 03:** Epic 04 unblocks.
- **After Epics 04 + 05:** Epic 06 unblocks — this is the terminal epic for Layer 0.
- **Epic 02** has no downstream blockers within Layer 0, but every epic in Layer 1+ will import it.

---

## Architecture Document References

- [[02-tech-stack-decisions]] — SQLite driver, Makefile, CGo
- [[08-data-model]] — Full schema, pragmas, sqlc, ID strategy, migration strategy
- [[03-provider-architecture]], [[04-code-intelligence-and-rag]], [[05-agent-loop]], [[06-context-assembly]], [[09-project-brain]] — Config sections aggregated into `sirtopham.yaml`
