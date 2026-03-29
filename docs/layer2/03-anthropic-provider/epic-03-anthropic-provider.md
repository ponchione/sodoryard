# Layer 2, Epic 03: Anthropic Provider

**Layer:** 2 — Provider Architecture
**Status:** ⬜ Not started
**Dependencies:** [[layer-2-epic-01-provider-types]], [[layer-2-epic-02-anthropic-credentials]], Layer 0 Epic 02 (logging)

---

## Description

Implement the Anthropic provider — the primary and most complex inference path. This covers the full `Provider` interface: building Messages API requests with OAuth-specific headers, parsing Anthropic's typed content block responses (text, tool_use, thinking) into sirtopham's unified types, streaming via Server-Sent Events with all event types (content_block_start/delta/stop, message_delta, message_stop), extended thinking support, and prompt caching via `cache_control` markers. This is the most complex provider because Anthropic's API has unique content block structures that differ from OpenAI's format.

---

## Definition of Done

- [ ] Implements the `Provider` interface from [[layer-2-epic-01-provider-types]]
- [ ] `Complete` method: sends a full Messages API request, receives response, parses content blocks (text, tool_use, thinking) into unified `Response` type
- [ ] `Stream` method: opens SSE connection, parses all event types (`content_block_start`, `content_block_delta`, `content_block_stop`, `message_delta`, `message_stop`), emits unified `StreamEvent` values on the returned channel
- [ ] `Models` method: returns available Claude models with context window sizes
- [ ] Request building includes correct headers: `Authorization: Bearer` (from credential manager), `anthropic-version`, `anthropic-beta` headers for OAuth and extended thinking
- [ ] Extended thinking support: sends `thinking` parameter when configured, parses thinking content blocks from responses, emits `ThinkingDelta` stream events
- [ ] Prompt caching: sets `cache_control` markers on system prompt blocks per the three-breakpoint strategy from [[05-agent-loop]]
- [ ] Content block parsing: Anthropic responses with mixed text + tool_use + thinking blocks are correctly decomposed into sirtopham's unified types
- [ ] Tool call parsing: `tool_use` content blocks → unified `ToolCall` type with ID, name, input JSON
- [ ] Usage extraction: input tokens, output tokens, cache read tokens, cache creation tokens from response metadata
- [ ] Error handling per [[03-provider-architecture]]: 401/403 → actionable auth error (no retry), 429 → exponential backoff (3 attempts), 500/502/503 → backoff retry, malformed response → log raw + structured error
- [ ] Context cancellation: `Complete` and `Stream` respect `ctx.Done()` and close HTTP connections cleanly
- [ ] Integration test against a mock HTTP server that returns realistic Anthropic SSE streams
- [ ] Unit tests for content block parsing, SSE event parsing, error classification

---

## Architecture References

- [[03-provider-architecture]] — "Provider 1: Anthropic" section. Full specification: credential flow, API call format, content blocks, streaming format, concerns.
- [[03-provider-architecture]] — "Error Handling" section. Per-status-code behavior.
- [[05-agent-loop]] — "System Prompt Construction" section. Three cache breakpoints (base prompt, assembled context, conversation history).
- [[05-agent-loop]] — "Response Handling" section. Text blocks, thinking blocks, tool use blocks — how the agent loop consumes provider output.
- [[05-agent-loop]] — "Prompt Caching Strategy" section. Cache layout and `cache_control` marker placement.
- [[06-context-assembly]] — "Prompt Cache Layout" section. Cache breakpoint 2 (assembled context) placement.

---

## Notes for the Implementing Agent

This is the highest-complexity epic in Layer 2. The SSE streaming parser is the hardest part — Anthropic's streaming format uses nested event types where a `content_block_start` announces the block type (text, tool_use, thinking), subsequent `content_block_delta` events carry incremental content, and `content_block_stop` closes the block. The parser must maintain state across events to know which block type each delta belongs to.

The `cache_control` markers go on system message content blocks. The exact API mechanism is: include `{"type": "ephemeral"}` as the `cache_control` field on the content blocks that should serve as cache breakpoints. The system message may contain multiple text blocks (base prompt, assembled context), each potentially cache-marked.

For the `Complete` method, the response is the same format as streaming but delivered as a single JSON response. Parse the same content block structure, just without the SSE event wrapping.

Hermes reference: `agent/anthropic_adapter.py`. The streaming parser there handles the same event types.
