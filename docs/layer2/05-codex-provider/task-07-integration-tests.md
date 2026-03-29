# Task 07: Integration Test with Mock HTTP Server

**Epic:** 05 — Codex Provider
**Status:** ⬚ Not started
**Dependencies:** Task 04 (Complete method), Task 05 (Stream method)

---

## Description

Write integration tests that exercise the full `CodexProvider` round-trip (request building, HTTP call, response parsing) against a local `httptest.Server` that returns realistic Responses API payloads. These tests validate that the provider correctly serializes requests, parses responses, handles errors, and streams events end-to-end. The tests bypass the real credential flow by injecting a pre-set token and using `WithBaseURL` and `WithHTTPClient` options.

## Acceptance Criteria

- [ ] File `internal/provider/codex/integration_test.go` exists and declares `package codex`

- [ ] The test file uses `//go:build integration` build tag so integration tests can be run separately via `go test -tags=integration ./internal/provider/codex/...`

- [ ] A test helper function creates a `CodexProvider` with injected state, bypassing the real `codex` CLI check:
  ```go
  func newTestProvider(t *testing.T, serverURL string) *CodexProvider
  ```
  This helper constructs a `CodexProvider` with:
  - `baseURL` set to `serverURL`
  - `httpClient` set to the test server's client or `&http.Client{Timeout: 10 * time.Second}`
  - `cachedToken` set to `"test-token-123"` (pre-populated to skip credential delegation)
  - `tokenExpiry` set to 1 hour in the future (so no refresh is triggered)
  - `codexBinPath` set to `"/usr/bin/true"` or any valid binary (never actually called)

- [ ] **Test: Complete with text response.** Start an `httptest.Server` that:
  - Validates the incoming request has `Authorization: Bearer test-token-123`
  - Validates the request body is valid JSON with `"model": "o3"`, `"stream": false`, and an `"input"` array
  - Returns HTTP 200 with this JSON body:
    ```json
    {
        "id": "resp_test_001",
        "object": "response",
        "model": "o3",
        "output": [
            {
                "type": "message",
                "id": "msg_1",
                "role": "assistant",
                "content": [{"type": "output_text", "text": "The bug is in the auth handler."}]
            }
        ],
        "usage": {
            "input_tokens": 200,
            "output_tokens": 50,
            "input_tokens_details": {"cached_tokens": 0},
            "output_tokens_details": {"reasoning_tokens": 0}
        }
    }
    ```
  - Assert the unified `Response` has one `ContentBlock{Type: "text", Text: "The bug is in the auth handler."}`, `StopReason` is `StopReasonEndTurn`, `Usage.InputTokens` is 200, `Usage.OutputTokens` is 50

- [ ] **Test: Complete with tool call response.** Mock server returns:
  ```json
  {
      "id": "resp_test_002",
      "object": "response",
      "model": "o3",
      "output": [
          {
              "type": "reasoning",
              "id": "rs_1",
              "encrypted_content": "base64encrypteddata"
          },
          {
              "type": "message",
              "id": "msg_1",
              "role": "assistant",
              "content": [{"type": "output_text", "text": "Let me read that file."}]
          },
          {
              "type": "function_call",
              "id": "fc_1",
              "call_id": "call_1",
              "name": "file_read",
              "arguments": "{\"path\":\"auth.go\"}"
          }
      ],
      "usage": {
          "input_tokens": 500,
          "output_tokens": 150,
          "input_tokens_details": {"cached_tokens": 100},
          "output_tokens_details": {"reasoning_tokens": 80}
      }
  }
  ```
  Assert the unified `Response` has three content blocks in order: (1) reasoning block with `Text: "base64encrypteddata"`, (2) text block with `Text: "Let me read that file."`, (3) tool_use block with `ID: "call_1"`, `Name: "file_read"`, `Input: json.RawMessage("{\"path\":\"auth.go\"}")`. `StopReason` is `StopReasonToolUse`. `Usage.CacheReadTokens` is 100, `Usage.CacheCreationTokens` is 0

- [ ] **Test: Complete with 401 error.** Mock server returns HTTP 401 with body `{"error": "invalid_token"}`. Assert the returned error is a `*provider.ProviderError` with `StatusCode: 401`, `Message` containing `"Codex authentication failed"`, `Retriable: false`

