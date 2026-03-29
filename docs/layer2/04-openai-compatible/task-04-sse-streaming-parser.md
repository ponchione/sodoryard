# Task 04: SSE Streaming Parser

**Epic:** 04 — OpenAI-Compatible Provider
**Status:** ⬚ Not started
**Dependencies:** Task 02 (request translation)

---

## Description

Implement the `Stream` method on `OpenAIProvider` that sends a streaming chat completion request (`"stream": true`) and parses the Server-Sent Events (SSE) response. The parser processes `data: {...}` lines, accumulates incremental tool call arguments across multiple chunks, and emits unified `StreamEvent` values on a channel. This task handles the full SSE lifecycle including the `data: [DONE]` sentinel and partial JSON assembly for tool calls.

## Acceptance Criteria

- [ ] The `Stream` method is defined with the following signature:
  ```go
  // Stream sends a streaming chat completion request and returns a channel
  // of unified stream events. The channel is closed when the stream ends
  // or an error occurs. The caller must drain the channel.
  func (p *OpenAIProvider) Stream(ctx context.Context, req *provider.Request) (<-chan provider.StreamEvent, error)
  ```

- [ ] The HTTP request is built identically to `Complete` (same URL, headers, auth logic) except:
  - The body uses `buildChatRequest(p.model, req, true)` (stream=true).
  - The response is NOT fully read into memory; the body is read line-by-line.

- [ ] Error handling for the initial HTTP response (before streaming begins) follows the same rules as `Complete`:
  - 401/403: return `nil, error` with message `"OpenAI-compatible provider '<name>' authentication failed. Check API key configuration."` — no retry.
  - 429: return `nil, error` with message `"OpenAI-compatible provider '<name>': rate limited"` — no retry on streaming requests (retry only applies to non-streaming `Complete`).
  - 500/502/503: return `nil, error` with message `"OpenAI-compatible provider '<name>': server error (HTTP <status>)"` — no retry on streaming.
  - Connection refused: return `nil, error` with message `"OpenAI-compatible provider '<name>' at <base_url> is not reachable. Is the model server running?"`.
  - Other non-200: return `nil, error` with message `"OpenAI-compatible provider '<name>': unexpected HTTP status <status>"`.

- [ ] On a 200 response, `Stream` returns a channel and launches a goroutine to parse SSE lines. The goroutine closes the channel when done and closes the response body.

- [ ] The following internal type represents one SSE chunk:
  ```go
  // streamChunk is one parsed SSE data payload from the streaming response.
  type streamChunk struct {
      ID      string               `json:"id"`
      Object  string               `json:"object"` // "chat.completion.chunk"
      Choices []streamChunkChoice  `json:"choices"`
      Usage   *chatUsage           `json:"usage,omitempty"`
  }

  // streamChunkChoice is one entry in a streaming chunk's choices array.
  type streamChunkChoice struct {
      Index        int              `json:"index"`
      Delta        streamDelta      `json:"delta"`
      FinishReason *string          `json:"finish_reason"` // null until final chunk
  }

  // streamDelta holds incremental content or tool call fragments.
  type streamDelta struct {
      Role      string              `json:"role,omitempty"`
      Content   string              `json:"content,omitempty"`
      ToolCalls []streamToolCall    `json:"tool_calls,omitempty"`
  }

  // streamToolCall is an incremental tool call fragment in a streaming delta.
  type streamToolCall struct {
      Index    int                  `json:"index"`
      ID       string              `json:"id,omitempty"`        // present only in the first chunk for this call
      Type     string              `json:"type,omitempty"`      // "function", present only in first chunk
      Function streamFunctionCall  `json:"function,omitempty"`
  }

  // streamFunctionCall holds incremental function name/arguments fragments.
  type streamFunctionCall struct {
      Name      string `json:"name,omitempty"`      // present only in first chunk
      Arguments string `json:"arguments,omitempty"` // appended across chunks
  }
  ```

- [ ] SSE line parsing rules:
  - Read the response body line-by-line using `bufio.Scanner`.
  - Lines starting with `data: ` (note the space after the colon) contain a payload.
  - The payload `[DONE]` (i.e., the line is exactly `data: [DONE]`) signals end-of-stream. Close the channel and return from the goroutine.
  - Empty lines (blank or whitespace-only) are skipped.
  - Lines starting with `:` (SSE comments) are skipped.
  - Any other line format is skipped (defensive parsing).

- [ ] For each `data: {...}` line, strip the `data: ` prefix and JSON-unmarshal into `streamChunk`. If unmarshalling fails, emit a `provider.StreamEventError` with message `"OpenAI-compatible provider '<name>': failed to parse stream chunk: <error>"` and continue processing subsequent lines (do not abort the stream).

- [ ] Content text streaming: when `delta.Content` is non-empty, emit a `provider.StreamEventText` with the content fragment.

- [ ] Tool call streaming requires accumulation across multiple chunks:
  - Maintain a `map[int]*accumulatedToolCall` keyed by `streamToolCall.Index`.
  - When a chunk contains `delta.ToolCalls`, for each entry:
    - If this index is not yet in the map, create a new `accumulatedToolCall` and store the `ID`, `Name` from the first fragment.
    - Append `Function.Arguments` to the accumulator's arguments string buffer.
  - When `finish_reason` is `"tool_calls"`, iterate the accumulated map in index order and emit one `provider.StreamEventToolUse` per tool call with:
    - `ID` = accumulated ID
    - `Name` = accumulated Name
    - `Input` = `json.RawMessage(accumulatedArguments)` (the fully concatenated arguments string)

- [ ] The accumulated tool call helper struct:
  ```go
  // accumulatedToolCall collects incremental tool call fragments.
  type accumulatedToolCall struct {
      ID        string
      Name      string
      Arguments strings.Builder
  }
  ```

- [ ] Finish reason handling: when a chunk has a non-nil `finish_reason`:
  - `"stop"` -> emit `provider.StreamEventStop` with `StopReason: provider.StopReasonEndTurn`
  - `"tool_calls"` -> first emit all accumulated tool calls (as above), then emit `provider.StreamEventStop` with `StopReason: provider.StopReasonToolUse`
  - `"length"` -> emit `provider.StreamEventStop` with `StopReason: provider.StopReasonMaxTokens`

- [ ] Usage in streaming: if the final chunk includes a `usage` object (some providers send it), emit a `provider.StreamEventUsage` with:
  - `InputTokens` = `usage.PromptTokens`
  - `OutputTokens` = `usage.CompletionTokens`
  - `CacheReadTokens` = 0
  - `CacheCreationTokens` = 0

- [ ] Context cancellation: the goroutine checks `ctx.Done()` between processing lines. If the context is cancelled, emit `provider.StreamEventError` with the context error message and close the channel.

- [ ] The goroutine uses `defer` to close the response body and the channel, ensuring no resource leaks regardless of how the stream ends.

- [ ] If the scanner encounters an I/O error (e.g., connection drop mid-stream), emit `provider.StreamEventError` with message `"OpenAI-compatible provider '<name>': stream read error: <error>"` and close the channel.
