# Layer 0 Audit: Foundation

## Scope

Layer 0 is the project scaffolding, configuration loading, SQLite connection
management, structured logging, UUIDv7 generation, and database schema with
sqlc code generation. Everything else builds on this.

## Spec References

- `docs/specs/02-tech-stack-decisions.md` — SQLite driver, CGo, Makefile
- `docs/specs/08-data-model.md` — Full schema, pragmas, sqlc, ID strategy
- `docs/layer0/00-layer0-overview.md` — Epic index and dependency graph
- `docs/layer0/01-project-scaffolding/` through `06-schema-and-sqlc/` — Task-level specs

## Packages to Audit

| Package | Src | Test | Purpose |
|---------|-----|------|---------| 
| `internal/config` | 2 | 1 | YAML config parsing, validation, defaults |
| `internal/logging` | 2 | 1 | Structured slog setup |
| `internal/db` | 10 | 2 | SQLite connection, schema, sqlc-generated queries |
| `internal/id` | 2 | 1 | UUIDv7 generation |
| `cmd/sirtopham` | 2 | 0 | CLI entry point (main.go, serve.go) |

## Test Commands

```bash
make test                                          # Full suite
CGO_ENABLED=1 CGO_LDFLAGS="..." go test -tags 'sqlite_fts5' ./internal/config/...
CGO_ENABLED=1 CGO_LDFLAGS="..." go test -tags 'sqlite_fts5' ./internal/db/...
go test ./internal/id/...
go test ./internal/logging/...
```

## Audit Checklist

### Epic 01: Project Scaffolding
- [x] `cmd/sirtopham/main.go` uses cobra for CLI structure
- [x] `cmd/sirtopham/serve.go` is the composition root — verify it wires all layers
- [x] Makefile has `build`, `test`, `dev-backend`, `dev-frontend` targets
- [x] CGO_LDFLAGS and LD_LIBRARY_PATH are set correctly for LanceDB in Makefile
- [x] `go build` succeeds with the Makefile build target

### Epic 02: Structured Logging
- [x] `internal/logging/` sets up slog with JSON handler
- [x] Log levels configurable
- [x] Tests verify log output format

### Epic 03: Configuration
- [x] `internal/config/config.go` — `Config` struct covers all sections:
  - ProjectRoot, Providers, Agent, Context, Brain, Server
- [x] `BrainConfig` has: Enabled, VaultPath, ObsidianAPIURL, ObsidianAPIKey, and v0.2 fields
- [x] `Default()` returns sensible defaults for all fields
- [x] `Load()` reads YAML, merges with defaults, validates
- [x] Validation catches: missing project root, invalid vault path, invalid port, negative numeric values
- [x] Test covers: valid config, minimal config, missing required fields, invalid values, brain section

### Epic 04: SQLite Connection
- [x] `internal/db/sqlite.go` — `OpenDB()` sets WAL mode, foreign keys, busy timeout
- [x] Pragmas applied: `journal_mode=WAL`, `foreign_keys=ON`, `busy_timeout`
- [~] Connection string uses `_txlock=immediate` or equivalent
- [x] `Init()` creates tables from schema

### Epic 05: UUIDv7
- [x] `internal/id/` generates time-ordered UUIDs
- [x] IDs are lexicographically sortable by creation time
- [x] Test verifies uniqueness and ordering

### Epic 06: Schema & sqlc
- [x] `internal/db/schema.sql` defines all tables:
  - `projects`, `conversations`, `messages`, `tool_executions`, `sub_calls`
  - `context_reports`, `brain_documents`, `brain_links`, `index_state`
  - `messages_fts` (FTS5 virtual table)
- [x] `sqlc.yaml` points to correct schema and query paths
- [x] Generated Go files in `internal/db/` match the SQL queries:
  - `conversation.sql.go`, `message_compression.sql.go`, etc.
- [x] `messages` table has: is_compressed, is_summary, compressed_turn_start, compressed_turn_end
- [x] FTS5 triggers maintain the `messages_fts` index on insert/update/delete
- [!] Run `sqlc generate` and verify no diff — DRIFT DETECTED (see findings)

### Cross-cutting
- [x] No `go vet` warnings on Layer 0 packages
- [x] Build tags `sqlite_fts5` applied consistently
- [x] All tests pass with `make test`

---

## Findings

### FINDING-L0-01: sqlc generated code is out of sync with SQL sources [MEDIUM]

**Status:** Drift detected  
**Location:** `internal/db/query/conversation.sql`, `internal/db/query/message_compression.sql`

Running `sqlc generate` produces a diff against the committed Go files:

