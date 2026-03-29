# Task 03: JSON and Text Output Formats

**Epic:** 02 — Structured Logging
**Status:** ⬚ Not started
**Dependencies:** Task 01

---

## Description

Support two output formats: JSON (for production) and text (for development). The format selection should switch between `slog.JSONHandler` and `slog.TextHandler`.

## Acceptance Criteria

- [ ] Accepts format as a string ("json", "text")
- [ ] "json" produces structured JSON log lines
- [ ] "text" produces human-readable text output
- [ ] Invalid format strings produce a clear error
