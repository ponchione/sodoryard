# Task 05: Unit Tests for Request and Response Translation

**Epic:** 04 — OpenAI-Compatible Provider
**Status:** ⬚ Not started
**Dependencies:** Task 02 (request translation), Task 03 (response translation), Task 04 (streaming parser)

---

## Description

Write comprehensive unit tests for the pure translation functions that convert between sirtopham's unified types and the OpenAI Chat Completions format. These tests exercise `buildChatRequest`, `translateResponse`, and the SSE chunk parsing logic in isolation with no HTTP or network dependencies. Each test uses concrete, fully specified JSON structures to verify field-level correctness.

## Acceptance Criteria

- [ ] File `internal/provider/openai/translate_test.go` exists and declares `package openai`.

- [ ] **Test: system prompt concatenation.** Given a unified `Request` with two `SystemBlock` entries (text `"You are a coding assistant."` and text `"Always explain your reasoning."`), `buildChatRequest` produces a messages array whose first element is:
  ```json
  {"role": "system", "content": "You are a coding assistant.\n\nAlways explain your reasoning."}
  ```
  Verify the `role` is `"system"` and `content` is the two texts joined with `"\n\n"`.

- [ ] **Test: empty system blocks.** Given a unified `Request` with nil or empty `SystemBlocks`, `buildChatRequest` produces a messages array with no `"system"` role message.

- [ ] **Test: user message translation.** Given a unified `Message` with `Role: "user"` and a single text content block `"Fix the auth bug"`, the output message is:
  ```json
  {"role": "user", "content": "Fix the auth bug"}
  ```

- [ ] **Test: assistant message with text only.** Given a unified `Message` with `Role: "assistant"` and a single text content block `"I'll check the code."`, the output message is:
  ```json
  {"role": "assistant", "content": "I'll check the code."}
  ```
  Verify `tool_calls` is omitted (nil/empty).

- [ ] **Test: assistant message with text and tool calls.** Given a unified `Message` with `Role: "assistant"`, a text block `"I found the issue."`, and one `ToolUseBlock` with `ID: "call_1"`, `Name: "file_read"`, `Input: json.RawMessage('{"path":"auth.go"}')`, the output message is:
  ```json
  {
      "role": "assistant",
      "content": "I found the issue.",
      "tool_calls": [
          {
              "id": "call_1",
              "type": "function",
              "function": {
                  "name": "file_read",
                  "arguments": "{\"path\":\"auth.go\"}"
              }
          }
      ]
  }
  ```

- [ ] **Test: assistant message with tool calls only (no text).** Given a unified `Message` with `Role: "assistant"` and only a `ToolUseBlock` (no text blocks), the output message has an empty `content` (omitted from JSON) and a populated `tool_calls` array.

- [ ] **Test: tool result message.** Given a unified `Message` with `Role: "tool"`, `ToolCallID: "call_1"`, and text content `"package auth..."`, the output message is:
  ```json
  {"role": "tool", "tool_call_id": "call_1", "content": "package auth..."}
  ```

- [ ] **Test: tool definitions translation.** Given a unified `ToolDefinition` with `Name: "file_read"`, `Description: "Read a file"`, and `InputSchema: json.RawMessage('{"type":"object","properties":{"path":{"type":"string"}}}')`, the output tool is:
  ```json
  {
      "type": "function",
      "function": {
          "name": "file_read",
          "description": "Read a file",
          "parameters": {"type": "object", "properties": {"path": {"type": "string"}}}
      }
  }
  ```

- [ ] **Test: no tools.** Given a unified `Request` with nil `Tools`, the output `chatRequest.Tools` is nil and the `"tools"` key is absent from the JSON-marshalled output.

- [ ] **Test: temperature and max_tokens set.** Given `req.Temperature = float64Ptr(0.7)` and `req.MaxTokens = intPtr(8192)`, verify the output `chatRequest.Temperature` is `0.7` and `MaxTokens` is `8192`.

- [ ] **Test: temperature and max_tokens nil.** Given nil `Temperature` and `MaxTokens`, verify those fields are omitted from JSON output.

- [ ] **Test: stream flag.** Verify `buildChatRequest(model, req, true)` produces `"stream": true` and `buildChatRequest(model, req, false)` produces `"stream": false`.

