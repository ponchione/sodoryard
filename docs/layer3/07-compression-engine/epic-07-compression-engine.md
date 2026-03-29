# Layer 3, Epic 07: Compression Engine

**Layer:** 3 — Context Assembly
**Epic:** 07 of 07
**Status:** ⬚ Not started
**Dependencies:** [[layer-3-epic-01-context-assembly-types]]; Layer 2: [[layer-2-epic-01-provider-types]], [[layer-2-epic-07-provider-router]] (provider for summarization calls)

---

## Description

Implement conversation history compression using the head-tail preservation algorithm adopted from Hermes Agent. When conversation history grows past a configurable threshold (default 50% of the model's context window), the compression engine summarizes middle turns using an auxiliary model while preserving the head (foundational context) and tail (recent working context).

Compression is invoked by the agent loop when the budget manager signals that history is too large. It operates on the persisted message history in SQLite — marking compressed messages with `is_compressed = 1`, inserting a synthetic summary message with `is_summary = 1`, and sanitizing orphaned tool call pairs.

---

## Definition of Done

### Compression Trigger Detection

- [ ] **Preflight check (rough estimate):** Estimate total message tokens as `total_chars / 4`. Return `true` if this exceeds the compression threshold (default: 50% of model context window). This is called before each LLM API call.
- [ ] **Post-response check (exact):** Accept `prompt_tokens` from the API usage response. Return `true` if this exceeds the compression threshold. Called after each LLM response.
- [ ] **Reactive trigger:** Return `true` when the API returns HTTP 400 with `"context_length_exceeded"` or HTTP 413. Called from the agent loop's error handling path.
- [ ] Threshold is configurable: `CompressionThreshold` (default 0.50, meaning 50% of context window)

### Head-Tail Preservation Algorithm

- [ ] `Compress(ctx context.Context, conversationID string, modelContextLimit int) error` method
- [ ] **Step 1 — Protect the first N messages** (configurable: `CompressionHeadPreserve`, default 3). These are system prompt context, initial user message, and first assistant response.
- [ ] **Step 2 — Protect the last M messages** (configurable: `CompressionTailPreserve`, default 4). These are the most recent turns — the active working context.
- [ ] **Step 3 — Extract middle turns.** All messages between head and tail boundaries. These are the compression candidates.
- [ ] **Step 4 — Summarize** the middle using an auxiliary model:
  - Call the configured compression model (default: local Docker container) via the Layer 2 provider interface with `purpose = "compression"`
  - Summary prompt instructs the model to produce a concise structured summary preserving: key decisions made, file paths mentioned, work completed, errors encountered and resolutions
  - Summary is prefixed with `[CONTEXT COMPACTION]`
- [ ] **Step 5 — Persist compression:**
  - Set `is_compressed = 1` on all middle messages
  - INSERT a synthetic summary message with: `role = 'user'`, `is_summary = 1`, `compressed_turn_start` and `compressed_turn_end` recording the turn range covered
  - Summary message `sequence` = midpoint between last head message sequence and first tail message sequence (REAL bisection per [[08-data-model]])
- [ ] **Step 6 — Sanitize orphaned tool calls:**
  - Scan remaining uncompressed messages for assistant messages containing `tool_calls` in their content JSON where the corresponding `role=tool` result messages have been compressed
  - For these orphaned messages, remove the `tool_calls` entries from the content JSON (or remove the tool_use blocks from the content array)
  - This prevents API rejections from malformed conversation structure (assistant claims tool_use but no tool result follows)

### Cascading Compression

- [ ] If compression fires a second time in a long conversation, the existing summary message is marked `is_compressed = 1` along with newly compressed messages
- [ ] A new summary is generated covering the full compressed range: the old summary text is included as input to the summarization call along with the new middle messages
- [ ] Only one active (non-compressed) summary exists at any point
- [ ] The reconstruction query (`WHERE is_compressed = 0 ORDER BY sequence`) continues to work correctly after multiple compression rounds

### Fallback

- [ ] If auxiliary model summarization fails (model unavailable, API error, timeout), fall back to truncation: mark middle messages as compressed WITHOUT inserting a summary
- [ ] The conversation continues with less context but does not crash
- [ ] Log the summarization failure with structured logging: provider, model, error message, number of messages that would have been summarized

### Cache Invalidation

- [ ] After compression completes, return a signal indicating that the conversation history shape has changed
- [ ] The agent loop uses this signal to invalidate cached system prompt state (set `_cached_system_prompt = nil`)
- [ ] Cache block 1 (base prompt) and block 2 (assembled context) are unaffected — only block 3 (history prefix) is invalidated

### Tests

- [ ] Preflight check: 500k chars of history, 200k context limit → compression triggered (500k/4 = 125k tokens, threshold at 100k tokens)
- [ ] Head-tail preservation: 20 messages, protect first 3 and last 4 → 13 messages in middle extracted
- [ ] Summary insertion: sequence value is midpoint between head end and tail start
- [ ] Cascading: two rounds of compression → old summary compressed, new summary covers full range, reconstruction query returns correct message order
- [ ] Orphaned tool call sanitization: assistant message with tool_use whose result was compressed → tool_use block removed from content JSON
- [ ] Fallback: summarization model fails → middle messages compressed without summary, no crash
- [ ] Reconstruction query after compression: `WHERE is_compressed = 0 ORDER BY sequence` returns head + summary + tail in correct order
- [ ] REAL sequence bisection: after compression, a new message appended to the conversation gets the next integer sequence (no collision with the bisected summary sequence)

---

## Architecture References

- [[06-context-assembly]] — "Compression" section (Trigger, Threshold, Algorithm: Head-Tail Preservation, Fallback, Cache Invalidation After Compression)
- [[08-data-model]] — `messages` table: `is_compressed`, `is_summary`, `compressed_turn_start`, `compressed_turn_end`, `sequence` (REAL type), Compression Model section, Sequence Numbering section, Cancellation Safety section
- [[05-agent-loop]] — Error Recovery (context overflow triggers reactive compression), Persistence (iteration-level commits that compression must respect)

---

## Implementation Notes

- The compression engine operates on persisted messages in SQLite, not in-memory message arrays. It reads messages, determines what to compress, calls the summarization model, and writes the compression results back to SQLite. The agent loop then re-reads the (now compressed) history for the next iteration.
- The summarization prompt should be specific: "Summarize the following conversation turns into a concise summary. Preserve: file paths mentioned, key decisions made, work completed, errors encountered and their resolutions. Do not include code blocks. Format as a structured summary with bullet points."
- The orphan sanitization in step 6 requires parsing assistant message content JSON (the `[{"type":"tool_use",...}]` array), checking for each `tool_use` block whether a corresponding `role=tool` message with matching `tool_use_id` still exists in the uncompressed set, and rewriting the content JSON without the orphaned blocks. This is fiddly but essential — Anthropic's API rejects conversations where tool_use blocks lack corresponding tool results.
- The compression threshold check has two modes (preflight estimate and post-response exact). The agent loop should call both at appropriate points. The compression engine provides the check functions; the agent loop decides when to invoke compression.
- The `purpose = "compression"` tag on the provider call ensures the sub_call is tracked correctly in the sub_calls table and doesn't appear as a chat turn.
- Sequence midpoint calculation: `(lastHeadSeq + firstTailSeq) / 2.0`. For cascading compression, subsequent midpoints bisect whatever gap exists. IEEE 754 doubles support thousands of bisections.
