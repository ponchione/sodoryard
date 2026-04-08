# 09 — Project Brain

**Status:** Draft, carried into v0.2 scoping **Last Updated:** 2026-04-07 **Author:** Mitchell

Note: portions of this draft still describe the pre-MCP Obsidian Local REST design. The implemented runtime has since moved to an MCP-backed vault backend; treat REST-specific details here as historical planning context, not the current supported path.

Current phase note: v0.1 shipped the brain as reactive tools only. The first v0.2 slice is now landed: context assembly performs proactive keyword-backed brain retrieval against the MCP/vault backend, budget fitting has a real brain tier, serialization/reporting expose those hits, reserve/estimate/reconcile token-budget reporting is visible on the context report surface, and a maintained live validation package now exists. The current product/runtime decision is explicit: keyword-backed MCP/vault retrieval is the supported operator-facing path for now, while semantic/index-backed brain retrieval remains future work unless code and validation land for it. The maintained live proofs now cover three prompt families: fact canary, rationale/decision notes, and prior-debugging/history notes.

---

## Overview

The project brain is a persistent, project-scoped knowledge base backed by an Obsidian vault. It accumulates intelligence over the lifetime of working on a codebase — architectural decisions, debugging insights, conventions, session histories, relationship maps, and anything else worth persisting across sessions.

Both the developer and the agent are co-authors. The developer works directly in Obsidian — reading, editing, organizing, browsing the graph view. The agent reads from and writes to the vault via tools, and context assembly queries the brain alongside code RAG to surface relevant project knowledge on each turn.

This is sirtopham's long-term memory. Conversations are ephemeral — they live for a session, get compressed, eventually fade. The brain is where durable insights get extracted and persisted. A conversation is a working session. The brain is the institutional knowledge that sessions contribute to.

---

## Why Obsidian

Obsidian is not just a markdown renderer. It's a structured knowledge tool with primitives that a plain directory of files can't replicate.

**Wikilinks (`[[double brackets]]`).** Bidirectional linking between documents. If "auth-architecture.md" links to `[[provider-design]]`, Obsidian tracks that relationship in both directions. This is a graph — not a pile of files. When the agent writes `[[context-assembly-decisions]]` in a debugging note, that's a semantic connection that retrieval can exploit. "Find everything linked to context assembly" becomes a graph traversal, not a keyword search.

**Tags.** `#architecture`, `#debugging`, `#convention`, `#tech-debt`. Lightweight categorization that the agent applies when writing and that retrieval filters on.

**Frontmatter (YAML).** Structured metadata — created date, author, status, related topics. The agent writes this, the retrieval layer queries on it, Obsidian renders it cleanly.

**Graph view.** Obsidian's built-in graph visualization shows how documents connect — clusters of related knowledge, orphaned documents, interconnection density. We don't build this; Obsidian gives it for free.

**Canvas.** Spatial layouts of cards, notes, and connections. Architectural diagrams, flow maps, decision trees — all inside the vault. Future territory, but available.

**Plugin ecosystem.** Dataview for structured queries across documents. Templater for document templates. Git integration for version-controlled vaults. Available when needed.

**Local-first, file-based.** Obsidian vaults are directories of markdown files on disk. No proprietary format, no server dependency, no sync requirement. Architecturally aligned with sirtopham.

---

## Architecture

The brain is not a feature bolted onto sirtopham. It's a first-class component with its own storage, retrieval logic, tools, and lifecycle.

### Integration Model

Historical-design note: the diagram below still contains older REST/indexer boxes to preserve planning context, but the live runtime path today is narrower: MCP/vault-backed keyword retrieval and tool access are real; the semantic/indexer pieces shown here are future-facing unless separately landed.

Obsidian runs alongside sirtopham as the human-facing vault UI, but the implemented runtime path is now the in-process MCP-backed vault backend. sirtopham talks to the vault through `internal/brain/mcpclient` and MCP `vault_*` tools rather than the older Obsidian Local REST API design.

