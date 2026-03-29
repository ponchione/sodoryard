# Layer 4 — Epic 03: Search Tools

**Layer:** 4 (Tool System)
**Package:** `internal/tool/`
**Status:** ⬚ Not Started
**Dependencies:**
- Layer 4 Epic 01: Tool Interface, Registry & Executor
- Layer 1 Epic 08: Searcher (`internal/rag/searcher.go` — multi-query expansion, dependency hops, vector search)

**Architecture Refs:**
- [[05-agent-loop]] §Tool Set — search_text, search_semantic definitions
- [[04-code-intelligence-and-rag]] §Searcher — basic search and work-order-aware search interfaces
- [[06-context-assembly]] — context assembly uses the same searcher but is NOT a tool consumer; `search_semantic` is the agent's reactive fallback when proactive assembly was insufficient

---

## What This Epic Covers

Two search tools that implement the `Tool` interface:

**`search_text` (Pure):** Ripgrep-based text search across the project. Takes a pattern (string or regex), optional file glob filter, and context lines count. Returns structured matches with file path, line number, matched line, and surrounding context. The structured output enables the UI to render clickable file:line results. Requires `rg` (ripgrep) to be installed on the system.

**`search_semantic` (Pure):** RAG-based semantic search against the code intelligence layer. Takes a natural language query and optional filters (language, chunk_type). Calls into the Layer 1 searcher's vector search pipeline — embed the query, search LanceDB, deduplicate, re-rank by hit count, one-hop call graph expansion. Returns code chunks with file paths, function names, descriptions, similarity scores, and line ranges. This is the agent's on-demand access to the full codebase knowledge — the reactive complement to context assembly's proactive retrieval.

---

## Definition of Done

### search_text
- [ ] Implements `Tool` interface with purity `Pure`
- [ ] Parameters: `pattern` (required), `file_glob` (optional, e.g., `"*.go"`), `context_lines` (optional, default 2), `max_results` (optional, default 50)
- [ ] Shells out to `rg` (ripgrep) with appropriate flags: `--json` for structured output, `--context` for surrounding lines, `--max-count` for result cap, `--glob` for file filtering
- [ ] Parses ripgrep JSON output into structured results: `{path, line_number, line_content, context_before, context_after}`
- [ ] Formats results as human-readable text with file:line headers for LLM consumption
- [ ] Runs from `projectRoot` as working directory
- [ ] Respects project's exclude patterns (`.git/`, `vendor/`, `node_modules/` — passed as `--glob '!pattern'` flags)
- [ ] If `rg` is not installed, returns clear error: "ripgrep (rg) is required but not found in PATH"
- [ ] If no matches found, returns "No matches found for pattern: ..." (not an error — success=true)
- [ ] JSON Schema accurately describes all parameters
- [ ] Unit tests: successful search with results, no results, file glob filtering, regex pattern, ripgrep not found

### search_semantic
- [ ] Implements `Tool` interface with purity `Pure`
- [ ] Parameters: `query` (required), `language` (optional filter), `chunk_type` (optional filter, e.g., "function", "type"), `max_results` (optional, default 15)
- [ ] Calls Layer 1 `Searcher.Search()` (or equivalent basic search method) with the query and filters
- [ ] Returns results formatted with: file path, function/type name, description (from RAG pipeline), similarity score, line range
- [ ] Each result formatted as a readable block the LLM can use to decide what to read or edit next
- [ ] If no results above the relevance threshold, returns "No semantically relevant code found for query: ..." (success=true)
- [ ] If the RAG index is empty or not initialized, returns a clear error suggesting `sirtopham index`
- [ ] JSON Schema accurately describes all parameters
- [ ] Unit tests with mock searcher: successful search with ranked results, empty results, index not initialized error
- [ ] Integration test: register both tools, dispatch via executor, verify results

---

## Key Design Notes

**search_text vs shell:** The agent also has a `shell` tool and could run `grep` or `rg` directly. `search_text` exists because it provides structured output (file:line results the UI can render as clickable links) and respects project exclude patterns automatically. The LLM should prefer `search_text` over `shell` for code search.

**search_semantic is the reactive fallback.** Per [[06-context-assembly]] §Design Principles, context assembly runs proactive RAG every turn. If the assembled context was insufficient, the agent uses `search_semantic` reactively. The `ContextAssemblyReport.AgentUsedSearchTool` metric tracks how often this happens — high frequency indicates the proactive assembly needs tuning.

**Searcher dependency:** `search_semantic` depends on a `Searcher` instance from Layer 1. This is injected at tool construction time (constructor parameter), not looked up globally. The same `Searcher` instance that context assembly uses.

---

## Consumed By

- [[layer4-epic01-tool-interface]] — registered in the tool registry
- Layer 5 (Agent Loop) — dispatched via the executor
- [[06-context-assembly]] — `AgentUsedSearchTool` quality metric tracks reactive usage
