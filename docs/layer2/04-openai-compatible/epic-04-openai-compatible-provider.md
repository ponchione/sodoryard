# Layer 2, Epic 04: OpenAI-Compatible Provider

**Layer:** 2 — Provider Architecture
**Status:** ⬜ Not started
**Dependencies:** [[layer-2-epic-01-provider-types]], Layer 0 Epic 02 (logging), Layer 0 Epic 03 (config)

---

## Description

Implement the OpenAI-compatible provider — a standard Chat Completions API client that works with any endpoint implementing `/v1/chat/completions`. This is the simplest provider and covers local models (Qwen, DeepSeek, Mercury via Docker/Ollama on localhost), OpenRouter, vLLM, llama.cpp server, and any other OpenAI-compatible endpoint. Configuration-driven: base URL, optional API key (from env var or config), model name, and context length are all per-instance settings.

---

## Definition of Done

- [ ] Implements the `Provider` interface from [[layer-2-epic-01-provider-types]]
- [ ] `Complete` method: sends Chat Completions request, parses response into unified `Response` type
- [ ] `Stream` method: opens SSE connection for streaming Chat Completions, parses `data: {...}` lines, emits unified `StreamEvent` values
- [ ] `Models` method: returns configured model(s) with context window size from config
- [ ] Configurable per-instance: `base_url`, `api_key` (optional — not needed for localhost), `api_key_env` (env var name), `model`, `context_length`
- [ ] Supports multiple named instances (e.g., "local" on localhost:8080, "openrouter" on openrouter.ai) — each is a separate provider instance
- [ ] Request translation: sirtopham's unified `Request` type → OpenAI Chat Completions format (messages array, tools array, model, temperature, max_tokens)
- [ ] Response translation: OpenAI response format → sirtopham's unified `Response` type (assistant message with text + tool calls, usage)
- [ ] Tool calling: translates sirtopham's tool definitions to OpenAI's function calling format; parses tool_calls from responses
- [ ] Graceful handling of models that don't support tool calling: if tool calls are in the request but the model returns plain text, surface this cleanly (no crash, log a warning)
- [ ] Streaming format: standard OpenAI SSE (`data: {"choices":[{"delta":...}]}`) parsed correctly
- [ ] Error handling: 401/403 → auth error, 429 → exponential backoff, 500+ → retry with backoff, per [[03-provider-architecture]]
- [ ] Context cancellation respected on both `Complete` and `Stream`
- [ ] Unit tests for request/response translation
- [ ] Integration test against a mock HTTP server returning realistic OpenAI-format responses

---

## Architecture References

- [[03-provider-architecture]] — "Provider 3: OpenAI-Compatible" section. Use cases, configuration format, concerns (tool calling variance, streaming standardization, context length).
- [[03-provider-architecture]] — "Error Handling" section.
- [[02-tech-stack-decisions]] — "Local LLM Inference" section. Docker container on port 8080, configurable model.

---

## Notes for the Implementing Agent

This is the simplest provider. The OpenAI Chat Completions streaming format is well-documented and widely implemented. The main translation work is mapping between sirtopham's Anthropic-influenced type system (content blocks with explicit types) and OpenAI's simpler format (message.content as a string, tool_calls as a separate field).

The provider should be instantiated per-configuration-entry. If the config has both a "local" and an "openrouter" entry of type `openai-compatible`, each gets its own provider instance with its own base URL and credentials. The router ([[layer-2-epic-07-provider-router]]) handles selecting between them.

The `context_length` field in config is important — local models have wildly different context sizes (4k for some, 128k for others). This value feeds into the `Model` struct consumed by [[06-context-assembly]]'s budget manager.
