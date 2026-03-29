# 03 — Provider Architecture

**Status:** Draft v0.1
**Last Updated:** 2026-03-27
**Author:** Mitchell

---

## Overview

sirtopham's provider layer is responsible for all communication with LLM inference services. The key architectural insight — derived from studying Hermes Agent's implementation — is that the primary inference path does not use per-token API keys. Instead, it reuses OAuth credentials from existing CLI tool subscriptions (Claude Code, OpenAI Codex), making direct API calls with those tokens.

This document covers the unified provider interface, the three backend implementations, credential lifecycle management, and routing logic.

---

## Unified Provider Interface

All providers implement a common Go interface. The agent loop and every other layer that needs LLM inference only knows about this interface.

```go
type Provider interface {
    // Complete sends a request and returns the full response.
    // Used for non-streaming contexts (context assembly classification, etc.)
    Complete(ctx context.Context, req *Request) (*Response, error)

    // Stream sends a request and returns a channel of streaming events.
    // Used for user-facing responses in the web UI.
    Stream(ctx context.Context, req *Request) (<-chan StreamEvent, error)

    // Models returns the list of available models for this provider.
    Models(ctx context.Context) ([]Model, error)

    // Name returns the provider identifier (e.g., "anthropic", "codex", "local").
    Name() string
}
```

**Request** contains: messages (conversation history), tools (JSON Schema tool definitions), model identifier, temperature, max tokens, and provider-specific options.

**Response** contains: assistant message (text + tool calls), usage (input/output tokens), model used, latency, and stop reason.

**StreamEvent** is a union type: `TokenDelta`, `ToolCallStart`, `ToolCallDelta`, `ToolCallEnd`, `Usage`, `Error`, `Done`.

