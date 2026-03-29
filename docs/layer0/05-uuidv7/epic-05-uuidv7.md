# Epic 05: UUIDv7 Generation

**Phase:** Build Phase 1 — Layer 0
**Status:** ⬚ Not started
**Dependencies:** [[01-project-scaffolding]]
**Blocks:** [[06-schema-and-sqlc]] (ID generation for projects and conversations)

---

## Description

Implement or integrate a UUIDv7 generator for external-facing entity IDs (`projects` and `conversations` per doc 08). UUIDv7 is time-ordered, which means chronological listing doesn't require a separate timestamp sort. This is a small utility package but a prerequisite for the schema since the ID format is baked into table definitions.

---

## Definition of Done

- [ ] `internal/id/` (or similar) package exports a function that generates UUIDv7 strings
- [ ] Generated IDs are valid UUIDv7 (version 7 bits set correctly)
- [ ] IDs generated in sequence are lexicographically ordered (time-ordering property holds)
- [ ] Unit tests verify format, uniqueness across 10k generations, and ordering guarantee

---

## Design Notes

**From doc 08 — ID Strategy:**
- UUIDv7 (TEXT) for externally-referenced entities: `projects`, `conversations`
- INTEGER AUTOINCREMENT for high-frequency internal tables: `messages`, `sub_calls`, `tool_executions`, etc.
- UUIDv7 IDs appear in REST URLs, WebSocket connections, and the web UI's URL bar

**Implementation options:**
- Use an existing Go library (e.g., `github.com/google/uuid` v1.6+ supports UUIDv7)
- Hand-roll per RFC 9562 — it's ~30 lines of code

Either approach is fine. The interface is one function: `func New() string`.

---

## Architecture References

- [[08-data-model]] — ID strategy, UUIDv7 for projects and conversations
