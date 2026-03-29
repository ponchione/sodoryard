# Task 02: Three-Block Cache Layout

**Epic:** 05 — System Prompt Builder
**Status:** ⬚ Not started
**Dependencies:** Task 01, Layer 2 Epic 01 (provider types — cache_control field)

---

## Description

Implement the three-block prompt cache layout by adding Anthropic `cache_control` markers at the correct breakpoints in the provider Request. The three cacheable regions are: (1) base system prompt — identical across all sessions, (2) assembled context — frozen within a turn, identical across iterations, and (3) conversation history prefix — grows monotonically, each new turn only appends. Getting cache markers at the right positions is critical for latency — a cache hit on 50k+ tokens of conversation history eliminates reprocessing. The markers are placed on the last content block of each region.

## Acceptance Criteria

- [ ] **Cache Block 1 breakpoint:** the last content block of the base system prompt section has `cache_control: {"type": "ephemeral"}` added. This block is identical across all calls in all sessions, giving maximum cache hit rate
- [ ] **Cache Block 2 breakpoint:** the last content block of the assembled context section has `cache_control: {"type": "ephemeral"}` added. This block changes at turn start but is frozen across all iterations within a turn, so iterations 2+ get cache hits on context
- [ ] **Cache Block 3 breakpoint:** the last message of the conversation history prefix (the last completed turn's messages) has `cache_control: {"type": "ephemeral"}` added. History grows monotonically — each new turn appends, so the prefix is stable and cacheable
- [ ] **Fresh content (uncached):** current turn messages (user message + in-progress tool results) have no cache markers — this is the only part that changes between iterations
- [ ] If assembled context is empty (no block 2), block 1's cache marker covers the base system prompt and the next marker is on block 3 (history)
- [ ] If history is empty (first turn), only blocks 1 and 2 have cache markers
- [ ] Cache markers are placed on the correct field of the content block structure per the Anthropic API format — the `cache_control` field on the last content block of each section
- [ ] History prefix stability: adding a new message to the history does not change any content in earlier history messages — the prefix remains byte-identical for cache hits
- [ ] Package compiles with `go build ./internal/agent/...`
