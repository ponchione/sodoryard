# Task 03: Fallback Logic and Error Handling

**Epic:** 07 — Provider Router
**Status:** ⬚ Not started
**Dependencies:** Task 02 (Complete, Stream, override routing)

---

## Description

Extend the Router's Complete and Stream methods to implement fallback routing when the primary provider fails with a retriable error. The fallback is a single additional attempt on the configured fallback provider. Auth errors (HTTP 401, 403) are never retried or fallen back and must surface immediately with an actionable remediation message. Non-retriable errors also surface immediately without fallback. This task modifies the existing routing code from Task 02 to wrap provider calls with error classification and conditional fallback.

## Acceptance Criteria

- [ ] After the primary provider's Complete call returns an error, classify the error before returning it:

  **Error classification rules:**
  1. Type-assert the error to `*provider.ProviderError` (or use `errors.As`).
  2. If the error is a `ProviderError` with `StatusCode` 401 or 403 (regardless of the `Retriable` field): this is an **auth error**. Do NOT fall back. Return the error immediately, wrapped with an actionable message. The wrapped message format: `"authentication failed for provider <name> (HTTP <statusCode>): <original error message>. Check your API key in sirtopham.yaml or environment variables."`.
  3. If the error is a `ProviderError` with `Retriable == true` (status codes 429, 500, 502, 503, or network errors): this is a **retriable error**. Attempt fallback if `r.config.Fallback` is non-nil.
  4. If the error is a `ProviderError` with `Retriable == false` and status code is NOT 401/403: this is a **non-retriable error**. Do NOT fall back. Return the error immediately.
  5. If the error is not a `ProviderError` (e.g., context canceled, unexpected error): Do NOT fall back. Return the error immediately.

- [ ] When a retriable error triggers fallback in Complete:
  - Log the primary error at WARN level: `"primary provider failed, attempting fallback"` with attrs `primary_provider=<name>`, `error=<err>`, `fallback_provider=<fallback name>`
  - Look up `r.config.Fallback.Provider` in `r.providers`. If the fallback provider is not registered, return the original primary error wrapped with: `"primary provider failed and fallback provider not available: <fallback name>"`.
  - Set `req.Model` to `r.config.Fallback.Model`
  - Call `fallbackProvider.Complete(ctx, req)` — exactly ONE attempt, no further retries
  - If the fallback succeeds, return its response and nil error
  - If the fallback also fails, log the primary error at WARN level: `"both primary and fallback providers failed"` with attrs `primary_provider=<name>`, `primary_error=<original err>`, `fallback_provider=<fallback name>`, `fallback_error=<fallback err>`. Return the fallback error (the original primary error is logged but not returned).

- [ ] When a retriable error triggers fallback in Stream:
  - The same classification and fallback logic applies as in Complete
  - Log the primary error at WARN level with the same message and attrs as Complete
  - Call `fallbackProvider.Stream(ctx, req)` with `req.Model` set to `r.config.Fallback.Model`
  - If the fallback succeeds, return its channel and nil error
  - If the fallback also fails, log both errors and return the fallback error

- [ ] When `r.config.Fallback` is nil (no fallback configured), retriable errors from the primary provider are returned directly without any fallback attempt

- [ ] Auth error wrapping applies to both Complete and Stream paths identically

- [ ] Extract error classification into an internal helper shared by Complete and Stream:

  ```go
  type errorClass int

  const (
      errorClassAuth      errorClass = iota // 401, 403 — never retry, never fallback
      errorClassRetriable                    // 429, 500, 502, 503, network — attempt fallback
      errorClassFatal                        // all other errors — no fallback
  )

  func classifyError(err error) errorClass
  ```

  `classifyError` uses `errors.As` to extract `*provider.ProviderError`. If the error is a `ProviderError` with `StatusCode` 401 or 403, return `errorClassAuth`. If `Retriable == true`, return `errorClassRetriable`. Otherwise return `errorClassFatal`. If the error is not a `ProviderError`, return `errorClassFatal`.

- [ ] Restore `req.Model` to its pre-fallback value after the fallback call completes (whether success or failure) to avoid mutating the caller's request object

- [ ] All fallback-related log messages include the `primary_provider`, `fallback_provider`, and relevant error attrs for debugging

- [ ] The package compiles with no errors
