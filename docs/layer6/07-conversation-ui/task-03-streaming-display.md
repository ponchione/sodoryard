# Task 03: Streaming Display (Token Deltas and Thinking Blocks)

**Epic:** 07 — Conversation UI
**Status:** ⬚ Not started
**Dependencies:** Task 01, Task 02

---

## Description

Implement real-time streaming display for token deltas and thinking blocks. As the agent generates a response, `token` events arrive and are appended to the current assistant message in real-time, making the message visibly grow character by character. Thinking blocks (`thinking_start`, `thinking_delta`, `thinking_end`) render as collapsible sections that stream content live. The streaming state is managed separately from completed messages to avoid re-rendering the entire thread on every token.

## Acceptance Criteria

- [ ] **Token streaming:** `token` events append text to an accumulating buffer for the current assistant message. The displayed message grows incrementally as tokens arrive
- [ ] Streaming is smooth — no visible jank, flicker, or layout shifts during token-by-token rendering
- [ ] The streaming message is rendered with markdown formatting applied in real-time (or re-applied on each update). Incomplete markdown (e.g., an opening ` ``` ` without a closing one) degrades gracefully without breaking the layout
- [ ] **Streaming state management:** The current in-progress message is stored separately from the completed messages array. On `turn_complete`, the accumulated message is finalized and moved to the completed messages list
- [ ] **Performance:** If token-by-token updates cause jank, batch token events (e.g., accumulate tokens for ~16ms and update state once per animation frame via `requestAnimationFrame`). Start without batching — add only if needed
- [ ] **Thinking blocks:** `thinking_start` begins a new collapsible section labeled "Thinking..." (collapsed by default)
- [ ] `thinking_delta` events stream content into the thinking section in real-time
- [ ] `thinking_end` finalizes the thinking block
- [ ] Thinking blocks are expandable/collapsible — clicking toggles visibility of the thinking content
- [ ] Thinking blocks from completed turns (loaded via REST) are also rendered as collapsible sections
- [ ] Multiple thinking blocks in a single turn are each rendered as separate collapsible sections
- [ ] Partially streamed content remains visible after cancellation — `turn_cancelled` does not clear the accumulated text
