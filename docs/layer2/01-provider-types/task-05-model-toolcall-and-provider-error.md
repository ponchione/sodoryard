# Task 05: Model, ToolCall, ToolResult, and ProviderError Types

**Epic:** 01 — Provider Types & Interface
**Status:** ⬚ Not started
**Dependencies:** Task 01 (Provider Interface and Request Struct)

---

## Description

Define the `Model`, `ToolCall`, `ToolResult`, and `ProviderError` types in `internal/provider/types.go`. `Model` describes an LLM model's capabilities and is returned by `Provider.Models()`. `ToolCall` and `ToolResult` are the request/response pair for tool execution. `ProviderError` is a structured error type that all provider implementations use for consistent error handling, carrying HTTP status codes, retry eligibility, and the originating provider name. `ProviderError` implements both the `error` and `Unwrap()` interfaces for integration with Go's `errors.Is`/`errors.As` chain.

## Acceptance Criteria

- [ ] File `internal/provider/types.go` exists with `package provider`
- [ ] The `Model` struct is defined with exactly these five fields:
  ```go
  type Model struct {
      ID               string `json:"id"`
      Name             string `json:"name"`
      ContextWindow    int    `json:"context_window"`
      SupportsTools    bool   `json:"supports_tools"`
      SupportsThinking bool   `json:"supports_thinking"`
  }
  ```
- [ ] `Model.ContextWindow` is `int` representing the maximum number of tokens in the model's context window (used by the budget manager to decide when to compress)
- [ ] `Model.SupportsTools` indicates whether the model can receive tool definitions and produce `tool_use` content blocks
- [ ] `Model.SupportsThinking` indicates whether the model supports extended thinking (reasoning/thinking content blocks)
- [ ] The `ToolCall` struct is defined with exactly these three fields:
  ```go
  type ToolCall struct {
      ID    string          `json:"id"`
      Name  string          `json:"name"`
      Input json.RawMessage `json:"input"`
  }
  ```
- [ ] `ToolCall.ID` is a unique identifier for the tool call (e.g., `"tc_01abc..."`)
- [ ] `ToolCall.Input` is `json.RawMessage` containing the JSON object of tool arguments
- [ ] The `ToolResult` struct is defined with exactly these three fields:
  ```go
  type ToolResult struct {
      ToolUseID string `json:"tool_use_id"`
      Content   string `json:"content"`
      IsError   bool   `json:"is_error,omitempty"`
  }
  ```
- [ ] `ToolResult.ToolUseID` matches the `ToolCall.ID` it responds to
- [ ] `ToolResult.IsError` is `true` when the tool execution failed; `false` (omitted in JSON) on success
- [ ] The `ProviderError` struct is defined with exactly these five fields:
  ```go
  type ProviderError struct {
      Provider   string
      StatusCode int
      Message    string
      Retriable  bool
      Err        error
  }
  ```
- [ ] `ProviderError.Provider` identifies which provider produced the error (e.g., `"anthropic"`, `"openai"`, `"codex"`)
- [ ] `ProviderError.StatusCode` is the HTTP status code from the API response (0 if not an HTTP error, e.g., network failure)
- [ ] `ProviderError.Retriable` is `true` for HTTP status codes 429 (rate limit), 500 (internal server error), 502 (bad gateway), 503 (service unavailable), and for network/connection errors where `StatusCode` is 0. `Retriable` is `false` for HTTP status codes 400 (bad request), 401 (unauthorized), 403 (forbidden), and all other status codes.
- [ ] The `Error()` method is defined with this signature:
  ```go
  func (e *ProviderError) Error() string
  ```
  It returns a string in the format `"<provider>: <message> (status <statusCode>)"` when `StatusCode > 0`, or `"<provider>: <message>"` when `StatusCode` is 0
- [ ] The `Unwrap()` method is defined with this signature:
  ```go
  func (e *ProviderError) Unwrap() error
  ```
  It returns `e.Err`, enabling `errors.Is()` and `errors.As()` to traverse the error chain
- [ ] A constructor function `NewProviderError` is defined:
  ```go
  func NewProviderError(provider string, statusCode int, message string, err error) *ProviderError
  ```
  It sets `Provider`, `StatusCode`, `Message`, `Err`, and automatically determines `Retriable` based on the status code: `true` for 429, 500, 502, 503, or when `statusCode == 0` and `err != nil` (network error); `false` otherwise
- [ ] The file imports `encoding/json` (for `json.RawMessage` in `ToolCall`) and `fmt` (for `Error()` string formatting)
- [ ] The file compiles with `go build ./internal/provider/...` with no errors