This interface must be sufficient for all three providers. The design constraint is: design around the hardest provider (Anthropic's native API with its unique content block structure), not the easiest.

---

## Provider 1: Anthropic (Claude via Subscription OAuth)

### How It Works

The developer has a Claude Pro/Max subscription and uses Claude Code. Claude Code stores OAuth credentials on disk. sirtopham discovers these credentials and uses them to make direct Anthropic Messages API calls.

### Credential Discovery

Claude Code stores credentials at `~/.claude/.credentials.json`. The file contains OAuth tokens (access token, refresh token, expiry). sirtopham reads this file directly.

**Discovery order** (matches Hermes's precedence):
1. `ANTHROPIC_API_KEY` environment variable — if set, use as a traditional API key (X-Api-Key header). This is the escape hatch for users who want to pay per-token.
2. `~/.claude/.credentials.json` — OAuth tokens from Claude Code. Preferred path for subscription users.

### Token Refresh

OAuth tokens expire. sirtopham must handle refresh transparently:
- Before each API call, check if the access token is expired or near expiry (within 2-minute buffer).
- If expired, use the refresh token to obtain a new access token via Anthropic's OAuth token endpoint.
- Write the refreshed credentials back to `~/.claude/.credentials.json` (so Claude Code also benefits from the refresh).
- If refresh fails (revoked, network error), surface a clear error to the user: "Claude credentials expired. Run `claude login` to re-authenticate."

### API Call Format

Anthropic's Messages API with OAuth-specific headers:

```
POST https://api.anthropic.com/v1/messages
Authorization: Bearer <access_token>
anthropic-version: 2023-06-01
anthropic-beta: interleaved-thinking-2025-05-14,oauth-2025-04-20
Content-Type: application/json
```

**Key differences from API-key auth:**
- Uses `Authorization: Bearer` header instead of `X-Api-Key`.
- Requires OAuth-specific beta headers.
- May require additional beta header for Claude Code compatibility (`claude-code-20250219` — needs verification).

### Anthropic-Specific Concerns

- **Content block structure:** Anthropic responses contain typed content blocks (text, tool_use, thinking). The provider must parse these into sirtopham's unified response format.
- **Prompt caching:** Anthropic supports prompt caching that dramatically reduces costs on long conversations. The provider should set cache control markers on the system prompt and early conversation turns. Since we're on a subscription this may be less critical for cost, but it improves latency.
- **Streaming format:** Anthropic uses Server-Sent Events with specific event types (content_block_start, content_block_delta, content_block_stop, message_delta, message_stop). The streaming parser must handle all of these.

---

## Provider 2: OpenAI Codex (via CLI Credential Delegation)

### How It Works

The developer has a Codex subscription and uses the Codex CLI. Unlike the Anthropic path, sirtopham delegates credential management to the Codex CLI binary rather than managing tokens directly.

### Credential Flow

Hermes's approach (which we replicate):
1. Check if `codex` CLI is installed and credentials exist at `~/.codex/auth.json`.
2. Before each inference call, run `codex refresh` to ensure the JWT is valid.
3. Read the access token from `~/.codex/auth.json`.
4. Make direct API calls to OpenAI's endpoints using that token.

**Why delegate to the CLI?** OpenAI's OAuth flow for Codex is more complex than Anthropic's, and the Codex CLI handles device-code auth, token storage, and refresh. Reimplementing this would be fragile and would break whenever OpenAI changes their auth flow. Delegating to the CLI is the pragmatic choice.

### API Call Format

Codex uses OpenAI's Responses API (not Chat Completions):

```
POST https://api.openai.com/v1/responses
Authorization: Bearer <access_token>
Content-Type: application/json
```

**Key differences from standard OpenAI Chat Completions:**
- Different endpoint (`/v1/responses` vs `/v1/chat/completions`).
- Different request/response schema. The Responses API uses a different message format.
- Supports encrypted reasoning content (Codex can return encrypted chain-of-thought that we preserve in history but don't display).

### Codex-Specific Concerns

- **CLI dependency:** The `codex` binary must be installed. sirtopham should check for this at startup and provide a clear error if missing.
- **Refresh latency:** Shelling out to `codex refresh` adds latency before each call. May want to cache the token and only refresh when near expiry, checking the expiry timestamp in auth.json.
- **Model selection:** Codex exposes multiple models (GPT-5 variants). The user should be able to select which model to use. Auto-detection from the Codex API is possible.

---

## Provider 3: OpenAI-Compatible (Local Models, OpenRouter, Custom)

### How It Works

A standard OpenAI Chat Completions-compatible client that works with any endpoint implementing the `/v1/chat/completions` API.

### Use Cases

- **Local models:** Qwen2.5-Coder-7B, DeepSeek-R1-32B, Mercury 2, or any model running in a Docker container or via Ollama, exposed on localhost.
- **OpenRouter:** Access to 200+ models via a single API key. Pay-per-token but useful for fallback or experimentation.
- **Any OpenAI-compatible endpoint:** vLLM, llama.cpp server, text-generation-inference, etc.

### Configuration

```yaml
providers:
  local:
    type: openai-compatible
    base_url: http://localhost:8080/v1
    model: qwen2.5-coder-7b
    # No API key needed for local
  openrouter:
    type: openai-compatible
    base_url: https://openrouter.ai/api/v1
    api_key_env: OPENROUTER_API_KEY
    model: anthropic/claude-sonnet-4
```

### Concerns

- **Tool calling support varies wildly.** Some local models support tool calling natively, some don't, some sort-of do. The provider should gracefully handle models that don't support tools (fall back to prompt-based tool use, or surface a warning).
- **Streaming format is standardized** for OpenAI-compatible endpoints, so this is the simplest provider to implement.
- **Context length varies by model.** Config should allow specifying context_length per provider/model.

---

## Provider Router

The router selects which provider handles a given request.

### Routing Logic (v0.1 — Manual)

For v0.1, routing is explicit:
- **Default provider** is configured in `sirtopham.yaml` (e.g., anthropic with claude-sonnet-4-6).
- **User override** per-conversation or per-turn via the web UI.
- **Fallback** if the primary provider fails (rate limit, auth error): configurable backup provider.

```yaml
routing:
  default:
    provider: anthropic
    model: claude-sonnet-4-6
  fallback:
    provider: local
    model: qwen2.5-coder-7b
```

### Routing Logic (Future — Automatic)

Future versions could classify turn complexity and route accordingly:
- Simple factual questions → local model (free, fast)
- Code generation, complex refactoring → frontier model (high quality)
- Classification could be done by a tiny local model or by keyword heuristics.

This is explicitly out of scope for v0.1. Start manual, automate later.

---

## Credential Storage & Security

**Principle:** sirtopham does not store credentials itself. It reads them from the credential stores managed by Claude Code and Codex CLI.

- Anthropic credentials: read from `~/.claude/.credentials.json` (managed by Claude Code)
- Codex credentials: read from `~/.codex/auth.json` (managed by Codex CLI)
- API keys (OpenRouter, etc.): read from environment variables or `sirtopham.yaml`
- No credentials are stored in sirtopham's own database or config files (except API keys for optional services that don't have a CLI tool).

**File locking:** When reading/writing credential files (especially for Anthropic token refresh), use advisory file locking to avoid races with Claude Code itself accessing the same file.

---

## Error Handling

Each provider must handle:

- **Auth failure (401/403):** Surface a clear message identifying which provider failed and how to re-authenticate. Do not retry auth failures.
- **Rate limiting (429):** Retry with exponential backoff. Surface to user if retries exhausted. Consider falling back to the configured fallback provider.
- **Server error (500/502/503):** Retry with backoff (3 attempts max). Fall back if retries exhausted.
- **Network error:** Same as server error.
- **Token refresh failure:** Surface the error with specific remediation ("Run `claude login`" or "Run `codex auth`").
- **Malformed response:** Log the raw response, return a structured error. Do not crash.

---

## Cost Tracking

Even though subscription-based access has no per-token cost, tracking token usage is valuable for:
- Understanding conversation complexity
- Comparing model efficiency
- Detecting context window pressure
- Informing future routing decisions

Every provider call records to the `sub_calls` SQLite table:
- Provider name
- Model used
- Input tokens
- Output tokens
- Latency (ms)
- Conversation ID
- Turn number
- Success/failure
- Error message (if any)

---

## Implementation Priority

1. **Anthropic provider** — Primary inference path, highest quality, most complex (OAuth, content blocks, streaming SSE).
2. **OpenAI-compatible provider** — Simplest to implement, enables local models immediately.
3. **Codex provider** — Requires Codex CLI dependency, Responses API adapter. Implement after the other two are working.

---

## Open Questions

- **Anthropic beta headers:** Need to verify the exact set of beta headers required for OAuth access. Hermes uses `claude-code-20250219` and `oauth-2025-04-20` — are these stable or do they change?
- **Codex Responses API stability:** The Responses API is relatively new. How stable is the schema? Do we need to handle version differences?
- **Prompt caching with OAuth:** Does Anthropic prompt caching work the same way with OAuth tokens as with API keys? Needs testing.
- **Copilot as a fourth provider?** Hermes supports GitHub Copilot (separate from Codex). Worth considering if the developer has a Copilot subscription, but lower priority.

---

## References

- Hermes Agent provider resolution: `hermes_cli/runtime_provider.py`, `hermes_cli/auth.py`
- Hermes Anthropic adapter: `agent/anthropic_adapter.py`
- Hermes Codex adapter: `agent/codex_responses.py`
- Claude Code authentication docs: https://code.claude.com/docs/en/authentication
- Anthropic Messages API: https://docs.anthropic.com/en/api/messages
- DeepWiki Hermes provider analysis: https://deepwiki.com/NousResearch/hermes-agent/4.2-provider-resolution-and-api-adapters
- DeepWiki Hermes auth analysis: https://deepwiki.com/NousResearch/hermes-agent/2.4-authentication-and-providers
