# Task 04: Provider Health Tracking, Startup Validation, and Graceful Degradation

**Epic:** 07 — Provider Router
**Status:** ⬚ Not started
**Dependencies:** Task 03 (fallback logic, error classification)

---

## Description

Add provider health tracking that updates after every Complete and Stream call, implement startup validation that checks provider availability and handles missing providers gracefully, and ensure the Models() aggregation tags models with their provider source. The health tracking gives the web UI visibility into provider status. Startup validation ensures the system starts with whatever providers are available, logging warnings for unavailable ones rather than blocking startup, while guaranteeing at least one provider is usable.

## Acceptance Criteria

- [ ] After every successful Complete call on any provider (primary or fallback), update that provider's health record under write lock:
  - Set `Healthy = true`
  - Set `LastSuccessAt = time.Now()`

- [ ] After every failed Complete call on any provider (primary or fallback), update that provider's health record under write lock:
  - Set `Healthy = false`
  - Set `LastError = err`
  - Set `LastErrorAt = time.Now()`

- [ ] After every successful Stream call (when the channel is returned without error), update the provider's health record the same as a successful Complete call. Note: Stream errors that occur mid-stream (after the channel is returned) are NOT tracked by the router — only the initial Stream call error is tracked.

- [ ] After every failed Stream call (when Stream returns an error), update the provider's health record the same as a failed Complete call

- [ ] Health updates must be performed in a helper method to avoid duplication:

```go
func (r *Router) markSuccess(providerName string)
func (r *Router) markFailure(providerName string, err error)
```

  Both methods acquire `r.mu.Lock()` and release on return. They are no-ops if `providerName` is not in `r.health` (defensive, should not happen).

- [ ] Implement a startup validation function:

```go
func (r *Router) Validate(ctx context.Context) error
```

  The Validate method performs the following checks in order:
  1. If `len(r.providers) == 0`, return error: `"no providers configured; add at least one provider to sirtopham.yaml"`
  2. For each registered provider named `"anthropic"`: call its auth validation method (attempt `GetAuthHeader` or equivalent) with a context that has a 5-second timeout. If it fails, log at WARN level: `"anthropic provider credentials could not be verified"` with attr `error=<err>`. Do NOT unregister the provider or block startup.
  3. For each registered provider named `"codex"`: check if the `codex` binary exists on PATH using `exec.LookPath("codex")`. If not found, log at WARN level: `"codex binary not found on PATH, unregistering codex provider"`. Unregister the provider by deleting it from `r.providers` and `r.health`.
  4. For each registered provider named `"local"` or any OpenAI-compatible provider: attempt a lightweight connectivity check (HTTP HEAD request to the provider's base URL with a 2-second timeout). If unreachable, log at WARN level: `"local provider unreachable, unregistering"` with attr `base_url=<url>`, `error=<err>`. Unregister the provider.
  5. After all provider checks, verify the default provider is still registered. If `r.config.Default.Provider` is NOT in `r.providers`, log at ERROR level: `"configured default provider not available, falling back to first available"` with attr `configured=<default name>`. Set `r.config.Default.Provider` to the first available provider name (iteration order of the map — any available provider is acceptable). If no providers remain after unregistration, return error: `"no providers available after startup validation; check provider configuration and connectivity"`
  6. Return nil if at least one provider remains registered

- [ ] For the codex PATH check, use `exec.LookPath("codex")` from `os/exec`. Import only the standard library for this check.

- [ ] For the local provider connectivity check, use `http.NewRequestWithContext` to create a HEAD request and `http.DefaultClient.Do` to execute it, with a 2-second deadline context. Only check connectivity if the provider exposes a base URL (if not available via interface, skip the connectivity check and log at DEBUG level).

- [ ] The ProviderHealth() method (already defined in Task 01) returns a shallow copy of the health map. Verify it correctly reflects health updates after Complete/Stream calls.

- [ ] Ensure the Models() method (from Task 02) returns models from all currently-registered providers. Models from providers that were unregistered during Validate() are excluded since those providers are no longer in `r.providers`.

- [ ] All new exported methods have GoDoc comments
- [ ] The package compiles with no errors
