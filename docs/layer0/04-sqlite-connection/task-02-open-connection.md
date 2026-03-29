# Task 02: Connection Open Function

**Epic:** 04 — SQLite Connection Manager
**Status:** ⬚ Not started
**Dependencies:** Task 01, Epic 03

---

## Description

Implement the connection open function in `internal/db/` that accepts a database file path (sourced from config), creates parent directories if needed, and returns a `*sql.DB` handle. The file path is passed as a string parameter — the caller (cmd layer) is responsible for reading it from config (Epic 03).

## Acceptance Criteria

- [ ] Exported function with signature `OpenDB(ctx context.Context, filePath string) (*sql.DB, error)` (or similar — must accept file path as string, return `*sql.DB` and `error`)
- [ ] Parent directories are created if they don't exist (using `os.MkdirAll`)
- [ ] Function returns a descriptive error if parent directory creation fails (e.g., permission denied, invalid path)
- [ ] A new database file is created if it doesn't exist
- [ ] The returned handle is usable for queries
