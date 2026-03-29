# Task 03: Apply and Verify SQLite Pragmas

**Epic:** 04 — SQLite Connection Manager
**Status:** ⬚ Not started
**Dependencies:** Task 02

---

## Description

After opening the connection, apply all four required pragmas and verify each one by querying it back. Pragmas must be set once on connection open, not per-query.

## Pragmas

```sql
PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 5000;
PRAGMA foreign_keys = ON;
PRAGMA synchronous = NORMAL;
```

## Acceptance Criteria

- [ ] All four pragmas are applied immediately after `sql.Open()`, before returning the handle to callers
- [ ] Each pragma's value is queried back and verified to match the expected value
- [ ] WAL mode is confirmed active (`PRAGMA journal_mode` returns the string `"wal"`)
- [ ] `PRAGMA busy_timeout` returns the integer `5000`
- [ ] `PRAGMA foreign_keys` returns the integer `1`
- [ ] `PRAGMA synchronous` returns the integer `1` (NORMAL)
- [ ] If any pragma fails to apply or verify, the function returns an error and closes the connection
