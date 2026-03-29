# Task 02: Response, Usage, and StopReason Types

**Epic:** 01 — Provider Types & Interface
**Status:** ⬚ Not started
**Dependencies:** Task 01 (Provider Interface and Request Struct)

---

## Description

Define the `Response`, `Usage`, and `StopReason` types in `internal/provider/response.go`. The `Response` struct is returned by `Provider.Complete()` and carries the assistant's reply as a slice of `ContentBlock` values, token usage statistics, the model that actually served the request, the stop reason, and measured latency. `Usage` tracks all four token counters including Anthropic prompt caching counters. `StopReason` is a string type with four named constants covering every way an LLM response can terminate.

## Acceptance Criteria

- [ ] File `internal/provider/response.go` exists with `package provider`
- [ ] The `StopReason` type is defined as a named string type:
  ```go
  type StopReason string
  ```
- [ ] Exactly four `StopReason` constants are defined:
  ```go
  const (
      StopReasonEndTurn   StopReason = "end_turn"
      StopReasonToolUse   StopReason = "tool_use"
      StopReasonMaxTokens StopReason = "max_tokens"
      StopReasonCancelled StopReason = "cancelled"
  )
  ```
- [ ] The `Usage` struct is defined with exactly these four fields:
  ```go
  type Usage struct {
      InputTokens         int `json:"input_tokens"`
      OutputTokens        int `json:"output_tokens"`
      CacheReadTokens     int `json:"cache_read_tokens"`
      CacheCreationTokens int `json:"cache_creation_tokens"`
  }
  ```
- [ ] The `Response` struct is defined with exactly these five fields:
  ```go
  type Response struct {
      Content    []ContentBlock `json:"content"`
      Usage      Usage          `json:"usage"`
      Model      string         `json:"model"`
      StopReason StopReason     `json:"stop_reason"`
      LatencyMs  int64          `json:"-"`
  }
  ```
- [ ] `Response.LatencyMs` has the JSON tag `json:"-"` so it is excluded from serialization (it is a runtime-only measurement)
- [ ] `Response.Content` is `[]ContentBlock` (the `ContentBlock` type is defined in Task 03; forward reference within the same package is acceptable)
- [ ] A helper method `func (u Usage) Total() int` is defined that returns `u.InputTokens + u.OutputTokens`
- [ ] A helper method `func (u Usage) Add(other Usage) Usage` is defined that returns a new `Usage` with each of the four fields summed from `u` and `other`
- [ ] The file compiles with `go build ./internal/provider/...` with no errors (once Task 03 provides `ContentBlock`)
