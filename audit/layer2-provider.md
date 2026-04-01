# Layer 2 Audit: Provider Architecture

## Scope

Layer 2 is the LLM provider abstraction: a unified interface for Anthropic (OAuth),
OpenAI-compatible (API key), and Codex (CLI delegation) backends. It includes
credential lifecycle management, streaming support, sub-call tracking for cost
attribution, and a routing layer that selects providers per request.

## Spec References

- `docs/specs/03-provider-architecture.md` — Full architecture
- `docs/layer2/layer-2-overview.md` — Epic index (7 epics)
- `docs/layer2/01-provider-types/` through `07-provider-router/` — Task-level specs

## Packages to Audit

| Package | Src | Test | Purpose |
|---------|-----|------|---------|
| `internal/provider` | 5 | 1 | Interface, types, message, response, stream |
| `internal/provider/anthropic` | 8 | 4 | OAuth credentials, API client, streaming |
| `internal/provider/openai` | 5 | 4 | API key auth, request translation, streaming |
| `internal/provider/codex` | 6 | 5 | CLI delegation, response parsing, streaming |
| `internal/provider/tracking` | 2 | 1 | Sub-call tracking wrapper |
| `internal/provider/router` | 3 | 2 | Provider selection, fallback, health |

## Test Commands

```bash
go test ./internal/provider/...
```

## Audit Checklist

### Epic 01: Provider Types & Interface
- [ ] `internal/provider/` defines the `Provider` interface:
  - `Complete(ctx, *Request) (*Response, error)`
  - `Stream(ctx, *Request) (<-chan StreamEvent, error)`
  - `Models(ctx) ([]Model, error)`
  - `Name() string`
- [ ] `Request` struct has: Messages, Model, MaxTokens, Tools, SystemPrompt, Purpose, ConversationID, TurnNumber
- [ ] `Response` struct has: Content ([]ContentBlock), StopReason, Usage (InputTokens, OutputTokens), Model
- [ ] `ContentBlock` supports: text, tool_use, tool_result types
- [ ] `StreamEvent` types: content_delta, tool_use_start, tool_use_delta, message_complete, error
- [ ] `ToolCall` and `ToolResult` message types for function calling
- [ ] `ContentBlocksFromRaw(json.RawMessage)` parses assistant content correctly
- [ ] `NewUserMessage`, `NewAssistantMessage`, `NewToolResultMessage` constructors

### Epic 02: Anthropic Credentials
- [ ] `internal/provider/anthropic/` handles OAuth token lifecycle
- [ ] Reads credentials from `~/.claude/.credentials.json` or equivalent
- [ ] Token refresh when expired
- [ ] File locking for concurrent access safety
- [ ] Test covers: valid creds, expired token refresh, missing file, corrupt file

### Epic 03: Anthropic Provider
- [ ] Implements `Provider` interface
- [ ] Request translation: sirtopham types → Anthropic API format
- [ ] Response translation: Anthropic API → sirtopham types
- [ ] Streaming: SSE parsing, content_block_start/delta/stop events
- [ ] Tool use blocks in streaming responses assembled correctly
- [ ] Error handling: rate limits (429), auth failures (401), server errors (5xx)
- [ ] Retry logic with backoff for transient errors
- [ ] Test covers: complete request, streaming, tool use, error scenarios

### Epic 04: OpenAI-Compatible Provider
- [ ] Implements `Provider` interface
- [ ] API key auth via `Authorization: Bearer` header
- [ ] Request translation: sirtopham types → OpenAI chat completions format
- [ ] Tool calls translated between OpenAI function_call format and sirtopham ToolCall
- [ ] Streaming via SSE `data: [DONE]` protocol
- [ ] Works with any OpenAI-compatible endpoint (local models, together.ai, etc.)
- [ ] Test covers: complete, stream, tool calls, error handling

### Epic 05: Codex Provider
- [ ] Implements `Provider` interface
- [ ] Delegates to Codex CLI process
- [ ] Request/response translation between sirtopham and Codex formats
- [ ] Streaming support
- [ ] Credential management for Codex CLI
- [ ] Test covers: delegation, response parsing, error handling

### Epic 06: Sub-Call Tracking
- [ ] `internal/provider/tracking/` wraps any Provider with tracking
- [ ] Records: model, tokens_in, tokens_out, duration_ms, conversation_id, turn_number
- [ ] Persists to `sub_calls` SQLite table
- [ ] Does not interfere with streaming
- [ ] Test covers: tracked complete, tracked stream, persistence verification

### Epic 07: Provider Router
- [ ] `internal/provider/router/` selects provider per request
- [ ] Model-to-provider mapping from config
- [ ] Fallback when primary provider fails
- [ ] Health checking (mark provider unhealthy after repeated failures)
- [ ] `RegisterProvider` and `RouteRequest` methods
- [ ] Test covers: routing by model, fallback, health state transitions

### Cross-cutting
- [ ] No goroutine leaks in streaming paths (channels properly closed)
- [ ] Context cancellation propagated through all providers
- [ ] Provider errors are descriptive (include status code, model, provider name)
- [ ] `go test -race ./internal/provider/...` — no data races
