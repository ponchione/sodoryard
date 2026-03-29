# Epic 04: SQLite Connection Manager

**Phase:** Build Phase 1 — Layer 0
**Status:** ⬚ Not started
**Dependencies:** [[01-project-scaffolding]], [[03-configuration]] (database file path from config)
**Blocks:** [[06-schema-and-sqlc]]

---

## Description

Finalize the SQLite driver decision (`mattn/go-sqlite3` vs `modernc.org/sqlite` — doc 02 leans toward mattn since CGo is already accepted), implement a connection manager that opens the database file, applies the required pragmas (WAL mode, `busy_timeout=5000`, `foreign_keys=ON`, `synchronous=NORMAL` per doc 08), and provides a clean `*sql.DB` handle for consumers. Handle the database file path from config.

---

## Definition of Done

- [ ] SQLite driver decision finalized and dependency pinned in `go.mod`
- [ ] `internal/db/` package exports a function that opens a SQLite database with all four pragmas applied and verified
- [ ] Pragmas are verified post-connection (query them back to confirm they took effect)
- [ ] WAL mode is confirmed active
- [ ] The connection manager accepts a file path (from config) and handles creation of the file and parent directories
- [ ] Unit tests verify pragma state on a fresh database
- [ ] Unit tests verify concurrent read/write works under WAL
- [ ] Unit tests verify clean shutdown (close without data loss)

---

## Key Decisions

**SQLite Pragmas (from doc 08):**
```sql
PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 5000;
PRAGMA foreign_keys = ON;
PRAGMA synchronous = NORMAL;
```

These are set once on connection open, not per-query.

**Driver choice (from doc 02):** Leaning `mattn/go-sqlite3` since CGo is already required for tree-sitter. `modernc.org/sqlite` (pure Go) is the alternative — no architectural impact either way. Decide during implementation.

---

## Architecture References

- [[02-tech-stack-decisions]] — SQLite driver decision, CGo acceptance
- [[08-data-model]] — Pragma requirements, WAL mode rationale, concurrent read/write needs
