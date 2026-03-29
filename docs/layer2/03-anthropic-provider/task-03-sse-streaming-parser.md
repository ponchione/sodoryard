# Task 03: SSE Streaming Parser

**Epic:** 03 — Anthropic Provider
**Status:** ⬚ Not started
**Dependencies:** Task 01 (provider struct, constructor, `buildHTTPRequest` with `stream=true`, API request types)

---

## Description

Implement the `Stream` method on `AnthropicProvider` and the stateful SSE streaming parser that processes Anthropic's Server-Sent Events format. The `Stream` method calls `buildHTTPRequest` with `stream=true`, executes the HTTP POST, and spawns a goroutine that reads the SSE event stream line by line, maintaining state to track which content block type (text, thinking, tool_use) each delta belongs to. For tool_use blocks, the parser accumulates partial JSON fragments and emits a `ToolCallEnd` with the full assembled JSON when the block stops. The parser emits unified `provider.StreamEvent` values on a channel that the caller consumes.

## Acceptance Criteria

- [ ] The `Stream` method is defined on `AnthropicProvider`:
  ```go
  func (p *AnthropicProvider) Stream(ctx context.Context, req *provider.Request) (<-chan provider.StreamEvent, error)
  ```
- [ ] `Stream` calls `p.buildHTTPRequest(ctx, req, true)` to construct the HTTP request with `stream` set to `true`
- [ ] `Stream` executes `p.httpClient.Do(httpReq)` and checks the HTTP status code; if the status is not 200, it reads the response body (up to 1024 bytes), closes it, and returns `nil, *provider.ProviderError` with `Provider: "anthropic"`, the HTTP status code, and the response body as the message
- [ ] `Stream` creates a `chan provider.StreamEvent` with buffer size 32 and returns it along with `nil` error
- [ ] `Stream` spawns a goroutine that reads the SSE stream, emits events on the channel, and closes the channel when done
- [ ] The goroutine defers `close(ch)` and `resp.Body.Close()`
- [ ] An unexported struct maintains parser state across SSE events:
  ```go
  type streamState struct {
      activeBlocks map[int]activeBlock // keyed by content block index
  }

  type activeBlock struct {
      blockType   string // "text", "thinking", or "tool_use"
      toolID      string // only set for tool_use blocks
      toolName    string // only set for tool_use blocks
      jsonAccum   strings.Builder // accumulates partial_json for tool_use blocks
  }
  ```
- [ ] The SSE parser reads lines from the response body using `bufio.Scanner` with `scanner.Split(bufio.ScanLines)`
- [ ] The SSE parser recognizes two line types:
  - Lines starting with `event: ` — the remainder is the event type string
  - Lines starting with `data: ` — the remainder is the JSON payload
  - Empty lines are ignored (they are SSE event separators)
  - Lines starting with `:` are SSE comments and are ignored
