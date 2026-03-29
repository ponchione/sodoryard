# Task 03: Message and ContentBlock Types

**Epic:** 01 — Provider Types & Interface
**Status:** ⬚ Not started
**Dependencies:** Task 01 (Provider Interface and Request Struct), Task 02 (Response, Usage, and StopReason Types)

---

## Description

Define the `Message`, `Role`, and `ContentBlock` types in `internal/provider/message.go`. Messages use role-based discrimination: user and tool messages carry plain text content, while assistant messages carry a JSON array of typed content blocks (text, thinking, tool_use). The `Content` field is stored as `json.RawMessage` to preserve API-faithful serialization. Typed `ContentBlock` variants and helper constructors provide ergonomic Go-side access for building and parsing messages.

## Acceptance Criteria

- [ ] File `internal/provider/message.go` exists with `package provider`
- [ ] The `Role` type is defined as a named string type:
  ```go
  type Role string
  ```
- [ ] Exactly three `Role` constants are defined:
  ```go
  const (
      RoleUser      Role = "user"
      RoleAssistant Role = "assistant"
      RoleTool      Role = "tool"
  )
  ```
- [ ] The `Message` struct is defined with exactly these four fields:
  ```go
  type Message struct {
      Role      Role            `json:"role"`
      Content   json.RawMessage `json:"content"`
      ToolUseID string          `json:"tool_use_id,omitempty"`
      ToolName  string          `json:"tool_name,omitempty"`
  }
  ```
- [ ] `Message.Content` is `json.RawMessage` so that user/tool messages store a JSON string and assistant messages store a JSON array of content blocks without lossy conversion
- [ ] `Message.ToolUseID` and `Message.ToolName` are only populated when `Role` is `RoleTool`; this invariant is documented in a comment
- [ ] The `ContentBlock` struct is defined with exactly these six fields:
  ```go
  type ContentBlock struct {
      Type     string          `json:"type"`
      Text     string          `json:"text,omitempty"`
      Thinking string          `json:"thinking,omitempty"`
      ID       string          `json:"id,omitempty"`
      Name     string          `json:"name,omitempty"`
      Input    json.RawMessage `json:"input,omitempty"`
  }
  ```
- [ ] `ContentBlock.Type` is one of `"text"`, `"thinking"`, or `"tool_use"` (documented in a comment)
- [ ] For `Type == "text"`: only `Text` is populated
- [ ] For `Type == "thinking"`: only `Thinking` is populated
- [ ] For `Type == "tool_use"`: `ID`, `Name`, and `Input` are populated
- [ ] Constructor `NewTextBlock` is defined:
  ```go
  func NewTextBlock(text string) ContentBlock
  ```
  Returns `ContentBlock{Type: "text", Text: text}`
- [ ] Constructor `NewThinkingBlock` is defined:
  ```go
  func NewThinkingBlock(thinking string) ContentBlock
  ```
  Returns `ContentBlock{Type: "thinking", Thinking: thinking}`
- [ ] Constructor `NewToolUseBlock` is defined:
  ```go
  func NewToolUseBlock(id, name string, input json.RawMessage) ContentBlock
  ```
  Returns `ContentBlock{Type: "tool_use", ID: id, Name: name, Input: input}`
- [ ] Helper `NewUserMessage` is defined:
  ```go
  func NewUserMessage(text string) Message
  ```
  Returns a `Message` with `Role: RoleUser` and `Content` set to the JSON-marshaled string of `text` (i.e., a quoted JSON string like `"hello"`)
- [ ] Helper `NewToolResultMessage` is defined:
  ```go
  func NewToolResultMessage(toolUseID, toolName, content string) Message
  ```
  Returns a `Message` with `Role: RoleTool`, `ToolUseID` set, `ToolName` set, and `Content` set to the JSON-marshaled string of `content`
- [ ] Helper `ContentBlocksFromRaw` is defined:
  ```go
  func ContentBlocksFromRaw(raw json.RawMessage) ([]ContentBlock, error)
  ```
  Unmarshals `raw` (a JSON array) into `[]ContentBlock` and returns it; returns a descriptive error wrapping the underlying `json.Unmarshal` error if the input is not a valid JSON array of content blocks
- [ ] `NewUserMessage` and `NewToolResultMessage` marshal the text content with `json.Marshal` so special characters (quotes, newlines, unicode) are properly escaped
- [ ] The file imports `encoding/json` and `fmt` (for error wrapping in `ContentBlocksFromRaw`)
- [ ] The file compiles with `go build ./internal/provider/...` with no errors
