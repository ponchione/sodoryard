# Task 02: Request Translation

**Epic:** 04 — OpenAI-Compatible Provider
**Status:** ⬚ Not started
**Dependencies:** Task 01 (provider struct and config)

---

## Description

Implement the translation layer that converts sirtopham's unified `Request` type into the OpenAI Chat Completions JSON request body. This covers system prompt handling, all four message roles (`system`, `user`, `assistant`, `tool`), tool definitions, and request parameters like `temperature` and `max_tokens`. The translation is a pure function with no I/O, making it straightforward to test in isolation.

## Acceptance Criteria

- [ ] File `internal/provider/openai/translate.go` exists and declares `package openai`.

- [ ] The following internal types are defined to represent the OpenAI request JSON shape:
  ```go
  // chatRequest is the top-level JSON body for POST /chat/completions.
  type chatRequest struct {
      Model       string            `json:"model"`
      Messages    []chatMessage     `json:"messages"`
      Tools       []chatTool        `json:"tools,omitempty"`
      Temperature *float64          `json:"temperature,omitempty"`
      MaxTokens   *int              `json:"max_tokens,omitempty"`
      Stream      bool              `json:"stream"`
  }

  // chatMessage represents one message in the messages array.
  type chatMessage struct {
      Role       string         `json:"role"`                  // "system", "user", "assistant", "tool"
      Content    string         `json:"content,omitempty"`     // text content (empty string omitted)
      ToolCalls  []chatToolCall `json:"tool_calls,omitempty"`  // assistant tool invocations
      ToolCallID string         `json:"tool_call_id,omitempty"` // for role=tool, references the call
  }

  // chatToolCall represents one tool invocation from the assistant.
  type chatToolCall struct {
      ID       string           `json:"id"`
      Type     string           `json:"type"` // always "function"
      Function chatFunctionCall `json:"function"`
  }

  // chatFunctionCall holds the function name and JSON-encoded arguments.
  type chatFunctionCall struct {
      Name      string `json:"name"`
      Arguments string `json:"arguments"` // JSON string
  }

  // chatTool represents a tool definition in the tools array.
  type chatTool struct {
      Type     string       `json:"type"` // always "function"
      Function chatFunction `json:"function"`
  }

  // chatFunction describes a callable function.
  type chatFunction struct {
      Name        string          `json:"name"`
      Description string          `json:"description,omitempty"`
      Parameters  json.RawMessage `json:"parameters,omitempty"` // JSON Schema object
  }
  ```

- [ ] The translation function is defined with the following signature:
  ```go
  // buildChatRequest translates a unified Request into the OpenAI chat
  // completions request body. The model parameter comes from the provider config.
  func buildChatRequest(model string, req *provider.Request, stream bool) chatRequest
  ```

- [ ] System prompt handling: all entries in `req.SystemBlocks` are concatenated into a single string (joined with `"\n\n"` separator) and emitted as the first message with `role: "system"`. The `cache_control` field on system blocks is ignored (OpenAI handles caching internally). If `req.SystemBlocks` is empty, no system message is emitted.

- [ ] User messages: a unified `Message` with `Role == "user"` and a single text content block becomes `{"role": "user", "content": "<text>"}`.

- [ ] Assistant messages: a unified `Message` with `Role == "assistant"` is translated as follows:
  - Text content blocks are concatenated (joined with `"\n"`) into the `content` field.
  - Each `ToolUseBlock` in the message becomes an entry in the `tool_calls` array: `{"id": "<block.ID>", "type": "function", "function": {"name": "<block.Name>", "arguments": "<block.Input as JSON string>"}}`. The `arguments` field is the raw JSON string of `block.Input` (a `json.RawMessage` converted via `string()`).
  - If there are no text blocks, `content` is set to the empty string (omitted from JSON by `omitempty`).

- [ ] Tool result messages: a unified `Message` with `Role == "tool"` becomes `{"role": "tool", "tool_call_id": "<msg.ToolCallID>", "content": "<msg.Content as text>"}`.

- [ ] Tool definitions: each `req.Tools` entry (a `ToolDefinition`) becomes:
  ```json
  {
      "type": "function",
      "function": {
          "name": "<tool.Name>",
          "description": "<tool.Description>",
          "parameters": <tool.InputSchema as raw JSON>
      }
  }
  ```
  If `req.Tools` is nil or empty, the `tools` field is omitted from the JSON (via `omitempty`).

- [ ] `Temperature` is set from `req.Temperature` if non-nil. `MaxTokens` is set from `req.MaxTokens` if non-nil. If nil, the pointer fields in `chatRequest` remain nil and are omitted from JSON.

- [ ] The `model` field is always set to the model string parameter passed into `buildChatRequest`.

- [ ] The `stream` field is set to the `stream` bool parameter passed into `buildChatRequest`.

- [ ] All translation logic is pure (no network calls, no side effects) so it can be unit-tested directly.
