# Layer 3, Epic 03: Query Extraction & Momentum

**Layer:** 3 — Context Assembly
**Epic:** 03 of 07
**Status:** ⬚ Not started
**Dependencies:** [[layer-3-epic-01-context-assembly-types]]

---

## Description

Implement query extraction (translating `ContextNeeds` into concrete RAG search queries) and conversation momentum tracking (scanning recent turns to determine the active module and recently-touched files). These are tightly coupled — momentum feeds into query extraction as the third query source.

Both are pure string processing. No LLM calls, no I/O. Query extraction produces 1-3 semantic queries from the user's message. Momentum scanning extracts file paths from recent tool calls in conversation history and computes the longest common directory prefix.

---

## Definition of Done

### Query Extraction

- [ ] `QueryExtractor` function or struct that takes a user message and `ContextNeeds` and produces `[]string` (1-3 semantic queries)
- [ ] **Source 1 — Cleaned message:**
  - Strip conversational filler: "hey", "can you", "please", "I think", "I want you to", "could you", "help me", "let's" (and similar)
  - Strip punctuation
  - Cap at ~50 words
  - For long messages with multiple sentences, split at sentence boundaries, take up to 2 queries
- [ ] **Source 2 — Technical keyword extraction:**
  - Extract words with underscores (`validate_token`), camelCase (`validateToken`), PascalCase (`ValidateToken`), dot notation (`auth.Service`)
  - Extract HTTP methods (GET, POST, PUT, DELETE, PATCH)
  - Extract status codes (200, 401, 404, 500)
  - Extract programming domain terms: middleware, handler, router, schema, migration, query, endpoint, service, repository, controller, factory, adapter
  - Join extracted terms into a supplementary query if they differ meaningfully from the cleaned message
- [ ] **Source 3 — Momentum-enhanced query:**
  - If `ContextNeeds.MomentumModule` is set (e.g., `internal/auth`), prepend it to the cleaned message query
  - "Fix the tests" with momentum module `internal/auth` → "internal/auth fix the tests"
- [ ] **Query cap:** Maximum 3 queries returned. If all three sources produce distinct queries, return all three. If source 2 overlaps substantially with source 1, skip it.
- [ ] **Explicit entity exclusion:** File paths and symbol names from `ContextNeeds.ExplicitFiles` and `ContextNeeds.ExplicitSymbols` do NOT become queries — they are handled deterministically by the retrieval orchestrator (Epic 04)

### Conversation Momentum

- [ ] `MomentumTracker` function or struct that scans recent conversation history and populates `MomentumFiles` and `MomentumModule` on `ContextNeeds`
- [ ] Scans the last N turns (configurable via `MomentumLookbackTurns`, default 2) of conversation history
- [ ] Extracts file paths from tool calls in those turns:
  - `file_read` calls: extract the `path` argument from the tool_use input JSON
  - `file_write` / `file_edit` calls: extract the `path` argument
  - `search_text` / `search_semantic` results: extract file paths from the tool result content
- [ ] `MomentumFiles`: deduplicated list of all extracted file paths
- [ ] `MomentumModule`: longest common directory prefix among extracted paths. If all paths are in `internal/auth/`, the module is `internal/auth`. If paths span `internal/auth/` and `internal/config/`, the module is `internal` (or empty if no meaningful common prefix exists)
- [ ] Momentum is only applied when the turn analyzer detects a continuation signal or weak signals (no explicit files/symbols). Strong-signal turns ignore momentum.

### Tests

- [ ] Query extraction: "Fix the auth middleware validation" → cleaned query without filler words
- [ ] Query extraction: long multi-sentence message → splits into up to 2 queries
- [ ] Query extraction: message with technical terms (camelCase, underscores) → source 2 produces supplementary query
- [ ] Momentum: conversation history with `file_read` calls to `internal/auth/middleware.go` and `internal/auth/service.go` → `MomentumModule = "internal/auth"`, `MomentumFiles = ["internal/auth/middleware.go", "internal/auth/service.go"]`
- [ ] Momentum: no tool calls in recent history → empty momentum
- [ ] Momentum-enhanced query: "fix the tests" with `MomentumModule = "internal/auth"` → query becomes "internal/auth fix the tests"
- [ ] Integration: weak-signal message ("keep going") with active momentum → momentum files surfaced, momentum-enhanced query generated
- [ ] Integration: strong-signal message ("fix `internal/config/loader.go`") with stale momentum from auth → momentum does not override explicit signals

---

## Architecture References

- [[06-context-assembly]] — "Component: Query Extraction" section (Three-Source Strategy, Query Cap, Explicit Entity Handling), "Component: Conversation Momentum" section (Implementation, How Momentum Is Used, What Momentum Does NOT Do)

---

## Implementation Notes

- The filler word list for source 1 should be a static set, not a regex monster. Start simple. Overly aggressive stripping risks removing meaningful words.
- Sentence splitting for source 1 can be simple: split on `.`, `?`, `!` followed by whitespace. No NLP library needed.
- Momentum scanning requires parsing the `content` JSON of assistant messages to find `tool_use` blocks and extracting the `input` JSON. This is the same JSON format used in the message storage model ([[08-data-model]]).
- The longest common directory prefix computation should handle edge cases: single file (prefix is its directory), files in the project root (empty prefix), no files (empty prefix).
- The boundary between Turn Analyzer (E02) and this epic: the turn analyzer detects continuation signals and sets flags. This epic computes the actual momentum values (scanning history, computing prefix) and builds queries. If the turn analyzer already populated `MomentumFiles` and `MomentumModule` during continuation detection, this epic uses those values for query enhancement.
