# Task 03: Provider-Specific Cache Behavior and Tool Schema Injection

**Epic:** 05 — System Prompt Builder
**Status:** ⬚ Not started
**Dependencies:** Task 02, Layer 4 Epic 01 (tool Registry — Schemas)

---

## Description

Implement provider-specific branching for cache marker behavior and tool schema injection into the provider Request. Different providers handle caching differently: Anthropic requires explicit `cache_control` markers, OpenAI/Codex uses automatic prefix caching (no markers needed), and local/OpenAI-compatible providers have no caching support. The builder must select the correct behavior based on the provider name in the config. Additionally, tool schemas from the tool registry are injected into the Request's tools field regardless of provider.

## Acceptance Criteria

- [ ] **Anthropic provider:** all three `cache_control` markers from Task 02 are included in the Request. This is the full cache layout behavior
- [ ] **OpenAI/Codex provider:** no `cache_control` markers are included — OpenAI handles prefix caching automatically. The message array structure is identical (same ordering, same content) but without cache control fields
- [ ] **Local/OpenAI-compatible provider:** no `cache_control` markers are included. The Request is constructed with the same content but no caching hints
- [ ] Provider selection is based on `PromptConfig.ProviderName` string matching (e.g., `"anthropic"`, `"openai"`, `"local"`)
- [ ] Unknown provider names default to no caching (same as local) with a debug log
- [ ] **Tool schema injection:** `PromptConfig.ToolSchemas` are mapped to the `Request.Tools` field in the provider-expected format. Each tool schema includes name, description, and input JSON schema
- [ ] Tool schemas are included regardless of provider — all providers support tool/function calling
- [ ] On the final iteration (when tools should be disabled per the agent loop's iteration limit), the builder respects an optional `DisableTools bool` field in `PromptConfig` — when true, the `Request.Tools` field is empty/nil, forcing a text-only response
- [ ] Extended thinking configuration is included in the Request for providers that support it (Anthropic). For providers that don't support extended thinking, the field is omitted
- [ ] Package compiles with `go build ./internal/agent/...`
