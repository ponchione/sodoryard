# Task 03: Per-Conversation Metrics Display (Tokens, Cache, Tools)

**Epic:** 10 — Settings & Metrics
**Status:** ⬚ Not started
**Dependencies:** Task 01, Epic 03 Task 03 (per-conversation metrics endpoint)

---

## Description

Build the per-conversation metrics display that shows token usage, cache hit rate, and tool usage breakdown for the current conversation. This component is placed within the conversation view (as a collapsible section, tab, or panel near the context inspector). Data is fetched from the per-conversation metrics REST endpoint and refreshed when navigating to a conversation or when a turn completes.

## Acceptance Criteria

- [ ] **Metrics placement:** Displayed within the conversation view as a collapsible section, tab panel, or alongside the context inspector (agent's choice)
- [ ] Metrics data fetched from `GET /api/metrics/conversation/:id` using the API client
- [ ] **Token usage display:** Shows total tokens in, total tokens out, total cache read tokens, and total LLM calls as summary cards or a data table
- [ ] Token counts formatted with thousands separators for readability (e.g., "14,200" not "14200")
- [ ] **Cache hit rate:** Displayed as a percentage with color coding: green (>50%), yellow (20-50%), red (<20%)
- [ ] Cache hit rate includes a brief label: "Cache Hit Rate: 67%" — validates that the caching strategy is working
- [ ] **Tool usage table:** Per-tool breakdown displayed as a simple table with columns: tool name, call count, average duration (formatted as ms or seconds), failure count
- [ ] Tool rows sorted by call count descending (most-used tools first)
- [ ] Failed tool calls highlighted (red count or warning icon) if failure_count > 0
- [ ] **Data refresh:** Metrics re-fetched when navigating to a different conversation or when a `turn_complete` event fires on the active conversation
- [ ] **Loading state:** Spinner or skeleton displayed while metrics are loading
- [ ] **Empty state:** For new conversations with no metrics data yet, display "No metrics available — send a message to start" rather than zeroed-out cards
- [ ] Metrics section is collapsible (collapsed by default) to avoid cluttering the conversation view
