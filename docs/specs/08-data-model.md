# 08 — Data Model

**Status:** Historical SQLite draft, superseded for canonical memory **Last Updated:** 2026-05-03 **Author:** Mitchell

**Supersession note:** This draft is not current guidance for Shunter-mode projects. Shunter project memory is the canonical state plane; `.yard/yard.db` is not a Shunter input or supported compatibility surface. Use `SHUNTER_PROJECT_MEMORY_SPEC.md`, `README.md`, and the current operator specs for live storage behavior.

---

## Overview

This historical draft described the earlier SQLite persistence design for conversations, messages, tool executions, LLM sub-calls, context assembly reports, indexing state, and brain metadata. Current Shunter-mode runtime state is owned by Shunter project memory, with LanceDB and graph stores treated as derived indexes.

---

## Foundational Decisions

### Query Generation: sqlc

All database access uses [sqlc](https://sqlc.dev/) for type-safe Go code generation from SQL queries. No ORM. SQL is written directly, sqlc generates the Go structs and methods. This matches the pattern used across all of Mitchell's Go projects.

### Timestamps: ISO8601 TEXT

All timestamp columns use `TEXT` with ISO8601 strings (`2026-03-28T14:30:00Z`). SQLite has no native datetime type. TEXT with ISO8601 sorts correctly via string comparison, maps cleanly to Go's `time.Time` via sqlc, and avoids the silent-garbage landmine of SQLite's datetime functions.

### ID Strategy: Pragmatic Per-Table

IDs use the type that best fits each table's access pattern:

- **UUIDv7 or deterministic external TEXT IDs:** For externally-referenced entities — `projects`, `conversations`, chains, launches, custom launch presets, and background operations. These IDs appear in REST URLs, WebSocket connections, event streams, TUI lists, and browser URLs. UUIDv7 is preferred where no human-readable chain ID is required because it is time-ordered.
- **INTEGER AUTOINCREMENT:** For high-frequency internal tables — `messages`, `sub_calls`, `tool_executions`, `context_reports`, `brain_documents`, `brain_links`, `index_state`. These are never exposed in URLs. Autoincrement is fast, compact, and provides natural insertion ordering.

### Migration Strategy

During active development, the canonical schema still lives in a single `schema.sql` file and fresh `yard init` databases are created from that schema. The runtime also carries a small set of idempotent compatibility upgrades for live dev databases that predate recent fields or triggers:

- rebuild `messages_fts` triggers so `role='tool'` messages are indexed
- add `context_reports.token_budget_json` when missing
- ensure the chain orchestrator tables and indexes exist
- ensure launch draft and preset tables and indexes exist (`launches`, `launch_presets`)

This is not a general migration framework. It is a narrow bridge for dev-era databases whose data is useful enough to preserve. Once the schema stabilizes (v0.5+), migrate to versioned migration files — either golang-migrate or hand-written `.sql` files with a version table. The schema is simple enough that manual migrations are viable for a personal tool.

### SQLite Pragmas

The agent loop writes messages and sub_calls while operator surfaces read them concurrently. WAL mode is required for this read/write concurrency:

```sql
PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 5000;
PRAGMA foreign_keys = ON;
PRAGMA synchronous = NORMAL;  -- safe with WAL, better write performance
```

These are set once on connection open, not per-query.

---

## Message Storage Model

The single most important design decision in the schema.

### The Problem

A single turn in the agent loop produces a sequence of API messages:

```
seq 0  role=user       content="fix the auth bug"
seq 1  role=assistant  content=[
         {type: "thinking", thinking: "Let me look at..."},
         {type: "text", text: "I'll check the auth code."},
         {type: "tool_use", id: "tc1", name: "file_read", input: {path: "auth.go"}},
         {type: "tool_use", id: "tc2", name: "search_text", input: {pattern: "ValidateToken"}}
       ]
seq 2  role=tool       tool_use_id="tc1"  content="package auth\n\nfunc ValidateToken..."
seq 3  role=tool       tool_use_id="tc2"  content="auth.go:15: func ValidateToken..."
seq 4  role=assistant  content=[
         {type: "thinking", thinking: "I see the issue..."},
         {type: "text", text: "Found the bug. Fixing now."},
         {type: "tool_use", id: "tc3", name: "file_edit", input: {...}}
       ]
seq 5  role=tool       tool_use_id="tc3"  content="Applied edit. Diff: ..."
seq 6  role=assistant  content=[
         {type: "text", text: "Fixed. The issue was..."}
       ]
```

This is exactly the message array the LLM API consumes. Every iteration appends to it. The next LLM call sends this entire array (plus new content) back to the provider.

### The Decision: API-Faithful Rows

One row per message in the API conversation format:

- **`role=user`**: `content` is plain text — the user's message as a string.
- **`role=assistant`**: `content` is a JSON array of content blocks, stored verbatim from the API response. Structure: `[{"type":"thinking","thinking":"..."}, {"type":"text","text":"..."}, {"type":"tool_use","id":"tc1","name":"file_read","input":{...}}]`. No transformation on the persistence path. Passed through as `json.RawMessage` in Go.
- **`role=tool`**: `content` is plain text — the tool execution result. `tool_use_id` links back to the specific `tool_use` block in the preceding assistant message. `tool_name` is denormalized for queryability.

The reconstruction query — the most critical query in the system — is:

```sql
SELECT role, content, tool_use_id, tool_name
FROM messages
WHERE conversation_id = ? AND is_compressed = 0
ORDER BY sequence;
```

No joins. No JSON parsing on the Go side. Iterate rows, build the messages slice, send to the LLM.

### Why Not Fully Normalized

The alternative — one row per content block (every text block, thinking block, tool_use block as separate rows) — was rejected because:

- Reconstruction requires grouping and reassembling content blocks into assistant messages. This is the hottest path in the system — it runs every iteration, potentially dozens of times per turn.
- The web UI parses the JSON array in TypeScript regardless of storage model, since the streaming protocol delivers typed content blocks.
- Tool usage analytics come from the dedicated `tool_executions` table (see below), not from parsing message content.
- sqlc handles JSON columns cleanly — `json.RawMessage` passes through without serialization overhead.

---

## Compression Model

When conversation history exceeds the compression threshold (50% of context window, per [[06-context-assembly]]), older messages are compressed. The schema handles this with flags, not deletion.

### How It Works

1. Middle messages (between the preserved head and tail) get `is_compressed = 1`.
2. A synthetic summary message is inserted with `is_summary = 1`, `role = 'user'` (prefixed with `[CONTEXT COMPACTION]`), and `compressed_turn_start` / `compressed_turn_end` recording the range it covers.
3. The summary's `sequence` value is the midpoint between the last head message's sequence and the first tail message's sequence (see Sequence Numbering below).
4. Original messages are never deleted. They remain in the database for debugging and auditing, filtered out by the reconstruction query.

### Cascading Compression

If compression fires a second time in a long conversation, the existing summary message is marked `is_compressed = 1` along with the newly compressed messages. A new summary is generated covering the full compressed range (summarize-the-summary plus raw messages). There is always exactly one active summary at any point. Reconstruction stays trivial: `WHERE is_compressed = 0 ORDER BY sequence`.

### Sequence Numbering

The `sequence` column uses `REAL` (not INTEGER) to support compression without rewriting existing sequences.

Normal messages get integer values: 0.0, 1.0, 2.0, ..., assigned by a monotonically increasing counter. Summary messages get the midpoint between the last head message's sequence and the first tail message's sequence. For example, if the head ends at sequence 2.0 and the tail starts at sequence 39.0, the summary gets sequence 20.5.

Second compression bisects whatever gap exists. Floating point bisection is effectively unlimited — IEEE 754 double precision supports thousands of bisections before precision matters. No conversation will approach this.

This approach preserves the `UNIQUE(conversation_id, sequence)` constraint, keeps the reconstruction query as a single `ORDER BY sequence`, and requires no rewriting of existing sequence values. The only downside is cosmetic: raw sequence values look irregular after compression (0, 1, 2, 20.5, 39, 40, ...). Nobody except the developer looking at raw rows during debugging ever sees them.

---

## Cancellation Safety

When the user cancels a turn mid-iteration, the in-flight iteration must be cleanly discarded ([[05-agent-loop]]). The `iteration` column on messages identifies which messages belong to the current iteration. The cancellation query:

```sql
DELETE FROM messages
WHERE conversation_id = ? AND turn_number = ? AND iteration = ?;
```

This removes the incomplete assistant message and any partial tool results from the cancelled iteration. Messages from completed iterations (earlier in the same turn) are preserved. The conversation is left in a consistent state — the user can send a new message immediately.

Corresponding `tool_executions` and `sub_calls` rows for the cancelled iteration are also deleted in the same transaction.

---

## Persistence Transaction Model

At the start of a turn, the agent loop inserts the user's message as its own `role=user` row before context assembly begins. That write is intentionally outside the per-iteration assistant/tool transaction so the user's message survives mid-turn failures or cancellation.

At the end of each completed iteration, the agent loop persists the canonical conversation history and analytics on two separate paths:

Message transaction (atomic today):
1. INSERT the assistant message (with content blocks JSON)
2. INSERT tool result messages (one per tool execution)
3. COMMIT

Analytics writes (best-effort today):
4. INSERT the `sub_calls` record from the tracked provider path
5. INSERT `tool_executions` records from the tool executor path

The current guarantee is therefore narrower than the original ideal: message rows for a completed iteration are atomic with each other, but analytics rows are not yet atomic with message persistence. If the process crashes or an analytics write fails after the message transaction commits, the conversation still reconstructs correctly from `messages`, but `sub_calls` or `tool_executions` may be missing for that iteration.

This tradeoff is currently acceptable because `messages` are the user-visible source of truth, while analytics are debugging/observability data. Cancellation cleanup remains fully transactional across all three tables for an in-flight iteration.

Context reports follow a different lifecycle: INSERT at turn start (with retrieval data), UPDATE quality columns (`agent_used_search_tool`, `context_hit_rate`, `agent_read_files_json`) after the turn completes. If the turn crashes, the retrieval data is preserved for debugging — only the quality metrics are missing.

---

## Schema

### projects

Project registration. Multi-project from day one.

```sql
CREATE TABLE projects (
    id                  TEXT PRIMARY KEY,  -- current shipped runtime uses root_path as the project ID
    name                TEXT NOT NULL,
    root_path           TEXT NOT NULL UNIQUE,
    language            TEXT,              -- primary language
    last_indexed_commit TEXT,              -- git SHA of last index
    last_indexed_at     TEXT,              -- ISO8601
    created_at          TEXT NOT NULL,     -- ISO8601
    updated_at          TEXT NOT NULL      -- ISO8601
);
```

Current shipped contract: project identity is path-keyed. `projects.id` is the project root path, and `root_path` remains unique so the same directory cannot be registered as two projects.

### conversations

```sql
CREATE TABLE conversations (
    id          TEXT PRIMARY KEY,  -- UUIDv7
    project_id  TEXT NOT NULL REFERENCES projects(id),
    title       TEXT,              -- auto-generated or user-set
    model       TEXT,              -- default model for this conversation
    provider    TEXT,              -- default provider
    created_at  TEXT NOT NULL,     -- ISO8601
    updated_at  TEXT NOT NULL      -- ISO8601
);

CREATE INDEX idx_conversations_project
    ON conversations(project_id, updated_at DESC);
```

Title starts null. Auto-generated after the first assistant response (derived from the first user message, or via a lightweight LLM call). User can override via the web UI.

No denormalized counters (message count, total tokens, turn count). These are cheap queries for a single-user app with at most hundreds of conversations.

### messages

The core table. One row per API message.

```sql
CREATE TABLE messages (
    id                    INTEGER PRIMARY KEY AUTOINCREMENT,
    conversation_id       TEXT    NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    role                  TEXT    NOT NULL CHECK (role IN ('user', 'assistant', 'tool')),
    content               TEXT,            -- plain text (user, tool) or JSON content blocks (assistant)
    tool_use_id           TEXT,            -- role=tool only: which tool_use block this responds to
    tool_name             TEXT,            -- role=tool only: tool that was executed
    turn_number           INTEGER NOT NULL,
    iteration             INTEGER NOT NULL, -- iteration within the turn (1-indexed)
    sequence              REAL    NOT NULL, -- global ordering within conversation (see Sequence Numbering)
    is_compressed         INTEGER NOT NULL DEFAULT 0,
    is_summary            INTEGER NOT NULL DEFAULT 0,
    compressed_turn_start INTEGER,         -- is_summary=1 only: first turn covered
    compressed_turn_end   INTEGER,         -- is_summary=1 only: last turn covered
    created_at            TEXT    NOT NULL, -- ISO8601

    UNIQUE(conversation_id, sequence)
);

CREATE INDEX idx_messages_conversation
    ON messages(conversation_id, is_compressed, sequence);
CREATE INDEX idx_messages_turn
    ON messages(conversation_id, turn_number);
```

**Content by role:**

|Role|content column|tool_use_id|tool_name|
|---|---|---|---|
|user|Plain text (the user's message)|NULL|NULL|
|assistant|JSON array of content blocks (text, thinking, tool_use)|NULL|NULL|
|tool|Plain text (tool execution result)|ID of the tool_use block|Name of the tool|

**Reconstruction query:**

```sql
SELECT role, content, tool_use_id, tool_name
FROM messages
WHERE conversation_id = ? AND is_compressed = 0
ORDER BY sequence;
```

### tool_executions

Analytics table for tool dispatch. Every tool execution gets a row, independent of message storage.

```sql
CREATE TABLE tool_executions (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    conversation_id TEXT    NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    turn_number     INTEGER NOT NULL,
    iteration       INTEGER NOT NULL,
    tool_use_id     TEXT    NOT NULL,   -- LLM-generated tool call ID
    tool_name       TEXT    NOT NULL,
    input           TEXT,               -- JSON: tool arguments (small, queryable)
    output_size     INTEGER,            -- byte count of result (not the result itself)
    normalized_size INTEGER,            -- byte count after result normalization/truncation prep
    error           TEXT,               -- error message if failed
    success         INTEGER NOT NULL,   -- 0/1
    duration_ms     INTEGER NOT NULL,
    created_at      TEXT    NOT NULL    -- ISO8601
);

CREATE INDEX idx_tool_exec_conversation
    ON tool_executions(conversation_id, turn_number);
CREATE INDEX idx_tool_exec_name
    ON tool_executions(tool_name);
```

`output_size` and `normalized_size` are scalar observability fields instead of the full output — the full result is already in the corresponding `role=tool` message row. `input` is stored as JSON because it's small (tool arguments) and queryable — "most frequently read file paths" is an extract from `file_read` inputs.

### sub_calls

Every LLM invocation regardless of source.

```sql
CREATE TABLE sub_calls (
    id                    INTEGER PRIMARY KEY AUTOINCREMENT,
    conversation_id       TEXT REFERENCES conversations(id) ON DELETE CASCADE, -- nullable
    message_id            INTEGER REFERENCES messages(id),                     -- nullable
    turn_number           INTEGER,
    iteration             INTEGER,         -- which iteration within the turn (1-indexed)
    provider              TEXT    NOT NULL,
    model                 TEXT    NOT NULL,
    purpose               TEXT    NOT NULL, -- 'chat', 'compression', 'title_generation'
    tokens_in             INTEGER NOT NULL,
    tokens_out            INTEGER NOT NULL,
    cache_read_tokens     INTEGER NOT NULL DEFAULT 0,
    cache_creation_tokens INTEGER NOT NULL DEFAULT 0,
    latency_ms            INTEGER NOT NULL,
    success               INTEGER NOT NULL, -- 0/1
    error_message         TEXT,
    created_at            TEXT    NOT NULL  -- ISO8601
);

CREATE INDEX idx_sub_calls_conversation
    ON sub_calls(conversation_id, turn_number);
CREATE INDEX idx_sub_calls_created
    ON sub_calls(created_at);
CREATE INDEX idx_sub_calls_purpose
    ON sub_calls(purpose);
```

**Key columns:**

- `message_id` links to the assistant message this LLM call produced. Nullable because compression calls and title generation calls don't produce conversation messages. Persisted in the same transaction as the message, so the ID is always known.
- `cache_read_tokens` and `cache_creation_tokens` validate the three-breakpoint prompt caching strategy from [[05-agent-loop]]. "What percentage of prompt tokens are cache hits across a session?" is a direct query against this table.
- `purpose` distinguishes conversation turns (`'chat'`) from ancillary calls (`'compression'`, `'title_generation'`). Indexed for filtering.

### context_reports

One per turn. Structured columns for queryable metrics, JSON for detailed payloads.

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
    token_budget_json     TEXT,            -- JSON: estimated/actual token budget report
    included_count        INTEGER,         -- chunks included in final context
    excluded_count        INTEGER,         -- chunks cut for budget or threshold

    -- Quality signals (computed after turn completes)
    agent_used_search_tool INTEGER,        -- 0/1: did the agent use search_semantic?
    agent_read_files_json  TEXT,           -- JSON: files the agent read via tool calls
    context_hit_rate       REAL,           -- overlap between assembled context and agent reads

    created_at            TEXT    NOT NULL, -- ISO8601

    UNIQUE(conversation_id, turn_number)
);

CREATE INDEX idx_context_reports_quality
    ON context_reports(agent_used_search_tool);
```

**Lifecycle:** INSERT at turn start with retrieval data (needs, signals, RAG results, budget). UPDATE quality columns (`agent_used_search_tool`, `context_hit_rate`, `agent_read_files_json`) after the turn completes. If the process crashes mid-turn, the retrieval data survives for debugging.

**Design split:** Scalar quality metrics (`agent_used_search_tool`, `context_hit_rate`, `budget_used`, `included_count`) are real columns for aggregation and filtering. Detailed payloads (RAG results with individual scores, signal traces) are JSON blobs parsed by the context inspector debug panel. Cheap analytics queries without sacrificing debugging detail.

### brain_documents

Derived index of brain vault content. Rebuilt from the vault by `yard brain index`. Agent writes through `brain_write` and `brain_update` mark the derived brain index stale; they do not promise immediate semantic/graph refresh. Developer edits also require an explicit `yard brain index` before derived metadata should be assumed fresh.

```sql
CREATE TABLE brain_documents (
    id                      INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id              TEXT NOT NULL REFERENCES projects(id),
    path                    TEXT NOT NULL,      -- relative to vault root
    title                   TEXT,               -- from first heading or filename
    content_hash            TEXT NOT NULL,       -- for change detection
    tags                    TEXT,               -- JSON array
    frontmatter             TEXT,               -- JSON object
    token_count             INTEGER,
    created_by              TEXT,               -- 'agent' or 'user'
    source_conversation_id  TEXT,               -- conversation that created it (if agent)
    created_at              TEXT NOT NULL,      -- ISO8601
    updated_at              TEXT NOT NULL,      -- ISO8601

    UNIQUE(project_id, path)
);

CREATE INDEX idx_brain_docs_project ON brain_documents(project_id);
```

### brain_links

Wikilink graph for bidirectional traversal.

```sql
CREATE TABLE brain_links (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id  TEXT NOT NULL REFERENCES projects(id),
    source_path TEXT NOT NULL,        -- document containing the link
    target_path TEXT NOT NULL,        -- document being linked to
    link_text   TEXT,                 -- display text of the wikilink

    UNIQUE(project_id, source_path, target_path)
);

CREATE INDEX idx_brain_links_source ON brain_links(project_id, source_path);
CREATE INDEX idx_brain_links_target ON brain_links(project_id, target_path);
```

### Chain Orchestrator Tables

The chain orchestrator owns additional `chains`, `steps`, and `events` tables in the same `.yard/yard.db` database. Their schema changes more frequently with orchestration behavior and is specified in [[15-chain-orchestrator]], including `steps.task_context`, JSON `events.event_data` payloads for `step_output` and process lifecycle events, and event timestamp indexes.

### Launch and Operation Tables

Durable launch and operation state is shared operator-surface state, not browser-owned state. The implemented `launches` table currently stores the project-local current TUI launch draft. It preserves the operator-authored work packet and selected agent plan before TUI-started work becomes a chain, but started chains are not linked back to launch rows yet. The implemented `launch_presets` table stores durable custom role/mode shapes; built-in presets are generated in code. Background operations remain future tables. The product lifecycle is specified in [[20-operator-console-tui]], with browser inspection boundaries in [[21-web-inspector]]. CLI-started chains do not require launch records.

```sql
CREATE TABLE IF NOT EXISTS launches (
    id                  TEXT NOT NULL,
    project_id          TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    status              TEXT NOT NULL DEFAULT 'draft',
    mode                TEXT NOT NULL,
    role                TEXT,
    allowed_roles       TEXT,
    roster              TEXT,
    source_task         TEXT,
    source_specs        TEXT,
    created_at          TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at          TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY(project_id, id)
);

CREATE INDEX idx_launches_project_updated ON launches(project_id, updated_at DESC);
CREATE INDEX idx_launches_status ON launches(project_id, status);

CREATE TABLE IF NOT EXISTS launch_presets (
    id                  TEXT NOT NULL,
    project_id          TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name                TEXT NOT NULL,
    mode                TEXT NOT NULL,
    role                TEXT,
    allowed_roles       TEXT,
    roster              TEXT,
    created_at          TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at          TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY(project_id, id),
    UNIQUE(project_id, name)
);

CREATE INDEX idx_launch_presets_project_updated ON launch_presets(project_id, updated_at DESC);
```

### index_state

Per-file indexing status for the code intelligence layer's incremental updates.

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

CREATE INDEX idx_index_state_project ON index_state(project_id);
```

---

## Full-Text Search

FTS5 on message content for the conversation sidebar's search feature.

```sql
CREATE VIRTUAL TABLE messages_fts USING fts5(
    content,
    content=messages,
    content_rowid=id
);

CREATE TRIGGER messages_fts_insert AFTER INSERT ON messages
WHEN NEW.role IN ('user', 'assistant', 'tool')
BEGIN
    INSERT INTO messages_fts(rowid, content) VALUES (NEW.id, NEW.content);
END;

CREATE TRIGGER messages_fts_delete AFTER DELETE ON messages
WHEN OLD.role IN ('user', 'assistant', 'tool')
BEGIN
    INSERT INTO messages_fts(messages_fts, rowid, content)
    VALUES ('delete', OLD.id, OLD.content);
END;

CREATE TRIGGER messages_fts_update AFTER UPDATE OF content ON messages
WHEN NEW.role IN ('user', 'assistant', 'tool')
BEGIN
    INSERT INTO messages_fts(messages_fts, rowid, content)
    VALUES ('delete', OLD.id, OLD.content);
    INSERT INTO messages_fts(rowid, content) VALUES (NEW.id, NEW.content);
END;
```

For assistant messages, this indexes the raw JSON content blocks. A search for "auth middleware" matches because those words appear in the text blocks within the JSON. It also matches if "auth" appears in a tool_use input — imperfect, but good enough for conversation search. Tool result messages are indexed too so conversation search can find important terminal output or tool errors that never appear in assistant prose. Extracting only text blocks on insert adds complexity for minimal gain.

Search query pattern:

```sql
SELECT c.id, c.title, c.updated_at, snippet(messages_fts, 0, '<b>', '</b>', '...', 32)
FROM messages_fts
JOIN messages m ON m.id = messages_fts.rowid
JOIN conversations c ON c.id = m.conversation_id
WHERE messages_fts MATCH ?
ORDER BY rank
LIMIT 20;
```

---

## Key Query Patterns

### Conversation History Reconstruction (Agent Loop)

The hottest query. Runs every iteration, potentially dozens of times per turn.

```sql
SELECT role, content, tool_use_id, tool_name
FROM messages
WHERE conversation_id = ? AND is_compressed = 0
ORDER BY sequence;
```

### Conversation List (Web UI Sidebar)

```sql
SELECT id, title, updated_at
FROM conversations
WHERE project_id = ?
ORDER BY updated_at DESC
LIMIT ? OFFSET ?;
```

### Turn Messages (Web UI Message Thread)

```sql
SELECT id, role, content, tool_use_id, tool_name, turn_number, iteration, sequence
FROM messages
WHERE conversation_id = ?
ORDER BY sequence;
```

The web UI shows all messages including compressed ones (greyed out or collapsed). Unlike the reconstruction query, this doesn't filter on `is_compressed`.

### Per-Conversation Token Usage (Metrics)

```sql
SELECT
    SUM(tokens_in) as total_in,
    SUM(tokens_out) as total_out,
    SUM(cache_read_tokens) as total_cache_hits,
    COUNT(*) as total_calls,
    SUM(latency_ms) as total_latency_ms
FROM sub_calls
WHERE conversation_id = ? AND purpose = 'chat';
```

### Cache Hit Rate (Metrics)

```sql
SELECT
    SUM(cache_read_tokens) * 100.0 / NULLIF(SUM(tokens_in), 0) as cache_hit_pct
FROM sub_calls
WHERE conversation_id = ? AND purpose = 'chat';
```

Validates whether the three-breakpoint caching strategy from [[05-agent-loop]] is working.

### Tool Usage Breakdown (Metrics)

```sql
SELECT
    tool_name,
    COUNT(*) as call_count,
    AVG(duration_ms) as avg_duration,
    SUM(CASE WHEN success = 0 THEN 1 ELSE 0 END) as failure_count
FROM tool_executions
WHERE conversation_id = ?
GROUP BY tool_name;
```

### Context Assembly Quality (Tuning)

```sql
SELECT
    COUNT(*) as total_turns,
    SUM(agent_used_search_tool) as reactive_search_turns,
    AVG(context_hit_rate) as avg_hit_rate,
    AVG(budget_used) as avg_budget_used
FROM context_reports
WHERE conversation_id = ?;
```

"How often is context assembly insufficient?" and "what's the average overlap between proactive assembly and what the agent actually needed?" — the two most important signals for tuning the relevance threshold, budget cap, and query extraction rules ([[06-context-assembly]]).

---

## Large Tool Output Handling

Tool results are stored inline in the `role=tool` message content column. The agent loop already truncates tool output at a configurable limit (default 50k tokens, per [[05-agent-loop]]). A truncated build log is a few hundred KB at most. SQLite handles large TEXT columns without issue at this scale.

If this becomes a problem in practice (database file size growing unreasonably), the mitigation is straightforward: externalize large outputs to files on disk, store the file path in the content column with a marker prefix (e.g., `file:///path/to/output`). The agent loop and web UI would need to handle this prefix. Not worth the complexity now.

---

## What Ports from topham

- **SQLite WAL mode and connection configuration.** Same pragmas, same setup pattern.
- **Sub-calls tracking concept.** topham tracked every LLM invocation in a sub_calls table. The schema is refined here (cache token columns, iteration tracking, message linkage) but the concept carries forward directly.
- **Index state pattern.** Per-file hash tracking for incremental indexing. Same concept, adapted for the new schema.

---

## What's Net-New

- **API-faithful message storage model.** topham had no conversation persistence — it was a pipeline tool with single-shot phases.
- **Compression flags and REAL sequence numbering.** Enables non-destructive compression with efficient reconstruction.
- **Iteration tracking.** Supports clean cancellation and per-iteration analytics.
- **Context reports table.** The observability system for context assembly tuning — no prior art.
- **Tool executions table.** Dedicated analytics for tool dispatch, decoupled from message storage.
- **FTS5 for conversation search.** Enables the sidebar search feature.
- **Brain metadata tables.** Derived indexes for the Obsidian vault integration.
- **Cache token tracking.** Validates the prompt caching strategy.

---

## Dependencies

- [[05-agent-loop]] — Writes messages, sub_calls, and tool_executions. Reads messages for conversation history reconstruction. Manages compression lifecycle.
- [[06-context-assembly]] — Writes context_reports. Reads brain_documents and brain_links for retrieval. Reads index_state for freshness.
- [[04-code-intelligence-and-rag]] — Reads and writes index_state for incremental indexing.
- [[07-web-interface-and-streaming]] — Queries conversations, messages, sub_calls, tool_executions, and context_reports for the web inspector.
- [[09-project-brain]] — Writes brain_documents and brain_links. Reads for retrieval and graph traversal.

---

## Open Questions

- **Backup strategy.** SQLite's `.backup` command is trivial. Worth automating as a pre-conversation hook or periodic background task? Low priority but cheap to implement.
- **Database size monitoring.** At what point does the single SQLite file become unwieldy? Thousands of conversations with full tool output history could reach hundreds of MB. Worth monitoring, but unlikely to be a problem for a personal tool.
- **FTS5 for brain documents.** The brain uses Obsidian's built-in search for keyword queries. Should sodoryard also maintain its own FTS5 index on brain document content for cases where Obsidian isn't running? Probably not for v0.1 — Obsidian is assumed to be running alongside sodoryard.
- **Archived conversations.** Should there be a soft-delete or archive flag on conversations? The sidebar could get long over months of use. Low priority — can add an `archived_at` column later.

---

## References

- sqlc: https://sqlc.dev/
- SQLite WAL mode: https://www.sqlite.org/wal.html
- SQLite FTS5: https://www.sqlite.org/fts5.html
- topham sub_calls: `internal/db/` (agent-conductor repo)
- Hermes compression: `agent/context_compressor.py` (compression lifecycle that drives the flags model)
- Anthropic content blocks: https://docs.anthropic.com/en/api/messages (message format stored in assistant content)
