# Task 04: Unit Tests

**Epic:** 06 — Sub-Call Tracking
**Status:** ⬚ Not started
**Dependencies:** Task 01 (TrackedProvider, Complete tracking), Task 02 (Stream tracking), Task 03 (SubCallStore interface, InsertSubCallParams, SQLiteSubCallStore)

---

## Description

Implement comprehensive unit tests for the sub-call tracking package in `internal/provider/tracking/tracked_test.go`. The tests verify that `TrackedProvider` correctly records sub-call data for both `Complete` and `Stream` calls across success, failure, and edge-case scenarios. All tests use mock implementations of `provider.Provider` and `SubCallStore` — no real SQLite database is involved. A central focus is verifying the critical invariant that tracking failures never block inference: when the store returns errors, the provider response must pass through unmodified.

## Acceptance Criteria

- [ ] File `internal/provider/tracking/tracked_test.go` exists with `package tracking_test` (external test package)
- [ ] A `mockProvider` struct is defined in the test file that implements `provider.Provider` with controllable return values:
  ```go
  type mockProvider struct {
      name       string
      completeResp *provider.Response
      completeErr  error
      streamCh     <-chan provider.StreamEvent
      streamErr    error
      modelsList   []provider.Model
  }
  ```
  Each method (`Complete`, `Stream`, `Models`, `Name`) returns the corresponding pre-configured value.
- [ ] A `mockStore` struct is defined in the test file that implements `SubCallStore` with call recording:
  ```go
  type mockStore struct {
      calls []tracking.InsertSubCallParams
      err   error // if non-nil, InsertSubCall returns this error
  }
  ```
  `InsertSubCall` appends the params to `calls` and returns `err`.
- [ ] **Test: Complete — successful call records correct sub-call data**
  - Setup: `mockProvider` returns a `*provider.Response` with `Model: "claude-sonnet-4-20250514"`, `Usage: {InputTokens: 1500, OutputTokens: 300, CacheReadTokens: 200, CacheCreationTokens: 50}`, and nil error
  - Setup: `Request` has `Purpose: "chat"`, `ConversationID: "conv-abc"`, `TurnNumber: 3`, `Iteration: 1`, `Model: "claude-sonnet-4-20250514"`
  - Act: call `TrackedProvider.Complete(ctx, req)`
  - Assert: `mockStore.calls` has exactly 1 entry
  - Assert: the recorded params have `Provider` equal to the mock provider's `Name()` return value
  - Assert: `Model == "claude-sonnet-4-20250514"`
  - Assert: `Purpose == "chat"`
  - Assert: `TokensIn == 1500`
  - Assert: `TokensOut == 300`
  - Assert: `CacheReadTokens == 200`
  - Assert: `CacheCreationTokens == 50`
  - Assert: `Success == 1`
  - Assert: `ErrorMessage == nil`
  - Assert: `LatencyMs > 0` (positive latency)
  - Assert: `ConversationID` is non-nil and equals `"conv-abc"`
  - Assert: `TurnNumber` is non-nil and equals `3`
  - Assert: `Iteration` is non-nil and equals `1`
  - Assert: `MessageID == nil` (not set by tracker)
  - Assert: `CreatedAt` is a valid ISO 8601 timestamp
  - Assert: the returned `(*Response, error)` is the exact same `(*provider.Response, nil)` from the mock (pointer equality)
- [ ] **Test: Complete — failed call records error with success=0**
  - Setup: `mockProvider` returns `nil` response and `errors.New("connection timeout")`
  - Setup: `Request` has `Purpose: "chat"`, `Model: "claude-sonnet-4-20250514"`, `ConversationID: ""`
  - Act: call `TrackedProvider.Complete(ctx, req)`
  - Assert: `mockStore.calls` has exactly 1 entry
  - Assert: `Success == 0`
  - Assert: `ErrorMessage` is non-nil and equals `"connection timeout"`
  - Assert: `TokensIn == 0`, `TokensOut == 0`, `CacheReadTokens == 0`, `CacheCreationTokens == 0`
  - Assert: `Model == "claude-sonnet-4-20250514"` (falls back to request model)
  - Assert: `ConversationID == nil` (empty string in request yields nil)
  - Assert: `LatencyMs > 0`
  - Assert: the returned error is the exact same error from the mock
  - Assert: the returned response is nil
- [ ] **Test: Complete — failed call with partial response extracts usage from response**
  - Setup: `mockProvider` returns a non-nil `*provider.Response` with `Usage: {InputTokens: 500, OutputTokens: 0}` AND a non-nil `errors.New("partial failure")`
  - Act: call `TrackedProvider.Complete(ctx, req)`
  - Assert: `mockStore.calls` has exactly 1 entry
  - Assert: `Success == 0`
  - Assert: `TokensIn == 500` (extracted from partial response, not defaulted to 0)
  - Assert: `ErrorMessage` is non-nil and equals `"partial failure"`
- [ ] **Test: Complete — store failure does not block inference (tracking failure isolation)**
  - Setup: `mockProvider` returns a valid `*provider.Response` with usage data and nil error
  - Setup: `mockStore.err = errors.New("database is locked")`
  - Act: call `TrackedProvider.Complete(ctx, req)`
  - Assert: the returned `(*Response, error)` is the exact same `(*provider.Response, nil)` from the mock (pointer equality for response, nil for error)
  - Assert: the caller has no way to detect the tracking failure — no error returned, no panic, no modified response
  - Assert: tracking failures never block inference
- [ ] **Test: Complete — optional fields are nil when request has zero values**
  - Setup: `Request` has `ConversationID: ""`, `TurnNumber: 0`, `Iteration: 0`
  - Act: call `TrackedProvider.Complete(ctx, req)`
  - Assert: `ConversationID == nil`, `TurnNumber == nil`, `Iteration == nil` in the recorded params
