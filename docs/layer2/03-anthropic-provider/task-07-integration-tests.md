# Task 07: Integration Tests with Mock HTTP Server

**Epic:** 03 — Anthropic Provider
**Status:** Not started
**Dependencies:** Task 01 (provider struct, constructor, request building), Task 02 (Complete method), Task 03 (Stream method), Task 04 (extended thinking, prompt caching), Task 05 (error handling, retry logic)

---

## Description

Write integration tests using `net/http/httptest.Server` that exercise the full Anthropic provider through realistic request/response cycles. The mock server returns Anthropic-shaped JSON responses for `Complete` and Anthropic-shaped SSE streams for `Stream`, verifying that the entire pipeline works end-to-end: credential injection, request serialization, HTTP transport, response parsing, streaming event emission, error classification, and retry behavior. These tests validate the provider's behavior against the real API contract without making external network calls.

## Acceptance Criteria

- [ ] File `internal/provider/anthropic/integration_test.go` exists with `package anthropic`
- [ ] All tests pass with `go test ./internal/provider/anthropic/...`
- [ ] Each test creates a `httptest.NewServer` with a handler that validates the incoming request and returns a canned response, then creates an `AnthropicProvider` using `WithBaseURL(server.URL)` and a mock `CredentialManager`

### Complete Method Integration Tests

- [ ] Test `TestIntegration_Complete_TextResponse`: the mock server handler:
  1. Asserts the request method is `POST` and path is `/v1/messages`
  2. Asserts the `anthropic-version` header is `"2023-06-01"`
  3. Asserts the `anthropic-beta` header contains `"interleaved-thinking-2025-05-14"` and `"oauth-2025-04-20"`
  4. Asserts the `Content-Type` header is `"application/json"`
  5. Asserts the `Authorization` header is `"Bearer test-token"` (from the mock credential manager)
  6. Decodes the request body JSON and asserts `model`, `max_tokens`, and `stream: false` are present
  7. Returns HTTP 200 with this JSON body:
  ```json
  {
      "id": "msg_test1",
      "type": "message",
      "role": "assistant",
      "content": [
          {"type": "text", "text": "The answer is 42."}
      ],
      "model": "claude-sonnet-4-6-20250514",
      "stop_reason": "end_turn",
      "usage": {"input_tokens": 50, "output_tokens": 12, "cache_read_input_tokens": 30, "cache_creation_input_tokens": 20}
  }
  ```

  The test calls `provider.Complete(ctx, req)` and asserts:
  - No error returned
  - `resp.Content` has one block with `Type: "text"`, `Text: "The answer is 42."`
  - `resp.StopReason` is `provider.StopReasonEndTurn`
  - `resp.Model` is `"claude-sonnet-4-6-20250514"`
  - `resp.Usage.InputTokens` is 50, `OutputTokens` is 12, `CacheReadTokens` is 30, `CacheCreationTokens` is 20
  - `resp.LatencyMs` is greater than 0

- [ ] Test `TestIntegration_Complete_ToolUseResponse`: the mock server returns a response with mixed content blocks (text + tool_use) and `stop_reason: "tool_use"`:
  ```json
  {
      "id": "msg_test2",
      "type": "message",
      "role": "assistant",
      "content": [
          {"type": "text", "text": "Let me read that file."},
          {"type": "tool_use", "id": "toolu_abc", "name": "file_read", "input": {"path": "main.go"}}
      ],
      "model": "claude-sonnet-4-6-20250514",
      "stop_reason": "tool_use",
      "usage": {"input_tokens": 100, "output_tokens": 45, "cache_read_input_tokens": 0, "cache_creation_input_tokens": 0}
  }
  ```
  The test asserts:
  - `resp.Content` has two blocks: first is text, second is tool_use
  - Second block has `ID: "toolu_abc"`, `Name: "file_read"`, `Input` contains `{"path": "main.go"}`
  - `resp.StopReason` is `provider.StopReasonToolUse`