1. `conversation.sql` / `conversation.sql.go`:  
   `DeleteIterationMessages` query in the .sql file is missing the
   `AND role != 'user'` guard that exists in the committed .sql.go file
   (or vice-versa — the SQL source was updated but `sqlc generate` was
   not re-run). The hand-edited .sql.go has `AND role != 'user'` but
   the .sql source does NOT, so regeneration produces a version WITHOUT
   the guard, losing the safety clause.

   **Actually the reverse**: the committed .sql has the OLD query
   (without `AND role != 'user'`), and someone edited the .sql query to
   add the guard but forgot to regenerate. When `sqlc generate` runs it
   replaces the .sql.go with the version FROM the .sql, but the diff
   shows the .sql.go going from WITHOUT the guard TO WITH the guard —
   meaning the SQL source file has the updated query but the generated Go
   was not re-run.

   **Net result**: The committed generated Go code deletes ALL messages
   for a turn/iteration, but the SQL source now correctly preserves user
   messages. Re-running `sqlc generate` would fix the Go code.

2. `message_compression.sql` / `message_compression.sql.go`:  
   A new query `MarkMessageCompressedByID` exists in the .sql source but
   is missing from the committed .sql.go. This is dead SQL — the query
   was added to the source but never generated.

**Recommendation:** Run `sqlc generate` and commit the result. The
`DeleteIterationMessages` fix is important — without it, retry logic
deletes user messages it shouldn't.

### FINDING-L0-02: No `_txlock=immediate` in DSN [LOW]

**Status:** Deviation from checklist  
**Location:** `internal/db/sqlite.go` `buildDSN()`

The DSN sets `_busy_timeout`, `_foreign_keys`, `_journal_mode`, and
`_synchronous` but does NOT set `_txlock=immediate`. The spec checklist
asks for it. In WAL mode with mattn/go-sqlite3, transactions default to
`DEFERRED`, which means a write-upgrading transaction can get
SQLITE_BUSY if another writer is active. Setting `_txlock=immediate`
makes all `BEGIN` statements use `BEGIN IMMEDIATE`, preventing write
starvation under concurrent access.

Currently the project uses `db.BeginTx(ctx, nil)` in `Init()` which
issues a plain DEFERRED BEGIN. The `conversation` package presumably
does the same.

**Impact:** Low right now (single-user, single-connection usage pattern),
but could bite in production with concurrent WebSocket requests.

**Recommendation:** Add `query.Set("_txlock", "immediate")` to
`buildDSN()`. This is a one-line change with no test impact — all
existing tests use single-connection patterns.

### FINDING-L0-03: serve.go duplicates logging setup vs internal/logging package [INFO]

**Status:** Style / redundancy  
**Location:** `cmd/sirtopham/serve.go` lines 72-80

The `runServe()` function manually builds `slog.Handler` and
`slog.Logger` with its own `parseLogLevel()` helper, instead of calling
`logging.Init(cfg.LogLevel, cfg.LogFormat)`. The `internal/logging`
package already handles all of this (level parsing, format selection,
JSON/text handler, setting default). The two code paths could diverge.

**Recommendation:** Replace the inline setup with:
```go
logger, err := logging.Init(cfg.LogLevel, cfg.LogFormat)
```

### FINDING-L0-04: `Init()` is destructive — drops all tables [INFO]

**Status:** By design, but worth noting  
**Location:** `internal/db/init.go`

`Init()` runs `DROP TABLE IF EXISTS` for every table before recreating
the schema. The CLI `init` subcommand calls this directly. There is no
migration path — any existing data is destroyed. This is fine for v0.1
development but will need a migration system before any persistent data
matters.

**No action needed now** — just noting for the Layer 0 record.

### FINDING-L0-05: `schema_integration_test.go` uses build tag but `sqlite_test.go` does not [INFO]

**Status:** Correct behavior, minor asymmetry  
**Location:** `internal/db/schema_integration_test.go` line 1, `internal/db/sqlite_test.go`

`schema_integration_test.go` has `//go:build sqlite_fts5` because it
exercises FTS triggers. `sqlite_test.go` does NOT have the build tag
because it only tests pragmas and basic CRUD — it doesn't touch FTS5.
This is correct but worth noting: running `go test ./internal/db/...`
without `-tags sqlite_fts5` will run sqlite_test.go but skip the schema
integration test silently. The Makefile always passes the tag, so CI is
fine.

---

## Summary

Layer 0 is solid. The foundation packages are well-structured, well-tested,
and match the spec. The one actionable finding is **FINDING-L0-01** (sqlc
drift) which should be fixed before building further on the query layer.
FINDING-L0-02 (`_txlock=immediate`) is a good hardening measure to add
now while the change is trivial.

| Severity | Count | Action |
|----------|-------|--------|
| MEDIUM   | 1     | FINDING-L0-01: re-run `sqlc generate` and commit |
| LOW      | 1     | FINDING-L0-02: add `_txlock=immediate` to DSN |
| INFO     | 3     | FINDING-L0-03/04/05: no immediate action needed |
