# Task 05: Streaming Parser (Responses API Events)

**Epic:** 05 — Codex Provider
**Status:** ⬚ Not started
**Dependencies:** Task 03 (request translation: `buildResponsesRequest` and all `responses*` types)

---

## Description

Implement the `Stream` method on `CodexProvider` that sends a streaming request to the Responses API and parses Server-Sent Events into unified `StreamEvent` values emitted on a channel. The Responses API streaming format uses event types prefixed with `response.` and carries incremental deltas for text, reasoning, and function call arguments. The parser must maintain state across events to track which output items are in progress and assemble complete tool calls.

## Acceptance Criteria

- [ ] The `Stream` method is defined on `CodexProvider`:
  ```go
  func (p *CodexProvider) Stream(ctx context.Context, req *provider.Request) (<-chan provider.StreamEvent, error)
  ```

- [ ] `Stream` calls `p.getAccessToken(ctx)` to obtain a valid Bearer token. If `getAccessToken` returns an error, `Stream` returns `nil` and the error

- [ ] `Stream` calls `buildResponsesRequest(req.Model, req, true)` with `stream=true` to build the request body. If `req.Model` is empty, it defaults to `"o3"`

- [ ] `Stream` creates an HTTP request targeting `p.baseURL + "/v1/responses"` with method `POST`, sets headers `Authorization: Bearer <token>` and `Content-Type: application/json`

- [ ] `Stream` executes `p.httpClient.Do(httpReq)` and checks the HTTP status code. For non-200 status codes, `Stream` reads the body (up to 1024 bytes), closes it, and returns `nil, *provider.ProviderError` with `Provider: "codex"` and classification:
  - 401 or 403: message `"Codex authentication failed. Run 'codex auth' to re-authenticate."`, `Retriable: false`
  - 429: message `"rate limited"`, `Retriable: true`
  - 500, 502, or 503: message `"server error: <body truncated to 512 bytes>"`, `Retriable: true`
  - Any other status: message `"unexpected status <code>: <body truncated to 1024 bytes>"`, `Retriable: false`

- [ ] On a 200 response, `Stream` creates a buffered channel `ch := make(chan provider.StreamEvent, 64)` and launches a goroutine to parse SSE events and send them on the channel

- [ ] `Stream` returns `ch, nil` immediately; the goroutine closes the channel when parsing completes

- [ ] The goroutine reads the response body line-by-line (using `bufio.Scanner`). SSE format: lines starting with `event: ` carry the event type, lines starting with `data: ` carry the JSON payload. Empty lines separate events

- [ ] The goroutine defers `resp.Body.Close()` and `close(ch)`

- [ ] The goroutine maintains parser state to track in-progress output items:
  ```go
  type streamState struct {
      currentToolCallID   string
      currentToolCallName string
      toolCallArgs        strings.Builder
  }
  ```

