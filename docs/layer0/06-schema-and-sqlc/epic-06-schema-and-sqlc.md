# Epic 06: Schema & sqlc Code Generation

**Phase:** Build Phase 1 — Layer 0
**Status:** ⬚ Not started
**Dependencies:** [[04-sqlite-connection]], [[05-uuidv7]]
**Blocks:** None within Layer 0. This is the terminal epic. All Layer 1+ data access depends on this.

---

## Description

Write the complete `schema.sql` file containing all table definitions from doc 08 (`projects`, `conversations`, `messages`, `tool_executions`, `sub_calls`, `context_reports`, `brain_documents`, `brain_links`, `index_state`, plus the FTS5 virtual table and triggers). Configure sqlc with the schema and write all query SQL files for the key query patterns documented in doc 08. Run sqlc codegen to produce type-safe Go access code. Implement a `sirtopham init` path that creates the database from schema (nuke-and-rebuild strategy per doc 08).

---

## Definition of Done

- [ ] `schema.sql` contains all nine tables, the FTS5 virtual table, all indexes, and the FTS triggers — matching doc 08 exactly
- [ ] sqlc is configured (`sqlc.yaml`) and generates Go code without errors
- [ ] Generated code compiles
- [ ] Key query patterns from doc 08 are covered in `.sql` query files:
  - Conversation history reconstruction (`WHERE is_compressed = 0 ORDER BY sequence`)
  - Conversation listing (`ORDER BY updated_at DESC LIMIT ? OFFSET ?`)
  - Turn messages for web UI (all messages including compressed, `ORDER BY sequence`)
  - FTS5 conversation search (`messages_fts MATCH ? ORDER BY rank`)
  - Per-conversation token usage aggregation
  - Cache hit rate calculation
  - Tool usage breakdown by name
  - Context assembly quality aggregation
- [ ] An initialization function creates all tables from schema on a fresh database
- [ ] Re-running init on an existing database drops and recreates (nuke-and-rebuild)
- [ ] Integration tests verify:
  - Table creation succeeds
  - Insert/query round-trips for each table
  - FTS5 search returns results for indexed messages
  - Reconstruction query returns messages in correct sequence order
  - Cascading deletes work (delete conversation → messages deleted)
  - REAL sequence column sorts correctly after simulated compression (integer sequences plus a midpoint summary at e.g. 20.5)
  - UUIDv7 IDs work correctly as TEXT primary keys

---

## Tables (from doc 08)

| Table | PK Type | Purpose |
|---|---|---|
| `projects` | UUIDv7 TEXT | Project registration, multi-project support |
| `conversations` | UUIDv7 TEXT | Conversations within a project |
| `messages` | AUTOINCREMENT | API-faithful message rows (user/assistant/tool) |
| `tool_executions` | AUTOINCREMENT | Tool dispatch analytics |
| `sub_calls` | AUTOINCREMENT | Every LLM invocation with cache token tracking |
| `context_reports` | AUTOINCREMENT | Per-turn context assembly observability |
| `brain_documents` | AUTOINCREMENT | Obsidian vault document metadata |
| `brain_links` | AUTOINCREMENT | Wikilink graph for bidirectional traversal |
| `index_state` | AUTOINCREMENT | Per-file indexing status for incremental updates |
| `messages_fts` | (FTS5 virtual) | Full-text search on message content |

## Key Schema Details

**Messages — API-faithful storage:**
- `role` is `user`, `assistant`, or `tool`
- `content` is plain text (user/tool) or JSON content blocks array (assistant)
- `sequence` is REAL to support compression midpoint insertion
- `is_compressed` and `is_summary` flags for non-destructive compression

**Sub-calls — cache validation:**
- `cache_read_tokens` and `cache_creation_tokens` columns validate the three-breakpoint prompt caching strategy

**Context reports — split design:**
- Scalar quality metrics as real columns for aggregation
- Detailed payloads (RAG results, signals) as JSON blobs

**FTS5 — triggers on insert/delete:**
- Indexes `user` and `assistant` message content
- Assistant content is raw JSON — imperfect but good enough for conversation search

---

## Architecture References

- [[08-data-model]] — Complete schema definitions, query patterns, design rationale
- [[05-agent-loop]] — Message persistence model, cancellation safety, iteration tracking
- [[06-context-assembly]] — Context report structure and quality metrics
- [[09-project-brain]] — Brain document and link table definitions
