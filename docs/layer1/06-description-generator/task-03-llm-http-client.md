# Task 03: LLM HTTP Client

**Epic:** 06 — Description Generator
**Status:** ⬚ Not started
**Dependencies:** Task 01 (DescriberConfig)

---

## Description

Implement the HTTP client that sends chat completion requests to the local LLM container's OpenAI-compatible API. This is a focused HTTP client — it POSTs to `/v1/chat/completions`, marshals the request, reads the response, and returns the assistant's message content. Error handling translates HTTP failures into descriptive Go errors. The client owns the `http.Client` with a configurable timeout.

## Package

`internal/rag/describer/client.go`

## Types

### Request Types

```go
type chatCompletionRequest struct {
    Model       string    `json:"model"`
    Messages    []Message `json:"messages"`
    Temperature float64   `json:"temperature"`
    MaxTokens   int       `json:"max_tokens"`
}

// Message is shared with the prompt builder.
type Message struct {
    Role    string `json:"role"`
    Content string `json:"content"`
}
```

### Response Types

```go
type chatCompletionResponse struct {
    Choices []chatCompletionChoice `json:"choices"`
}

type chatCompletionChoice struct {
    Message Message `json:"message"`
}
```

### Client Struct

```go
type llmClient struct {
    httpClient *http.Client
    baseURL    string
    model      string
    temperature float64
    maxTokens  int
}
```

### Constructor

```go
func newLLMClient(cfg config.DescriberConfig) *llmClient
```

- Creates an `http.Client` with `Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second`.
- Stores `cfg.BaseURL`, `cfg.Model`, `cfg.Temperature`, `cfg.MaxTokens`.

### Core Method

```go
func (c *llmClient) complete(ctx context.Context, messages []Message) (string, error)
```

Behavior:
1. Marshal `chatCompletionRequest` with the client's model, temperature, max_tokens, and the provided messages.
2. Create `http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/chat/completions", body)`.
3. Set header `Content-Type: application/json`.
4. Execute the request via `c.httpClient.Do(req)`.
5. Read the response body (limit to 1MB via `io.LimitReader` to guard against runaway responses).
6. Check HTTP status code. If not 200, return error: `"LLM request failed with status %d: %s"` including up to the first 500 bytes of the response body.
7. Unmarshal into `chatCompletionResponse`.
8. If `len(response.Choices) == 0`, return error: `"LLM returned no choices"`.
9. Return `response.Choices[0].Message.Content, nil`.

## Error Cases

| Condition | Error message pattern |
|---|---|
| Connection refused (container down) | Wrapped `net.OpError` — let Go's default error propagate; caller (describer) handles gracefully |
| Context cancelled/deadline exceeded | `context.Canceled` or `context.DeadlineExceeded` propagated |
| HTTP status != 200 | `"LLM request failed with status %d: %s"` |
| Empty choices array | `"LLM returned no choices"` |
| Response body unmarshal failure | `"failed to parse LLM response: %w"` wrapping the JSON error |
| Request body marshal failure | `"failed to marshal LLM request: %w"` (should never happen in practice) |

## Acceptance Criteria

- [ ] `llmClient` struct with constructor `newLLMClient(cfg)` that configures timeout from `cfg.TimeoutSeconds`
- [ ] `complete(ctx, messages)` sends POST to `{baseURL}/v1/chat/completions` with correct JSON body
- [ ] Request body includes `model`, `messages`, `temperature`, and `max_tokens` fields
- [ ] `Content-Type: application/json` header set on request
- [ ] Context passed to `http.NewRequestWithContext` for cancellation support
- [ ] Response body read limited to 1MB
- [ ] Non-200 HTTP status returns error with status code and truncated body
- [ ] Empty choices array returns `"LLM returned no choices"` error
- [ ] Successful response returns `choices[0].message.content`
