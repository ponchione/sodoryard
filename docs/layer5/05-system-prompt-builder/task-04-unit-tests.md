# Task 04: Unit Tests

**Epic:** 05 — System Prompt Builder
**Status:** ⬚ Not started
**Dependencies:** Tasks 01-03

---

## Description

Write comprehensive unit tests for the PromptBuilder verifying cache breakpoint placement, block ordering, provider-specific behavior, edge cases, and tool schema injection. These tests ensure the prompt construction is correct for all provider configurations and content combinations, which is critical since incorrect prompt assembly can cause cache misses (latency regression), API rejections (malformed messages), or context corruption.

## Acceptance Criteria

- [ ] **Cache breakpoint placement test (Anthropic):** build a prompt with all regions populated (base prompt, context, history, current turn). Verify exactly three `cache_control` markers are present. Verify marker 1 is on the last content block of the base prompt, marker 2 is on the last content block of the assembled context, marker 3 is on the last message of the history prefix. Verify no markers on current turn messages
- [ ] **Block ordering test:** build a prompt and inspect the message array. Verify ordering is: system content (base prompt + context) → history messages (in sequence order) → current turn messages. Verify system content appears as system-role content blocks, not user messages
- [ ] **Provider branching test (Anthropic):** build with `ProviderName = "anthropic"` — verify cache markers present
- [ ] **Provider branching test (OpenAI):** build with `ProviderName = "openai"` — verify no cache markers, but identical content and ordering
- [ ] **Provider branching test (local):** build with `ProviderName = "local"` — verify no cache markers
- [ ] **Empty context test:** build with nil `ContextPackage` — verify block 2 is omitted entirely (no empty content block), cache marker 1 covers base prompt, next marker is on history
- [ ] **Empty history test (first turn):** build with empty `History` — verify only blocks 1 and 2 have cache markers (block 3 has nothing to mark), current turn messages follow immediately after system content
- [ ] **History growth stability test:** build prompt A with history of 5 messages. Build prompt B with history of 7 messages (first 5 identical). Verify that the content of the first 5 history messages is byte-identical between A and B (prefix stability for cache hits)
- [ ] **Tool schema injection test:** build with 3 tool schemas — verify `Request.Tools` contains all 3 with correct name, description, and input schema. Build with `DisableTools = true` — verify `Request.Tools` is empty
- [ ] **Extended thinking test:** verify the Request includes extended thinking configuration for Anthropic, and omits it for providers that don't support it
- [ ] All tests pass with `go test ./internal/agent/...`
