# Task 04: Extended Thinking and Prompt Caching

**Epic:** 03 — Anthropic Provider
**Status:** Not started
**Dependencies:** Task 02 (Complete method and response parsing — thinking blocks must be parsed), Task 03 (SSE streaming parser — thinking deltas must be emitted)

---

## Description

Add extended thinking support and prompt caching to the Anthropic provider. For extended thinking, this means populating the `thinking` field in the API request body when thinking is enabled via `ProviderOptions`, and ensuring the required `anthropic-beta: interleaved-thinking-2025-05-14` header is present (already set in Task 01). For prompt caching, this means setting `cache_control: {"type": "ephemeral"}` markers on system prompt content blocks in the request body. Both features modify the request building path; response-side parsing of thinking content blocks is already handled by Task 02 (content block parsing) and Task 03 (thinking_delta SSE events).

## Acceptance Criteria

- [ ] An exported options struct is defined for Anthropic-specific provider options:
  ```go
  type AnthropicOptions struct {
      ThinkingEnabled bool `json:"thinking_enabled"`
      ThinkingBudget  int  `json:"thinking_budget"` // budget_tokens value; 0 means use default
  }
  ```
- [ ] A package-level constant defines the default thinking budget:
  ```go
  const DefaultThinkingBudget = 10000
  ```
- [ ] `buildRequestBody` (from Task 01) is modified to parse `req.ProviderOptions` as `AnthropicOptions` using `json.Unmarshal(req.ProviderOptions, &opts)`:
  - If `req.ProviderOptions` is nil or empty, thinking is disabled (no `thinking` field in the request body)
  - If `json.Unmarshal` fails, return a `*provider.ProviderError` with `Provider: "anthropic"`, `StatusCode: 0`, `Message: "invalid provider options: <json error>"`, `Retriable: false`
- [ ] When `AnthropicOptions.ThinkingEnabled` is `true`, `buildRequestBody` sets `apiRequest.Thinking` to:
  ```go
  &apiThinking{
      Type:         "enabled",
      BudgetTokens: budget, // AnthropicOptions.ThinkingBudget if > 0, else DefaultThinkingBudget (10000)
  }
  ```
- [ ] When `AnthropicOptions.ThinkingEnabled` is `false` or `ProviderOptions` is nil, `apiRequest.Thinking` remains `nil` and is omitted from the serialized JSON (due to `omitempty` tag)
- [ ] The serialized request body when thinking is enabled includes:
  ```json
  {
      "thinking": {
          "type": "enabled",
          "budget_tokens": 10000
      }
  }
  ```
- [ ] When thinking is enabled and `Temperature` is set on the request, `buildRequestBody` forces `apiRequest.Temperature` to `nil` (Anthropic requires temperature to be unset or 1.0 when thinking is enabled) and logs a warning: `"temperature is ignored when thinking is enabled"`
- [ ] Prompt caching is applied automatically by `buildRequestBody` when converting `req.SystemBlocks` to `[]apiSystemBlock`: if a `provider.SystemBlock` has a non-nil `CacheControl` field, the corresponding `apiSystemBlock.CacheControl` is set to `&apiCacheControl{Type: "ephemeral"}`. This means the caller (the agent loop) controls cache placement by setting `CacheControl` on the `SystemBlock` values in the request.
- [ ] The serialized system field in the request body when cache control is present looks like:
  ```json
  [
      {"type": "text", "text": "Base system prompt...", "cache_control": {"type": "ephemeral"}},
      {"type": "text", "text": "Assembled context...", "cache_control": {"type": "ephemeral"}}
  ]
  ```
- [ ] When `CacheControl` is nil on a `SystemBlock`, the corresponding `apiSystemBlock.CacheControl` is nil and `cache_control` is omitted from the JSON (due to `omitempty` tag)
- [ ] The `anthropic-beta` header set in `buildHTTPRequest` (Task 01) already includes `interleaved-thinking-2025-05-14` which enables interleaved thinking content blocks in responses; no additional header changes are needed in this task
- [ ] Thinking content blocks in non-streaming responses are already parsed by Task 02's content block parser (type `"thinking"` maps to `provider.ContentBlock{Type: "thinking", Thinking: block.Thinking}`)
- [ ] Thinking deltas in streaming responses are already handled by Task 03's SSE parser (`thinking_delta` events emit `provider.ThinkingDelta{Thinking: delta.thinking}`)
- [ ] A helper function is provided for callers to construct the provider options JSON:
  ```go
  func NewAnthropicOptions(thinkingEnabled bool, thinkingBudget int) json.RawMessage
  ```
  This marshals an `AnthropicOptions` struct and returns the result as `json.RawMessage`. If `thinkingBudget` is 0, it is set to `DefaultThinkingBudget` before marshaling. This function is a convenience; callers can also construct the JSON manually.
- [ ] The file compiles with `go build ./internal/provider/anthropic/...` with no errors
