# Phase 2 — Layer 1: Code Intelligence & RAG

**Layer:** 1 — Code Intelligence
**Architecture Doc:** [[04-code-intelligence-and-rag]]
**Depends On:** Phase 1 — Layer 0 (Foundation)
**Last Updated:** 2026-03-28

---

## Summary

Layer 1 implements sirtopham's core differentiator: semantic understanding of the codebase via tree-sitter parsing, Go AST analysis, LLM-generated descriptions, vector embeddings, and structural graph analysis. It provides the retrieval interfaces that [[06-context-assembly]] (Layer 3) calls into and the backend for the `search_semantic` tool defined in [[05-agent-loop]] (Layer 5).

Everything in this layer runs locally — tree-sitter CGo bindings, Go AST analysis, Docker containers for embeddings and descriptions, LanceDB for vector storage, SQLite for the structural graph. No external API calls.

---

## Epic Index

| #   | Epic                        | Tasks | Status | Dependencies                          |
| --- | --------------------------- | ----- | ------ | ------------------------------------- |
| 01  | [[01-types-and-interfaces/epic-01-types-and-interfaces]] | 13 | ⬜      | L0-E01                                |
| 02  | [[02-tree-sitter-parsers/epic-02-tree-sitter-parsers]]   | 13 | ⬜      | L1-E01                                |
| 03  | [[03-go-ast-parser/epic-03-go-ast-parser]]               | 11 | ⬜      | L1-E01                                |
| 04  | [[04-embedding-client/epic-04-embedding-client]]         | 6  | ⬜      | L1-E01, L0-E03                        |
| 05  | [[05-lancedb-store/epic-05-lancedb-store]]               | 10 | ⬜      | L1-E01, L0-E03                        |
| 06  | [[06-description-generator/epic-06-description-generator]] | 6 | ⬜    | L1-E01, L0-E03                        |
| 07  | [[07-indexing-pipeline/epic-07-indexing-pipeline]]        | 14 | ⬜      | L1-E02, L1-E03, L1-E04, L1-E05, L1-E06, L0-E03, L0-E04, L0-E06 |
| 08  | [[08-searcher/epic-08-searcher]]                         | 7  | ⬜      | L1-E04, L1-E05                        |
| 09  | [[09-structural-graph/epic-09-structural-graph]]         | 11 | ⬜      | L1-E03, L0-E04, L0-E06               |

---

## Dependency Graph

```
Layer 0 (complete):
  L0-E01 (scaffolding)
  L0-E02 (logging)
  L0-E03 (config)
  L0-E04 (sqlite)
  L0-E05 (uuidv7)
  L0-E06 (schema/sqlc)

Layer 1:

  L1-E01  Types & Interfaces
    │
    ├──→ L1-E02  Tree-sitter Parsers ─────────────────┐
    │                                                  │
    ├──→ L1-E03  Go AST Parser ──────────────────┐     │
    │     │                                      │     │
    │     └──→ L1-E09  Structural Graph          │     │
    │          (+ L0-E04, L0-E06)                │     │
    │                                            │     │
    ├──→ L1-E04  Embedding Client ──────────┐    │     │
    │     │  (+ L0-E03)                     │    │     │
    │     │                                 │    │     │
    │     ├──→ L1-E08  Searcher             │    │     │
    │     │    (+ L1-E05)                   │    │     │
    │     │                                 │    │     │
    ├──→ L1-E05  LanceDB Store ─────────────┤    │     │
    │     (+ L0-E03)                        │    │     │
    │                                       ↓    ↓     ↓
    └──→ L1-E06  Description Generator ──→ L1-E07  Indexing Pipeline
          (+ L0-E03)                       (+ L0-E03, L0-E04, L0-E06)
```

### Parallelism Opportunities

**Wave 1 (no Layer 1 dependencies):**
- L1-E01 — Types & Interfaces

**Wave 2 (depends only on L1-E01):**
- L1-E02 — Tree-sitter Parsers
- L1-E03 — Go AST Parser
- L1-E04 — Embedding Client
- L1-E05 — LanceDB Store
- L1-E06 — Description Generator

All five can run in parallel.

**Wave 3 (depends on Wave 2 subsets):**
- L1-E08 — Searcher (needs E04 + E05)
- L1-E09 — Structural Graph (needs E03)

These two can run in parallel with each other.

**Wave 4 (depends on all of Wave 2):**
- L1-E07 — Indexing Pipeline (needs E02 + E03 + E04 + E05 + E06)

---

## Layer 0 Dependencies

| Layer 0 Epic | Used By | What It Provides |
|---|---|---|
| L0-E01 (scaffolding) | L1-E01 | Go module, package layout, Makefile |
| L0-E02 (logging) | All L1 epics | Structured logging via `internal/logging/` |
| L0-E03 (config) | L1-E04, L1-E05, L1-E06, L1-E07 | `index` config section (include/exclude globs, max_file_size_bytes, embedding container URL, LLM container URL) |
| L0-E04 (sqlite) | L1-E07, L1-E09 | SQLite connection manager with WAL mode |
| L0-E06 (schema/sqlc) | L1-E07, L1-E09 | `index_state` table, sqlc-generated Go code |

---

## Interface Boundaries (Downstream Consumers)

Layer 1 provides interfaces consumed by later layers. These interfaces are defined in L1-E01 and implemented across subsequent epics:

| Interface | Implemented In | Consumed By |
|---|---|---|
| `Searcher` | L1-E08 | [[06-context-assembly]] (Layer 3), `search_semantic` tool (Layer 5) |
| `Store` (vector) | L1-E05 | L1-E07 (Indexer), L1-E08 (Searcher) |
| `Embedder` | L1-E04 | L1-E07 (Indexer), L1-E08 (Searcher) |
| `Parser` | L1-E02, L1-E03 | L1-E07 (Indexer) |
| `Describer` | L1-E06 | L1-E07 (Indexer) |
| `GraphStore` | L1-E09 | [[06-context-assembly]] (Layer 3) — blast radius queries |

---

## What Ports from topham

Almost all code in `internal/rag/` and `internal/graph/` from the agent-conductor repo ports directly. The primary translation effort is Go-to-Go refactoring (same language, different project structure) rather than cross-language reimplementation.

**Direct ports:** types, tree-sitter parser dispatcher, Go AST parser, embedder HTTP client, LanceDB store, describer, indexer pipeline, searcher, structural graph analyzers.

**Adaptation required:** Work-order-aware search → conversational query interface. File-hash change detection → git-diff incremental indexing. Pipeline-phase context → agent-tool context.

---

## References

- [[04-code-intelligence-and-rag]] — Primary architecture document for this layer
- [[02-tech-stack-decisions]] — tree-sitter CGo, nomic-embed-code, LanceDB, embedding dimensions
- [[05-agent-loop]] — `search_semantic` tool definition (Layer 1 provides the backend)
- [[06-context-assembly]] — Consumes searcher and structural graph interfaces
- [[08-data-model]] — `index_state` table schema
- topham source: `internal/rag/`, `internal/graph/` (agent-conductor repo)
