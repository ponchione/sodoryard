# Task 01: PromptBuilder Struct and BuildPrompt Method

**Epic:** 05 — System Prompt Builder
**Status:** ⬚ Not started
**Dependencies:** L5-E01 (types), L5-E02 (Message types), Layer 3 Epic 01 (FullContextPackage), Layer 2 Epic 01 (provider types)

---

## Description

Implement the `PromptBuilder` struct and its `BuildPrompt` method in `internal/agent/prompt.go`. This is the core component that assembles the final `provider.Request` from all its constituent parts: the base system prompt, assembled context, conversation history, current turn messages, and tool schemas. The builder produces a correctly ordered message array with the four content regions (base prompt, assembled context, history prefix, fresh content) and includes tool definitions. This task implements the basic assembly logic without cache markers — cache marker placement is handled in Task 02.

## Acceptance Criteria

- [ ] `PromptConfig` struct defined with fields: `BasePrompt string`, `ContextPackage *FullContextPackage` (nullable — may be nil if context assembly produced nothing), `History []Message` (reconstructed from conversation manager), `CurrentTurnMessages []Message` (user message + in-progress tool results), `ToolSchemas []ToolSchema` (from registry), `ProviderName string`, `ModelName string`, `ContextLimit int`
- [ ] `PromptBuilder` struct with configuration: base system prompt text (embedded or from config), extended thinking settings
- [ ] `NewPromptBuilder(basePrompt string) *PromptBuilder` constructor
- [ ] `BuildPrompt(config PromptConfig) (*provider.Request, error)` method that constructs the provider Request:
  - System message content: base prompt text, then assembled context text (if non-empty), forming the system-level content blocks
  - Conversation messages: history messages in sequence order, then current turn messages
  - Tools field: populated from `config.ToolSchemas`
  - Extended thinking: enabled by default in the request configuration
- [ ] If `ContextPackage` is nil or has empty `ContextText`, block 2 is omitted entirely — no empty content block in the system message
- [ ] Message ordering is deterministic: system content → history (ordered by sequence) → current turn messages
- [ ] The builder constructs a fresh Request each call — no mutable internal state, no cached prompt objects. This ensures compression invalidation works correctly (just rebuild from current data)
- [ ] Base system prompt text is thin (under 5k tokens): agent personality, behavioral guidelines, brief tool usage instructions
- [ ] Package compiles with `go build ./internal/agent/...`
