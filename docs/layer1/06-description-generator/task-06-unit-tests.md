# Task 06: Unit Tests

**Epic:** 06 — Description Generator
**Status:** ⬚ Not started
**Dependencies:** Task 02, Task 03, Task 04, Task 05

---

## Description

Comprehensive unit tests for the description generator package. Tests cover all four internal components (prompt builder, LLM client, response parser, describer orchestration) using mock HTTP servers for LLM interaction. These tests validate the full range of success paths, error handling, and the critical graceful failure behavior. This task is split into two independently-workable parts.

## Package

`internal/rag/describer/describer_test.go` (same package — tests unexported functions)

Additional files if needed for organization:
- `internal/rag/describer/prompt_test.go`
- `internal/rag/describer/parse_test.go`
- `internal/rag/describer/client_test.go`

---

## Part A: Pure Function Tests (~2h)

### Prompt Builder Tests (`prompt_test.go`)

**`TestTruncateFileContent`:**
- Content at exactly `maxLen` — returned unchanged
- Content below `maxLen` — returned unchanged
- Content above `maxLen` — truncated with `"\n... (truncated)"` suffix, total length <= `maxLen`
- Multi-byte characters (e.g., UTF-8 emoji) — truncation does not split a rune
- Empty string — returned as empty string

**`TestFormatRelationshipContext`:**
- Single chunk with all four relationship fields populated — output matches specified format
- Multiple chunks — each gets its own block
- Chunk with no relationship data (all fields empty) — skipped entirely
- All chunks have no relationship data — returns empty string
- Chunk with partial data (e.g., only `Calls` populated, others empty) — empty fields show `(none)`

**`TestBuildMessages`:**
- Returns exactly 2 messages (system + user)
- System message role is `"system"`, content matches the exact prompt text from Task 02
- User message role is `"user"`
- With non-empty relationship context — user message contains both file content and context separated by two newlines
- With empty relationship context — user message contains only file content, no trailing newlines

### Response Parser Tests (`parse_test.go`)

**`TestExtractJSON`:**

| Test name | Input | Expected output |
|---|---|---|
| `CleanArray` | `[{"name":"Foo","description":"bar"}]` | Same string, no error |
| `WithWhitespace` | `  \n[{"name":"Foo","description":"bar"}]\n  ` | Trimmed array, no error |
| `MarkdownFencesJSON` | `` ```json\n[{"name":"Foo","description":"bar"}]\n``` `` | Array without fences, no error |
| `MarkdownFencesBare` | `` ```\n[{"name":"Foo","description":"bar"}]\n``` `` | Array without fences, no error |
| `PreambleAndPostscript` | `Here you go:\n[{"name":"Foo","description":"bar"}]\nDone!` | Just the array, no error |
| `NoArray` | `This is just text with no JSON` | Error: `"no JSON array found"` |
| `EmptyString` | `""` | Error: `"no JSON array found"` |
| `ObjectNotArray` | `{"name":"Foo"}` | Error: `"no JSON array found"` |

**`TestParseDescriptions`:**

| Test name | Input | Expected output |
|---|---|---|
| `ValidSingle` | `[{"name":"Foo","description":"Does foo"}]` | `[]Description{{Name: "Foo", Description: "Does foo"}}` |
| `ValidMultiple` | `[{"name":"Foo","description":"Does foo"},{"name":"Bar","description":"Does bar"}]` | `[]Description{{Name: "Foo", Description: "Does foo"}, {Name: "Bar", Description: "Does bar"}}` |
| `SkipsEmptyName` | `[{"name":"","description":"orphan"},{"name":"Foo","description":"Does foo"}]` | `[]Description{{Name: "Foo", Description: "Does foo"}}` |
| `AllEmptyNames` | `[{"name":"","description":"a"},{"name":"","description":"b"}]` | Error: `"no valid descriptions"` |
| `MalformedJSON` | `[{"name": broken}]` | Error wrapping JSON parse failure |
| `WithCodeFences` | `` ```json\n[{"name":"Foo","description":"Does foo"}]\n``` `` | `[]Description{{Name: "Foo", Description: "Does foo"}}` |

