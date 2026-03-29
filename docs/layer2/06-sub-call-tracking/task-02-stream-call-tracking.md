# Task 02: Stream Call Tracking

**Epic:** 06 — Sub-Call Tracking
**Status:** ⬚ Not started
**Dependencies:** Task 01 (TrackedProvider struct, Complete tracking, SubCallStore interface reference)

---

## Description

Implement the `Stream` method on `TrackedProvider` that wraps the inner provider's stream channel with a tracking channel. The wrapper channel passes all `StreamEvent` values through unchanged to the downstream consumer while intercepting `StreamUsage`, `StreamDone`, and `StreamError` events to capture token counts, stop reason, and error details. When the inner channel closes (stream completes), the wrapper calculates wall-clock latency and writes a `sub_calls` row via the `SubCallStore`. The wrapper channel must close itself after the inner channel closes and the record is written, ensuring the consumer sees a clean channel lifecycle.

## Acceptance Criteria

- [ ] The `Stream` method on `TrackedProvider` replaces the Task 01 stub with the full tracking implementation:
  ```go
  func (tp *TrackedProvider) Stream(ctx context.Context, req *provider.Request) (<-chan provider.StreamEvent, error)
  ```
- [ ] The `Stream` method follows this exact flow:
  1. Record start time using `time.Now()`
  2. Call `tp.inner.Stream(ctx, req)` to get `(ch <-chan provider.StreamEvent, err error)`
  3. If the inner `Stream` returns an error: write a sub-call row with `Success=0`, `ErrorMessage` set to `err.Error()`, all token counts as `0`, latency calculated from start to now, and return `(nil, err)` unchanged
  4. If the inner `Stream` succeeds: create a wrapper channel `out := make(chan provider.StreamEvent)`, launch a goroutine that reads from `ch` and writes to `out`, and return `(out, nil)`
- [ ] When the inner `Stream` call itself fails (returns a non-nil error before any channel is created), a sub-call row is written with:
  - `Provider`: `tp.inner.Name()`
  - `Model`: `req.Model` (use the requested model since there is no response)
  - `Purpose`: `req.Purpose`
  - `TokensIn`: `0`
  - `TokensOut`: `0`
  - `CacheReadTokens`: `0`
  - `CacheCreationTokens`: `0`
  - `LatencyMs`: wall-clock latency from start to now
  - `Success`: `0`
  - `ErrorMessage`: pointer to `err.Error()`
  - `ConversationID`: pointer to `req.ConversationID` if non-empty, otherwise `nil`
  - `MessageID`: `nil`
  - `TurnNumber`: pointer to `req.TurnNumber` if non-zero, otherwise `nil`
  - `Iteration`: pointer to `req.Iteration` if non-zero, otherwise `nil`
  - `CreatedAt`: ISO 8601 UTC timestamp
- [ ] Tracking failures never block inference: if the store write fails when recording an inner `Stream` setup error, the error is logged at ERROR level and the original `(nil, err)` is still returned unchanged
- [ ] The wrapper goroutine maintains local state to accumulate tracking data during the stream:
  - `var finalUsage provider.Usage` — updated on each `StreamUsage` and `StreamDone` event
  - `var streamErr error` — set if a `StreamError` with `Fatal=true` is received
  - `var success bool` — defaults to `true`, set to `false` on fatal `StreamError`
- [ ] The wrapper goroutine reads from the inner channel in a `for event := range ch` loop and performs a type-switch on each event:
  - `provider.StreamUsage`: update `finalUsage` with `event.Usage` (overwrite all four fields: `InputTokens`, `OutputTokens`, `CacheReadTokens`, `CacheCreationTokens`)
  - `provider.StreamDone`: update `finalUsage` with `event.Usage` (the `StreamDone.Usage` field carries the final authoritative usage counts)
  - `provider.StreamError` where `event.Fatal == true`: set `streamErr = event.Err`, set `success = false`
  - All other event types (`TokenDelta`, `ThinkingDelta`, `ToolCallStart`, `ToolCallDelta`, `ToolCallEnd`, non-fatal `StreamError`): no tracking state update needed
- [ ] Every event received from the inner channel is forwarded to `out` regardless of type. The wrapper never drops, modifies, or reorders events. The consumer sees the exact same event stream as if there were no tracker.
- [ ] After the inner channel closes (the `for range` loop exits), the wrapper goroutine:
  1. Calculates latency as `time.Since(start).Milliseconds()`
  2. Builds an `InsertSubCallParams` struct:
     - `ConversationID`: pointer to `req.ConversationID` if non-empty, otherwise `nil`
     - `MessageID`: `nil`
     - `TurnNumber`: pointer to `req.TurnNumber` if non-zero, otherwise `nil`
     - `Iteration`: pointer to `req.Iteration` if non-zero, otherwise `nil`
     - `Provider`: `tp.inner.Name()`
     - `Model`: `req.Model` (the requested model; the stream events do not carry a model name)
     - `Purpose`: `req.Purpose`
     - `TokensIn`: `finalUsage.InputTokens`
     - `TokensOut`: `finalUsage.OutputTokens`
     - `CacheReadTokens`: `finalUsage.CacheReadTokens`
     - `CacheCreationTokens`: `finalUsage.CacheCreationTokens`
     - `LatencyMs`: calculated wall-clock latency
     - `Success`: `1` if `success` is true, `0` if false
     - `ErrorMessage`: pointer to `streamErr.Error()` if `streamErr` is non-nil, otherwise `nil`
     - `CreatedAt`: ISO 8601 UTC timestamp
  3. Calls `tp.store.InsertSubCall(context.Background(), params)` — uses `context.Background()` because the original request context may have been cancelled by the time the stream ends
  4. If the store write fails: logs at ERROR level with structured attributes: `"provider"` (`tp.inner.Name()`), `"model"` (`req.Model`), `"purpose"` (`req.Purpose`), `"tokens_in"` (`finalUsage.InputTokens`), `"tokens_out"` (`finalUsage.OutputTokens`), `"cache_read_tokens"` (`finalUsage.CacheReadTokens`), `"cache_creation_tokens"` (`finalUsage.CacheCreationTokens`), `"latency_ms"` (calculated latency), `"err"` (the store error)
  5. Closes the `out` channel via `close(out)`
- [ ] Tracking failures never block inference: if `SubCallStore.InsertSubCall` fails during stream completion, the error is logged but the `out` channel is still closed normally. The consumer does not observe any difference. No panic, no error event injected, no modification to the stream.
- [ ] The wrapper goroutine does not leak: it always terminates when the inner channel closes. If the inner channel is closed prematurely (e.g., context cancellation), the goroutine still runs its cleanup (write record, close `out`).
- [ ] If no `StreamUsage` or `StreamDone` event is received before the inner channel closes (abnormal stream termination), all token counts default to `0` and `Success` is set to `0`
- [ ] The wrapper channel is unbuffered (`make(chan provider.StreamEvent)`) to maintain backpressure parity with the inner channel
- [ ] The file compiles with `go build ./internal/provider/tracking/...` with no errors
