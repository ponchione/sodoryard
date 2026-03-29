# Task 01: Logger Initialization Function

**Epic:** 02 — Structured Logging
**Status:** ⬚ Not started
**Dependencies:** Epic 01

---

## Description

Create the core initialization function in `internal/logging/` that sets up an `slog.Logger`. It should accept configuration for log level and output format, and set the logger as the default via `slog.SetDefault`.

## Acceptance Criteria

- [ ] `internal/logging/` exports an initialization function (e.g., `Init` or `Setup`)
- [ ] Function accepts log level and format as parameters
- [ ] Calling the function sets `slog.SetDefault` with the configured logger
- [ ] Calling the function more than once is idempotent — subsequent calls reconfigure the default logger without panic or error (safe to call from tests and re-initialization paths)
- [ ] Logger writes to `os.Stderr` (not stdout — keep stdout clean for structured program output)