- [ ] **Test: response with text content only.** Given a `chatResponse` with `choices[0].message.content = "I found the issue."`, `finish_reason = "stop"`, and usage `{prompt_tokens: 100, completion_tokens: 50, total_tokens: 150}`, `translateResponse` produces a unified `Response` with:
  - One `TextBlock` with content `"I found the issue."`
  - `StopReason` = `provider.StopReasonEndTurn`
  - `Usage.InputTokens` = 100
  - `Usage.OutputTokens` = 50
  - `Usage.CacheReadTokens` = 0
  - `Usage.CacheCreationTokens` = 0

- [ ] **Test: response with tool calls.** Given a `chatResponse` with `choices[0].message.tool_calls` containing one entry (`id: "call_1"`, `function.name: "file_read"`, `function.arguments: '{"path":"auth.go"}'`) and `finish_reason = "tool_calls"`, `translateResponse` produces:
  - One `ToolUseBlock` with `ID = "call_1"`, `Name = "file_read"`, `Input = json.RawMessage('{"path":"auth.go"}')`
  - `StopReason` = `provider.StopReasonToolUse`

- [ ] **Test: response with text and tool calls.** Given content `"I found the issue."` and one tool call, `translateResponse` produces a `TextBlock` followed by a `ToolUseBlock` in the `Content` slice.

- [ ] **Test: finish reason mapping.** Verify all mappings:
  - `"stop"` -> `provider.StopReasonEndTurn`
  - `"tool_calls"` -> `provider.StopReasonToolUse`
  - `"length"` -> `provider.StopReasonMaxTokens`
  - `""` (empty string) -> `provider.StopReasonEndTurn` (fallback)
  - `"content_filter"` (unknown) -> `provider.StopReasonEndTurn` (fallback)

- [ ] **Test: empty choices.** Given a `chatResponse` with empty `choices` slice, `translateResponse` returns an error containing `"response contained no choices"`.

- [ ] **Test: full round-trip request.** Build a unified `Request` with system blocks, a user message, an assistant message with tool calls, a tool result, tool definitions, temperature 0.7, and max_tokens 8192. Call `buildChatRequest`, JSON-marshal the result, unmarshal back, and verify every field matches the expected OpenAI JSON structure:
  ```json
  {
      "model": "qwen2.5-coder-7b",
      "messages": [
          {"role": "system", "content": "You are a helpful assistant."},
          {"role": "user", "content": "Fix the auth bug"},
          {"role": "assistant", "content": "I'll check the code.", "tool_calls": [
              {"id": "call_1", "type": "function", "function": {"name": "file_read", "arguments": "{\"path\":\"auth.go\"}"}}
          ]},
          {"role": "tool", "tool_call_id": "call_1", "content": "package auth..."}
      ],
      "tools": [
          {"type": "function", "function": {"name": "file_read", "description": "Read a file", "parameters": {"type": "object", "properties": {"path": {"type": "string"}}}}}
      ],
      "temperature": 0.7,
      "max_tokens": 8192,
      "stream": false
  }
  ```

- [ ] **Test: SSE chunk with text content.** Parse the JSON payload `{"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}` into a `streamChunk` and verify `Choices[0].Delta.Content == "Hello"` and `Choices[0].FinishReason` is nil.

- [ ] **Test: SSE tool call accumulation.** Simulate three sequential chunks for a single tool call:
  1. `delta.tool_calls: [{"index":0, "id":"call_1", "type":"function", "function":{"name":"file_read", "arguments":""}}]`
  2. `delta.tool_calls: [{"index":0, "function":{"arguments":"{\"path\":"}}]`
  3. `delta.tool_calls: [{"index":0, "function":{"arguments":"\"auth.go\"}"}}]`
  Verify the accumulated tool call has `ID = "call_1"`, `Name = "file_read"`, `Arguments = '{"path":"auth.go"}'`.

- [ ] **Test: SSE finish_reason detection.** Parse a chunk with `"finish_reason":"stop"` and verify `Choices[0].FinishReason` is a non-nil pointer to `"stop"`.

- [ ] All tests use `t.Run` subtests with descriptive names and produce clear failure messages showing expected vs. actual values.
