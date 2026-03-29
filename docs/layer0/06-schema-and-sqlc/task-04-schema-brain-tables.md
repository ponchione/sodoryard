# Task 04: Schema — Brain Tables (brain_documents, brain_links)

**Epic:** 06 — Schema & sqlc Code Generation
**Status:** ⬚ Not started
**Dependencies:** Task 01

---

## Description

Add the `brain_documents` and `brain_links` tables to `schema.sql`. These support the Obsidian vault integration: document metadata (derived index of vault content) and wikilink graph for bidirectional traversal.

**Note:** Doc 08 and doc 09 have minor discrepancies for `brain_documents`. Doc 08 is authoritative for schema — use `INTEGER PRIMARY KEY AUTOINCREMENT` (not TEXT UUID), `source_conversation_id` (not `source_session_id`), and ISO8601 `TEXT` timestamps (not `DATETIME`).

## Column Definitions (from doc 08)

### brain_documents

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
```

### brain_links

```sql
CREATE TABLE brain_links (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id  TEXT NOT NULL REFERENCES projects(id),
    source_path TEXT NOT NULL,        -- document containing the link
    target_path TEXT NOT NULL,        -- document being linked to
    link_text   TEXT,                 -- display text of the wikilink

    UNIQUE(project_id, source_path, target_path)
);
```

## Acceptance Criteria

- [ ] `brain_documents` table created with all columns listed above: AUTOINCREMENT PK, FK to `projects(id)`, `path` TEXT NOT NULL, `title` TEXT, `content_hash` TEXT NOT NULL, `tags` TEXT (JSON array), `frontmatter` TEXT (JSON object), `token_count` INTEGER, `created_by` TEXT, `source_conversation_id` TEXT, timestamps as TEXT (ISO8601), and `UNIQUE(project_id, path)`
- [ ] `brain_links` table created with all columns listed above: AUTOINCREMENT PK, FK to `projects(id)`, `source_path` TEXT NOT NULL, `target_path` TEXT NOT NULL, `link_text` TEXT, and `UNIQUE(project_id, source_path, target_path)`
- [ ] Both tables match doc 08 definitions exactly
