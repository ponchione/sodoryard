# Task 04: Context-Enriched Child Loggers

**Epic:** 02 — Structured Logging
**Status:** ⬚ Not started
**Dependencies:** Task 01

---

## Description

Provide a function to create child loggers enriched with key-value pairs via `slog.With`. This enables request/conversation context propagation (e.g., `conversation_id`, `turn_number`) so that all log lines from a given scope carry consistent context.

## Acceptance Criteria

- [ ] Exported function creates a child logger with additional key-value pairs (e.g., `WithContext(logger, "conversation_id", "abc-123", "turn_number", 5)` or `logger.With("conversation_id", "abc-123")`)
- [ ] Child logger output includes the enriched fields on every log line
- [ ] Multiple levels of enrichment (child of child) propagate all ancestor fields — e.g., a logger enriched with `conversation_id` then further enriched with `turn_number` produces lines containing both fields