```
┌─────────────────────────────────┐     ┌──────────────────────────┐
│  sirtopham                      │     │  Obsidian                │
│                                 │     │                          │
│  Agent Loop                     │     │  Project Brain Vault     │
│    ├─ brain_read ──────────────────→  │    ├─ architecture/      │
│    ├─ brain_write ─────────────────→  │    ├─ debugging/         │
│    ├─ brain_update ────────────────→  │    ├─ conventions/       │
│    ├─ brain_search (keyword) ──────→  │    ├─ sessions/          │
│    │                            │     │    └─ notes/             │
│    └─ brain_search (semantic) ──┐│    │                          │
│                                 ││    │  Local REST API Plugin   │
│  Context Assembly               ││    │    (localhost:27124)     │
│    ├─ brain keyword query ─────────→  │                          │
│    └─ brain semantic query ─────┘│    │  Graph View, Canvas,     │
│                                 │     │  Plugins, Search, etc.   │
│  Brain Indexer                  │     │                          │
│    ├─ Vector embeddings         │     └──────────────────────────┘
│    │  (LanceDB, brain collection)            ↕
│    ├─ Wikilink graph (SQLite)         Developer works directly
│    └─ Frontmatter/tag index           in Obsidian alongside
│                                       sirtopham
│  Web UI                         │
│    └─ "Open in Obsidian" links  │
│                                 │
└─────────────────────────────────┘
```

### What Lives Where

**In Obsidian (source of truth):** All brain documents. Markdown files with frontmatter, wikilinks, tags. The developer reads, edits, organizes, and browses here. Obsidian's graph view visualizes the knowledge structure.

**In sirtopham today (tools + proactive retrieval):** The agent-facing interface and the current proactive retrieval source of truth. Read/write/search operations go through the MCP/vault backend. Proactive context assembly currently uses keyword search from that same backend.

**In future brain-index work (not yet the active runtime path):** Vector embeddings of brain documents in a separate LanceDB collection. A parsed wikilink graph stored in SQLite. Extracted frontmatter metadata and tags for structured queries. If this becomes real operator-facing behavior, it should be described as a derived layer under the MCP/vault source of truth rather than implied as already-landed runtime.

**In sirtopham current v0.2 runtime (context assembly):** Brain keyword retrieval runs in parallel with code RAG during context assembly. Results compete for budget alongside code chunks, are serialized into a distinct Project Brain section, and appear in context reports/inspector payloads.

---

## Vault Structure

The vault is an Obsidian vault at a configurable path. The directory structure is freeform — the agent and developer organize however makes sense. Flat, nested, by date, by topic. The retrieval layer searches content regardless of file location.

### Typical Structure

```
brain-vault/
├── .obsidian/                        # Obsidian config (sirtopham ignores this)
├── architecture/
│   ├── provider-design.md
│   ├── rag-pipeline-audit.md
│   ├── context-assembly-decisions.md
│   └── agent-loop-design.md
├── debugging/
│   ├── lancedb-cgo-gotchas.md
│   ├── tree-sitter-generics-workaround.md
│   └── oauth-token-refresh-race.md
├── conventions/
│   ├── error-handling.md
│   ├── anti-patterns.md
│   └── testing-patterns.md
├── sessions/
│   ├── 2026-03-27-tech-stack.md
│   ├── 2026-03-28-agent-loop.md
│   └── 2026-03-28-context-assembly.md
├── notes/
│   ├── token-refresh-file-locking.md
│   ├── codex-responses-api-quirks.md
│   └── nomic-embed-query-prefix.md
└── templates/
    ├── session-summary.md
    ├── debugging-journal.md
    └── decision-record.md
```

### Document Format

Brain documents are Obsidian-native markdown. The agent writes documents that work naturally in Obsidian:

