# Task 01: Provider Struct, Constructor, and Request Building

**Epic:** 03 — Anthropic Provider
**Status:** Not started
**Dependencies:** L2-E01 (all provider types: `Provider` interface, `Request`, `Response`, `StreamEvent`, `Message`, `ContentBlock`, `SystemBlock`, `CacheControl`, `ToolDefinition`, `Model`, `ProviderError`, `Usage`, `StopReason`), L2-E02 (credential manager: `CredentialManager`, `GetAuthHeader`), L0-E02 (structured logging)

---

## Description

Define the `AnthropicProvider` struct, its constructor `NewAnthropicProvider`, and the request body builder that translates a unified `provider.Request` into the Anthropic Messages API JSON format. This includes the `Name()` and `Models()` methods, HTTP header construction (using the credential manager for auth), and the full request body serialization covering messages, system blocks with cache control, tools, model selection, temperature, max tokens, and streaming flag. The request builder is the foundation that both `Complete` (Task 02) and `Stream` (Task 03) call before making the HTTP request.

## Acceptance Criteria

- [ ] File `internal/provider/anthropic/provider.go` exists with `package anthropic`
- [ ] The `AnthropicProvider` struct is defined:
  ```go
  type AnthropicProvider struct {
      creds      *CredentialManager
      httpClient *http.Client
      baseURL    string // default: "https://api.anthropic.com"
  }
  ```
- [ ] A functional option type and option functions are defined:
  ```go
  type ProviderOption func(*AnthropicProvider)

  func WithHTTPClient(c *http.Client) ProviderOption
  func WithBaseURL(url string) ProviderOption
  ```
- [ ] `WithHTTPClient` sets `p.httpClient` to the provided client, overriding the default
- [ ] `WithBaseURL` sets `p.baseURL` to the provided URL (no trailing slash), overriding the default `"https://api.anthropic.com"`
- [ ] The constructor is defined:
  ```go
  func NewAnthropicProvider(creds *CredentialManager, opts ...ProviderOption) *AnthropicProvider
  ```
- [ ] `NewAnthropicProvider` sets default `baseURL` to `"https://api.anthropic.com"` and default `httpClient` to `&http.Client{Timeout: 5 * time.Minute}` (long timeout for streaming responses), then applies all option functions
- [ ] The `Name` method is defined:
  ```go
  func (p *AnthropicProvider) Name() string
  ```
  Returns the string `"anthropic"`
- [ ] The `Models` method is defined:
  ```go
  func (p *AnthropicProvider) Models(ctx context.Context) ([]provider.Model, error)
  ```
  Returns a static slice of exactly three models:
  - `{ID: "claude-sonnet-4-6-20250514", Name: "Claude Sonnet 4.6", ContextWindow: 200000, SupportsTools: true, SupportsThinking: true}`
  - `{ID: "claude-opus-4-6-20250515", Name: "Claude Opus 4.6", ContextWindow: 200000, SupportsTools: true, SupportsThinking: true}`
  - `{ID: "claude-haiku-4-5-20251001", Name: "Claude Haiku 4.5", ContextWindow: 200000, SupportsTools: true, SupportsThinking: true}`
- [ ] An unexported method builds an HTTP request from a unified `provider.Request`:
  ```go
  func (p *AnthropicProvider) buildHTTPRequest(ctx context.Context, req *provider.Request, stream bool) (*http.Request, error)
  ```
- [ ] `buildHTTPRequest` calls `p.creds.GetAuthHeader(ctx)` to obtain the auth header name and value; if this returns an error, `buildHTTPRequest` returns a `*provider.ProviderError` with `Provider: "anthropic"`, `StatusCode: 0`, `Message: "failed to obtain credentials: <underlying error>"`, `Retriable: false`
- [ ] `buildHTTPRequest` sets exactly these HTTP headers on the request:
  - The auth header returned by `GetAuthHeader` (either `X-Api-Key: <api_key>` or `Authorization: Bearer <access_token>`)
  - `anthropic-version: 2023-06-01`
  - `anthropic-beta: interleaved-thinking-2025-05-14,oauth-2025-04-20`
  - `Content-Type: application/json`
