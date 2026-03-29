# Layer 2, Epic 06: Sub-Call Tracking

**Layer:** 2 — Provider Architecture
**Status:** ⬜ Not started
**Dependencies:** [[layer-2-epic-01-provider-types]], Layer 0 Epic 02 (logging), Layer 0 Epic 04 (SQLite connection manager), Layer 0 Epic 06 (schema & sqlc — `sub_calls` table)

---

## Description

Implement the cross-cutting sub-call tracking layer that wraps any provider to record every LLM invocation to the `sub_calls` SQLite table. This captures provider name, model, input/output tokens, cache read/creation tokens, latency, success/failure, error messages, and purpose classification (chat, compression, title_generation). The tracker is a decorator/middleware that sits between the router and the underlying providers — it does not modify request or response behavior, only observes and records.

---

## Definition of Done

- [ ] Tracking wrapper implements the `Provider` interface from [[layer-2-epic-01-provider-types]] — wraps any inner provider transparently
- [ ] Every `Complete` call records a row to `sub_calls`: provider, model, tokens_in, tokens_out, cache_read_tokens, cache_creation_tokens, latency_ms, success (0/1), error_message, purpose, created_at
- [ ] Every `Stream` call records a row to `sub_calls` after the stream completes (usage data arrives in the final stream events)
- [ ] `purpose` field populated from request metadata — the caller specifies whether this is a "chat", "compression", or "title_generation" call
- [ ] `conversation_id`, `turn_number`, `iteration`, and `message_id` fields populated from request context when available (nullable for non-conversation calls like compression)
- [ ] Latency measured as wall-clock time from request start to response complete (for `Complete`) or stream-done event (for `Stream`)
- [ ] Cache token columns (`cache_read_tokens`, `cache_creation_tokens`) extracted from Anthropic usage metadata; zero for providers that don't report cache stats
- [ ] Failed calls (HTTP errors, timeouts, parse errors) still recorded with `success=0` and the error message
- [ ] Tracking failures (SQLite write errors) are logged but do NOT cause the provider call itself to fail — the tracking layer must never break inference
- [ ] Uses sqlc-generated code from Layer 0 Epic 06 for database writes
- [ ] Unit tests: verify that wrapping a mock provider produces correct `sub_calls` rows for success, failure, and streaming scenarios
- [ ] Unit test: verify that a SQLite write failure during tracking does not propagate to the caller

---

## Architecture References

- [[03-provider-architecture]] — "Cost Tracking" section. What to record and why.
- [[08-data-model]] — `sub_calls` table schema. Column definitions including `cache_read_tokens`, `cache_creation_tokens`, `purpose`, `message_id`, `iteration`.
- [[05-agent-loop]] — "Persistence" section. Sub-calls recorded per iteration, `message_id` links to the assistant message. Persisted in the same transaction as messages.
- [[06-context-assembly]] — Compression calls have `purpose='compression'`.

---

## Notes for the Implementing Agent

The tracking wrapper pattern is straightforward: implement `Provider`, delegate all calls to the inner provider, measure timing and extract usage from the response, write to SQLite. The key subtlety is streaming — usage data for streamed responses arrives in the final `Usage` event, not upfront. The tracker must consume the stream, extract usage when the `Done` event arrives, write the record, and re-emit all events to the downstream consumer unchanged.

There's a transaction coordination question with [[05-agent-loop]]: the agent loop persists messages and sub_calls in a single atomic transaction per iteration. The tracker should support this by allowing the caller to provide a transaction or by returning the sub-call data for the caller to persist. The simplest approach: the tracker writes its own row immediately (fire-and-forget to SQLite), and the agent loop's transaction updates the `message_id` link afterward. Alternatively, the tracker returns sub-call metadata and the agent loop handles all persistence. Either works — the implementing agent should pick the simpler path. The critical constraint is that tracking failures never block inference.

The `purpose` field is caller-supplied, not inferred. The agent loop passes "chat", the compression system passes "compression", title generation passes "title_generation". Add a field to `Request` or use Go context values to carry this.