```markdown
---
created: 2026-03-28
author: agent
session: abc-123
tags: [debugging, cgo, lancedb]
status: active
---

# LanceDB CGo Nil Slice Segfault

## Problem

Passing a nil Go slice to the LanceDB CGo bindings causes a segfault
in the C layer. This manifests as a SIGSEGV with no Go stack trace —
the crash is below the CGo boundary.

## Root Cause

The C layer dereferences the slice pointer without nil checking.
The Go slice header has a nil data pointer when the slice is nil,
and the C code assumes it's always valid.

## Workaround

Always pre-allocate slices before passing to LanceDB:

```go
// BAD — nil slice causes segfault
var embeddings []float32
store.Insert(embeddings)

// GOOD — empty but non-nil slice
embeddings := make([]float32, 0, expectedSize)
store.Insert(embeddings)
`` `

## Impact

Affects any code path that might pass an empty result set to
LanceDB — particularly the indexer when processing files with
no extractable symbols.

## Related

- [[tech-stack-decisions]] — why we accepted CGo as a dependency
- [[rag-pipeline-audit]] — the LanceDB evaluation that first surfaced this
- [[error-handling]] — our convention for handling CGo boundary errors
```

---

## What Goes In the Brain

The brain accumulates any project knowledge worth persisting across sessions:

**Architectural decisions and rationale.** Why we chose Go, why CGo is accepted, how the provider architecture works. The *why* behind choices — exactly what's lost when conversations end.

**Debugging journals.** Hard-won operational knowledge. "The tree-sitter Go parser doesn't handle generics well. Workaround: fall back to Go AST parser for files with type parameters." These are the war stories that save hours of rediscovery.

**Conventions not derivable from code.** "We don't use go-git because of index desync issues — always shell out to git." The convention extractor in [[04-code-intelligence-and-rag]] derives patterns from code analysis. The brain stores conventions that require judgment — anti-patterns, rationale, exceptions to rules.

**Implementation notes.** Specific technical details too granular for architecture docs but too important to lose. "The Anthropic OAuth token refresh writes back to ~/.claude/.credentials.json. Use advisory file locking to avoid races with Claude Code."

**Session summaries.** Breadcrumb trail of what's been done. "2026-03-28: Designed context assembly system. Key decisions: always-on RAG, rule-based turn analyzer, 3 cache breakpoints, 30k token budget cap."

**Relationship maps.** Architectural knowledge that's implicit in the code but takes significant reading to reconstruct. "The payment flow goes: handler → service → gateway → Stripe API. The gateway package owns all external HTTP calls."

**Known issues and tech debt.** "The description generator sometimes produces vague descriptions for small utility functions. Impact: reduced retrieval quality for helper functions. Future fix: use a better local model or add few-shot examples."

There is no size limit on the vault. No document count limit. No enforced structure. The brain grows as large as it needs to. The retrieval layer handles finding what's relevant; the developer and agent handle curation.

---

## Retrieval

Current runtime truth is intentionally narrower than earlier drafts implied.

### What is live today

Brain retrieval currently has one real operator-facing path:
- MCP/vault-backed keyword search for both `brain_search` and proactive context assembly retrieval
- post-hoc tag filtering in `brain_search`
- `_log.md` operational notes excluded from proactive context and tool search results
- analyzer/query shaping that can emit brain-oriented signals such as `brain_intent` / `brain_seeking_intent`, plus stopword-stripped fallback keyword queries for weak long-form prompts

This is the primary retrieval mode for structured knowledge with clear textual anchors.

### What is not live yet

The following ideas still belong to future-facing design, not current operator expectations:
- semantic/vector-backed brain retrieval via a separate LanceDB collection
- wikilink/backlink graph traversal as a live retrieval source
- automatic brain indexing pipelines beyond the current MCP/vault-backed keyword path

Those may still be worthwhile v0.2+ work, but the docs should not imply they are already part of the runtime contract.

---

## Indexing

Historical/future-design note: this section describes the older planned indexing model, not the current operator-facing runtime contract. Today the live brain path is MCP/vault-backed keyword retrieval; the vector/graph indexing story below remains reserved future work unless code and validation land for it.

### Obsidian Handles Keyword Search

