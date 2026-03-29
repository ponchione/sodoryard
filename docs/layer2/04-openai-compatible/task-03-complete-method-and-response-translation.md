# Task 03: Complete Method and Response Translation

**Epic:** 04 — OpenAI-Compatible Provider
**Status:** ⬚ Not started
**Dependencies:** Task 02 (request translation)

---

## Description

Implement the `Complete` method on `OpenAIProvider` that performs a non-streaming HTTP POST to `{base_url}/chat/completions`, parses the JSON response, and translates the OpenAI response format back into sirtopham's unified `Response` type. This includes content extraction, tool call parsing, finish reason mapping, usage statistics, and comprehensive error handling with retry logic for transient failures.

## Acceptance Criteria

- [ ] The `Complete` method is defined with the following signature:
  ```go
  // Complete sends a non-streaming chat completion request and returns
  // the unified response. It retries on transient server errors and rate limits.
  func (p *OpenAIProvider) Complete(ctx context.Context, req *provider.Request) (*provider.Response, error)
  ```

- [ ] The following internal types are defined to represent the OpenAI response JSON shape:
  ```go
  // chatResponse is the top-level JSON response from POST /chat/completions.
  type chatResponse struct {
      ID      string         `json:"id"`
      Object  string         `json:"object"`
      Model   string         `json:"model"`
      Choices []chatChoice   `json:"choices"`
      Usage   chatUsage      `json:"usage"`
  }

  // chatChoice is one entry in the choices array.
  type chatChoice struct {
      Index        int         `json:"index"`
      Message      chatMessage `json:"message"`
      FinishReason string      `json:"finish_reason"`
  }

  // chatUsage holds token counts from the response.
  type chatUsage struct {
      PromptTokens     int `json:"prompt_tokens"`
      CompletionTokens int `json:"completion_tokens"`
      TotalTokens      int `json:"total_tokens"`
  }
  ```

- [ ] The HTTP request is built as follows:
  - Method: `POST`
  - URL: `p.baseURL + "/chat/completions"`
  - Header `Content-Type`: `application/json`
  - Header `Authorization`: `Bearer <p.apiKey>` — only set if `p.apiKey` is non-empty. If `p.apiKey` is empty, the `Authorization` header is omitted entirely.
  - Body: JSON-encoded `chatRequest` from `buildChatRequest(p.model, req, false)` (stream=false).

- [ ] The request respects the `ctx` context; if the context is cancelled, the HTTP call returns immediately with the context error.

- [ ] Response status code handling:
  - **200**: parse the JSON body as `chatResponse` and proceed to translation.
  - **401 or 403**: return error `"OpenAI-compatible provider '<name>' authentication failed. Check API key configuration."` — do NOT retry.
  - **429 (rate limit)**: retry with exponential backoff. Base delay 1 second, multiplied by 2 each attempt, plus random jitter of 0 to 500ms. Maximum 3 total attempts. If all attempts fail, return error `"OpenAI-compatible provider '<name>': rate limited after 3 attempts"`.
  - **500, 502, or 503 (server error)**: retry with the same exponential backoff strategy as 429. Maximum 3 total attempts. If all attempts fail, return error `"OpenAI-compatible provider '<name>': server error (HTTP <status>) after 3 attempts"`.
  - **Any other non-200 status**: return error `"OpenAI-compatible provider '<name>': unexpected HTTP status <status>"` — do NOT retry.

- [ ] Connection errors (e.g., `net.OpError`, connection refused) are detected and return: `"OpenAI-compatible provider '<name>' at <base_url> is not reachable. Is the model server running?"` — do NOT retry connection refused errors.

- [ ] Response JSON parsing: if `json.Unmarshal` fails on a 200 response, return error `"OpenAI-compatible provider '<name>': failed to parse response JSON: <underlying error>"`.

- [ ] If `choices` is empty in a 200 response, return error `"OpenAI-compatible provider '<name>': response contained no choices"`.

- [ ] Translation from `chatResponse` to unified `provider.Response`:
  - Use `choices[0]` only (ignore additional choices).
  - If `choices[0].message.content` is non-empty, create a `TextBlock` with that content and add it to `Response.Content`.
  - For each entry in `choices[0].message.tool_calls`, create a `ToolUseBlock` with:
    - `ID` = `tool_call.ID`
    - `Name` = `tool_call.Function.Name`
    - `Input` = `json.RawMessage(tool_call.Function.Arguments)` (the arguments string converted to raw JSON bytes)
  - Add each `ToolUseBlock` to `Response.Content` after any `TextBlock`.

- [ ] Finish reason mapping from `choices[0].finish_reason`:
  - `"stop"` maps to `provider.StopReasonEndTurn`
  - `"tool_calls"` maps to `provider.StopReasonToolUse`
  - `"length"` maps to `provider.StopReasonMaxTokens`
  - Any other value (including empty string) maps to `provider.StopReasonEndTurn` as a fallback.

- [ ] Usage mapping:
  - `Response.Usage.InputTokens` = `chatResponse.Usage.PromptTokens`
  - `Response.Usage.OutputTokens` = `chatResponse.Usage.CompletionTokens`
  - `Response.Usage.CacheReadTokens` = `0` (OpenAI handles caching internally, no explicit metrics)
  - `Response.Usage.CacheCreationTokens` = `0`

- [ ] If the response contains `tool_calls` that are empty arrays or the `arguments` field is empty, those entries are skipped without error.

- [ ] The response translation function is extracted as a separate internal function for testability:
  ```go
  // translateResponse converts an OpenAI chatResponse to a unified provider.Response.
  func translateResponse(resp *chatResponse) (*provider.Response, error)
  ```