- [ ] Test `TestIntegration_Complete_ThinkingResponse`: the mock server returns a response with thinking + text content blocks:
  ```json
  {
      "id": "msg_test3",
      "type": "message",
      "role": "assistant",
      "content": [
          {"type": "thinking", "thinking": "Let me analyze the code..."},
          {"type": "text", "text": "I found the issue in auth.go."}
      ],
      "model": "claude-sonnet-4-6-20250514",
      "stop_reason": "end_turn",
      "usage": {"input_tokens": 80, "output_tokens": 30, "cache_read_input_tokens": 0, "cache_creation_input_tokens": 0}
  }
  ```
  The test asserts:
  - `resp.Content` has two blocks: first is `Type: "thinking"` with `Thinking: "Let me analyze the code..."`, second is `Type: "text"`

- [ ] Test `TestIntegration_Complete_RequestBodyValidation`: the mock server decodes the full request body and asserts all fields are correctly serialized:
  - `system` array has two entries, first with `cache_control: {"type": "ephemeral"}`, second without
  - `messages` array is correctly serialized
  - `tools` array has the expected tool definition with `input_schema`
  - `thinking` object is present with `type: "enabled"` and `budget_tokens: 10000`
  - `stream` is `false`

### Stream Method Integration Tests

- [ ] Test `TestIntegration_Stream_TextOnly`: the mock server returns this SSE stream (written with `Flusher`):
  ```
  event: message_start
  data: {"type":"message_start","message":{"id":"msg_s1","type":"message","role":"assistant","model":"claude-sonnet-4-6-20250514","usage":{"input_tokens":50,"output_tokens":0,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}

  event: content_block_start
  data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

  event: content_block_delta
  data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}

  event: content_block_delta
  data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}

  event: content_block_stop
  data: {"type":"content_block_stop","index":0}

  event: message_delta
  data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":8}}

  event: message_stop
  data: {"type":"message_stop"}
  ```
  The mock server sets `Content-Type: text/event-stream` and uses `http.Flusher` to flush after each SSE event pair.

  The test collects all events from the channel and asserts this exact sequence:
  1. `provider.StreamUsage` with `Usage.InputTokens: 50`
  2. `provider.TokenDelta{Text: "Hello"}`
  3. `provider.TokenDelta{Text: " world"}`
  4. `provider.StreamDone{StopReason: provider.StopReasonEndTurn}` with `Usage.OutputTokens: 8`

- [ ] Test `TestIntegration_Stream_ThinkingAndText`: the mock server returns an SSE stream with a thinking block followed by a text block:
  ```
  event: message_start
  data: {"type":"message_start","message":{"id":"msg_s2","type":"message","role":"assistant","model":"claude-sonnet-4-6-20250514","usage":{"input_tokens":80,"output_tokens":0,"cache_read_input_tokens":60,"cache_creation_input_tokens":20}}}

  event: content_block_start
  data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}

  event: content_block_delta
  data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"Let me"}}

  event: content_block_delta
  data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":" analyze..."}}

  event: content_block_stop
  data: {"type":"content_block_stop","index":0}

  event: content_block_start
  data: {"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}

  event: content_block_delta
  data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"I found the issue."}}

  event: content_block_stop
  data: {"type":"content_block_stop","index":1}

  event: message_delta
  data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":25}}

  event: message_stop
  data: {"type":"message_stop"}
  ```
  The test collects all events and asserts this sequence:
  1. `provider.StreamUsage` with `Usage.InputTokens: 80`, `CacheReadTokens: 60`, `CacheCreationTokens: 20`
  2. `provider.ThinkingDelta{Thinking: "Let me"}`
  3. `provider.ThinkingDelta{Thinking: " analyze..."}`
  4. `provider.TokenDelta{Text: "I found the issue."}`
  5. `provider.StreamDone{StopReason: provider.StopReasonEndTurn}` with `Usage.OutputTokens: 25`