The Obsidian REST API plugin exposes Obsidian's own search. sirtopham doesn't need to maintain a separate FTS5 index for keyword queries — Obsidian does this automatically as files change in the vault.

### sirtopham Handles Vector Search and Graph

The brain indexer runs on startup and incrementally:

**On startup:** Scan the vault directory (via REST API or filesystem). For each document: compute content hash, compare to stored hash, re-embed if changed. Parse wikilinks and rebuild the link graph. Extract frontmatter and tags.

**On agent write:** When the agent creates or updates a document via `brain_write` or `brain_update`, immediately update the vector embeddings and link graph for that document. No waiting for a reindex cycle. The document is semantically searchable on the very next turn.

**On developer edit:** Detected on the next startup scan or mid-session if the indexer runs periodic checks (configurable). Developer edits made in Obsidian are picked up and re-embedded.

### Chunking Strategy

Brain documents are split at `##` heading boundaries (same as the markdown fallback parser in the code indexer). Each section becomes a separate vector. This means a long architecture document with 8 sections produces 8 embeddings, each retrievable independently. The section heading provides context for what the chunk is about.

Short documents (under ~1000 characters) are embedded as a single chunk.

### Index Storage

- **Vector embeddings:** Separate LanceDB collection (`brain_chunks`), same LanceDB instance as code. Schema includes: document_id, chunk_index, chunk_text, embedding, document_path, document_title, tags (JSON), created_at, updated_at.
- **Wikilink graph:** SQLite table `brain_links` with columns: source_path, target_path, link_text. Enables bidirectional traversal.
- **Document metadata:** SQLite table `brain_documents` with columns: path, title, content_hash, tags (JSON), frontmatter (JSON), created_at, updated_at, created_by, source_session_id, token_count.

These tables live in the main sirtopham SQLite database alongside conversation and metrics tables.

---

## Tools

Four tools for the agent. All project-scoped — they operate on the current project's brain vault via the MCP/vault backend.

### brain_search

Search the brain by keyword query. Returns document titles, paths, and relevant snippets. Semantic/index-backed search is still future work unless the runtime explicitly grows that path.

**Purity:** Pure (read-only)

**Parameters:**
- `query` (string, required): The search query
- `mode` (string, optional): currently `keyword` is the only implemented runtime behavior; older `semantic` / `auto` wording in this draft should be treated as future-facing design, not a guaranteed live contract
- `tags` ([]string, optional): Filter by tags
- `max_results` (int, optional): Maximum results to return (default 10)

**Returns:** Ranked list of matches with: document path, title, relevant snippet, match score, tags, linked documents.

### brain_read

Read a specific brain document by path. Returns the full markdown content.

**Purity:** Pure (read-only)

**Parameters:**
- `path` (string, required): Path relative to vault root
- `include_backlinks` (bool, optional): If true, also return a list of documents that link to this one (default false)

**Returns:** Document content, frontmatter metadata, outgoing wikilinks, and optionally backlinks.

### brain_write

Create a new document or overwrite an existing one. The agent writes Obsidian-native markdown — frontmatter, wikilinks, tags.

**Purity:** Mutating

**Parameters:**
- `path` (string, required): Path relative to vault root (creates parent directories if needed)
- `content` (string, required): Full markdown content including frontmatter

**Behavior:**
- Creates or overwrites the file through the MCP/vault backend
- Any future index/graph refresh behavior should be documented separately when it actually exists
- Returns confirmation with the document path

### brain_update

Append to or edit a section of an existing document. More surgical than full overwrite — the agent can add a section to a debugging journal or update a specific heading without rewriting the entire file.

**Purity:** Mutating

**Parameters:**
- `path` (string, required): Path relative to vault root
- `operation` (string, required): "append", "prepend", or "replace_section"
- `content` (string, required): Content to add or replace with
- `section` (string, optional): Heading text to target for `replace_section` (e.g., "## Workaround")

**Behavior:**
- Reads the current document, applies the operation, writes back via REST API
- Updates vector embeddings and link graph
- Returns the updated document content

