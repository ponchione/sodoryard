# Task 07: sqlc Configuration

**Epic:** 06 — Schema & sqlc Code Generation
**Status:** ⬚ Not started
**Dependencies:** Task 01

---

## Description

Create `sqlc.yaml` configuration file pointing at the schema and query files. Configure it for SQLite and Go code generation targeting the `internal/db/` package.

## Acceptance Criteria

- [ ] `sqlc.yaml` exists with correct engine (SQLite), schema path, query paths, and Go output config
- [ ] `sqlc generate` runs without configuration errors (queries not yet needed)
- [ ] Output targets `internal/db/` or an appropriate sub-package
