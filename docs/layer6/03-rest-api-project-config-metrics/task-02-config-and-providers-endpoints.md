# Task 02: Config and Providers Endpoints

**Epic:** 03 — REST API Project/Config/Metrics
**Status:** ⬚ Not started
**Dependencies:** Epic 01 (HTTP Server Foundation), Layer 0 Epic 03 (config), Layer 2 Epic 07 (provider router)

---

## Description

Implement the `GET /api/config`, `PUT /api/config`, and `GET /api/providers` endpoints. The config GET endpoint returns the current runtime configuration relevant to the UI. The config PUT endpoint allows updating the default provider/model as a runtime override (not persisted to disk). The providers endpoint returns the live state of all configured providers with their available models and connection status, sourced from the provider router.

## Acceptance Criteria

- [ ] `GET /api/config` returns a JSON object with: `{default_provider, default_model, fallback_provider, fallback_model, agent: {max_iterations, extended_thinking}, providers: [{name, type, models: [...]}]}`
- [ ] Config values reflect the current runtime state, including any overrides applied via `PUT /api/config`
- [ ] `PUT /api/config` accepts a JSON body with mutable fields: `{default_provider?: string, default_model?: string}`
- [ ] Only the fields present in the PUT body are updated — omitted fields remain unchanged
- [ ] Invalid provider or model names return HTTP 400 with `{"error": "unknown provider: <name>"}` or `{"error": "model <name> not available on provider <provider>"}`
- [ ] PUT does NOT persist changes to `sirtopham.yaml` — changes are runtime overrides that reset on server restart
- [ ] Returns the full updated config object (same shape as GET) after a successful PUT
- [ ] `GET /api/providers` returns `[{name, type, status, models: [{name, context_window}]}]` sourced from the provider router's `ListProviders()` and `Models()` methods
- [ ] Provider `status` is `"available"` or `"unavailable"` based on whether the provider's credentials are configured and reachable
- [ ] All three endpoints are registered on the server's router