**`TestFirst200Chars`:**
- String shorter than 200 characters — returned unchanged
- String at exactly 200 characters — returned unchanged
- String over 200 characters with multi-byte runes at the boundary — truncated at rune boundary, length <= 200 runes

---

## Part B: HTTP and Integration Tests (~2h)

### LLM Client Tests (`client_test.go`)

All tests use `httptest.NewServer` to mock the LLM container.

**`TestComplete_Success`:**
- Mock returns valid `chatCompletionResponse` with one choice
- Verify request method is POST, path is `/v1/chat/completions`, Content-Type is `application/json`
- Verify request body contains correct `model`, `messages`, `temperature`, `max_tokens`
- Verify returned string matches `choices[0].message.content`

**`TestComplete_HTTPError`:**
- Mock returns HTTP 500 with body `"internal server error"`
- Verify error contains status code `500` and body text

**`TestComplete_EmptyChoices`:**
- Mock returns `{"choices":[]}`
- Verify error message is `"LLM returned no choices"`

**`TestComplete_InvalidJSON`:**
- Mock returns HTTP 200 with body `"not json at all"`
- Verify error wraps a JSON unmarshal error

**`TestComplete_ContextCancellation`:**
- Create a context, cancel it before sending request
- Verify error is `context.Canceled`

**`TestComplete_Timeout`:**
- Mock handler sleeps longer than client timeout (set timeout to 1 second, handler sleeps 2 seconds)
- Verify error indicates timeout

### Describer Integration Tests (`describer_test.go`)

These use a mock HTTP server to simulate the full end-to-end flow.

**`TestDescribeFile_Success`:**
- Mock LLM returns valid JSON array of descriptions
- Provide file content and pre-formatted relationship context string
- Verify returned `[]Description` contains expected entries
- Verify the request body sent to the mock contains the system prompt, file content, and relationship context

**`TestDescribeFile_EmptyFile`:**
- Call with empty `fileContent`
- Verify returns `nil, nil` immediately
- Verify mock server received no requests (short-circuit)

**`TestDescribeFile_LLMDown`:**
- Use a URL that points to nothing (e.g., `http://127.0.0.1:1` — a port nothing listens on)
- Verify returns `nil, nil`
- Verify logger received a Warn-level message

**`TestDescribeFile_LLMTimeout`:**
- Mock handler sleeps 5 seconds; config timeout is 1 second
- Verify returns `nil, nil`

**`TestDescribeFile_InvalidResponse`:**
- Mock LLM returns `"I don't understand your request"`
- Verify returns `nil, nil`
- Verify logger received a Warn-level message containing the response preview

**`TestDescribeFile_LargeFile`:**
- Provide file content exceeding 6000 characters (caller is responsible for truncation, but verify the describer passes content through to the LLM as-is)
- Verify the content sent to the mock LLM matches the input (no internal truncation)

---

## Acceptance Criteria

- [ ] **Part A:** All prompt builder tests pass: `TruncateFileContent` (5 cases), `FormatRelationshipContext` (5 cases), `buildMessages` (5 cases)
- [ ] **Part A:** All response parser tests pass: `extractJSON` (8 cases), `parseDescriptions` (6 cases) returning `[]rag.Description`
- [ ] **Part A:** `first200Chars` tests pass: short string, exactly 200 chars, over 200 with multi-byte runes at boundary
- [ ] **Part B:** All LLM client tests pass: success, HTTP error, empty choices, invalid JSON, context cancellation, timeout
- [ ] **Part B:** All describer integration tests pass: success, empty file, LLM down, timeout, invalid response, large file
- [ ] Tests use `httptest.NewServer` for mock LLM — no real HTTP calls
- [ ] Tests verify graceful failure behavior: LLM errors return `nil, nil`, not an error
- [ ] Tests verify logging: Warn-level messages emitted on failure (use a test logger or `slog.Handler` that captures records)
- [ ] `go test ./internal/rag/describer/...` passes with zero failures

## Sizing Note

This task is split into two independently-workable parts to stay within the 4-hour budget.
