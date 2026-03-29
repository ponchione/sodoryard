# Layer 2, Epic 01: Provider Types & Interface

**Layer:** 2 — Provider Architecture
**Status:** ⬜ Not started
**Dependencies:** Layer 0 Epic 01 (project scaffolding), Layer 0 Epic 03 (config loading)

---

## Description

Define the unified `Provider` interface and all shared types that every provider implements and every consumer depends on. This includes `Request`, `Response`, `StreamEvent` (union type), `Model`, `Usage`, `Message`, `ContentBlock`, `ToolCall`, `ToolResult`, and provider-specific option types. The interface must be designed around the hardest provider (Anthropic's content block structure), not the easiest. No provider implementations — just the contract and the types.

This is the package `internal/provider/` with types in a shared location that avoids circular imports when provider implementations live in sub-packages (`internal/provider/anthropic/`, `internal/provider/openai/`, `internal/provider/codex/`).

---

## Definition of Done

- [ ] `internal/provider/` package exists with exported types
- [ ] `Provider` interface defined with `Complete`, `Stream`, `Models`, and `Name` methods matching [[03-provider-architecture]]
- [ ] `Request` struct defined: messages (conversation history), tools (JSON Schema definitions), model identifier, temperature, max tokens, system prompt (with cache_control support), provider-specific options
- [ ] `Response` struct defined: assistant message (text + tool calls + thinking blocks), usage (input/output tokens, cache read/creation tokens), model used, latency, stop reason
- [ ] `StreamEvent` type hierarchy defined: `TokenDelta`, `ThinkingDelta`, `ToolCallStart`, `ToolCallDelta`, `ToolCallEnd`, `Usage`, `Error`, `Done` — matching the event types from [[05-agent-loop]]
- [ ] `Message` type with role discrimination (user/assistant/tool) and content blocks — matches the API-faithful storage model from [[08-data-model]]
- [ ] `ContentBlock` types: text, thinking, tool_use — designed to round-trip with Anthropic's content block structure
- [ ] `Model` struct with: ID, name, context window size, supports_tools flag, supports_thinking flag
- [ ] Types compile cleanly with no external dependencies beyond stdlib
- [ ] Package layout supports sub-packages for provider implementations without circular imports

---

## Architecture References

- [[03-provider-architecture]] — "Unified Provider Interface" section. Interface definition, Request/Response/StreamEvent specs.
- [[05-agent-loop]] — "Streaming to the Web UI" section. StreamEvent types must align with the agent loop's event emission.
- [[05-agent-loop]] — "System Prompt Construction" section. Request must support cache_control markers on system prompt blocks.
- [[08-data-model]] — "Message Storage Model" section. Message/ContentBlock types must be compatible with API-faithful persistence.
- [[06-context-assembly]] — "Budget Manager" section. Model.ContextWindowSize consumed by the budget manager.

---

## Notes for the Implementing Agent

The `StreamEvent` types defined here are sirtopham's internal representation. They are NOT the raw SSE events from Anthropic or the raw streaming chunks from OpenAI. Each provider's streaming parser translates from its wire format into these unified events. Design them to carry everything the agent loop and web UI need, not to mirror any specific provider's format.

The `Message` type should use `json.RawMessage` for assistant content blocks, matching the persistence strategy from [[08-data-model]] — content blocks are stored as JSON arrays and passed through without transformation on the hot path.

Context window sizes per model are needed by [[06-context-assembly]]'s budget manager. The `Model` struct should carry this information so the router can surface it to consumers.
