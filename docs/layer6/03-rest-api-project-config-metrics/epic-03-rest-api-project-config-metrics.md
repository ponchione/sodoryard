# Layer 6, Epic 03: REST API — Project, Config & Metrics

**Layer:** 6 — Web Interface & Streaming
**Status:** ⬚ Not started
**Dependencies:** [[layer-6-epic-01-http-server-foundation]], Layer 0 Epic 03 (config), Layer 0 Epic 06 (schema & sqlc), Layer 2 Epic 07 (provider router)

---

## Description

Implement the REST API endpoints for project information, configuration management, and per-conversation metrics. The project endpoints expose the current project's root path, primary language, and file tree. The config endpoints allow reading and updating runtime configuration (primarily model/provider selection). The metrics endpoints query the `sub_calls`, `tool_executions`, and `context_reports` tables to surface per-conversation statistics for the frontend's metrics panel and context inspector.

---

## Definition of Done

- [ ] `GET /api/project` — Returns project info: `{id, name, root_path, language, last_indexed_at, last_indexed_commit}`
- [ ] `GET /api/project/tree` — Returns the project file tree as a nested JSON structure. Respects the same include/exclude globs from config that the indexer uses. Depth-limited (default 3 levels) to keep response size manageable
- [ ] `GET /api/project/file?path=<relative_path>` — Returns file contents as plain text with metadata (path, language detected from extension, line count). Rejects paths outside the project root (path traversal protection). Does NOT do server-side syntax highlighting — that's the frontend's job
- [ ] `GET /api/config` — Returns the current runtime config relevant to the UI: default provider/model, fallback provider/model, configured providers with their available models (from provider router), agent settings (max_iterations, extended_thinking)
- [ ] `PUT /api/config` — Updates mutable runtime config. For v0.1, the only mutable setting is the default provider/model. Returns the updated config. Does NOT persist to `sirtopham.yaml` — runtime override only (config file is the source of truth on restart)
- [ ] `GET /api/providers` — Returns list of configured providers with their available models and status. Uses provider router's `ListProviders()` / `Models()` from Layer 2 Epic 07
- [ ] `GET /api/metrics/conversation/:id` — Returns per-conversation metrics aggregated from sqlc queries:
  - Token usage: total tokens in/out, cache read tokens, total calls (from `sub_calls` WHERE purpose='chat')
  - Cache hit rate: `cache_read_tokens / tokens_in` percentage (from `sub_calls`)
  - Tool usage breakdown: per-tool call count, avg duration, failure count (from `tool_executions`)
  - Context assembly quality: total turns, reactive search count, avg hit rate, avg budget used (from `context_reports`)
- [ ] `GET /api/metrics/conversation/:id/context/:turn` — Returns the full `ContextAssemblyReport` for a specific turn. Returns the JSON columns from `context_reports`: needs_json, signals_json, rag_results_json, brain_results_json, graph_results_json, budget_breakdown_json, plus scalar quality metrics. Consumed by the context inspector debug panel
- [ ] `GET /api/metrics/conversation/:id/context/:turn/signals` — Returns the narrow ordered signal/query stream for operator observability without requiring the consumer to reconstruct sequencing from the full report
- [ ] All endpoints return proper HTTP status codes and consistent JSON error format
- [ ] Endpoints registered on the server's router

---

## Architecture References

- [[07-web-interface-and-streaming]] — §REST API Endpoints: project, config, metrics endpoints
- [[08-data-model]] — §Key Query Patterns: per-conversation token usage, cache hit rate, tool usage breakdown, context assembly quality. All SQL queries are defined there
- [[06-context-assembly]] — §Context Assembly Report: the `ContextAssemblyReport` struct that the context endpoint returns. Fields: needs, signals, RAG results, brain results, budget breakdown, included/excluded chunks, quality metrics
- [[03-provider-architecture]] — §Provider Router: provider list and model enumeration for the providers/config endpoints

---

## Notes for the Implementing Agent

The file tree endpoint should walk the project directory using the same include/exclude glob logic from the indexer config. It returns a nested structure like `{name: "internal", type: "dir", children: [{name: "auth", type: "dir", children: [...]}]}`. Keep it simple — no file sizes, no git status, just the tree structure.

The metrics queries are defined in [[08-data-model]] §Key Query Patterns. They're straightforward aggregations. The sqlc-generated code from Layer 0 Epic 06 should already have these queries available (or the implementing agent adds new queries to `queries.sql` and reruns sqlc).

The context report endpoint (`/api/metrics/conversation/:id/context/:turn`) is critical — it feeds the context inspector debug panel ([[layer-6-epic-09-context-inspector]]). Return the JSON columns as-is (they're already structured data); the frontend parses them. The companion `/signals` endpoint is intentionally narrower and should expose the ordered signal/query flow operators actually need for debugging retrieval decisions.

The providers endpoint is distinct from the config endpoint — it returns the live state of provider availability (which providers are configured, which models they offer, connection status), while config returns the user's preferences.
