# L1-E05 — LanceDB Store

**Layer:** 1 — Code Intelligence
**Epic:** 05
**Status:** ⬜ Not Started
**Dependencies:** L1-E01 (types & interfaces), L0-E03 (config)

---

## Description

Implement the LanceDB vector store integration via CGo bindings. This is the persistence layer for code chunk embeddings — it stores chunks with their vectors and supports cosine similarity search with optional metadata filters. The store handles a single `chunks` table with the schema defined in [[04-code-intelligence-and-rag]], plus operations for upsert (delete-by-ID then insert), vector search, metadata queries, and schema versioning.

Ports from topham's `internal/rag/store.go`. The LanceDB integration was audited and confirmed to be correct — Arrow record construction is proper, the API surface is minimal and clean.

---

## Package

`internal/rag/store/` — LanceDB vector store integration.

---

## Definition of Done

- [ ] Implements the `Store` interface from [[L1-E01-types-and-interfaces]]
- [ ] LanceDB CGo bindings compile successfully with `CGO_ENABLED=1`
- [ ] **Schema:** Single `chunks` table with columns for: identity (id, project_name), location (file_path, language, chunk_type), content (name, signature, body, description), metadata (line_start, line_end, content_hash, indexed_at), relationships (calls, called_by, types_used, implements_ifaces, imports — as JSON strings), embedding vector (float32, 3584 dimensions)
- [ ] **Upsert:** Delete existing records by ID, then insert new records. Atomic per-chunk operation. Handles the case where the chunk doesn't exist yet (pure insert)
- [ ] **Vector search:** Cosine similarity search with configurable `topK`. Supports optional metadata filters: language, chunk_type, file_path prefix
- [ ] **Metadata queries:** GetByFilePath (all chunks for a file path), GetByName (chunks matching a symbol name — used for call graph lookups)
- [ ] **DeleteByFilePath:** Remove all chunks for a given file path (used during re-indexing when a file is deleted or fully re-parsed)
- [ ] **Schema versioning:** `SchemaVersion` constant checked on open. If the stored version doesn't match, drop and recreate the table (triggers full re-index)
- [ ] **Arrow record construction:** Proper Apache Arrow record building for batch inserts. Relationship fields serialized to JSON strings before storage
- [ ] Constructor accepts `dataDir string` and `projectName string` parameters. Config wiring (reading these values from `internal/config/`) is deferred to the integration layer (L1-E07 Indexing Pipeline), which reads config and passes values to the constructor.
- [ ] **Close:** Clean shutdown of LanceDB connection
- [ ] Error handling: database open failure, write errors, search errors, schema mismatch
- [ ] Unit tests with an in-memory or temp-directory LanceDB instance: insert, upsert, search, metadata queries, delete, schema version check
- [ ] Integration test: insert 50+ chunks with real embeddings (can be random float32 vectors for testing), verify search returns correct top-K ordering by cosine similarity

---

## Architecture References

- [[04-code-intelligence-and-rag]] — "Component: Vector Store (LanceDB)" (schema, operations, schema versioning)
- [[02-tech-stack-decisions]] — "Vector Store: LanceDB (Pending Evaluation)" (CGo bindings, evaluation verdict)
- topham source: `internal/rag/store.go`

---

## Notes

- The LanceDB data directory is per-project. The path should be derived from config (e.g., `~/.sirtopham/projects/<project-name>/lancedb/`).
- LanceDB doesn't have native upsert — the delete-then-insert pattern is the standard approach. This is correct and matches topham's implementation.
- The `GetByName` method is critical for the searcher's one-hop call graph expansion ([[L1-E08-searcher]]). Given a function name from a chunk's `Calls` or `CalledBy` list, the store must be able to look up that chunk by name efficiently.
- Arrow record construction is the most error-prone part of the LanceDB integration. Column types must match exactly (especially float32 for embeddings, string for JSON relationship fields). topham's implementation handles this correctly — port carefully.
- The Makefile needs LanceDB-specific CGo linker flags. These may already be partially set up from Layer 0 if the Makefile anticipated CGo dependencies.