- [ ] When a `data:` line is encountered, the parser dispatches based on the most recently seen `event:` type. The parser handles exactly these six SSE event types:

  **1. `message_start`** — The `data` JSON has shape `{"type":"message_start","message":{"id":"msg_...","usage":{"input_tokens":N,"output_tokens":0,"cache_read_input_tokens":N,"cache_creation_input_tokens":N}}}`. The parser emits a `provider.StreamUsage` event with `Usage.InputTokens`, `Usage.CacheReadTokens`, and `Usage.CacheCreationTokens` extracted from `message.usage`.

  **2. `content_block_start`** — The `data` JSON has shape `{"type":"content_block_start","index":N,"content_block":{...}}`. The `content_block` object has a `type` field that is one of `"text"`, `"thinking"`, or `"tool_use"`. The parser records the block in `streamState.activeBlocks[index]` with the block type. For `"tool_use"` blocks, the `content_block` also contains `"id"` and `"name"` fields, which are stored in the `activeBlock`. For `"tool_use"` blocks, the parser emits a `provider.ToolCallStart{ID: content_block.id, Name: content_block.name}` event.

  **3. `content_block_delta`** — The `data` JSON has shape `{"type":"content_block_delta","index":N,"delta":{...}}`. The parser looks up `streamState.activeBlocks[index]` to determine the block type, then dispatches based on the `delta.type` field:
    - `"text_delta"`: delta has field `"text"`. Parser emits `provider.TokenDelta{Text: delta.text}`.
    - `"thinking_delta"`: delta has field `"thinking"`. Parser emits `provider.ThinkingDelta{Thinking: delta.thinking}`.
    - `"input_json_delta"`: delta has field `"partial_json"`. Parser appends `delta.partial_json` to `activeBlock.jsonAccum` (does NOT emit an event yet). Parser also emits `provider.ToolCallDelta{ID: activeBlock.toolID, Delta: delta.partial_json}`.
    - Unknown delta type: log at `slog.LevelWarn` with attributes `"delta_type"` (the unrecognized type string) and `"raw_json"` (the raw delta JSON bytes as a string). Skip the event and continue parsing.

  **4. `content_block_stop`** — The `data` JSON has shape `{"type":"content_block_stop","index":N}`. The parser looks up `streamState.activeBlocks[index]`. If the block type is `"tool_use"`, the parser emits `provider.ToolCallEnd{ID: activeBlock.toolID, Input: json.RawMessage(activeBlock.jsonAccum.String())}` with the fully accumulated JSON. The parser then removes the block from `streamState.activeBlocks`.

  **5. `message_delta`** — The `data` JSON has shape `{"type":"message_delta","delta":{"stop_reason":"..."},"usage":{"output_tokens":N}}`. The parser extracts `stop_reason` and `output_tokens`. It does NOT emit an event here; these values are used by the `message_stop` handler.

  **6. `message_stop`** — The `data` JSON has shape `{"type":"message_stop"}`. The parser emits `provider.StreamDone{StopReason: stopReason, Usage: finalUsage}` where `stopReason` is mapped from the `message_delta` stop_reason string (`"end_turn"` to `provider.StopReasonEndTurn`, `"tool_use"` to `provider.StopReasonToolUse`, `"max_tokens"` to `provider.StopReasonMaxTokens`, unknown to `provider.StopReasonEndTurn`) and `finalUsage` combines the input tokens from `message_start` with the output tokens from `message_delta`.

- [ ] Deserialization of the `data` JSON payloads uses these unexported types:
  ```go
  type sseMessageStart struct {
      Message struct {
          ID    string   `json:"id"`
          Usage apiUsage `json:"usage"`
      } `json:"message"`
  }

  type sseContentBlockStart struct {
      Index        int `json:"index"`
      ContentBlock struct {
          Type string `json:"type"`
          ID   string `json:"id,omitempty"`
          Name string `json:"name,omitempty"`
      } `json:"content_block"`
  }

  type sseContentBlockDelta struct {
      Index int `json:"index"`
      Delta struct {
          Type        string `json:"type"`
          Text        string `json:"text,omitempty"`
          Thinking    string `json:"thinking,omitempty"`
          PartialJSON string `json:"partial_json,omitempty"`
      } `json:"delta"`
  }

  type sseContentBlockStop struct {
      Index int `json:"index"`
  }

  type sseMessageDelta struct {
      Delta struct {
          StopReason string `json:"stop_reason"`
      } `json:"delta"`
      Usage struct {
          OutputTokens int `json:"output_tokens"`
      } `json:"usage"`
  }
  ```
- [ ] If `json.Unmarshal` fails for any SSE data payload, the parser emits `provider.StreamError{Err: err, Fatal: false, Message: "failed to parse SSE event: <event_type>: <json error>"}` and continues processing subsequent events
- [ ] If `bufio.Scanner` encounters an I/O error (`scanner.Err() != nil`), the parser emits `provider.StreamError{Err: scanner.Err(), Fatal: true, Message: "SSE stream read error: <error>"}` and exits the goroutine
- [ ] The goroutine monitors `ctx.Done()` in a select alongside channel sends; if the context is cancelled, it emits `provider.StreamError{Err: ctx.Err(), Fatal: true, Message: "stream cancelled"}` and exits
- [ ] Channel sends use a select with `ctx.Done()` to avoid goroutine leaks when the consumer abandons the channel:
  ```go
  select {
  case ch <- event:
  case <-ctx.Done():
      return
  }
  ```
- [ ] The file imports: `bufio`, `context`, `encoding/json`, `io`, `strings`, `net/http`, and `internal/provider` (the project's unified provider types package)
- [ ] The file compiles with `go build ./internal/provider/anthropic/...` with no errors
