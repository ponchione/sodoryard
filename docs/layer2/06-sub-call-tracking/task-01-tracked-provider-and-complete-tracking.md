# Task 01: TrackedProvider Struct and Complete Call Tracking

**Epic:** 06 — Sub-Call Tracking
**Status:** ⬚ Not started
**Dependencies:** L2-E01 (provider types — `Provider` interface, `Request`, `Response`, `Usage`), L0-E02 (logging — `*slog.Logger`), L0-E04 (SQLite connection manager)

---

## Description

Define the `TrackedProvider` struct in `internal/provider/tracking/tracked.go` and implement the `Complete` method that wraps an inner `Provider`, delegates the call, measures wall-clock latency, extracts token usage from the response, and writes a `sub_calls` row via the `SubCallStore` interface. The `TrackedProvider` implements the `Provider` interface so it can be substituted transparently wherever a provider is expected. The `Models` and `Name` methods delegate directly to the inner provider. The `Complete` implementation must record both successful and failed calls, and tracking failures (SQLite write errors) must never block inference — they are logged and swallowed.

## Acceptance Criteria

- [ ] File `internal/provider/tracking/tracked.go` exists with `package tracking`
- [ ] The `TrackedProvider` struct is defined with exactly these fields:
  ```go
  type TrackedProvider struct {
      inner  provider.Provider
      store  SubCallStore
      logger *slog.Logger
  }
  ```
  `inner` is the wrapped provider that does the actual LLM call. `store` is the persistence interface for writing sub-call records. `logger` is used for logging tracking failures.
- [ ] The constructor function is defined:
  ```go
  func NewTrackedProvider(inner provider.Provider, store SubCallStore, logger *slog.Logger) *TrackedProvider
  ```
  It returns a `*TrackedProvider` with all three fields set from the arguments. No nil checks are required at construction time.
- [ ] `TrackedProvider` satisfies the `provider.Provider` interface by implementing all four methods: `Complete`, `Stream`, `Models`, `Name`
- [ ] The `Name` method delegates directly to the inner provider:
  ```go
  func (tp *TrackedProvider) Name() string
  ```
  Returns `tp.inner.Name()` unchanged.
- [ ] The `Models` method delegates directly to the inner provider:
  ```go
  func (tp *TrackedProvider) Models(ctx context.Context) ([]provider.Model, error)
  ```
  Returns `tp.inner.Models(ctx)` unchanged. No tracking is performed for `Models` calls.
- [ ] The `Complete` method is defined:
  ```go
  func (tp *TrackedProvider) Complete(ctx context.Context, req *provider.Request) (*provider.Response, error)
  ```
- [ ] The `Complete` method follows this exact flow:
  1. Record start time using `time.Now()`
  2. Call `tp.inner.Complete(ctx, req)` to get `(resp *provider.Response, err error)`
  3. Record end time using `time.Now()`, calculate latency as `end.Sub(start).Milliseconds()`
  4. Build an `InsertSubCallParams` struct (see Task 03 for the type) populated from the request and response
  5. Call `tp.store.InsertSubCall(ctx, params)` to persist the record
  6. If the store write fails: log at ERROR level and continue (do NOT return the store error)
  7. Return the original `(resp, err)` from the inner provider, completely unchanged
- [ ] On a successful inner `Complete` call (err == nil), the `InsertSubCallParams` is populated as:
  - `ConversationID`: pointer to `req.ConversationID` if non-empty, otherwise `nil`
  - `MessageID`: `nil` (message ID is linked later by the agent loop)
  - `TurnNumber`: pointer to `req.TurnNumber` if non-zero, otherwise `nil`
  - `Iteration`: pointer to `req.Iteration` if non-zero, otherwise `nil`
  - `Provider`: `tp.inner.Name()`
  - `Model`: `resp.Model`
  - `Purpose`: `req.Purpose`
  - `TokensIn`: `resp.Usage.InputTokens`
  - `TokensOut`: `resp.Usage.OutputTokens`
  - `CacheReadTokens`: `resp.Usage.CacheReadTokens`
  - `CacheCreationTokens`: `resp.Usage.CacheCreationTokens`
  - `LatencyMs`: calculated wall-clock latency in milliseconds
  - `Success`: `1`
  - `ErrorMessage`: `nil`
  - `CreatedAt`: `time.Now().UTC().Format(time.RFC3339)` (ISO 8601)
- [ ] On a failed inner `Complete` call (err != nil), the `InsertSubCallParams` is populated as:
  - `ConversationID`: pointer to `req.ConversationID` if non-empty, otherwise `nil`
  - `MessageID`: `nil`
  - `TurnNumber`: pointer to `req.TurnNumber` if non-zero, otherwise `nil`
  - `Iteration`: pointer to `req.Iteration` if non-zero, otherwise `nil`
  - `Provider`: `tp.inner.Name()`
  - `Model`: `req.Model` (use the requested model since there is no response)
  - `Purpose`: `req.Purpose`
  - `TokensIn`: `0` (no usage data available)
  - `TokensOut`: `0`
  - `CacheReadTokens`: `0`
  - `CacheCreationTokens`: `0`
  - `LatencyMs`: calculated wall-clock latency in milliseconds
  - `Success`: `0`
  - `ErrorMessage`: pointer to `err.Error()`
  - `CreatedAt`: `time.Now().UTC().Format(time.RFC3339)` (ISO 8601)
- [ ] On a failed inner `Complete` call where the response is non-nil (some providers return partial responses alongside errors): token counts are extracted from `resp.Usage` instead of defaulting to zero
- [ ] When the `SubCallStore.InsertSubCall` call fails, the error is logged at ERROR level using `tp.logger.Error(...)` with the following structured attributes: `"err"` (the store error), `"provider"` (provider name), `"model"` (model name), `"purpose"` (purpose string), `"tokens_in"` (input tokens), `"tokens_out"` (output tokens), `"latency_ms"` (latency). This ensures all sub-call data that would have been written is recoverable from logs.
- [ ] Tracking failures never block inference: if `SubCallStore.InsertSubCall` returns an error, the `Complete` method still returns the original `(resp, err)` from the inner provider. The store error is not returned, not wrapped, and not appended. The caller has no way to detect that tracking failed.
- [ ] The `Stream` method is stubbed in this task (full implementation in Task 02) but must exist so the type satisfies the `Provider` interface:
  ```go
  func (tp *TrackedProvider) Stream(ctx context.Context, req *provider.Request) (<-chan provider.StreamEvent, error)
  ```
  The stub delegates directly to `tp.inner.Stream(ctx, req)` without any tracking.
- [ ] The file imports only standard library packages (`context`, `log/slog`, `time`) and the project's `internal/provider` package
- [ ] The file compiles with `go build ./internal/provider/tracking/...` with no errors
