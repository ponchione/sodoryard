# Task 06: Integration Tests

**Epic:** 07 — Provider Router
**Status:** ⬚ Not started
**Dependencies:** Tasks 01-04 (all router implementation)

---

## Description

Write integration tests that exercise the full Router lifecycle: construction, provider registration, startup validation, routing through Complete and Stream with fallback scenarios, health tracking across multiple calls, and Models aggregation. These tests use more realistic mock providers that simulate latency, multi-step failures, and recovery patterns. Tests go in `internal/provider/router/router_integration_test.go` with a `//go:build integration` tag, or alternatively in `router_test.go` with helper-driven setup that tests multi-step scenarios.

## Acceptance Criteria

- [ ] **TestIntegration_FullLifecycle:** Test the complete router lifecycle in a single test:
  1. Create a RouterConfig with Default=("anthropic", "claude-sonnet-4-6") and Fallback=("local", "qwen2.5-coder-7b").
  2. Construct the Router via NewRouter.
  3. Register a mock "anthropic" provider (initially healthy, returns successful responses).
  4. Register a mock "local" provider (initially healthy, returns successful responses).
  5. Call Complete with an empty model. Assert it routes to "anthropic" and returns success.
  6. Assert ProviderHealth()["anthropic"].Healthy is true and LastSuccessAt is set.
  7. Reconfigure the "anthropic" mock to return a retriable error (ProviderError with StatusCode=502, Retriable=true).
  8. Call Complete again. Assert it attempts "anthropic" first (fails), then falls back to "local" (succeeds).
  9. Assert ProviderHealth()["anthropic"].Healthy is false and LastError is set.
  10. Assert ProviderHealth()["local"].Healthy is true and LastSuccessAt is updated.
  11. Reconfigure "anthropic" mock back to healthy.
  12. Call Complete again. Assert it routes to "anthropic" (succeeds) — the router does not remember previous failures for routing decisions; it always tries the default first.
  13. Assert ProviderHealth()["anthropic"].Healthy is true and LastSuccessAt is updated.

- [ ] **TestIntegration_PerRequestOverrideWithFallback:** Test that per-request override interacts correctly with fallback:
  1. Register "anthropic" (offers "claude-sonnet-4-6") and "local" (offers "qwen2.5-coder-7b", returns success).
  2. Configure Default=("anthropic", "claude-sonnet-4-6"), Fallback=("local", "qwen2.5-coder-7b").
  3. Call Complete with req.Model="qwen2.5-coder-7b". Assert it routes to "local" directly (override, NOT fallback). Assert "anthropic" was not called.
  4. Configure "local" mock to return a retriable error (StatusCode=503, Retriable=true).
  5. Call Complete with req.Model="qwen2.5-coder-7b" again. Assert it routes to "local" (override, fails), then falls back to the configured Fallback provider. Since Fallback is also "local", the fallback call also goes to "local". Assert "local" completeCalls is 2 for this call (one override attempt + one fallback attempt). Assert the returned error is from the fallback attempt.

- [ ] **TestIntegration_AuthErrorSurfacesImmediately:** Test that auth errors never trigger fallback, even across multiple calls:
  1. Register "anthropic" (returns ProviderError with StatusCode=401, Retriable=false, Message="invalid api key") and "local" (returns success).
  2. Configure Fallback=("local", "qwen2.5-coder-7b").
  3. Call Complete. Assert error message contains `"authentication failed for provider anthropic (HTTP 401)"` and `"Check your API key"`.
  4. Assert "local" completeCalls is 0.
  5. Assert ProviderHealth()["anthropic"].Healthy is false.
  6. Call Complete again (auth errors don't self-heal). Assert the same auth error is returned. Assert "local" was still never called.

- [ ] **TestIntegration_BothProvidersFail:** Test that when both primary and fallback fail, the fallback error is returned:
  1. Register "anthropic" (returns retriable error: StatusCode=503, Message="service unavailable") and "local" (returns a different error: StatusCode=500, Message="internal error").
  2. Configure Fallback=("local", "qwen2.5-coder-7b").
  3. Call Complete. Assert the returned error is the "local" provider's error (StatusCode=500, "internal error"), NOT the "anthropic" error.
  4. Assert ProviderHealth()["anthropic"].Healthy is false.
  5. Assert ProviderHealth()["local"].Healthy is false.

- [ ] **TestIntegration_StreamFallback:** Test Stream routing with fallback:
  1. Register "anthropic" (Stream returns a retriable error) and "local" (Stream returns a channel with test events).
  2. Configure Fallback=("local", "qwen2.5-coder-7b").
  3. Call Stream. Assert the returned channel is from the "local" mock.
  4. Read events from the channel and assert they match expected test events.
  5. Assert ProviderHealth()["anthropic"].Healthy is false.
  6. Assert ProviderHealth()["local"].Healthy is true.

- [ ] **TestIntegration_Validate_NoProviders:** Construct a Router and call Validate without registering any providers. Assert error message is `"no providers configured; add at least one provider to sirtopham.yaml"`.

- [ ] **TestIntegration_Validate_DefaultProviderUnavailable:** Register two mock providers "anthropic" and "local". Configure Default=("anthropic", "claude-sonnet-4-6"). Before Validate, manually remove "anthropic" from the router's providers (simulating it being unregistered during validation). Call Validate. Assert no error (because "local" is still available). Assert `r.config.Default.Provider` has been updated to "local" (or whatever the remaining provider is).

- [ ] **TestIntegration_ModelsAggregation:** Register "anthropic" (offers ["claude-sonnet-4-6", "claude-haiku-3.5"]) and "local" (offers ["qwen2.5-coder-7b", "llama3-8b"]). Call Models(). Assert the returned slice has exactly 4 models. Assert model IDs include all four expected values.

- [ ] **TestIntegration_ConcurrentRequests:** Test that the router handles concurrent Complete calls safely:
  1. Register "anthropic" (returns success after a 10ms simulated delay) and "local" (returns success).
  2. Launch 20 goroutines, each calling Complete with empty model.
  3. Wait for all goroutines to finish. Assert no panics, no data races.
  4. Assert "anthropic" completeCalls is 20.
  5. Assert ProviderHealth()["anthropic"].Healthy is true.
  6. Run with `go test -race` to verify no race conditions.

- [ ] All tests pass with `go test ./internal/provider/router/...` (and `go test -race` for the concurrency test)
- [ ] No test relies on external services, real API keys, or network calls — all providers are mocks
- [ ] Each test function is independent and can run in isolation
