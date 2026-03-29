# Task 06: Unit Tests

**Epic:** 03 — Anthropic Provider
**Status:** Not started
**Dependencies:** Task 01 (provider struct, request building), Task 02 (Complete method, response parsing), Task 03 (SSE streaming parser), Task 04 (extended thinking, prompt caching), Task 05 (error handling, retry logic)

---

## Description

Write unit tests for the Anthropic provider covering request building, content block parsing, SSE event parsing, error classification, and retry logic. These tests do NOT make real HTTP calls; they test individual functions and methods in isolation using constructed inputs and expected outputs. Each test function targets a specific behavior and uses table-driven tests where multiple input/output cases are verified.

## Acceptance Criteria

- [ ] File `internal/provider/anthropic/provider_test.go` exists with `package anthropic`
- [ ] All tests pass with `go test ./internal/provider/anthropic/...`

### Request Building Tests

- [ ] Test `TestBuildRequestBody_Defaults`: calls `buildRequestBody` with a minimal `provider.Request` (only `Messages` set with one user message `"hello"`) and `stream=false`. Asserts: `Model` is `"claude-sonnet-4-6-20250514"`, `MaxTokens` is `8192`, `Temperature` is nil, `Stream` is `false`, `Thinking` is nil, `System` is nil, `Tools` is nil
- [ ] Test `TestBuildRequestBody_AllFields`: calls `buildRequestBody` with a fully populated `provider.Request`:
  - `Model: "claude-opus-4-6-20250515"`
  - `MaxTokens: 4096`
  - `Temperature: ptr(0.7)` (where `ptr` returns `*float64`)
  - Two `SystemBlocks`: one with `CacheControl` set to `{Type: "ephemeral"}`, one without
  - Two `Messages`: one user message `"Fix the auth bug"`, one assistant message with a text content block
  - One `ToolDefinition`: `{Name: "file_read", Description: "Read a file", InputSchema: json.RawMessage(...)}`
  - `stream=true`

  Asserts the `apiRequest` fields match: `Model` is `"claude-opus-4-6-20250515"`, `MaxTokens` is `4096`, `Temperature` is `0.7`, `Stream` is `true`, first system block has `cache_control` set, second does not, messages are converted, tool is converted
- [ ] Test `TestBuildRequestBody_ToolMessage`: calls `buildRequestBody` with a `provider.Message` that has `Role: provider.RoleTool`, `ToolUseID: "toolu_1"`, and `Content` set to a JSON string. Asserts the resulting `apiMessage` has `Role: "user"` and `Content` is a JSON array containing `[{"type":"tool_result","tool_use_id":"toolu_1","content":"..."}]`
- [ ] Test `TestBuildRequestBody_ThinkingEnabled`: calls `buildRequestBody` with `ProviderOptions` set to `AnthropicOptions{ThinkingEnabled: true, ThinkingBudget: 5000}`. Asserts `apiRequest.Thinking` is non-nil with `Type: "enabled"` and `BudgetTokens: 5000`
- [ ] Test `TestBuildRequestBody_ThinkingDefaultBudget`: calls `buildRequestBody` with `ProviderOptions` set to `AnthropicOptions{ThinkingEnabled: true, ThinkingBudget: 0}`. Asserts `apiRequest.Thinking.BudgetTokens` is `10000` (the default)
- [ ] Test `TestBuildRequestBody_ThinkingDisablesTemperature`: calls `buildRequestBody` with `ProviderOptions` set to `AnthropicOptions{ThinkingEnabled: true}` and `Temperature` set to `ptr(0.5)`. Asserts `apiRequest.Temperature` is nil

### HTTP Request Tests

- [ ] Test `TestBuildHTTPRequest_Headers_OAuth`: creates an `AnthropicProvider` with a mock `CredentialManager` that returns `("Authorization", "Bearer test-token", nil)`. Calls `buildHTTPRequest`. Asserts headers: `Authorization: Bearer test-token`, `anthropic-version: 2023-06-01`, `anthropic-beta: interleaved-thinking-2025-05-14,oauth-2025-04-20`, `Content-Type: application/json`
- [ ] Test `TestBuildHTTPRequest_Headers_APIKey`: creates an `AnthropicProvider` with a mock `CredentialManager` that returns `("X-Api-Key", "sk-test-key", nil)`. Calls `buildHTTPRequest`. Asserts header `X-Api-Key: sk-test-key` is present and `Authorization` header is absent
- [ ] Test `TestBuildHTTPRequest_URL`: asserts the request URL is `<baseURL>/v1/messages` and method is `POST`
- [ ] Test `TestBuildHTTPRequest_CredentialError`: creates a mock `CredentialManager` that returns an error. Asserts `buildHTTPRequest` returns a `*provider.ProviderError` with `Retriable: false`

