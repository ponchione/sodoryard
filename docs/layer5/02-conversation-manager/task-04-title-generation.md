# Task 04: Title Generation

**Epic:** 02 — Conversation Manager
**Status:** ⬚ Not started
**Dependencies:** Task 01, Layer 2 Epic 01 (Provider interface), Layer 2 Epics 06-07 (sub-call tracking, router)

---

## Description

Implement asynchronous conversation title generation in `internal/conversation/title.go`. After the first assistant response in a new conversation, the agent loop fires off a lightweight LLM call to generate a short descriptive title from the first user message. The call is async (runs in a goroutine), non-fatal (failure is logged but doesn't affect the conversation), and recorded as a sub-call with `purpose='title_generation'`. The generated title is written back to the conversation's `title` field.

## Acceptance Criteria

- [ ] `GenerateTitle(ctx context.Context, provider Provider, conversationID string, firstMessage string) error` function defined
- [ ] Uses `Provider.Complete()` (non-streaming) with a short system prompt instructing the model to produce a 5-8 word title summarizing the user's request
- [ ] The prompt is minimal — no extended thinking, no tools, low max_tokens (e.g., 50)
- [ ] On success: calls `SetTitle(conversationID, generatedTitle)` to persist the title
- [ ] On failure: logs a warning with the error and returns without propagating the error. The conversation retains its null/empty title
- [ ] The sub-call is recorded via the Layer 2 sub-call tracking middleware with `purpose='title_generation'` — this happens automatically when using the wrapped provider, no explicit recording needed in this function
- [ ] Title text is trimmed of surrounding whitespace and quotes (models often wrap titles in quotes)
- [ ] If the first message is very short (e.g., "hi", "hello"), a sensible default title is generated rather than something like "Greeting"
- [ ] The function is designed to be called from a goroutine — it does not block the agent loop's main turn execution
- [ ] Package compiles with `go build ./internal/conversation/...`
