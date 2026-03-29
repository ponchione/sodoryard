# Layer 2, Epic 05: Codex Provider

**Layer:** 2 — Provider Architecture
**Status:** ⬜ Not started
**Dependencies:** [[layer-2-epic-01-provider-types]], Layer 0 Epic 02 (logging), Layer 0 Epic 03 (config)

---

## Description

Implement the Codex provider — OpenAI's Responses API accessed via credentials delegated to the `codex` CLI binary. This covers the credential delegation flow (checking CLI availability, running `codex refresh`, reading tokens from `~/.codex/auth.json`), translating sirtopham's unified request format to the Responses API schema (which differs from Chat Completions), parsing responses including encrypted reasoning content, and streaming. The Codex provider is the third-priority implementation per [[03-provider-architecture]].

---

## Definition of Done

- [ ] Implements the `Provider` interface from [[layer-2-epic-01-provider-types]]
- [ ] Startup check: verifies `codex` CLI binary is available on PATH; returns a clear error if missing ("Codex CLI not found. Install it from https://... and run `codex auth`.")
- [ ] Credential delegation: shells out to `codex refresh` before API calls when the token is expired or near-expiry
- [ ] Token caching: reads `~/.codex/auth.json`, extracts access token and expiry, caches in memory, only refreshes when near-expiry (avoids shelling out on every call)
- [ ] `Complete` method: sends Responses API request to `POST https://api.openai.com/v1/responses`, parses response into unified `Response` type
- [ ] `Stream` method: streaming Responses API, parses events, emits unified `StreamEvent` values
- [ ] `Models` method: returns available Codex models (GPT-5 variants) with context window sizes
- [ ] Request translation: sirtopham's unified `Request` type → Responses API format (different message schema from Chat Completions)
- [ ] Response translation: Responses API format → sirtopham's unified types (text + tool calls + encrypted reasoning)
- [ ] Encrypted reasoning: preserves encrypted chain-of-thought content blocks in the response (stored in conversation history per [[08-data-model]] but not displayed in the UI)
- [ ] Tool calling: translates sirtopham's tool definitions to Responses API tool format; parses tool calls from responses
- [ ] Error handling: auth failures → "Run `codex auth` to re-authenticate", 429 → backoff, 500+ → retry, per [[03-provider-architecture]]
- [ ] `codex refresh` failure handling: if the CLI exits non-zero, surface the stderr as an actionable error
- [ ] Context cancellation respected
- [ ] Unit tests for request/response translation, token caching logic
- [ ] Integration test against a mock HTTP server for the Responses API format

---

## Architecture References

- [[03-provider-architecture]] — "Provider 2: OpenAI Codex" section. Full specification: credential flow, API call format, Responses API differences, concerns (CLI dependency, refresh latency, model selection).
- [[03-provider-architecture]] — "Credential Storage & Security" section. Codex credentials at `~/.codex/auth.json`.
- [[03-provider-architecture]] — "Error Handling" section.
- [[02-tech-stack-decisions]] — "Frontier LLM Access" section. Subscription credential reuse rationale.

---

## Notes for the Implementing Agent

The Responses API (`/v1/responses`) has a different schema from Chat Completions (`/v1/chat/completions`). Do NOT use the OpenAI-compatible provider as a base — the request and response formats differ enough that the translation layer is distinct. Key differences: the message format is different, tool definitions use a different structure, and the response includes response-level metadata that Chat Completions doesn't have.

The `codex refresh` shell-out adds latency (~100-300ms). The token caching strategy is critical: read `~/.codex/auth.json` once, extract the expiry, and only shell out again when the token is within 2 minutes of expiry. This matches the Anthropic credential manager's buffer strategy.

Hermes reference: `agent/codex_responses.py` for the Responses API adapter and `hermes_cli/runtime_provider.py` for the credential delegation pattern.

Encrypted reasoning content (Codex's chain-of-thought) should be preserved as-is in the assistant message content blocks. It's opaque data — store it, pass it through, don't try to decrypt or display it. The data model's `json.RawMessage` content storage handles this naturally.
