# Task 06: Integration Test with Mock HTTP Server

**Epic:** 04 — OpenAI-Compatible Provider
**Status:** ⬚ Not started
**Dependencies:** Task 03 (Complete method), Task 04 (Stream method)

---

## Description

Write integration tests that exercise the full `OpenAIProvider` against a local `httptest.Server` returning realistic Chat Completions responses and SSE streams. These tests verify the complete path from unified `Request` through HTTP transport to unified `Response`/`StreamEvent`, including header handling, error code behavior, retry logic, and connection failure scenarios.

## Acceptance Criteria

- [ ] File `internal/provider/openai/integration_test.go` exists and declares `package openai`.

- [ ] All tests use `httptest.NewServer` to create a local HTTP server. The server's URL is used as the `BaseURL` in `OpenAIConfig`. Tests do not make any external network calls.

- [ ] **Test: successful non-streaming completion.** The mock server handler at `/chat/completions`:
  - Verifies the request method is `POST`.
  - Verifies `Content-Type` header is `application/json`.
  - Verifies `Authorization` header is `Bearer test-key-123`.
  - Parses the request body and verifies `model` is `"test-model"`, `stream` is `false`, and `messages` contains the expected entries.
  - Returns HTTP 200 with body:
    ```json
    {
        "id": "chatcmpl-test1",
        "object": "chat.completion",
        "model": "test-model",
        "choices": [
            {
                "index": 0,
                "message": {"role": "assistant", "content": "The fix is to add nil checks."},
                "finish_reason": "stop"
            }
        ],
        "usage": {"prompt_tokens": 42, "completion_tokens": 15, "total_tokens": 57}
    }
    ```
  - The test calls `provider.Complete(ctx, req)` and verifies:
    - No error returned.
    - `Response.Content` contains one `TextBlock` with content `"The fix is to add nil checks."`.
    - `Response.StopReason` == `provider.StopReasonEndTurn`.
    - `Response.Usage.InputTokens` == 42.
    - `Response.Usage.OutputTokens` == 15.
    - `Response.Usage.CacheReadTokens` == 0.
    - `Response.Usage.CacheCreationTokens` == 0.

- [ ] **Test: successful completion with tool calls.** The mock returns:
  ```json
  {
      "id": "chatcmpl-test2",
      "object": "chat.completion",
      "model": "test-model",
      "choices": [
          {
              "index": 0,
              "message": {
                  "role": "assistant",
                  "content": "Let me read the file.",
                  "tool_calls": [
                      {
                          "id": "call_abc",
                          "type": "function",
                          "function": {"name": "file_read", "arguments": "{\"path\":\"main.go\"}"}
                      }
                  ]
              },
              "finish_reason": "tool_calls"
          }
      ],
      "usage": {"prompt_tokens": 80, "completion_tokens": 25, "total_tokens": 105}
  }
  ```
  Verify `Response.Content` has a `TextBlock` then a `ToolUseBlock` with `ID = "call_abc"`, `Name = "file_read"`, `Input` equal to `json.RawMessage('{"path":"main.go"}')`. Verify `StopReason` == `provider.StopReasonToolUse`.

- [ ] **Test: no Authorization header when API key is empty.** Create an `OpenAIProvider` with no API key. The mock handler verifies the `Authorization` header is absent (empty string from `r.Header.Get("Authorization")`). The test confirms `Complete` succeeds without auth.

- [ ] **Test: 401 authentication error.** The mock returns HTTP 401 with body `{"error":{"message":"Invalid API key"}}`. Verify `Complete` returns an error containing `"authentication failed. Check API key configuration."`. Verify the mock received exactly 1 request (no retries).

- [ ] **Test: 429 rate limit with retry.** The mock handler tracks request count with an atomic counter. The first two requests return HTTP 429 with body `{"error":{"message":"Rate limit exceeded"}}`. The third request returns HTTP 200 with a valid completion response. Verify `Complete` succeeds and the mock received exactly 3 requests. Verify the total elapsed time is at least 2 seconds (confirming backoff occurred: ~1s + ~2s minimum).

- [ ] **Test: 500 server error exhausts retries.** The mock always returns HTTP 500 with body `{"error":{"message":"Internal server error"}}`. Verify `Complete` returns an error containing `"server error (HTTP 500) after 3 attempts"`. Verify the mock received exactly 3 requests.

- [ ] **Test: context cancellation.** Create a context with `context.WithCancel`, cancel it immediately, then call `Complete`. Verify the returned error is or wraps `context.Canceled`.

- [ ] **Test: successful SSE streaming.** The mock handler returns HTTP 200 with `Content-Type: text/event-stream` and writes the following lines (each followed by `\n\n`):
  ```
  data: {"id":"chatcmpl-stream1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}

  data: {"id":"chatcmpl-stream1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}

  data: {"id":"chatcmpl-stream1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":" world"},"finish_reason":null}]}

  data: {"id":"chatcmpl-stream1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

  data: [DONE]
  ```
  Call `provider.Stream(ctx, req)`, drain the channel, and verify the following events in order:
  1. `StreamEventText` with content `"Hello"`
  2. `StreamEventText` with content `" world"`
  3. `StreamEventStop` with `StopReason == provider.StopReasonEndTurn`
  Verify the channel is closed after all events.

- [ ] **Test: SSE streaming with tool calls.** The mock returns a stream with incremental tool call fragments:
  ```
  data: {"id":"chatcmpl-stream2","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}

  data: {"id":"chatcmpl-stream2","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"Let me check."},"finish_reason":null}]}

  data: {"id":"chatcmpl-stream2","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_s1","type":"function","function":{"name":"file_read","arguments":""}}]},"finish_reason":null}]}

  data: {"id":"chatcmpl-stream2","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"path\":"}}]},"finish_reason":null}]}

  data: {"id":"chatcmpl-stream2","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"auth.go\"}"}}]},"finish_reason":null}]}

  data: {"id":"chatcmpl-stream2","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}

  data: [DONE]
  ```
  Drain the channel and verify events in order:
  1. `StreamEventText` with content `"Let me check."`
  2. `StreamEventToolUse` with `ID = "call_s1"`, `Name = "file_read"`, `Input = json.RawMessage('{"path":"auth.go"}')`
  3. `StreamEventStop` with `StopReason == provider.StopReasonToolUse`

- [ ] **Test: SSE streaming error on initial connect.** The mock returns HTTP 503. Verify `Stream` returns `nil` channel and an error containing `"server error (HTTP 503)"`.

- [ ] **Test: plain text response when tools requested.** The mock returns a 200 completion where the assistant's response is plain text content (no `tool_calls` array) even though the request included tool definitions. Verify `Complete` returns a `Response` with a `TextBlock` and `StopReason == provider.StopReasonEndTurn` (no crash, no error). This validates graceful handling of models that do not support tool calling.

- [ ] **Test: connection refused.** Create an `OpenAIProvider` with `BaseURL` pointing to `http://127.0.0.1:<unused-port>` (a port with nothing listening, e.g., use `net.Listen` to get a free port then close it). Call `Complete` and verify the error contains `"is not reachable. Is the model server running?"`.

- [ ] All mock handlers use `t.Helper()` for clean failure reporting. Tests use `t.Run` subtests for each scenario. Timeouts on channel drains are enforced (e.g., `select` with `time.After(5 * time.Second)`) to prevent tests from hanging indefinitely.