- [ ] **Test: Stream — successful stream records correct sub-call data**
  - Setup: create a channel and send events in this order: `TokenDelta{Text: "Hello"}`, `TokenDelta{Text: " world"}`, `StreamUsage{Usage: {InputTokens: 1000, OutputTokens: 100, CacheReadTokens: 50, CacheCreationTokens: 0}}`, `StreamDone{StopReason: "end_turn", Usage: {InputTokens: 1000, OutputTokens: 150, CacheReadTokens: 50, CacheCreationTokens: 0}}`, then close the channel
  - Setup: `Request` has `Purpose: "compression"`, `ConversationID: "conv-xyz"`, `TurnNumber: 2`, `Iteration: 0`, `Model: "claude-sonnet-4-20250514"`
  - Act: call `TrackedProvider.Stream(ctx, req)`, consume all events from the returned channel
  - Assert: all 4 events are received in order: two `TokenDelta`, one `StreamUsage`, one `StreamDone`
  - Assert: `mockStore.calls` has exactly 1 entry (written after stream completes)
  - Assert: `Purpose == "compression"`
  - Assert: `TokensIn == 1000` (from `StreamDone.Usage`, not the earlier `StreamUsage`)
  - Assert: `TokensOut == 150` (from `StreamDone.Usage`)
  - Assert: `CacheReadTokens == 50`
  - Assert: `CacheCreationTokens == 0`
  - Assert: `Success == 1`
  - Assert: `ErrorMessage == nil`
  - Assert: `LatencyMs > 0`
  - Assert: `Iteration == nil` (zero value in request yields nil)
  - Assert: `ConversationID` is non-nil and equals `"conv-xyz"`
- [ ] **Test: Stream — fatal error during stream records failure**
  - Setup: create a channel and send events: `TokenDelta{Text: "partial"}`, `StreamError{Err: errors.New("rate limit"), Fatal: true, Message: "rate limit exceeded"}`, then close the channel
  - Setup: `Request` has `Purpose: "chat"`, `Model: "gpt-4o"`
  - Act: call `TrackedProvider.Stream(ctx, req)`, consume all events
  - Assert: both events are received in order
  - Assert: `mockStore.calls` has exactly 1 entry
  - Assert: `Success == 0`
  - Assert: `ErrorMessage` is non-nil and contains `"rate limit"`
  - Assert: `TokensIn == 0`, `TokensOut == 0` (no usage event was received)
- [ ] **Test: Stream — inner Stream call fails immediately**
  - Setup: `mockProvider.streamErr = errors.New("connection refused")`
  - Act: call `TrackedProvider.Stream(ctx, req)`
  - Assert: the returned channel is nil
  - Assert: the returned error is the exact same error from the mock
  - Assert: `mockStore.calls` has exactly 1 entry with `Success == 0`, `ErrorMessage` containing `"connection refused"`, all token counts `0`
  - Assert: tracking failures never block inference
- [ ] **Test: Stream — store failure during stream completion does not block inference**
  - Setup: create a channel that sends `StreamDone{...}` then closes
  - Setup: `mockStore.err = errors.New("disk full")`
  - Act: call `TrackedProvider.Stream(ctx, req)`, consume all events
  - Assert: all events are received unchanged
  - Assert: the wrapper channel closes normally (no panic, no hang)
  - Assert: tracking failures never block inference — the consumer sees the exact same events regardless of store failure
- [ ] **Test: Stream — no usage events before channel closes records zeros with failure**
  - Setup: create a channel that immediately closes without sending any events
  - Act: call `TrackedProvider.Stream(ctx, req)`, consume events
  - Assert: the returned channel closes
  - Assert: `mockStore.calls` has exactly 1 entry with `TokensIn == 0`, `TokensOut == 0`, `CacheReadTokens == 0`, `CacheCreationTokens == 0`, `Success == 0`
- [ ] **Test: Stream — events pass through unmodified (wrapper transparency)**
  - Setup: create a channel and send one of each event type: `TokenDelta{Text: "a"}`, `ThinkingDelta{Thinking: "b"}`, `ToolCallStart{ID: "tc_1", Name: "read"}`, `ToolCallDelta{ID: "tc_1", Delta: "{}"}`, `ToolCallEnd{ID: "tc_1", Input: json.RawMessage("{}") }`, `StreamUsage{Usage: {...}}`, `StreamError{Err: errors.New("warn"), Fatal: false, Message: "warning"}`, `StreamDone{...}`, then close
  - Act: call `TrackedProvider.Stream(ctx, req)`, collect all events
  - Assert: exactly 8 events are received
  - Assert: each event matches the input event by type and field values (the wrapper does not drop, reorder, or modify any event)
- [ ] **Test: Name and Models delegate to inner provider**
  - Setup: `mockProvider.name = "anthropic"`, `mockProvider.modelsList = []provider.Model{{ID: "claude-sonnet-4-20250514"}}`
  - Act: call `TrackedProvider.Name()` and `TrackedProvider.Models(ctx)`
  - Assert: `Name()` returns `"anthropic"`
  - Assert: `Models(ctx)` returns the same slice
  - Assert: `mockStore.calls` is empty (no tracking for Name or Models)
- [ ] All tests use `testing.T` and standard `testing` assertions (or a project-standard assertion library if one exists)
- [ ] All tests pass with `go test ./internal/provider/tracking/...` with no failures
- [ ] No test depends on real SQLite, real network calls, or real time delays (use mocks for all external dependencies; latency assertions use `> 0` not exact values)
