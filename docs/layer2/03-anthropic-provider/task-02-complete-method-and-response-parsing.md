# Task 02: Complete Method and Response Parsing

**Epic:** 03 — Anthropic Provider
**Status:** Not started
**Dependencies:** Task 01 (provider struct, constructor, `buildHTTPRequest`, API request types)

---

## Description

Implement the non-streaming `Complete` method on `AnthropicProvider`. This method calls `buildHTTPRequest` with `stream=false`, executes the HTTP POST to the Anthropic Messages API (`POST <baseURL>/v1/messages`), reads the full JSON response body, and parses Anthropic's typed content blocks into the unified `provider.Response`. The response parser handles all three content block types (text, thinking, tool_use), extracts token usage including cache counters, maps the Anthropic stop reason to the unified `StopReason`, and measures request latency. Error responses (non-2xx status codes) are classified and returned as `*provider.ProviderError` without retry (retry logic is added in Task 05).

## Acceptance Criteria

- [ ] The `Complete` method is defined on `AnthropicProvider`:
  ```go
  func (p *AnthropicProvider) Complete(ctx context.Context, req *provider.Request) (*provider.Response, error)
  ```
- [ ] `Complete` calls `p.buildHTTPRequest(ctx, req, false)` to construct the HTTP request with `stream` set to `false`
- [ ] `Complete` records the start time via `time.Now()` before calling `p.httpClient.Do(httpReq)` and computes `LatencyMs` as `time.Since(start).Milliseconds()` after the response is received
- [ ] `Complete` reads the entire response body using `io.ReadAll(resp.Body)` and defers `resp.Body.Close()`
- [ ] If the HTTP status code is not 200, `Complete` returns a `*provider.ProviderError` with `Provider: "anthropic"`, `StatusCode` set to the HTTP status code, `Message` set to the response body (truncated to 1024 bytes if longer), and `Retriable` determined automatically by `provider.NewProviderError`
- [ ] The Anthropic JSON response is deserialized into this unexported type:
  ```go
  type apiResponse struct {
      ID         string            `json:"id"`
      Type       string            `json:"type"`
      Role       string            `json:"role"`
      Content    []apiContentBlock `json:"content"`
      Model      string            `json:"model"`
      StopReason string            `json:"stop_reason"`
      Usage      apiUsage          `json:"usage"`
  }
  ```
- [ ] The `apiContentBlock` type is defined:
  ```go
  type apiContentBlock struct {
      Type     string          `json:"type"`
      Text     string          `json:"text,omitempty"`
      Thinking string          `json:"thinking,omitempty"`
      ID       string          `json:"id,omitempty"`
      Name     string          `json:"name,omitempty"`
      Input    json.RawMessage `json:"input,omitempty"`
  }
  ```
- [ ] The `apiUsage` type is defined:
  ```go
  type apiUsage struct {
      InputTokens          int `json:"input_tokens"`
      OutputTokens         int `json:"output_tokens"`
      CacheReadInputTokens int `json:"cache_read_input_tokens"`
      CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
  }
  ```
- [ ] `Complete` parses each `apiContentBlock` in the response and converts it to a `provider.ContentBlock`:
  - When `apiContentBlock.Type == "text"`: produces `provider.ContentBlock{Type: "text", Text: block.Text}`
  - When `apiContentBlock.Type == "thinking"`: produces `provider.ContentBlock{Type: "thinking", Thinking: block.Thinking}`
  - When `apiContentBlock.Type == "tool_use"`: produces `provider.ContentBlock{Type: "tool_use", ID: block.ID, Name: block.Name, Input: block.Input}`
  - When `apiContentBlock.Type` is any other value: the block is skipped (not included in the response) and a warning is logged with the unknown type
- [ ] `Complete` maps the Anthropic `stop_reason` string to `provider.StopReason`:
  - `"end_turn"` maps to `provider.StopReasonEndTurn`
  - `"tool_use"` maps to `provider.StopReasonToolUse`
  - `"max_tokens"` maps to `provider.StopReasonMaxTokens`
  - Any other value maps to `provider.StopReasonEndTurn` (safe default) and a warning is logged
- [ ] `Complete` maps `apiUsage` to `provider.Usage`:
  - `apiUsage.InputTokens` maps to `Usage.InputTokens`
  - `apiUsage.OutputTokens` maps to `Usage.OutputTokens`
  - `apiUsage.CacheReadInputTokens` maps to `Usage.CacheReadTokens`
  - `apiUsage.CacheCreationInputTokens` maps to `Usage.CacheCreationTokens`
- [ ] `Complete` returns a fully populated `*provider.Response`:
  ```go
  &provider.Response{
      Content:    contentBlocks, // []provider.ContentBlock parsed from apiContentBlock slice
      Usage:      usage,         // provider.Usage mapped from apiUsage
      Model:      apiResp.Model,
      StopReason: stopReason,    // provider.StopReason mapped from apiResp.StopReason
      LatencyMs:  latencyMs,     // int64 milliseconds
  }
  ```
- [ ] If `json.Unmarshal` of the response body fails, `Complete` returns a `*provider.ProviderError` with `Provider: "anthropic"`, `StatusCode: 0`, `Message: "failed to parse response: <json error>"`, `Retriable: false`
- [ ] If the context is cancelled before or during the HTTP call, `Complete` returns the context error wrapped in a `*provider.ProviderError` with `Provider: "anthropic"`, `StatusCode: 0`, `Message: "request cancelled"`, `Retriable: false`
- [ ] The file compiles with `go build ./internal/provider/anthropic/...` with no errors
