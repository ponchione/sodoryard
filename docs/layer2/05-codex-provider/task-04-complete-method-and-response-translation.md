# Task 04: Complete Method and Response Translation

**Epic:** 05 — Codex Provider
**Status:** ⬚ Not started
**Dependencies:** Task 03 (request translation: `buildResponsesRequest` and all `responses*` types)

---

## Description

Implement the non-streaming `Complete` method on `CodexProvider`. This method obtains a valid access token via the credential delegation flow, calls `buildResponsesRequest` with `stream=false`, sends an HTTP POST to `POST <baseURL>/v1/responses`, reads the full JSON response, and parses the Responses API output items into the unified `provider.Response`. The response parser handles three output item types: `message` (containing `output_text` content), `function_call` (tool invocations), and `reasoning` (encrypted chain-of-thought). Error responses are classified into actionable errors with appropriate retry behavior.

## Acceptance Criteria

- [ ] The `Complete` method is defined on `CodexProvider`:
  ```go
  func (p *CodexProvider) Complete(ctx context.Context, req *provider.Request) (*provider.Response, error)
  ```

- [ ] `Complete` calls `p.getAccessToken(ctx)` to obtain a valid Bearer token. If `getAccessToken` returns an error, `Complete` returns that error immediately (it is already a `*provider.ProviderError`)

- [ ] `Complete` calls `buildResponsesRequest(req.Model, req, false)` to build the request body. If `req.Model` is empty, it defaults to `"o3"` before calling the builder

- [ ] `Complete` marshals the request body with `json.Marshal` and creates an HTTP request via `http.NewRequestWithContext(ctx, "POST", p.baseURL+"/v1/responses", bytes.NewReader(body))`

- [ ] `Complete` sets exactly these HTTP headers on the request:
  - `Authorization: Bearer <access_token>`
  - `Content-Type: application/json`

- [ ] `Complete` records the start time via `time.Now()` before calling `p.httpClient.Do(httpReq)` and computes `LatencyMs` as `time.Since(start).Milliseconds()` after the response body is read

- [ ] `Complete` reads the entire response body using `io.ReadAll(resp.Body)` and defers `resp.Body.Close()`

- [ ] If the HTTP status code is 401 or 403, `Complete` returns a `*provider.ProviderError` with:
  - `Provider: "codex"`
  - `StatusCode: <actual status code>`
  - `Message: "Codex authentication failed. Run ` + "`codex auth`" + ` to re-authenticate."`
  - `Retriable: false`

- [ ] If the HTTP status code is 429, `Complete` retries with exponential backoff: base delay 1 second, multiplied by 2 on each retry, maximum 3 attempts total (1s, 2s, 4s). If all attempts fail, returns a `*provider.ProviderError` with:
  - `Provider: "codex"`
  - `StatusCode: 429`
  - `Message: "rate limited after 3 attempts: <response body truncated to 512 bytes>"`
  - `Retriable: false` (all retries exhausted)

- [ ] If the HTTP status code is 500, 502, or 503, `Complete` retries with exponential backoff: base delay 1 second, multiplied by 2 on each retry, maximum 3 attempts total. If all attempts fail, returns a `*provider.ProviderError` with:
  - `Provider: "codex"`
  - `StatusCode: <actual status code>`
  - `Message: "server error after 3 attempts: <response body truncated to 512 bytes>"`
  - `Retriable: false` (all retries exhausted)

- [ ] For any other non-200 status code not covered above, `Complete` returns a `*provider.ProviderError` with:
  - `Provider: "codex"`
  - `StatusCode: <actual status code>`
  - `Message: <response body truncated to 1024 bytes>`
  - `Retriable: false`

- [ ] The retry loop respects context cancellation: before each retry attempt, check `ctx.Err()` and return immediately if the context is done