### Response Parsing Tests

- [ ] Test `TestParseResponse_TextOnly`: parses this JSON response body:
  ```json
  {"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"Hello world"}],"model":"claude-sonnet-4-6-20250514","stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}
  ```
  Asserts: one `ContentBlock` with `Type: "text"`, `Text: "Hello world"`, `StopReason` is `provider.StopReasonEndTurn`, `Usage.InputTokens` is 10, `Usage.OutputTokens` is 5
- [ ] Test `TestParseResponse_MixedBlocks`: parses a response with three content blocks: `thinking`, `text`, `tool_use`. Asserts all three are correctly parsed into `provider.ContentBlock` values with correct types and fields
- [ ] Test `TestParseResponse_ToolUse`: parses a response with a `tool_use` content block containing `{"type":"tool_use","id":"toolu_1","name":"file_read","input":{"path":"auth.go"}}`. Asserts `ContentBlock.ID` is `"toolu_1"`, `Name` is `"file_read"`, `Input` is `json.RawMessage({"path":"auth.go"})`
- [ ] Test `TestParseResponse_CacheUsage`: parses a response with `cache_read_input_tokens: 1200` and `cache_creation_input_tokens: 323`. Asserts `Usage.CacheReadTokens` is 1200 and `Usage.CacheCreationTokens` is 323
- [ ] Test `TestParseResponse_StopReasons`: table-driven test mapping each Anthropic stop_reason to the expected `provider.StopReason`: `"end_turn"` to `StopReasonEndTurn`, `"tool_use"` to `StopReasonToolUse`, `"max_tokens"` to `StopReasonMaxTokens`, `"unknown_value"` to `StopReasonEndTurn`
- [ ] Test `TestParseResponse_MalformedJSON`: passes invalid JSON to the response parser. Asserts a `*provider.ProviderError` is returned with `Retriable: false` and message containing `"failed to parse response"`

### Error Classification Tests

- [ ] Test `TestClassifyError_StatusCodes`: table-driven test with rows:
  | Status Code | Expected Retriable | Expected Message Contains |
  |---|---|---|
  | 401 | false | "authentication failed" |
  | 403 | false | "authentication failed" |
  | 429 | true | "rate limit" |
  | 400 | false | "bad request" |
  | 500 | true | "internal server error" |
  | 502 | true | "bad gateway" |
  | 503 | true | "service unavailable" |
  | 418 | false | "API error (418)" |
- [ ] Test `TestClassifyNetworkError`: passes a network error (e.g., `&net.OpError{}`). Asserts `Retriable: true`, `StatusCode: 0`, message contains `"network error"`

### Retry Logic Tests

- [ ] Test `TestDoWithRetry_SuccessFirstAttempt`: `fn` returns a 200 response on the first call. Asserts the response is returned and `fn` was called exactly once
- [ ] Test `TestDoWithRetry_RetryOn429`: `fn` returns 429 on first call, 200 on second call. Asserts the 200 response is returned and `fn` was called exactly twice
- [ ] Test `TestDoWithRetry_RetryOn500`: `fn` returns 500 on first two calls, 200 on third call. Asserts the 200 response is returned and `fn` was called exactly three times
- [ ] Test `TestDoWithRetry_NoRetryOn401`: `fn` returns 401. Asserts the error is returned immediately and `fn` was called exactly once
- [ ] Test `TestDoWithRetry_NoRetryOn400`: `fn` returns 400. Asserts the error is returned immediately and `fn` was called exactly once
- [ ] Test `TestDoWithRetry_ExhaustedRetries`: `fn` returns 503 on all three calls. Asserts the error from the last attempt is returned and `fn` was called exactly three times
- [ ] Test `TestDoWithRetry_ContextCancellation`: passes a pre-cancelled context. Asserts the function returns promptly with a context-related error
- [ ] Test `TestDoWithRetry_NetworkError`: `fn` returns a network error (nil response, non-nil error). Asserts retry occurs (network errors are retriable)

### Models Test

- [ ] Test `TestModels`: calls `Models(ctx)` and asserts exactly three models are returned with IDs `"claude-sonnet-4-6-20250514"`, `"claude-opus-4-6-20250515"`, `"claude-haiku-4-5-20251001"`, all with `ContextWindow: 200000`, `SupportsTools: true`, `SupportsThinking: true`

### Test Helpers

- [ ] A test helper `ptr[T any](v T) *T` is defined in the test file to create pointers to literal values (used for `*float64` temperature)
- [ ] A test helper or mock for `CredentialManager` is used in HTTP request tests that returns controlled auth header values without touching the filesystem
- [ ] The file compiles and all tests pass with `go test -v ./internal/provider/anthropic/...`
