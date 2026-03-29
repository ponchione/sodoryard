# Task 05: Describer Implementation

**Epic:** 06 — Description Generator
**Status:** ⬚ Not started
**Dependencies:** Task 01 (config), Task 02 (prompt builder), Task 03 (LLM HTTP client), Task 04 (response parser), L1-E01 (Describer interface, Description type)

---

## Description

Wire everything together into the concrete `Describer` implementation that satisfies the `Describer` interface from L1-E01. This is the public entry point for the package. It orchestrates: build prompt messages from the already-truncated file content and pre-formatted relationship context, call the LLM, parse the response. The caller is responsible for truncating file content (via `TruncateFileContent`) and formatting relationship context (via `FormatRelationshipContext`) before calling `DescribeFile`. The critical behavior is graceful failure: any error from the LLM call or response parsing results in an empty slice and a log warning — never an error return that would block the indexing pipeline.

## Package

`internal/rag/describer/describer.go`

## Struct and Constructor

```go
// Describer generates semantic descriptions of code symbols by calling a local LLM.
type Describer struct {
    client          *llmClient
    logger          *slog.Logger
}

// New creates a Describer from the given config. The logger is used
// for warning-level messages when the LLM call fails or returns invalid output.
func New(cfg config.DescriberConfig, logger *slog.Logger) *Describer
```

- Creates `llmClient` via `newLLMClient(cfg)`.
- Stores the provided logger (from L0-E02).

## Interface Method

```go
// DescribeFile sends file content and relationship context to a local LLM and returns
// a []Description for each function/type in the file. On any failure (LLM unreachable,
// timeout, invalid response), returns nil and logs a warning.
//
// fileContent must already be truncated by the caller (via TruncateFileContent).
// relationshipContext must already be formatted by the caller (via FormatRelationshipContext).
func (d *Describer) DescribeFile(ctx context.Context, fileContent string, relationshipContext string) ([]rag.Description, error)
```

**Note on return type:** The `error` return is always `nil` due to the graceful failure policy. The error return is preserved in the signature to satisfy the `Describer` interface, but this implementation never returns a non-nil error. All failures are logged and result in an empty slice (`nil, nil`).

### Behavior

1. If `fileContent` is empty, return `nil, nil` immediately (nothing to describe).
2. Call `buildMessages(fileContent, relationshipContext)`.
3. Call `d.client.complete(ctx, messages)`.
   - If error: log at `Warn` level with `"msg", "description generation failed", "error", err.Error()`, return `nil, nil`.
4. Call `parseDescriptions(rawResponse)`.
   - If error: log at `Warn` level with `"msg", "failed to parse LLM descriptions", "error", err.Error(), "response_preview", first200Chars(rawResponse)`, return `nil, nil`.
5. Return the parsed `[]rag.Description, nil`.

### Helper

```go
func first200Chars(s string) string
```

Returns the first 200 characters of `s` (rune-safe), or `s` itself if shorter. Used for log previews of malformed responses without flooding logs.

## Graceful Failure Scenarios

| Failure | Logged message | Return value |
|---|---|---|
| LLM container not running (connection refused) | `"description generation failed"` + connection error | `nil, nil` |
| LLM request timeout | `"description generation failed"` + deadline exceeded | `nil, nil` |
| Context cancelled (indexing aborted) | `"description generation failed"` + context canceled | `nil, nil` |
| HTTP 500 from LLM | `"description generation failed"` + status error | `nil, nil` |
| LLM returns garbage (no JSON) | `"failed to parse LLM descriptions"` + parse error + response preview | `nil, nil` |
| LLM returns empty array `[]` | `"failed to parse LLM descriptions"` + "no valid descriptions" | `nil, nil` |
| Empty file content | (no log) | `nil, nil` |

## Acceptance Criteria

- [ ] `Describer` struct with `New(cfg, logger)` constructor
- [ ] `DescribeFile` signature matches E01-T07: `(ctx context.Context, fileContent string, relationshipContext string) ([]rag.Description, error)`
- [ ] `DescribeFile` implements the `Describer` interface from L1-E01
- [ ] Empty file content returns `nil, nil` immediately without calling LLM
- [ ] No internal file truncation — caller is responsible (via `TruncateFileContent`)
- [ ] No internal relationship context formatting — caller passes the pre-formatted string
- [ ] Prompt messages built via `buildMessages` and sent via `llmClient.complete`
- [ ] LLM client errors logged at Warn level and return `nil, nil` (not an error)
- [ ] Response parse errors logged at Warn level with response preview and return `nil, nil`
- [ ] Error return is always `nil` (graceful failure policy)
- [ ] `first200Chars` helper truncates at rune boundary
- [ ] The `*Describer` type satisfies the `rag.Describer` interface (compile-time check via `var _ rag.Describer = (*Describer)(nil)`)
