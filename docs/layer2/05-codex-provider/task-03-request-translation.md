# Task 03: Request Translation (Unified to Responses API)

**Epic:** 05 — Codex Provider
**Status:** ⬚ Not started
**Dependencies:** Task 01 (provider struct, models)

---

## Description

Implement the translation layer that converts sirtopham's unified `provider.Request` into the OpenAI Responses API JSON request body. The Responses API (`POST /v1/responses`) uses a fundamentally different schema from Chat Completions: the message field is called `input` (not `messages`), tool results are `function_call_output` content blocks inside user messages (not separate `role: "tool"` messages), assistant tool calls are `function_call` content blocks (not `tool_calls` array), and tool definitions use `name`/`parameters` at the top level (not nested under `function`). This translation is a pure function with no I/O.

## Acceptance Criteria

- [ ] File `internal/provider/codex/translate.go` exists and declares `package codex`

- [ ] The following unexported types represent the Responses API request JSON shape:

  ```go
  // responsesRequest is the top-level JSON body for POST /v1/responses.
  type responsesRequest struct {
      Model     string              `json:"model"`
      Input     []responsesInput    `json:"input"`
      Tools     []responsesTool     `json:"tools,omitempty"`
      Stream    bool                `json:"stream"`
      Reasoning *responsesReasoning `json:"reasoning,omitempty"`
  }

  // responsesInput represents one item in the input array.
  // For system/user/assistant roles, this is a message with either string
  // content or an array of content blocks.
  type responsesInput struct {
      Role    string      `json:"role"`              // "system", "user", "assistant"
      Content interface{} `json:"content"`           // string or []responsesContentBlock
  }

  // responsesContentBlock represents a typed content block within a message.
  type responsesContentBlock struct {
      Type      string `json:"type"`                        // "text", "function_call", "function_call_output"
      Text      string `json:"text,omitempty"`              // for type="text"
      ID        string `json:"id,omitempty"`                // for type="function_call"
      CallID    string `json:"call_id,omitempty"`           // for type="function_call" and "function_call_output"
      Name      string `json:"name,omitempty"`              // for type="function_call"
      Arguments string `json:"arguments,omitempty"`         // for type="function_call" (JSON string)
      Output    string `json:"output,omitempty"`            // for type="function_call_output"
  }

  // responsesTool represents a tool definition in the tools array.
  type responsesTool struct {
      Type        string          `json:"type"`        // always "function"
      Name        string          `json:"name"`
      Description string          `json:"description,omitempty"`
      Parameters  json.RawMessage `json:"parameters,omitempty"` // JSON Schema object
  }

  // responsesReasoning controls reasoning behavior.
  type responsesReasoning struct {
      Effort           string `json:"effort"`            // "high", "medium", "low"
      EncryptedContent string `json:"encrypted_content"` // "retain"
  }
  ```

- [ ] The translation function is defined:
  ```go
  // buildResponsesRequest translates a unified Request into the Responses API
  // request body. The model parameter comes from the provider config or request.
  func buildResponsesRequest(model string, req *provider.Request, stream bool) responsesRequest
  ```

- [ ] `buildResponsesRequest` sets `responsesRequest.Model` to the `model` parameter

- [ ] `buildResponsesRequest` sets `responsesRequest.Stream` to the `stream` parameter

- [ ] System prompt handling: all entries in `req.SystemBlocks` are concatenated into a single string (joined with `"\n\n"` separator) and emitted as the first input item with `role: "system"` and string content: `responsesInput{Role: "system", Content: "<concatenated text>"}`. The `cache_control` field on system blocks is ignored (the Responses API does not support Anthropic-style cache control). If `req.SystemBlocks` is empty, no system input item is emitted

- [ ] User messages: a unified `Message` with `Role == "user"` becomes `responsesInput{Role: "user", Content: "<text>"}` where the text content is extracted from `msg.Content` (a `json.RawMessage` containing a JSON string that must be unquoted via `json.Unmarshal` into a `string`)

- [ ] Assistant messages: a unified `Message` with `Role == "assistant"` is translated as follows:
  - The `msg.Content` field (a `json.RawMessage`) is unmarshaled into `[]provider.ContentBlock` using `provider.ContentBlocksFromRaw(msg.Content)`
  - Each `ContentBlock` with `Type == "text"` becomes `responsesContentBlock{Type: "text", Text: block.Text}`
  - Each `ContentBlock` with `Type == "tool_use"` becomes `responsesContentBlock{Type: "function_call", ID: "fc_" + block.ID, CallID: block.ID, Name: block.Name, Arguments: string(block.Input)}` where `block.Input` is the raw JSON string of arguments
  - `ContentBlock` values with `Type == "thinking"` are skipped (the Responses API uses encrypted reasoning, not plaintext thinking)
  - The resulting slice of `responsesContentBlock` is set as the `Content` field: `responsesInput{Role: "assistant", Content: blocks}`

- [ ] Tool result messages: a unified `Message` with `Role == "tool"` becomes a user message containing a `function_call_output` content block:
  ```go
  responsesInput{
      Role: "user",
      Content: []responsesContentBlock{{
          Type:   "function_call_output",
          CallID: msg.ToolUseID,
          Output: "<text content from msg.Content>",
      }},
  }
  ```
  The text content is extracted from `msg.Content` by unmarshaling the `json.RawMessage` into a string

- [ ] Tool definitions: each `req.Tools` entry (a `provider.ToolDefinition`) becomes:
  ```go
  responsesTool{
      Type:        "function",
      Name:        tool.Name,
      Description: tool.Description,
      Parameters:  tool.InputSchema,
  }
  ```
  If `req.Tools` is nil or empty, the `tools` field is omitted from the JSON (via `omitempty`)

- [ ] Reasoning configuration: `buildResponsesRequest` always sets `responsesRequest.Reasoning` to `&responsesReasoning{Effort: "high", EncryptedContent: "retain"}` for models `"o3"` and `"o4-mini"`. For model `"gpt-4.1"` (which has no reasoning capability), `Reasoning` is left as `nil` (omitted from JSON)

- [ ] The `model` field in `responsesRequest` is always set to the model string parameter passed into `buildResponsesRequest`

- [ ] All translation logic is pure (no network calls, no side effects, no file I/O) so it can be unit-tested directly

- [ ] The file imports: `encoding/json`, `strings`, and `github.com/<module>/internal/provider`

- [ ] The file compiles with `go build ./internal/provider/codex/...` with no errors
