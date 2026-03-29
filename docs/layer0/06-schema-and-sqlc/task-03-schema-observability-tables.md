# Task 03: Schema — Observability and Index Tables (context_reports, index_state)

**Epic:** 06 — Schema & sqlc Code Generation
**Status:** ⬚ Not started
**Dependencies:** Task 01

---

## Description

Add the `context_reports` and `index_state` tables to `schema.sql`. `context_reports` uses a split design: scalar quality metrics as real columns for aggregation and filtering, detailed payloads as JSON blobs for the context inspector debug panel. `index_state` tracks per-file indexing status for the code intelligence layer's incremental updates.

## Column Definitions (from doc 08)

### context_reports

```sql
CREATE TABLE context_reports (
    id                    INTEGER PRIMARY KEY AUTOINCREMENT,
    conversation_id       TEXT    NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    turn_number           INTEGER NOT NULL,

    -- Latency
    analysis_latency_ms   INTEGER,
    retrieval_latency_ms  INTEGER,
    total_latency_ms      INTEGER,

    -- Turn analyzer output (detailed, JSON)
    needs_json            TEXT,            -- JSON: ContextNeeds struct
    signals_json          TEXT,            -- JSON: Signal array from turn analyzer

    -- Retrieval results (detailed payloads, JSON)
    rag_results_json      TEXT,            -- JSON: pre-filtering RAG hits with scores
    brain_results_json    TEXT,            -- JSON: brain hits with scores
    graph_results_json    TEXT,            -- JSON: structural graph results
    explicit_files_json   TEXT,            -- JSON: files directly fetched

    -- Budget accounting (scalar + JSON)
    budget_total          INTEGER,         -- tokens available for context
    budget_used           INTEGER,         -- tokens consumed
    budget_breakdown_json TEXT,            -- JSON: {"rag": 15000, "brain": 5000, ...}
    included_count        INTEGER,         -- chunks included in final context
    excluded_count        INTEGER,         -- chunks cut for budget or threshold

    -- Quality signals (computed after turn completes)
    agent_used_search_tool INTEGER,        -- 0/1: did the agent use search_semantic?
    agent_read_files_json  TEXT,           -- JSON: files the agent read via tool calls
    context_hit_rate       REAL,           -- overlap between assembled context and agent reads

    created_at            TEXT    NOT NULL, -- ISO8601

    UNIQUE(conversation_id, turn_number)
);
```

**Lifecycle:** INSERT at turn start with retrieval data (needs, signals, RAG results, budget). UPDATE quality columns (`agent_used_search_tool`, `context_hit_rate`, `agent_read_files_json`) after the turn completes.

### index_state

```sql
CREATE TABLE index_state (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id      TEXT NOT NULL REFERENCES projects(id),
    file_path       TEXT NOT NULL,      -- relative to project root
    file_hash       TEXT NOT NULL,      -- content hash for change detection
    chunk_count     INTEGER NOT NULL,   -- chunks produced from this file
    last_indexed_at TEXT NOT NULL,      -- ISO8601

    UNIQUE(project_id, file_path)
);
```

## Acceptance Criteria

- [ ] `context_reports` table created with all columns listed above: AUTOINCREMENT PK, FK to `conversations(id) ON DELETE CASCADE`, 3 latency columns (INTEGER), 2 turn-analyzer JSON columns, 4 retrieval-result JSON columns, 3 budget-accounting scalar columns + 1 JSON breakdown, 3 quality-signal columns (`agent_used_search_tool` INTEGER, `agent_read_files_json` TEXT, `context_hit_rate` REAL), `created_at` TEXT, and `UNIQUE(conversation_id, turn_number)`
- [ ] `index_state` table created with all columns listed above: AUTOINCREMENT PK, FK to `projects(id)`, `file_path` TEXT, `file_hash` TEXT, `chunk_count` INTEGER, `last_indexed_at` TEXT, and `UNIQUE(project_id, file_path)`
- [ ] Both tables match doc 08 definitions exactly
