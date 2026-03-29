# Task 02: Configurable Log Level

**Epic:** 02 — Structured Logging
**Status:** ⬚ Not started
**Dependencies:** Task 01

---

## Description

Support configurable log levels: debug, info, warn, error. The level should be parsed from a string (for config file / CLI flag compatibility) and applied to the handler so that messages below the threshold are suppressed.

## Acceptance Criteria

- [ ] Accepts level as a string ("debug", "info", "warn", "error")
- [ ] Invalid level strings produce a clear error
- [ ] Messages below the configured level are not emitted
