# Layer 2, Epic 02: Anthropic Credential Manager

**Layer:** 2 — Provider Architecture
**Status:** ⬜ Not started
**Dependencies:** [[layer-2-epic-01-provider-types]], Layer 0 Epic 02 (logging), Layer 0 Epic 03 (config)

---

## Description

Implement the Anthropic credential lifecycle: discovery from `~/.claude/.credentials.json` or `ANTHROPIC_API_KEY` environment variable, OAuth token validation, transparent refresh via Anthropic's OAuth token endpoint, write-back of refreshed credentials, and advisory file locking to avoid races with Claude Code accessing the same file. This is an isolated component consumed by the Anthropic provider — separating it keeps the credential complexity out of the API call path.

---

## Definition of Done

- [ ] Credential discovery follows the precedence order from [[03-provider-architecture]]: (1) `ANTHROPIC_API_KEY` env var → API key mode, (2) `~/.claude/.credentials.json` → OAuth mode
- [ ] Reads and parses `~/.claude/.credentials.json` — extracts access token, refresh token, expiry timestamp
- [ ] Detects expired or near-expiry tokens (within 2-minute buffer)
- [ ] Performs OAuth token refresh using the refresh token against Anthropic's token endpoint
- [ ] Writes refreshed credentials back to `~/.claude/.credentials.json`
- [ ] Uses advisory file locking (flock) when reading/writing the credential file to avoid races with Claude Code
- [ ] Returns the correct auth header format: `Authorization: Bearer <token>` for OAuth, `X-Api-Key: <key>` for API key mode
- [ ] Returns clear, actionable error messages on failure: "Claude credentials expired. Run `claude login` to re-authenticate." / "~/.claude/.credentials.json not found. Install Claude Code and run `claude login`."
- [ ] Thread-safe — safe to call from concurrent goroutines (the agent loop may make overlapping provider calls)
- [ ] Unit tests with mock credential files (both valid and expired scenarios)
- [ ] Unit tests for refresh flow (mock HTTP responses from token endpoint)

---

## Architecture References

- [[03-provider-architecture]] — "Credential Discovery" and "Token Refresh" sections under Provider 1: Anthropic.
- [[03-provider-architecture]] — "Credential Storage & Security" section. File locking requirement.
- [[03-provider-architecture]] — "API Call Format" section. OAuth vs API-key header differences.
- [[01-project-vision-and-principles]] — "Zero API cost for inference" principle. OAuth credential reuse is the primary access path.

---

## Notes for the Implementing Agent

The credential manager should expose a simple interface — something like `GetAuthHeader(ctx) (string, string, error)` returning header name and value. The Anthropic provider calls this before each API request. The manager handles caching, expiry checking, and refresh internally.

Hermes Agent's implementation is the reference: `hermes_cli/auth.py` and `hermes_cli/runtime_provider.py`. The logic is being reimplemented in Go, not ported line-by-line. Key Hermes patterns to replicate: the precedence order (env var → credential file), the 2-minute expiry buffer, and the write-back after refresh.

The beta headers (`oauth-2025-04-20`, `claude-code-20250219`) are part of the API call, not the credential manager. Those belong in [[layer-2-epic-03-anthropic-provider]].
