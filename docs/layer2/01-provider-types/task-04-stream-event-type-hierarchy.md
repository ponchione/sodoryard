# Task 04: StreamEvent Type Hierarchy

**Epic:** 01 — Provider Types & Interface
**Status:** ⬚ Not started
**Dependencies:** Task 02 (Response, Usage, and StopReason Types — needs `Usage` and `StopReason`)

---

## Description

Define the `StreamEvent` interface and all eight concrete event types in `internal/provider/stream.go`. StreamEvent uses the sealed interface pattern (a private marker method) to prevent external packages from implementing the interface, ensuring the set of variants is closed. Each variant carries the data needed for its specific purpose: incremental text deltas, thinking deltas, tool call lifecycle events, usage updates, errors, and stream completion. Consumers use type-switch on the interface to handle each variant.

## Acceptance Criteria

- [ ] File `internal/provider/stream.go` exists with `package provider`
- [ ] The `StreamEvent` interface is defined with a single unexported marker method:
  ```go
  type StreamEvent interface {
      streamEvent()
  }
  ```
- [ ] The marker method `streamEvent()` is unexported (lowercase), which prevents types outside the `provider` package from satisfying the interface
- [ ] The `TokenDelta` struct is defined and implements `StreamEvent`:
  ```go
  type TokenDelta struct {
      Text string
  }
  func (TokenDelta) streamEvent() {}
  ```
  `Text` contains an incremental text fragment from the LLM response
- [ ] The `ThinkingDelta` struct is defined and implements `StreamEvent`:
  ```go
  type ThinkingDelta struct {
      Thinking string
  }
  func (ThinkingDelta) streamEvent() {}
  ```
  `Thinking` contains an incremental thinking/reasoning text fragment (extended thinking content)
- [ ] The `ToolCallStart` struct is defined and implements `StreamEvent`:
  ```go
  type ToolCallStart struct {
      ID   string
      Name string
  }
  func (ToolCallStart) streamEvent() {}
  ```
  Signals the beginning of a tool call; `ID` is the tool call identifier (e.g., `"tc_..."`), `Name` is the tool name (e.g., `"file_read"`)
- [ ] The `ToolCallDelta` struct is defined and implements `StreamEvent`:
  ```go
  type ToolCallDelta struct {
      ID    string
      Delta string
  }
  func (ToolCallDelta) streamEvent() {}
  ```
  `Delta` contains an incremental JSON argument fragment for the tool call identified by `ID`
- [ ] The `ToolCallEnd` struct is defined and implements `StreamEvent`:
  ```go
  type ToolCallEnd struct {
      ID    string
      Input json.RawMessage
  }
  func (ToolCallEnd) streamEvent() {}
  ```
  Signals tool call arguments are complete; `Input` contains the full, assembled JSON arguments
- [ ] The `StreamUsage` struct is defined and implements `StreamEvent`:
  ```go
  type StreamUsage struct {
      Usage Usage
  }
  func (StreamUsage) streamEvent() {}
  ```
  Carries intermediate token usage data; `Usage` is the `Usage` struct from `response.go` with fields `InputTokens`, `OutputTokens`, `CacheReadTokens`, `CacheCreationTokens`
- [ ] The `StreamError` struct is defined and implements `StreamEvent`:
  ```go
  type StreamError struct {
      Err     error
      Fatal   bool
      Message string
  }
  func (StreamError) streamEvent() {}
  ```
  `Fatal` is `true` for non-recoverable errors (stream should terminate); `false` for recoverable errors (stream may continue). `Message` is a human-readable error description. `Err` is the underlying Go error.
- [ ] The `StreamDone` struct is defined and implements `StreamEvent`:
  ```go
  type StreamDone struct {
      StopReason StopReason
      Usage      Usage
  }
  func (StreamDone) streamEvent() {}
  ```
  Signals stream completion; `StopReason` is one of `"end_turn"`, `"tool_use"`, `"max_tokens"`, `"cancelled"`. `Usage` carries final token counts (may duplicate a preceding `StreamUsage` event).
- [ ] Exactly eight types implement `StreamEvent`: `TokenDelta`, `ThinkingDelta`, `ToolCallStart`, `ToolCallDelta`, `ToolCallEnd`, `StreamUsage`, `StreamError`, `StreamDone`
- [ ] Each of the eight types has the method `func (<Type>) streamEvent() {}` with a value receiver (not pointer receiver)
- [ ] The file imports `encoding/json` (for `json.RawMessage` in `ToolCallEnd`)
- [ ] The file compiles with `go build ./internal/provider/...` with no errors