- [ ] The Responses API JSON response is deserialized into these unexported types:
  ```go
  type responsesResponse struct {
      ID     string               `json:"id"`
      Object string               `json:"object"`
      Model  string               `json:"model"`
      Output []responsesOutputItem `json:"output"`
      Usage  responsesUsage       `json:"usage"`
  }

  type responsesOutputItem struct {
      Type             string                    `json:"type"`              // "message", "function_call", "reasoning"
      ID               string                    `json:"id"`
      Role             string                    `json:"role,omitempty"`    // "assistant" for type="message"
      Content          []responsesOutputContent  `json:"content,omitempty"` // for type="message"
      CallID           string                    `json:"call_id,omitempty"` // for type="function_call"
      Name             string                    `json:"name,omitempty"`    // for type="function_call"
      Arguments        string                    `json:"arguments,omitempty"` // for type="function_call" (JSON string)
      EncryptedContent string                    `json:"encrypted_content,omitempty"` // for type="reasoning"
  }

  type responsesOutputContent struct {
      Type string `json:"type"` // "output_text"
      Text string `json:"text"`
  }

  type responsesUsage struct {
      InputTokens        int                    `json:"input_tokens"`
      OutputTokens       int                    `json:"output_tokens"`
      InputTokensDetails responsesInputDetails  `json:"input_tokens_details"`
      OutputTokensDetails responsesOutputDetails `json:"output_tokens_details"`
  }

  type responsesInputDetails struct {
      CachedTokens int `json:"cached_tokens"`
  }

  type responsesOutputDetails struct {
      ReasoningTokens int `json:"reasoning_tokens"`
  }
  ```

- [ ] `Complete` parses each `responsesOutputItem` in the response `Output` array and converts it to `provider.ContentBlock` values:
  - When `item.Type == "message"`: iterate over `item.Content`; each entry with `Type == "output_text"` produces `provider.ContentBlock{Type: "text", Text: content.Text}`
  - When `item.Type == "function_call"`: produces `provider.ContentBlock{Type: "tool_use", ID: item.CallID, Name: item.Name, Input: json.RawMessage(item.Arguments)}`; note that `CallID` (not `ID`) maps to the unified `ContentBlock.ID` because `CallID` is what tool result messages reference
  - When `item.Type == "reasoning"`: produces `provider.ContentBlock{Type: "reasoning", Text: item.EncryptedContent}`; this is opaque encrypted content that is stored in conversation history but not displayed to the user
  - When `item.Type` is any other value: the item is skipped and a warning is logged with the unknown type

- [ ] `Complete` determines the `StopReason` from the output items:
  - If any output item has `Type == "function_call"`: stop reason is `provider.StopReasonToolUse`
  - Otherwise: stop reason is `provider.StopReasonEndTurn`

- [ ] `Complete` maps `responsesUsage` to `provider.Usage`:
  - `responsesUsage.InputTokens` maps to `Usage.InputTokens`
  - `responsesUsage.OutputTokens` maps to `Usage.OutputTokens`
  - `responsesUsage.InputTokensDetails.CachedTokens` maps to `Usage.CacheReadTokens`
  - `Usage.CacheCreationTokens` is always `0` (the Responses API does not report cache creation tokens)

- [ ] `Complete` returns a fully populated `*provider.Response`:
  ```go
  &provider.Response{
      Content:    contentBlocks, // []provider.ContentBlock from all output items
      Usage:      usage,         // provider.Usage mapped from responsesUsage
      Model:      apiResp.Model,
      StopReason: stopReason,    // provider.StopReason
      LatencyMs:  latencyMs,     // int64 milliseconds
  }
  ```

- [ ] If `json.Unmarshal` of the response body fails, `Complete` returns a `*provider.ProviderError` with `Provider: "codex"`, `StatusCode: 0`, `Message: "failed to parse response: <json error>"`, `Retriable: false`

- [ ] If the context is cancelled before or during the HTTP call, `Complete` returns the context error wrapped in a `*provider.ProviderError` with `Provider: "codex"`, `StatusCode: 0`, `Message: "request cancelled"`, `Retriable: false`

- [ ] The file compiles with `go build ./internal/provider/codex/...` with no errors