---

## v0.2 Integration with Context Assembly

This section is no longer just distant future direction; the first runtime slice is already live. In v0.1, the brain was reactive-only and accessed through Layer 4 brain tools. In current v0.2 runtime, context assembly already performs proactive keyword retrieval from the MCP/vault backend and reports those results through the inspector/context-report path.

The current runtime answer is: proactive brain retrieval is sourced directly from the MCP/vault backend, keyword-only. Operational brain log notes like `_log.md` are now excluded from proactive context so they do not compete with real knowledge notes. The still-open product/runtime contract question is whether semantic/index-backed retrieval should join that path in v0.2 or stay future work. The code/docs should keep telling the current operator truth until a broader path is actually landed.

### How Brain Queries Are Derived

Current implementation reuses the deterministic query-extraction path and then applies a small amount of brain-specific routing during retrieval. That is enough to support the first live proof, but it is not yet a fully brain-aware analyzer/query pipeline.

Today the flow is roughly:

- User says "fix the auth middleware" → existing cleaned/technical queries can also drive brain keyword search for "auth middleware"
- User says "what is the runtime brain proof canary phrase" → analyzer emits a `brain_intent` signal, retrieval can prefer brain context over generic code RAG for that turn, and literal keyword search falls back to a stopword-stripped candidate such as "runtime brain proof canary"
- User says "walk me through the rationale behind our minimal content-first layout decision" → analyzer emits a `brain_seeking_intent` signal (`value: "rationale"`) on a narrow rationale/decision phrase set (`rationale behind`, `rationale for`, `design decision`, `design choice`, `design rationale`, `why did we`, `why are we`). Retrieval prefers brain context the same way as explicit brain prompts, and brain keyword candidates now include a longest-content-word fallback ("rationale") so long prose queries still reach the matching note when the full stopword-stripped phrase cannot substring-match the note body.
- User says "what's our convention for naming new pattern lists?" → analyzer emits a `brain_seeking_intent` signal (`value: "convention"`) on a narrow convention/policy phrase set (`how do we usually`, `how do we normally`, `what do we prefer`, `what's our convention`, `what is our convention`, `our convention for`, `our convention is`, `our policy for`, `our policy is`, `what's our policy`, `what is our policy`). Bare `how do we` is deliberately excluded because it collides with generic code-explanation noise.
- User says "have we seen a vite rebuild loop before? what was the fix?" → analyzer emits a `brain_seeking_intent` signal (`value: "history"`) on a narrow prior-debugging/history phrase set (`have we seen`, `have we hit`, `have we debugged`, `have we fixed`, `what was the fix`, `what was the workaround`, `what was the root cause`, `did we ever fix`, `did we already fix`, `prior debugging`, `past debugging`, `previously debugged`). Bare `did we` and bare `what was` are deliberately excluded because they collide with the rationale family, arbitrary past-tense questions, and debug prompts like `what was null here`. Only the first brain-seeking family to match a turn emits a signal, so a prompt that combines rationale + convention + history phrases still emits exactly one `brain_seeking_intent` — the precedence order is: explicit `brain_intent` → `rationale` → `convention` → `history`.

So the current operator truth is:
- the existing signal/query path is good enough to make proactive keyword-backed brain retrieval work for explicit brain prompts and all three non-explicit families (rationale/decision, convention/policy, prior-debugging/history)
- tag/semantic/wikilink-aware brain query expansion remains future work unless explicitly landed later

### Budget Fitting Priority

This is now the implemented runtime direction: brain results compete with code chunks for budget, and brain documents sit between explicit files and top RAG code hits:

1. **Explicit files** (user mentioned them directly)
2. **Brain documents** (project knowledge — architecture, debugging, conventions)
3. **Top RAG code hits** (above threshold, de-duped, re-ranked)
4. **Structural graph results** (callers/callees of identified symbols)
5. **Conventions** (derived from code analysis)
6. **Git context** (recent commits)
7. **Lower-ranked RAG code hits** (fill remaining budget)

