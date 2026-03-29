CREATE TABLE projects (
    id                  TEXT PRIMARY KEY,
    name                TEXT NOT NULL,
    root_path           TEXT NOT NULL UNIQUE,
    language            TEXT,
    last_indexed_commit TEXT,
    last_indexed_at     TEXT,
    created_at          TEXT NOT NULL,
    updated_at          TEXT NOT NULL
);

CREATE TABLE conversations (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    title       TEXT,
    model       TEXT,
    provider    TEXT,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE TABLE messages (
    id                    INTEGER PRIMARY KEY AUTOINCREMENT,
    conversation_id       TEXT    NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    role                  TEXT    NOT NULL CHECK (role IN ('user', 'assistant', 'tool')),
    content               TEXT,
    tool_use_id           TEXT,
    tool_name             TEXT,
    turn_number           INTEGER NOT NULL,
    iteration             INTEGER NOT NULL,
    sequence              REAL    NOT NULL,
    is_compressed         INTEGER NOT NULL DEFAULT 0,
    is_summary            INTEGER NOT NULL DEFAULT 0,
    compressed_turn_start INTEGER,
    compressed_turn_end   INTEGER,
    created_at            TEXT    NOT NULL,
    UNIQUE(conversation_id, sequence)
);

CREATE TABLE tool_executions (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    conversation_id TEXT    NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    turn_number     INTEGER NOT NULL,
    iteration       INTEGER NOT NULL,
    tool_use_id     TEXT    NOT NULL,
    tool_name       TEXT    NOT NULL,
    input           TEXT,
    output_size     INTEGER,
    error           TEXT,
    success         INTEGER NOT NULL,
    duration_ms     INTEGER NOT NULL,
    created_at      TEXT    NOT NULL
);

CREATE TABLE sub_calls (
    id                    INTEGER PRIMARY KEY AUTOINCREMENT,
    conversation_id       TEXT REFERENCES conversations(id) ON DELETE CASCADE,
    message_id            INTEGER REFERENCES messages(id),
    turn_number           INTEGER,
    iteration             INTEGER,
    provider              TEXT    NOT NULL,
    model                 TEXT    NOT NULL,
    purpose               TEXT    NOT NULL,
    tokens_in             INTEGER NOT NULL,
    tokens_out            INTEGER NOT NULL,
    cache_read_tokens     INTEGER NOT NULL DEFAULT 0,
    cache_creation_tokens INTEGER NOT NULL DEFAULT 0,
    latency_ms            INTEGER NOT NULL,
    success               INTEGER NOT NULL,
    error_message         TEXT,
    created_at            TEXT    NOT NULL
);

CREATE TABLE context_reports (
    id                     INTEGER PRIMARY KEY AUTOINCREMENT,
    conversation_id        TEXT    NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    turn_number            INTEGER NOT NULL,
    analysis_latency_ms    INTEGER,
    retrieval_latency_ms   INTEGER,
    total_latency_ms       INTEGER,
    needs_json             TEXT,
    signals_json           TEXT,
    rag_results_json       TEXT,
    brain_results_json     TEXT,
    graph_results_json     TEXT,
    explicit_files_json    TEXT,
    budget_total           INTEGER,
    budget_used            INTEGER,
    budget_breakdown_json  TEXT,
    included_count         INTEGER,
    excluded_count         INTEGER,
    agent_used_search_tool INTEGER,
    agent_read_files_json  TEXT,
    context_hit_rate       REAL,
    created_at             TEXT    NOT NULL,
    UNIQUE(conversation_id, turn_number)
);

CREATE TABLE brain_documents (
    id                     INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id             TEXT NOT NULL REFERENCES projects(id),
    path                   TEXT NOT NULL,
    title                  TEXT,
    content_hash           TEXT NOT NULL,
    tags                   TEXT,
    frontmatter            TEXT,
    token_count            INTEGER,
    created_by             TEXT,
    source_conversation_id TEXT,
    created_at             TEXT NOT NULL,
    updated_at             TEXT NOT NULL,
    UNIQUE(project_id, path)
);

CREATE TABLE brain_links (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id  TEXT NOT NULL REFERENCES projects(id),
    source_path TEXT NOT NULL,
    target_path TEXT NOT NULL,
    link_text   TEXT,
    UNIQUE(project_id, source_path, target_path)
);

CREATE TABLE index_state (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id      TEXT NOT NULL REFERENCES projects(id),
    file_path       TEXT NOT NULL,
    file_hash       TEXT NOT NULL,
    chunk_count     INTEGER NOT NULL,
    last_indexed_at TEXT NOT NULL,
    UNIQUE(project_id, file_path)
);

CREATE VIRTUAL TABLE messages_fts USING fts5(
    content,
    content=messages,
    content_rowid=id
);

CREATE TRIGGER messages_fts_insert AFTER INSERT ON messages
WHEN NEW.role IN ('user', 'assistant')
BEGIN
    INSERT INTO messages_fts(rowid, content) VALUES (NEW.id, NEW.content);
END;

CREATE TRIGGER messages_fts_delete AFTER DELETE ON messages
BEGIN
    INSERT INTO messages_fts(messages_fts, rowid, content)
    VALUES ('delete', OLD.id, OLD.content);
END;

CREATE INDEX idx_conversations_project ON conversations(project_id, updated_at DESC);
CREATE INDEX idx_messages_conversation ON messages(conversation_id, is_compressed, sequence);
CREATE INDEX idx_messages_turn ON messages(conversation_id, turn_number);
CREATE INDEX idx_tool_exec_conversation ON tool_executions(conversation_id, turn_number);
CREATE INDEX idx_tool_exec_name ON tool_executions(tool_name);
CREATE INDEX idx_sub_calls_conversation ON sub_calls(conversation_id, turn_number);
CREATE INDEX idx_sub_calls_created ON sub_calls(created_at);
CREATE INDEX idx_sub_calls_purpose ON sub_calls(purpose);
CREATE INDEX idx_context_reports_quality ON context_reports(agent_used_search_tool);
CREATE INDEX idx_brain_docs_project ON brain_documents(project_id);
CREATE INDEX idx_brain_links_source ON brain_links(project_id, source_path);
CREATE INDEX idx_brain_links_target ON brain_links(project_id, target_path);
CREATE INDEX idx_index_state_project ON index_state(project_id);
