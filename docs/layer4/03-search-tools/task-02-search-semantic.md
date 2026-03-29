# Task 02: search_semantic Implementation

**Epic:** 03 — Search Tools
**Status:** ⬚ Not started
**Dependencies:** Layer 4 Epic 01, Layer 1 Epic 08 (Searcher)

---

## Description

Implement the `search_semantic` tool as a `Pure` tool in `internal/tool/`. This tool provides RAG-based semantic search against the code intelligence layer by delegating to the Layer 1 `Searcher`. It embeds the natural language query, searches the vector index, and formats the results for the LLM. This is the agent's reactive access to the full codebase knowledge when proactive context assembly was insufficient.

## Acceptance Criteria

- [ ] Implements the `Tool` interface with `Purity() = Pure`
- [ ] `Name()` returns `"search_semantic"`, `Description()` returns a concise one-liner for the LLM
- [ ] Accepts JSON input with parameters: `query` (required string), `language` (optional string filter), `chunk_type` (optional string filter, e.g., `"function"`, `"type"`), `max_results` (optional int, default 15)
- [ ] Constructor accepts a `Searcher` interface from Layer 1 as a dependency (injected, not global)
- [ ] Calls `Searcher.Search()` (or equivalent) with the query and any filters
- [ ] Returns results formatted as readable blocks, each containing:
  - File path and line range (e.g., `internal/auth/middleware.go:42-68`)
  - Function/type name (e.g., `func ValidateToken`)
  - Description from the RAG pipeline (the LLM-generated summary of the chunk)
  - Similarity score (formatted to 2 decimal places)
- [ ] Results ordered by relevance (highest similarity first)
- [ ] Zero results: returns `Success=true` with `"No semantically relevant code found for query: '<query>'. Try rephrasing or use search_text for exact string matching."`
- [ ] If the RAG index is empty or not initialized: returns `Success=false` with `"Code index is empty or not built. Run 'sirtopham index' to build the code intelligence index."`
- [ ] `Schema()` returns valid JSON Schema with parameter types, descriptions, required fields, and defaults documented
