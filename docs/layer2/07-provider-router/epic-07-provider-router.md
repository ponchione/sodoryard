# Layer 2, Epic 07: Provider Router

**Layer:** 2 — Provider Architecture
**Status:** ⬜ Not started
**Dependencies:** [[layer-2-epic-03-anthropic-provider]], [[layer-2-epic-04-openai-compatible-provider]], [[layer-2-epic-05-codex-provider]], [[layer-2-epic-06-sub-call-tracking]], Layer 0 Epic 02 (logging), Layer 0 Epic 03 (config)

---

## Description

Implement the provider router — the entry point that the agent loop and context assembly layer call for all LLM inference. The router selects which provider handles a given request based on configuration (default provider/model), per-request overrides (conversation or turn-level model selection from the web UI), and fallback logic (if the primary provider fails with a retriable error, try the configured fallback). For v0.1, routing is entirely manual/explicit — no automatic complexity-based classification. The router wraps selected providers with the sub-call tracker and exposes the unified `Provider` interface to consumers.

---

## Definition of Done

- [ ] `Router` struct that holds all registered provider instances and the sub-call tracker
- [ ] Initialization: reads provider config from `sirtopham.yaml`, instantiates all configured providers (Anthropic, OpenAI-compatible instances, Codex), wraps each with the sub-call tracker
- [ ] Default routing: routes to the configured `routing.default.provider` + `routing.default.model` when no override is specified
- [ ] Per-request override: accepts provider/model override on the `Request` (for conversation-level or turn-level model selection from the web UI)
- [ ] Fallback logic: if the primary provider returns a retriable error (429, 500, 502, 503, network error), automatically retry on the configured `routing.fallback.provider` + `routing.fallback.model` (if configured)
- [ ] Auth errors (401/403) are NOT retried and NOT fallen back — surfaced immediately with actionable remediation message
- [ ] Implements the `Provider` interface itself — the agent loop calls the router as if it were a single provider; routing is transparent
- [ ] `Models` method: aggregates available models from all registered providers
- [ ] Provider health: tracks which providers are currently healthy (last call succeeded) vs degraded (recent failures). Exposes this for the web UI's settings panel.
- [ ] Startup validation: verifies at least one provider is configured and reachable (credential check for Anthropic/Codex, connectivity check for local models). Logs warnings for unreachable providers but doesn't block startup unless zero providers are available.
- [ ] Graceful degradation: if the Codex CLI is not installed, the Codex provider is simply not registered (warning logged). If Docker containers aren't running, local provider is not registered. The system works with whatever providers are available.
- [ ] Integration test: configure a router with mock providers, verify default routing, override routing, and fallback on primary failure
- [ ] Unit test: fallback logic (primary returns 429 → falls back to secondary)
- [ ] Unit test: auth error is NOT retried or fallen back

---

## Architecture References

- [[03-provider-architecture]] — "Provider Router" section. Routing logic (v0.1 manual), configuration format, future automatic routing (out of scope).
- [[03-provider-architecture]] — "Error Handling" section. Per-error-type behavior including fallback triggers.
- [[03-provider-architecture]] — "Implementation Priority" section. Provider instantiation order.
- [[05-agent-loop]] — Step 4 (LLM Request). The agent loop calls the router for all inference.
- [[06-context-assembly]] — "Budget Manager" section. Uses `Models()` to get context window sizes. Compression calls route through the router with `purpose="compression"`.
- [[07-web-interface-and-streaming]] — Settings panel needs model selection and provider status.

---

## Notes for the Implementing Agent

The router is the capstone of Layer 2. It's the only thing the rest of the system touches directly — the agent loop, context assembly, and any other LLM consumer all go through the router. Individual providers are internal implementation details.

The router itself implements `Provider`. This means consumers don't know or care about routing — they call `Complete` or `Stream` and the router handles provider selection, fallback, and tracking transparently. The router delegates to the appropriate tracked provider instance.

Fallback is simple for v0.1: one fallback provider/model pair in config. If the primary returns a retriable error after its own retry exhaustion (the individual providers handle their own retries per Epic 03/04/05), the router makes one attempt on the fallback. If the fallback also fails, the error surfaces to the caller. No cascading fallback chains.

Provider health tracking is lightweight — a boolean per provider (healthy/degraded) plus the timestamp and error of the last failure. The web UI's settings panel reads this to show provider status. No automatic recovery probing — the next real request serves as the health check.

The configuration shape from [[03-provider-architecture]]:

```yaml
routing:
  default:
    provider: anthropic
    model: claude-sonnet-4-6
  fallback:
    provider: local
    model: qwen2.5-coder-7b
```

This should already be loadable via Layer 0 Epic 03's config system.
