# Epic 02: Structured Logging

**Phase:** Build Phase 1 — Layer 0
**Status:** ⬚ Not started
**Dependencies:** [[01-project-scaffolding]]
**Blocks:** No Layer 0 epics. All Layer 1+ epics will import this package.

---

## Description

Set up a structured logging package wrapping Go's `log/slog`. Define log levels, establish a consistent structured output format (JSON for production, text for development), and provide a package-level API that all other packages import. Include request/conversation context propagation via `slog.With`.

---

## Definition of Done

- [ ] `internal/logging/` package exports initialization and context-enriched logger creation
- [ ] Configurable log level (debug/info/warn/error) and format (json/text)
- [ ] Unit tests verify log output structure and level filtering
- [ ] A logger can be enriched with key-value pairs (`conversation_id`, `turn_number`) and child loggers propagate them

---

## Architecture References

- [[02-tech-stack-decisions]] — Go standard library preference
- [[05-agent-loop]] — Logging requirements for sub-calls, tool dispatch, and turn lifecycle
