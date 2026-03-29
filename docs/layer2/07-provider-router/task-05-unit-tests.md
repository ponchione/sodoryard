# Task 05: Unit Tests

**Epic:** 07 — Provider Router
**Status:** ⬚ Not started
**Dependencies:** Tasks 01-04 (all router implementation)

---

## Description

Write comprehensive unit tests for the Router covering construction, registration, default routing, per-request model override, fallback on retriable errors, auth error bypass of fallback, health tracking updates, and the Models() aggregation. All tests use mock providers that implement the provider.Provider interface with controllable behavior (configurable responses, errors, and model lists). Tests go in `internal/provider/router/router_test.go`.

## Acceptance Criteria

- [ ] Create a `mockProvider` test helper in `router_test.go` that implements `provider.Provider` with the following controllable fields:

```go
type mockProvider struct {
    name         string
    models       []provider.Model
    modelsErr    error
    completeResp *provider.Response
    completeErr  error
    streamCh     <-chan provider.StreamEvent
    streamErr    error
    completeCalls int  // incremented on each Complete call
    streamCalls   int  // incremented on each Stream call
}
```

  The mock's `Complete` increments `completeCalls` and returns `completeResp, completeErr`. The mock's `Stream` increments `streamCalls` and returns `streamCh, streamErr`. The mock's `Models` returns `models, modelsErr`. The mock's `Name` returns `name`.

- [ ] **TestNewRouter_ValidConfig:** Construct a Router with a valid RouterConfig (Default.Provider="anthropic", Default.Model="claude-sonnet-4-6"). Assert no error returned. Assert the Router is non-nil.

- [ ] **TestNewRouter_MissingDefaultProvider:** Construct with empty Default.Provider. Assert error message contains `"routing.default.provider is required"`.

- [ ] **TestNewRouter_MissingDefaultModel:** Construct with empty Default.Model. Assert error message contains `"routing.default.model is required"`.

- [ ] **TestRegisterProvider_Success:** Register a mock provider named "anthropic". Assert no error. Assert ProviderHealth() contains an entry for "anthropic" with Healthy=true.

- [ ] **TestRegisterProvider_Nil:** Call RegisterProvider(nil). Assert error message contains `"cannot register nil provider"`.

- [ ] **TestRegisterProvider_Duplicate:** Register a mock provider named "anthropic" twice. Assert the second call returns an error containing `"provider already registered: anthropic"`.

- [ ] **TestComplete_DefaultRouting:** Register a mock provider named "anthropic" with a successful response. Create a RouterConfig with Default pointing to "anthropic"/"claude-sonnet-4-6". Call Complete with an empty req.Model. Assert the mock's completeCalls is 1. Assert the returned response matches the mock's response. Assert req.Model was set to "claude-sonnet-4-6" during the call (verify via the mock capturing the received request).

- [ ] **TestComplete_PerRequestOverride:** Register two mock providers: "anthropic" (offering model "claude-sonnet-4-6") and "local" (offering model "qwen2.5-coder-7b"). Set Default to "anthropic". Call Complete with req.Model="qwen2.5-coder-7b". Assert the "local" mock's completeCalls is 1 and the "anthropic" mock's completeCalls is 0. Assert the response comes from the "local" mock.

- [ ] **TestComplete_OverrideModelNotFound:** Register one mock provider "anthropic" offering model "claude-sonnet-4-6". Call Complete with req.Model="nonexistent-model". Assert the call falls through to the default provider. Assert "anthropic" mock's completeCalls is 1 (default was used).

- [ ] **TestComplete_FallbackOnRetriableError:** Register "anthropic" (returns a `*provider.ProviderError` with Retriable=true, StatusCode=502) and "local" (returns a successful response). Configure Fallback to "local"/"qwen2.5-coder-7b". Call Complete. Assert "anthropic" completeCalls is 1 (primary attempted). Assert "local" completeCalls is 1 (fallback attempted). Assert the returned response is from the "local" mock (fallback succeeded).

- [ ] **TestComplete_AuthErrorNoFallback:** Register "anthropic" (returns `*provider.ProviderError` with StatusCode=401, Retriable=false) and "local" (successful). Configure Fallback to "local". Call Complete. Assert "anthropic" completeCalls is 1. Assert "local" completeCalls is 0 (fallback NOT attempted). Assert the returned error message contains `"authentication failed"` and `"Check your API key"`.

- [ ] **TestComplete_AuthError403NoFallback:** Same as above but with StatusCode=403. Assert fallback is NOT attempted. Assert error contains `"authentication failed"`.

- [ ] **TestComplete_NonRetriableErrorNoFallback:** Register "anthropic" (returns `*provider.ProviderError` with StatusCode=400, Retriable=false) and "local" (successful). Configure Fallback to "local". Call Complete. Assert "local" completeCalls is 0. Assert the original error is returned.

- [ ] **TestComplete_FallbackAlsoFails:** Register "anthropic" (returns retriable error, StatusCode=503) and "local" (returns a different error). Configure Fallback to "local". Call Complete. Assert both mocks were called. Assert the returned error is the fallback error (not the primary error).

- [ ] **TestComplete_NoFallbackConfigured:** Register "anthropic" (returns retriable error). Configure with Fallback=nil. Call Complete. Assert the retriable error is returned directly without any fallback attempt.

- [ ] **TestStream_DefaultRouting:** Same as TestComplete_DefaultRouting but using Stream. Assert the returned channel is from the mock. Assert streamCalls is 1.

- [ ] **TestStream_FallbackOnRetriableError:** Same as TestComplete_FallbackOnRetriableError but using Stream. Assert fallback stream channel is returned.

- [ ] **TestHealthTracking_SuccessUpdatesHealth:** Register "anthropic" (returns success). Call Complete. Assert ProviderHealth()["anthropic"].Healthy is true. Assert LastSuccessAt is recent (within 1 second of now).

- [ ] **TestHealthTracking_FailureUpdatesHealth:** Register "anthropic" (returns an error). Call Complete. Assert ProviderHealth()["anthropic"].Healthy is false. Assert LastError is the error. Assert LastErrorAt is recent.

- [ ] **TestHealthTracking_FallbackUpdatesHealth:** Register "anthropic" (retriable error) and "local" (success). Call Complete with fallback configured. Assert ProviderHealth()["anthropic"].Healthy is false. Assert ProviderHealth()["local"].Healthy is true.

- [ ] **TestModels_AggregatesAllProviders:** Register "anthropic" (offers ["claude-sonnet-4-6", "claude-haiku-3.5"]) and "local" (offers ["qwen2.5-coder-7b"]). Call Models(). Assert the returned slice contains all 3 models.

- [ ] **TestModels_SkipsFailingProvider:** Register "anthropic" (Models returns error) and "local" (offers ["qwen2.5-coder-7b"]). Call Models(). Assert the returned slice contains only the 1 model from "local". Assert no error returned from Models().

- [ ] **TestName:** Assert Router.Name() returns "router".

- [ ] All tests pass with `go test ./internal/provider/router/...`
- [ ] No test relies on external services, network calls, or file system state