- [ ] The goroutine handles the following SSE event types, each with the specified behavior:

  **`response.output_text.delta`** — JSON payload contains `{"item_id":"...","content_index":0,"delta":"<text>"}`. Emit `provider.TokenDelta{Text: delta}` on the channel

  **`response.reasoning.delta`** — JSON payload contains `{"item_id":"...","delta":"<encrypted-chunk>"}`. These are encrypted reasoning deltas. Emit `provider.ThinkingDelta{Thinking: delta}` on the channel (even though it is encrypted, the stream event type is ThinkingDelta for consistency with the unified interface)

  **`response.output_item.added`** — JSON payload contains `{"output_index":N,"item":{"type":"...","id":"...","call_id":"...","name":"...",...}}`. When `item.type == "function_call"`, emit `provider.ToolCallStart{ID: item.call_id, Name: item.name}` and store `call_id` and `name` in `streamState`. When `item.type` is `"message"` or `"reasoning"`, no event is emitted (deltas will follow)

  **`response.function_call_arguments.delta`** — JSON payload contains `{"item_id":"...","delta":"<json-chunk>"}`. Emit `provider.ToolCallDelta{ID: streamState.currentToolCallID, Delta: delta}` and append `delta` to `streamState.toolCallArgs`

  **`response.output_item.done`** — JSON payload contains `{"output_index":N,"item":{...complete item...}}`. When `item.type == "function_call"`, emit `provider.ToolCallEnd{ID: item.call_id, Input: json.RawMessage(streamState.toolCallArgs.String())}` and reset `streamState.toolCallArgs`. When `item.type` is `"message"` or `"reasoning"`, no additional event is emitted

  **`response.completed`** — JSON payload contains `{"response":{"id":"...","status":"completed","usage":{"input_tokens":N,"output_tokens":N,"input_tokens_details":{"cached_tokens":N},"output_tokens_details":{"reasoning_tokens":N}}}}`. Parse usage and emit `provider.StreamDone{StopReason: stopReason, Usage: usage}` where:
    - `Usage.InputTokens` = `usage.input_tokens`
    - `Usage.OutputTokens` = `usage.output_tokens`
    - `Usage.CacheReadTokens` = `usage.input_tokens_details.cached_tokens`
    - `Usage.CacheCreationTokens` = `0`
    - `StopReason`: iterate `sseCompletedResponse.Output` and check if any item has `Type == "function_call"`. If at least one `function_call` item exists, set `StopReason = provider.StopReasonToolUse`. Otherwise, set `StopReason = provider.StopReasonEndTurn`

  **`response.content_part.added`**, **`response.content_part.done`**, **`response.created`**, **`response.output_item.added` (for non-function-call types)** — These events are structural bookkeeping. No unified `StreamEvent` is emitted for these; they are silently consumed

- [ ] Unknown event types (not listed above) are silently ignored (no error, no event emitted)

- [ ] The goroutine checks for context cancellation on each iteration of the SSE parse loop. If `ctx.Err() != nil`, emit `provider.StreamError{Err: ctx.Err(), Fatal: true, Message: "stream cancelled"}` and return

- [ ] If the `bufio.Scanner` encounters a read error, the goroutine emits `provider.StreamError{Err: err, Fatal: true, Message: "stream read error: <err>"}` and returns

- [ ] If JSON unmarshaling of an event's data payload fails, the goroutine emits `provider.StreamError{Err: err, Fatal: false, Message: "failed to parse stream event: <err>"}` and continues processing subsequent events (non-fatal)

- [ ] The following unexported types are used for parsing SSE event data payloads:
  ```go
  type sseTextDelta struct {
      ItemID       string `json:"item_id"`
      ContentIndex int    `json:"content_index"`
      Delta        string `json:"delta"`
  }

  type sseReasoningDelta struct {
      ItemID string `json:"item_id"`
      Delta  string `json:"delta"`
  }

  type sseOutputItemAdded struct {
      OutputIndex int                 `json:"output_index"`
      Item        sseOutputItemData   `json:"item"`
  }

  type sseOutputItemDone struct {
      OutputIndex int                 `json:"output_index"`
      Item        sseOutputItemData   `json:"item"`
  }

  type sseOutputItemData struct {
      Type             string `json:"type"`
      ID               string `json:"id"`
      CallID           string `json:"call_id,omitempty"`
      Name             string `json:"name,omitempty"`
      Arguments        string `json:"arguments,omitempty"`
      EncryptedContent string `json:"encrypted_content,omitempty"`
  }

  type sseFuncArgDelta struct {
      ItemID string `json:"item_id"`
      Delta  string `json:"delta"`
  }

  type sseCompleted struct {
      Response sseCompletedResponse `json:"response"`
  }

  type sseCompletedResponse struct {
      ID     string         `json:"id"`
      Status string         `json:"status"`
      Usage  responsesUsage `json:"usage"` // reuses the type from Task 04
      Output []sseOutputItemData `json:"output,omitempty"`
  }
  ```

- [ ] The file imports: `bufio`, `bytes`, `context`, `encoding/json`, `strings`, and `internal/provider` (the project's unified provider types package)

- [ ] The file compiles with `go build ./internal/provider/codex/...` with no errors
