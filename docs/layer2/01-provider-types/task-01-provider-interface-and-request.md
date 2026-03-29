# Task 01: Provider Interface and Request Struct

**Epic:** 01 — Provider Types & Interface
**Status:** ⬚ Not started
**Dependencies:** L0-E01 (project scaffolding), L0-E03 (config loading)

---

## Description

Define the `Provider` interface and the `Request` struct in `internal/provider/provider.go`. The `Provider` interface is the central abstraction that all LLM backends (Anthropic, OpenAI, Codex) implement. The `Request` struct carries every parameter needed to make an LLM call, including conversation history, tool definitions, model selection, sampling parameters, system prompt blocks with cache control support, and sub-call tracking metadata. This task also defines the supporting types `SystemBlock`, `CacheControl`, and `ToolDefinition` that are embedded in the request.

## Acceptance Criteria

- [ ] File `internal/provider/provider.go` exists with `package provider`
- [ ] The `Provider` interface is defined with exactly these four methods:
  ```go
  type Provider interface {
      Complete(ctx context.Context, req *Request) (*Response, error)
      Stream(ctx context.Context, req *Request) (<-chan StreamEvent, error)
      Models(ctx context.Context) ([]Model, error)
      Name() string
  }
  ```
- [ ] The `Request` struct is defined with exactly these fields:
  ```go
  type Request struct {
      Messages        []Message        `json:"messages"`
      Tools           []ToolDefinition `json:"tools,omitempty"`
      Model           string           `json:"model"`
      Temperature     *float64         `json:"temperature,omitempty"`
      MaxTokens       int              `json:"max_tokens"`
      SystemBlocks    []SystemBlock    `json:"system,omitempty"`
      ProviderOptions json.RawMessage  `json:"provider_options,omitempty"`
      Purpose         string           `json:"purpose,omitempty"`
      ConversationID  string           `json:"conversation_id,omitempty"`
      TurnNumber      int              `json:"turn_number,omitempty"`
      Iteration       int              `json:"iteration,omitempty"`
  }
  ```
- [ ] `Temperature` is `*float64` (pointer) so that zero-value and absent are distinguishable
- [ ] `ProviderOptions` is `json.RawMessage` so provider-specific config passes through without the core package knowing its shape
- [ ] `Purpose` is a plain string; valid values are `"chat"`, `"compression"`, `"title_generation"` (documented in a comment, not enforced by the type)
- [ ] The `CacheControl` struct is defined:
  ```go
  type CacheControl struct {
      Type string `json:"type"` // "ephemeral"
  }
  ```
- [ ] The `SystemBlock` struct is defined:
  ```go
  type SystemBlock struct {
      Text         string        `json:"text"`
      CacheControl *CacheControl `json:"cache_control,omitempty"`
  }
  ```
- [ ] The `ToolDefinition` struct is defined:
  ```go
  type ToolDefinition struct {
      Name        string          `json:"name"`
      Description string          `json:"description"`
      InputSchema json.RawMessage `json:"input_schema"`
  }
  ```
- [ ] `ToolDefinition.InputSchema` is `json.RawMessage` so JSON Schema tool definitions pass through without deserialization
- [ ] The file imports `context` and `encoding/json` (no other external dependencies)
- [ ] The file compiles with `go build ./internal/provider/...` with no errors
- [ ] Forward references to `Response`, `StreamEvent`, `Model`, and `Message` are in the same package (no import cycle); these types may be undefined until Tasks 02-05 are complete, but the file itself must have valid syntax
