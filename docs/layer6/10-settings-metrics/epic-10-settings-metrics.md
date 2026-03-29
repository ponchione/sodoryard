# Layer 6, Epic 10: Settings & Metrics Panels

**Layer:** 6 — Web Interface & Streaming
**Status:** ⬚ Not started
**Dependencies:** [[layer-6-epic-06-react-scaffolding]] (app shell, API client), [[layer-6-epic-03-rest-api-project-config-metrics]] (REST config/metrics/providers endpoints), Layer 2 Epic 07 (provider router — provider/model list)

---

## Description

Build the settings panel and per-conversation metrics display. The settings panel shows configured providers and their available models, allows changing the default provider/model, and supports per-conversation model override. The metrics panel shows per-conversation statistics: total token usage, cache hit rate, tool usage breakdown, and context assembly quality averages. These are informational panels — they expose the data needed to understand and tune sirtopham's behavior.

v0.1 scope: functional display of data from REST endpoints. No charts or graphs — tables and summary cards. Recharts-based visualizations (cost over time, token trends) are v0.5+ polish.

---

## Definition of Done

- [ ] **Settings panel access:** Reachable via a settings icon/button in the app header or sidebar. Opens as a modal, drawer, or dedicated route (agent's choice)
- [ ] **Provider list:** Displays all configured providers from `GET /api/providers`. Each provider shows: name, type (anthropic/codex/openai-compatible/local), status (available/unavailable), and list of available models with context window sizes
- [ ] **Default model selection:** The current default provider/model is displayed prominently. A dropdown or selector allows changing the default. Calls `PUT /api/config` with the new default. Change takes effect for the next turn (not mid-turn)
- [ ] **Per-conversation model override:** Within the conversation view, a model selector allows overriding the model for the current conversation. Sends `model_override` event via WebSocket (handled by [[layer-6-epic-04-websocket-handler]]). Visually indicates when a conversation is using a non-default model
- [ ] **Per-conversation metrics:** Displayed in the conversation view (a collapsible section, a tab, or via the context inspector panel — agent's choice for placement). Shows:
  - **Token usage:** Total tokens in, total tokens out, total cache read tokens, total LLM calls (from `sub_calls` aggregation)
  - **Cache hit rate:** Percentage of prompt tokens served from cache. This validates the three-breakpoint caching strategy from [[05-agent-loop]]. Displayed as a percentage with color coding (green >50%, yellow 20-50%, red <20%)
  - **Tool usage:** Per-tool breakdown: call count, average duration, failure count (from `tool_executions` aggregation). Displayed as a simple table
  - **Context assembly quality:** Average `context_hit_rate` across turns, percentage of turns where `agent_used_search_tool` was true, average budget utilization. Displayed as summary metrics
- [ ] **Metrics data:** Fetched via `GET /api/metrics/conversation/:id`. Refreshed when navigating to a conversation or when a turn completes
- [ ] **Project info:** The settings panel (or a dedicated section) shows basic project info from `GET /api/project`: project name, root path, primary language, last indexed timestamp, last indexed commit
- [ ] **No cross-conversation metrics for v0.1:** Global token trends, cost over time, model comparison charts are deferred to v0.5+

---

## Architecture References

- [[07-web-interface-and-streaming]] — §UI Components: "Settings panel: Model selection, provider config, tool permissions", "Metrics/stats: Token usage, cost per conversation, model breakdown"
- [[03-provider-architecture]] — §Provider Router: "routing is explicit" for v0.1. Default + per-conversation override via config or web UI
- [[05-agent-loop]] — §Prompt Caching Strategy: cache hit rate validation. "sirtopham's Cache Layout" with three cache breakpoints
- [[08-data-model]] — §Key Query Patterns: per-conversation token usage, cache hit rate, tool usage breakdown, context assembly quality — all SQL queries defined
- [[06-context-assembly]] — §Quality Metrics: AgentUsedSearchTool, ContextHitRate — aggregate versions for the metrics panel

---

## Notes for the Implementing Agent

The settings panel is lightweight for v0.1. The primary mutable setting is the default provider/model. Other config knobs (max_iterations, relevance_threshold, etc.) are edited via `sirtopham.yaml` directly — they don't need UI controls yet. The panel is mostly a read-only view of the current configuration.

The per-conversation model override is the most interesting interaction. It sends a `model_override` event over the WebSocket, which the backend handler updates on the conversation. The next turn uses the overridden model. The UI should reflect this — show which model was used for each turn (from the `model` field on `sub_calls` or the turn metadata).

For metrics placement: embedding them within the conversation view (as a collapsible section near the context inspector) avoids creating a separate route. The developer can see metrics and context inspector data together while working in a conversation.

The cache hit rate metric is particularly valuable. If it's consistently low, the prompt caching strategy isn't working — possibly because the system prompt or context is changing in unexpected ways. If it's consistently high (>70%), the caching is working as designed.

Token usage is informational even with subscription access (no per-token cost) because it indicates conversation complexity, context window pressure, and helps detect when compression should fire.