Rationale: brain documents contain high-level knowledge — architectural context, decision rationale, debugging insights. This is often more valuable than the fifth-ranked code function in the results. When the agent knows *why* the auth system is designed the way it is, it makes better decisions about *how* to modify it.

This ranking is a starting point for v0.2. The context inspector will reveal whether brain documents are genuinely helpful or displacing more valuable code context.

### Brain Budget Allocation

In v0.2, brain results get a configurable token budget within the overall MAX_CONTEXT_BUDGET:

```yaml
brain:
  max_brain_tokens: 8000              # Max tokens for brain content in assembled context
  brain_relevance_threshold: 0.30     # Separate threshold for brain semantic results
```

The brain budget is a soft cap within the overall budget — if brain results are highly relevant and code results are sparse, brain content can use more. If code results are dense and brain results are marginal, brain content uses less. The budget manager balances this dynamically.

### Serialization Format

In the current v0.2 runtime, brain results in the assembled context are serialized separately from code chunks:

```markdown
## Project Knowledge

### auth-architecture.md
Architecture decision: The auth system uses JWT tokens validated by middleware.
Token refresh is handled by the AuthService, not the middleware. The middleware
only validates — it never issues or refreshes tokens. This separation exists
because the refresh flow requires database access that the middleware layer
shouldn't have.

Related: [[provider-design]], [[error-handling]]

### tree-sitter-generics-workaround.md
The tree-sitter Go parser doesn't handle generics (type parameters) correctly.
When a Go file contains generic types, fall back to the Go AST parser instead.
This is detected by checking for `[` in type declarations during the parsing phase.

## Relevant Code

### internal/auth/middleware.go (lines 15-48)
...
```

If adopted in v0.2, brain content is serialized before code chunks in the assembled context. This positions project knowledge early in the context where attention is highest.

---

## Agent Writing to the Brain

The agent writes to the brain when it discovers durable knowledge. This is a deliberate act, not an automatic dump.

### System Prompt Guidance

The base system prompt includes guidance for when to create or update brain documents:

```
You have access to a project brain — an Obsidian vault of persistent project
knowledge. Use brain_write and brain_update to capture durable insights:

- After resolving a non-obvious bug, write a debugging journal entry
- When an architectural decision is made during conversation, document it
- When you discover a convention or anti-pattern, record it
- At the end of a substantial work session, write a session summary

Write in Obsidian-native markdown: use YAML frontmatter, [[wikilinks]] to
related documents, and #tags for categorization. Link to existing brain
documents when relevant.

Do not write brain documents for trivial interactions. The brain is for
knowledge worth preserving across sessions.
```

### Writing Triggers

**Agent-initiated:** The agent judges that something is worth persisting. A complex debugging session that uncovers a subtle issue. An architectural decision made during conversation. A convention discovered while reading code. The agent uses judgment — not every session produces a brain document.

**Developer-initiated:** The developer explicitly asks: "write that up in the brain", "add that to the debugging notes", "create a session summary." Direct, intentional knowledge capture.

**Not auto-generated.** The agent does not automatically summarize every session into a brain document. That would flood the brain with low-signal entries. Automatic session summaries are a future consideration (v0.3), gated on quality — only sessions where meaningful work was done.

### Curation

Both the developer and agent can edit and delete brain documents. The developer curates in Obsidian — reorganizing, merging related notes, deleting stale entries. The agent can update existing documents via `brain_update` — adding new information to a debugging journal, updating a decision record with new context.

The brain has no artificial constraints on size, structure, or organization. It grows organically. The retrieval layer handles finding what's relevant; the humans and agent handle keeping it useful.

---

## Brain Configuration

Current runtime uses the project brain vault plus the MCP/vault backend. The minimal operator-facing setup is:

```yaml
brain:
  enabled: true
  vault_path: ~/obsidian-vaults/sirtopham-brain
  log_brain_queries: true
```

Notes:
- `vault_path` is the source of truth for the brain content the tools and proactive retrieval operate on
- `log_brain_queries` gates both reactive `brain_search` trace logging and proactive brain-query debug logging
- older REST-specific fields in historical drafts (`obsidian_api_url`, `obsidian_api_key`) should be treated as pre-MCP design baggage unless/until they are reintroduced intentionally
- future semantic/index-backed brain settings should stay documented as reserved or experimental until the runtime actually uses them

---

## Data Model

### SQLite Tables

```sql
-- Brain document metadata (derived from vault content)
CREATE TABLE brain_documents (
    id TEXT PRIMARY KEY,              -- UUID
    project_id TEXT NOT NULL REFERENCES projects(id),
    path TEXT NOT NULL,               -- relative to vault root
    title TEXT,                       -- extracted from first heading or filename
    content_hash TEXT NOT NULL,       -- for change detection
    tags TEXT,                        -- JSON array of tags
    frontmatter TEXT,                 -- JSON of full frontmatter
    token_count INTEGER,             -- estimated token count
    created_by TEXT,                  -- 'agent' or 'user'
    source_session_id TEXT,           -- session that created this (if agent-created)
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    UNIQUE(project_id, path)
);

-- Wikilink graph
CREATE TABLE brain_links (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id TEXT NOT NULL REFERENCES projects(id),
    source_path TEXT NOT NULL,        -- document containing the link
    target_path TEXT NOT NULL,        -- document being linked to
    link_text TEXT,                   -- display text of the wikilink
    UNIQUE(project_id, source_path, target_path)
);

-- Indexes
CREATE INDEX idx_brain_docs_project ON brain_documents(project_id);
CREATE INDEX idx_brain_links_source ON brain_links(project_id, source_path);
CREATE INDEX idx_brain_links_target ON brain_links(project_id, target_path);
```

### LanceDB Collection

Separate collection `brain_chunks` in the same LanceDB instance as code:

| Column | Type | Description |
|---|---|---|
| id | string | `sha256(project_name + path + chunk_index)` |
| project_name | string | Project identifier |
| document_path | string | Path relative to vault root |
| document_title | string | Document title |
| chunk_index | int | Section index within document |
| chunk_text | string | Text content of the section |
| tags | string | JSON array of document tags |
| embedding | float32[3584] | nomic-embed-code vector |
| updated_at | string | ISO timestamp |

---

## Differences from Existing Components

**Brain vs. Code RAG ([[04-code-intelligence-and-rag]]):** Code RAG indexes source code — function bodies, type definitions, file structures. The brain stores knowledge *about* code — why things are the way they are, how systems relate, what to watch out for. Today the code path is semantic/vector-backed while the brain path is keyword-backed through the MCP/vault backend; that asymmetry is intentional until a broader brain retrieval path is actually implemented.

**Brain vs. Convention Extractor:** The convention extractor derives patterns mechanically from code analysis — "tests use `_test.go` suffix." The brain stores conventions that require judgment — "we don't use go-git because of index desync issues." They're complementary. The extractor tells you *what patterns exist*. The brain tells you *why certain patterns are followed* and *what patterns to avoid*.

**Brain vs. Conversation History:** Conversation history is ephemeral — it lives for a session, gets compressed, eventually summarized away. The brain is where the durable insights from conversations get extracted and persisted. After compression removes the details of a debugging session, the brain document about that bug survives intact.

**Brain vs. Hermes Memory:** Hermes uses MEMORY.md (~2200 chars) and USER.md (~1375 chars) — tiny, bounded, agent-curated scratchpads injected into every turn. The brain is unbounded, topic-organized, and retrieved contextually (not dumped wholesale). Hermes's approach is a notepad. The brain is a library.

---

## Dependencies

- [[06-context-assembly]] — Consumes brain results as a retrieval source; budget fitting allocates between brain and code context
- [[05-agent-loop]] — Four brain tools (`brain_read`, `brain_write`, `brain_update`, `brain_search`) in the tool registry
- [[04-code-intelligence-and-rag]] — Shared LanceDB instance (separate collection), shared embedding model (nomic-embed-code), shared embedding container
- [[08-data-model]] — `brain_documents` and `brain_links` tables in SQLite
- [[07-web-interface-and-streaming]] — "Open in Obsidian" links, brain results in context inspector
- MCP/vault brain backend — current runtime dependency for brain tools and proactive brain retrieval

---

## Future Directions

**Additional MCP productization (v0.5+):** The runtime already uses an MCP/vault backend internally. Future work here is about exposing that capability more broadly — for example surfacing brain tools as an MCP server for external tools, or standardizing richer backend contracts — rather than doing the original REST→MCP migration described in older drafts.

**MCP server exposure (v0.5+):** Expose sirtopham's brain tools as an MCP server, letting other tools (Claude Code, Codex) query the project brain. The brain becomes a shared knowledge layer across your entire tool chain.

**Obsidian URI integration (v0.3):** Use the `obsidian://` URI protocol to open specific documents from sirtopham's web UI. Click a brain reference in a conversation → Obsidian focuses that document.

**Session summary automation (v0.3):** At the end of sessions where meaningful work was done, the agent proposes a session summary for the brain. The developer approves, edits, or declines. Not fully automatic — gated on quality.

**Cross-project brain queries (v0.5+):** Search across multiple project brains. Patterns learned on project A that might apply to project B. Requires a brain registry that knows about all project vaults.

**Template system:** Obsidian Templater integration for standardized brain documents — decision records, debugging journals, session summaries. The agent uses templates when creating new documents for consistent structure.

---

## Build Phases

**v0.1 (foundation):** Brain tools were reactive-only. The agent could `brain_read`, `brain_write`, `brain_update`, and `brain_search`, but context assembly did not proactively include brain content.

**Current v0.2 state:** MCP/vault-backed proactive keyword retrieval is live in context assembly. Brain hits have an explicit budget tier, serialize into a Project Brain section, persist in context reports, and now have a dedicated ordered signal-flow endpoint at `/api/metrics/conversation/:id/context/:turn/signals`.

**Remaining v0.2 work:** Decide whether semantic/index-backed brain retrieval becomes real runtime behavior, package a repeatable live validation recipe, and keep query shaping/observability aligned with what the runtime actually does.

**v0.3+ ideas:** Obsidian URI links from the web UI, session summary proposals, richer brain-aware quality metrics, cross-project queries, and templated brain documents remain future-facing design rather than committed runtime behavior.

---

## Open Questions

- **Semantic/index-backed brain retrieval:** Should v0.2 stop at MCP/vault keyword search, or should a separate semantic/index-backed brain path become part of the real runtime contract?
- **Embedding model for prose vs code:** If semantic brain retrieval is added later, would a general-purpose embedding model outperform the current code-oriented defaults for prose-heavy notes?
- **Brain document size limits for future indexing:** Very long brain documents (5000+ words) would need chunking for effective embedding if index-backed retrieval becomes real.
- **Conflict resolution:** If the agent writes a brain document while the developer has the same file open in Obsidian, what exact UX does the current vault workflow produce? Worth verifying directly.
- **Brain search latency in context assembly:** Current keyword-backed proactive brain retrieval is cheap, but any future semantic/index path should still be validated against the context-assembly latency budget.

---

## References

- Obsidian: https://obsidian.md
- Obsidian Local REST API plugin: https://github.com/coddingtonbear/obsidian-local-rest-api
- Obsidian URI protocol: https://help.obsidian.md/Extending+Obsidian/Obsidian+URI
- Hermes Agent memory system: `tools/memory_tool.py`, `agent/prompt_builder.py` (bounded scratchpad we're improving on)
- Hermes Agent Honcho integration: `honcho_integration/` (vector-based cross-session recall — conceptually related)
- LanceDB Go bindings: `github.com/lancedb/lancedb-go`
- nomic-embed-code: https://huggingface.co/nomic-ai/nomic-embed-code