- [ ] `buildHTTPRequest` targets the URL `p.baseURL + "/v1/messages"` with HTTP method `POST`
- [ ] An unexported type represents the Anthropic request body:
  ```go
  type apiRequest struct {
      Model       string             `json:"model"`
      MaxTokens   int                `json:"max_tokens"`
      Temperature *float64           `json:"temperature,omitempty"`
      System      []apiSystemBlock   `json:"system,omitempty"`
      Messages    []apiMessage       `json:"messages"`
      Tools       []apiTool          `json:"tools,omitempty"`
      Stream      bool               `json:"stream"`
      Thinking    *apiThinking       `json:"thinking,omitempty"`
  }
  ```
- [ ] The `apiSystemBlock` type is defined:
  ```go
  type apiSystemBlock struct {
      Type         string            `json:"type"`
      Text         string            `json:"text"`
      CacheControl *apiCacheControl  `json:"cache_control,omitempty"`
  }
  ```
- [ ] The `apiCacheControl` type is defined:
  ```go
  type apiCacheControl struct {
      Type string `json:"type"` // always "ephemeral"
  }
  ```
- [ ] The `apiMessage` type is defined:
  ```go
  type apiMessage struct {
      Role    string          `json:"role"`
      Content json.RawMessage `json:"content"`
  }
  ```
- [ ] The `apiTool` type is defined:
  ```go
  type apiTool struct {
      Name        string          `json:"name"`
      Description string          `json:"description"`
      InputSchema json.RawMessage `json:"input_schema"`
  }
  ```
- [ ] The `apiThinking` type is defined:
  ```go
  type apiThinking struct {
      Type         string `json:"type"`          // always "enabled"
      BudgetTokens int    `json:"budget_tokens"`
  }
  ```
- [ ] An unexported method builds the API request body:
  ```go
  func (p *AnthropicProvider) buildRequestBody(req *provider.Request, stream bool) (*apiRequest, error)
  ```
- [ ] `buildRequestBody` sets `apiRequest.Model` to `req.Model`; if `req.Model` is empty, it defaults to `"claude-sonnet-4-6-20250514"`
- [ ] `buildRequestBody` sets `apiRequest.MaxTokens` to `req.MaxTokens`; if `req.MaxTokens` is 0, it defaults to `8192`
- [ ] `buildRequestBody` sets `apiRequest.Temperature` to `req.Temperature` (preserving nil if not set)
- [ ] `buildRequestBody` sets `apiRequest.Stream` to the `stream` parameter
- [ ] `buildRequestBody` converts `req.SystemBlocks` to `[]apiSystemBlock`: each `provider.SystemBlock` becomes `apiSystemBlock{Type: "text", Text: sb.Text}`; if `sb.CacheControl` is non-nil, `apiSystemBlock.CacheControl` is set to `&apiCacheControl{Type: sb.CacheControl.Type}`
- [ ] `buildRequestBody` converts `req.Messages` to `[]apiMessage`: each `provider.Message` becomes `apiMessage{Role: string(msg.Role), Content: msg.Content}`; `provider.RoleTool` messages are converted to role `"user"` with a JSON content array containing a single `tool_result` object: `[{"type": "tool_result", "tool_use_id": msg.ToolUseID, "content": <text from msg.Content>}]`
- [ ] `buildRequestBody` converts `req.Tools` to `[]apiTool`: each `provider.ToolDefinition` becomes `apiTool{Name: td.Name, Description: td.Description, InputSchema: td.InputSchema}`
- [ ] `buildRequestBody` does NOT set `apiRequest.Thinking` (this is handled by Task 04)
- [ ] `buildHTTPRequest` calls `buildRequestBody`, marshals the result with `json.Marshal`, and sets it as the HTTP request body with `bytes.NewReader`
- [ ] `buildHTTPRequest` passes the `ctx` to `http.NewRequestWithContext` so the request respects context cancellation
- [ ] The file imports: `bytes`, `context`, `encoding/json`, `net/http`, `time`, and `github.com/<module>/internal/provider`
- [ ] The file compiles with `go build ./internal/provider/anthropic/...` with no errors
