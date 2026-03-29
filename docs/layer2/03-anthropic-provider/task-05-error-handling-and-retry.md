# Task 05: Error Handling and Retry Logic

**Epic:** 03 — Anthropic Provider
**Status:** Not started
**Dependencies:** Task 02 (Complete method — wrap with retry), Task 03 (Stream method — wrap with retry on initial HTTP request)

---

## Description

Implement per-status-code error classification and exponential backoff retry logic for the Anthropic provider. The retry wrapper is applied to both `Complete` and `Stream` methods, retrying only on retriable errors (429 rate limit, 500/502/503 server errors, and network connection errors). Non-retriable errors (400 bad request, 401/403 auth failures) are returned immediately with specific, actionable error messages. The retry uses exponential backoff with jitter: base delay 1 second, doubling each attempt, max 3 attempts, with random jitter of 0-50% of the delay added to prevent thundering herd.

## Acceptance Criteria

- [ ] An unexported function classifies HTTP status codes and returns an actionable error:
  ```go
  func classifyError(statusCode int, body []byte) *provider.ProviderError
  ```
- [ ] `classifyError` returns a `*provider.ProviderError` with `Provider: "anthropic"` and `StatusCode` set for all cases:
  - **401 or 403**: `Message: "Anthropic authentication failed. Run 'claude login' to re-authenticate."`, `Retriable: false`
  - **429**: `Message: "Anthropic rate limit exceeded"`, `Retriable: true`
  - **400**: `Message: "Anthropic bad request: <body truncated to 1024 bytes>"`, `Retriable: false`
  - **500**: `Message: "Anthropic internal server error"`, `Retriable: true`
  - **502**: `Message: "Anthropic bad gateway"`, `Retriable: true`
  - **503**: `Message: "Anthropic service unavailable"`, `Retriable: true`
  - **Any other non-200 status**: `Message: "Anthropic API error (<statusCode>): <body truncated to 512 bytes>"`, `Retriable: false`
- [ ] An unexported function classifies network/connection errors:
  ```go
  func classifyNetworkError(err error) *provider.ProviderError
  ```
  Returns `*provider.ProviderError` with `Provider: "anthropic"`, `StatusCode: 0`, `Message: "Anthropic network error: <error>"`, `Retriable: true`, `Err: err`
- [ ] An unexported retry wrapper function is defined:
  ```go
  func (p *AnthropicProvider) doWithRetry(ctx context.Context, fn func() (*http.Response, error)) (*http.Response, error)
  ```
- [ ] `doWithRetry` executes `fn()` up to 3 times (1 initial attempt + 2 retries)
- [ ] On each attempt, if `fn()` returns a non-nil error (network-level error before receiving a response), `doWithRetry` calls `classifyNetworkError(err)` to determine if it is retriable
- [ ] On each attempt, if `fn()` returns a response with a non-200 status code, `doWithRetry` reads the body (up to 1024 bytes), closes the body, and calls `classifyError(statusCode, body)` to determine if it is retriable
- [ ] If the error is retriable and attempts remain, `doWithRetry` sleeps for the computed backoff delay before the next attempt
- [ ] If the error is not retriable, `doWithRetry` returns the `*provider.ProviderError` immediately without further retries
- [ ] If all 3 attempts are exhausted, `doWithRetry` returns the `*provider.ProviderError` from the last attempt
- [ ] Backoff delay for attempt N (0-indexed) is computed as:
  ```go
  baseDelay := 1 * time.Second
  delay := baseDelay * time.Duration(1<<uint(attempt)) // 1s, 2s, 4s
  jitter := time.Duration(rand.Int63n(int64(delay / 2))) // 0 to 50% of delay
  totalDelay := delay + jitter
  ```
- [ ] The backoff sleep respects context cancellation using a select:
  ```go
  select {
  case <-time.After(totalDelay):
      // continue to next attempt
  case <-ctx.Done():
      return nil, &provider.ProviderError{
          Provider:  "anthropic",
          Message:   "retry cancelled",
          Retriable: false,
          Err:       ctx.Err(),
      }
  }
  ```
- [ ] `Complete` (from Task 02) is modified to use `doWithRetry` for the HTTP call: instead of calling `p.httpClient.Do(httpReq)` directly, it wraps the call in `doWithRetry`:
  ```go
  resp, err := p.doWithRetry(ctx, func() (*http.Response, error) {
      return p.httpClient.Do(httpReq)
  })
  ```
- [ ] `Stream` (from Task 03) is modified to use `doWithRetry` for the initial HTTP call only (the SSE stream itself is not retried; if the stream breaks mid-way, a `StreamError` with `Fatal: true` is emitted on the channel):
  ```go
  resp, err := p.doWithRetry(ctx, func() (*http.Response, error) {
      return p.httpClient.Do(httpReq)
  })
  ```
- [ ] For `Complete`, when `doWithRetry` returns an error, the original error is returned to the caller (it is already a `*provider.ProviderError`)
- [ ] For `Stream`, when `doWithRetry` returns an error, `Stream` returns `nil, err` (the channel is not created)
- [ ] The random jitter uses `math/rand` (not `crypto/rand`) since this is not security-sensitive; the random source is seeded per-call or uses the global source
- [ ] The file imports: `context`, `io`, `math/rand`, `net/http`, `time`, and the provider package
- [ ] The file compiles with `go build ./internal/provider/anthropic/...` with no errors