- [ ] Test `TestIntegration_Stream_ToolUse`: the mock server returns an SSE stream with a text block followed by a tool_use block with incremental JSON:
  ```
  event: message_start
  data: {"type":"message_start","message":{"id":"msg_s3","type":"message","role":"assistant","model":"claude-sonnet-4-6-20250514","usage":{"input_tokens":100,"output_tokens":0,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}

  event: content_block_start
  data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

  event: content_block_delta
  data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"I'll read the file."}}

  event: content_block_stop
  data: {"type":"content_block_stop","index":0}

  event: content_block_start
  data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_xyz","name":"file_read"}}

  event: content_block_delta
  data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"path\":"}}

  event: content_block_delta
  data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"\"auth.go\"}"}}

  event: content_block_stop
  data: {"type":"content_block_stop","index":1}

  event: message_delta
  data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":35}}

  event: message_stop
  data: {"type":"message_stop"}
  ```
  The test collects all events and asserts this sequence:
  1. `provider.StreamUsage` with `Usage.InputTokens: 100`
  2. `provider.TokenDelta{Text: "I'll read the file."}`
  3. `provider.ToolCallStart{ID: "toolu_xyz", Name: "file_read"}`
  4. `provider.ToolCallDelta{ID: "toolu_xyz", Delta: "{\"path\":"}`
  5. `provider.ToolCallDelta{ID: "toolu_xyz", Delta: "\"auth.go\"}"}`
  6. `provider.ToolCallEnd{ID: "toolu_xyz", Input: json.RawMessage("{\"path\":\"auth.go\"}")}`
  7. `provider.StreamDone{StopReason: provider.StopReasonToolUse}` with `Usage.OutputTokens: 35`

### Error Handling Integration Tests

- [ ] Test `TestIntegration_Complete_AuthError`: the mock server returns HTTP 401 with body `{"error":{"type":"authentication_error","message":"invalid token"}}`. The test asserts:
  - `Complete` returns a `*provider.ProviderError`
  - `StatusCode` is 401
  - `Retriable` is false
  - `Message` contains `"authentication failed"`

- [ ] Test `TestIntegration_Complete_RateLimitRetry`: the mock server returns HTTP 429 on the first two requests, then HTTP 200 with a valid response on the third. The mock server tracks call count using an atomic counter. The test asserts:
  - `Complete` returns a successful response (no error)
  - The mock server received exactly 3 requests

- [ ] Test `TestIntegration_Complete_ServerErrorExhausted`: the mock server always returns HTTP 503. The test asserts:
  - `Complete` returns a `*provider.ProviderError`
  - `StatusCode` is 503
  - `Retriable` is true
  - The mock server received exactly 3 requests (1 initial + 2 retries)

- [ ] Test `TestIntegration_Stream_InitialHTTPError`: the mock server returns HTTP 500. The test asserts:
  - `Stream` returns `nil` channel and a `*provider.ProviderError`
  - After retry exhaustion, the error has `StatusCode: 500` and `Retriable: true`

- [ ] Test `TestIntegration_Complete_BadRequest`: the mock server returns HTTP 400 with body `{"error":{"type":"invalid_request_error","message":"max_tokens must be positive"}}`. The test asserts:
  - `Complete` returns a `*provider.ProviderError`
  - `StatusCode` is 400
  - `Retriable` is false
  - `Message` contains the response body text
  - The mock server received exactly 1 request (no retry)

### Context Cancellation Integration Tests

- [ ] Test `TestIntegration_Complete_ContextCancelled`: creates a context that is cancelled before calling `Complete`. Asserts an error is returned promptly (within 100ms)
- [ ] Test `TestIntegration_Stream_ContextCancelled`: creates a context, starts `Stream`, cancels the context mid-stream (after receiving at least one event). Asserts the channel is eventually closed and the last event is either a `StreamError` with `Fatal: true` or the channel closes without further events

### Test Infrastructure

- [ ] A helper function creates a standard mock server for reuse across tests:
  ```go
  func newMockServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *AnthropicProvider)
  ```
  This creates the `httptest.Server`, a mock `CredentialManager` returning `("Authorization", "Bearer test-token", nil)`, and an `AnthropicProvider` configured with `WithBaseURL(server.URL)` and the mock credential manager. It registers `t.Cleanup(server.Close)`.
- [ ] A helper function writes an SSE event to a `http.ResponseWriter`:
  ```go
  func writeSSE(w http.ResponseWriter, event string, data string)
  ```
  This writes `event: <event>\ndata: <data>\n\n` and calls `w.(http.Flusher).Flush()`
- [ ] The file compiles and all tests pass with `go test -v ./internal/provider/anthropic/...`