- [ ] **Test: Complete with 429 rate limit.** Mock server returns HTTP 429 on the first two requests, then HTTP 200 with a valid response on the third. Assert the final result is a successful `*provider.Response` (retries succeeded). The test should verify the server received exactly 3 requests

- [ ] **Test: Complete with 500 retry exhaustion.** Mock server always returns HTTP 500 with body `"internal server error"`. Assert the returned error is a `*provider.ProviderError` with `StatusCode: 500`, `Message` containing `"server error after 3 attempts"`, `Retriable: false`. The test should verify the server received exactly 3 requests

- [ ] **Test: Complete request body validation.** Mock server captures the request body and returns a 200 response. Send a request with system blocks `["You are helpful.", "Context: Go project"]`, one user message `"Fix the bug"`, and one tool definition `{Name: "file_read", Description: "Read a file", InputSchema: ...}`. Assert the captured request body contains:
  - `"model": "o3"`
  - First input item: `{"role": "system", "content": "You are helpful.\n\nContext: Go project"}`
  - Second input item: `{"role": "user", "content": "Fix the bug"}`
  - Tools array with one entry: `{"type": "function", "name": "file_read", ...}`
  - `"reasoning": {"effort": "high", "encrypted_content": "retain"}`

- [ ] **Test: Stream with text response.** Mock server returns HTTP 200 with `Content-Type: text/event-stream` and the following SSE payload:
  ```
  event: response.created
  data: {"type":"response.created","response":{"id":"resp_test_003","status":"in_progress"}}

  event: response.output_item.added
  data: {"type":"response.output_item.added","output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant"}}

  event: response.content_part.added
  data: {"type":"response.content_part.added","item_id":"msg_1","content_index":0,"part":{"type":"output_text","text":""}}

  event: response.output_text.delta
  data: {"type":"response.output_text.delta","item_id":"msg_1","content_index":0,"delta":"Hello "}

  event: response.output_text.delta
  data: {"type":"response.output_text.delta","item_id":"msg_1","content_index":0,"delta":"world!"}

  event: response.content_part.done
  data: {"type":"response.content_part.done","item_id":"msg_1","content_index":0,"part":{"type":"output_text","text":"Hello world!"}}

  event: response.output_item.done
  data: {"type":"response.output_item.done","output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":"Hello world!"}]}}

  event: response.completed
  data: {"type":"response.completed","response":{"id":"resp_test_003","status":"completed","usage":{"input_tokens":100,"output_tokens":10,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}}

  ```
  Collect all events from the stream channel. Assert:
  - Two `provider.TokenDelta` events with `Text` values `"Hello "` and `"world!"`
  - One `provider.StreamDone` event with `StopReason: StopReasonEndTurn`, `Usage.InputTokens: 100`, `Usage.OutputTokens: 10`
  - No `provider.StreamError` events

- [ ] **Test: Stream with tool call.** Mock server returns SSE payload that includes:
  - `response.output_item.added` with `item.type: "function_call"`, `item.call_id: "call_1"`, `item.name: "file_read"`
  - Two `response.function_call_arguments.delta` events with deltas `"{\"path\":"` and `"\"auth.go\"}"`
  - `response.output_item.done` with the complete function_call item
  - `response.completed` with usage
  Collect all events. Assert:
  - One `provider.ToolCallStart{ID: "call_1", Name: "file_read"}`
  - Two `provider.ToolCallDelta` events with `ID: "call_1"`
  - One `provider.ToolCallEnd{ID: "call_1", Input: json.RawMessage("{\"path\":\"auth.go\"}")}`
  - One `provider.StreamDone` with `StopReason: StopReasonToolUse`

- [ ] **Test: Stream context cancellation.** Start a mock server that sends one text delta event then blocks for 30 seconds. Create a context with 500ms timeout. Collect events. Assert at least one `provider.TokenDelta` followed by a `provider.StreamError` with `Fatal: true`

- [ ] All tests clean up their `httptest.Server` instances via `defer server.Close()`

- [ ] All tests use `t.Helper()` in helper functions for clean stack traces

- [ ] The file compiles and all tests pass with `go test -tags=integration ./internal/provider/codex/...`
