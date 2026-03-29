# L5-E05 — System Prompt Builder

**Layer:** 5 — Agent Loop
**Epic:** 05
**Status:** ⬚ Not started
**Dependencies:** L5-E02 (conversation history reconstruction), Layer 3 Epic 01 (FullContextPackage), Layer 3 Epic 06 (context assembly pipeline), Layer 2 Epic 01 (provider types — Request, Message, cache_control), Layer 4 Epic 01 (tool Registry — Schemas)

---

## Description

Implement the system prompt builder that constructs the three-block prompt cache layout from [[05-agent-loop]] §System Prompt Construction and [[06-context-assembly]] §Prompt Cache Layout. This component takes the base system prompt, the assembled context (FullContextPackage), conversation history (from conversation manager), and tool schemas (from tool registry), and produces the final message array with Anthropic cache_control markers at the three breakpoints. It handles the differences between providers — Anthropic requires explicit cache_control markers, OpenAI has automatic caching, and local models get no caching.

This is the bridge between context assembly (block 2) and the agent loop's LLM call (step 4). Getting the cache layout right is critical for latency — a cache hit on 50k tokens of conversation history means near-instant processing.

---

## Definition of Done

- [ ] `internal/agent/prompt.go` implements `PromptBuilder` with a method:
  ```go
  BuildPrompt(config PromptConfig) (*provider.Request, error)
  ```
  where PromptConfig includes: base system prompt text, FullContextPackage, conversation history ([]Message from reconstruction), tool schemas ([]ToolSchema from registry), current turn messages, provider name, model name, and model context limit

- [ ] **Cache Block 1 (base system prompt):** Contains agent personality, behavioral guidelines, tool usage instructions, and project conventions. Thin (under 5k tokens), identical across all calls. Marked with Anthropic `cache_control: {"type": "ephemeral"}` at the end

- [ ] **Cache Block 2 (assembled context):** Contains the serialized markdown from FullContextPackage. Frozen at turn start. Marked with a second `cache_control` marker

- [ ] **Cache Block 3 (conversation history prefix):** Contains all completed turns from prior turns. Grows monotonically. Marked with a third `cache_control` marker

- [ ] **Fresh content (uncached):** Current turn's messages — no cache marker

- [ ] Tool schemas injected into the provider Request's tools field

- [ ] **Provider-specific cache behavior:**
  - Anthropic: explicit `cache_control` markers on the three breakpoints
  - OpenAI/Codex: no markers needed — automatic prefix caching
  - Local/OpenAI-compatible: no caching — markers omitted
  - Builder selects behavior based on provider name

- [ ] Extended thinking is enabled by default

- [ ] Base system prompt template loaded from config or embedded as a Go string constant

- [ ] Unit tests verify cache breakpoint placement, block ordering, provider branching, empty context handling, history growth stability, and tool schema injection

---

## Architecture References

- [[05-agent-loop]] §System Prompt Construction — three-block layout, ordering
- [[05-agent-loop]] §Prompt Caching Strategy — cache mechanics, breakpoint placement, per-provider behavior
- [[06-context-assembly]] §Prompt Cache Layout — detailed analysis of three breakpoints, cache hit behavior
- [[06-context-assembly]] §Prompt Cache Layout §Cache Invalidation — what happens after compression

---

## Notes

- The Anthropic cache_control mechanism uses markers on content blocks within the system message. The exact API format is: include a `cache_control` field on the last content block of each cacheable section.
- The prompt builder should support cache invalidation after compression — when compression changes the history shape, the builder rebuilds from current state.
- The base system prompt should be **thin** — frontier models don't need verbose instructions.
- The `PromptBuilder` does NOT own the message array — it constructs a fresh Request each time from current state.
