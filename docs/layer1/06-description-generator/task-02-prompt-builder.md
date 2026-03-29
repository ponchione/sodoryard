# Task 02: Prompt Builder

**Epic:** 06 — Description Generator
**Status:** ⬚ Not started
**Dependencies:** L1-E01 (Chunk type with relationship fields)

---

## Description

Implement the prompt construction logic that prepares the LLM input for description generation. This includes public utility functions for file content truncation and relationship context formatting (called by the indexing pipeline before invoking `DescribeFile`), and the internal message assembly for the LLM. The prompt builder is a pure function with no side effects.

## Package

`internal/rag/describer/prompt.go`

## Functions

### `TruncateFileContent(content string, maxLen int) string` (exported)

Public utility for callers (e.g., the E07 indexing pipeline) to truncate file content before passing it to `DescribeFile`. Truncates to `maxLen` characters. If truncation occurs, appends `"\n... (truncated)"` as a signal to the LLM. If the content is at or below the limit, returns it unchanged.

- Truncation cuts at the character boundary (not mid-rune). Use `[]rune` conversion to avoid splitting a multi-byte character.
- The `"... (truncated)"` suffix is included within the `maxLen` budget (i.e., the raw content is cut at `maxLen - len("\n... (truncated)")` to leave room for the suffix).

### `FormatRelationshipContext(chunks []rag.Chunk) string` (exported)

Public utility for callers (e.g., the E07 indexing pipeline) to format relationship context before passing it to `DescribeFile`. The describer package owns this formatting logic, but the function is called BEFORE `DescribeFile` — the caller passes the resulting string as the `relationshipContext` parameter.

Builds a text block summarizing the relationship metadata for each chunk in the file. This context helps the LLM write descriptions that capture each function's role in the codebase, not just its body.

Output format (one block per chunk that has any relationship data):

```
--- Relationships ---
FuncName:
  Calls: OtherFunc, ThirdFunc
  Called by: CallerA, CallerB
  Types used: Claims, TokenConfig
  Implements: Validator

AnotherFunc:
  Calls: (none)
  Called by: Handler
  Types used: Request
  Implements: (none)
```

Rules:
- Skip chunks with no relationship data (all four fields empty).
- Omit the entire relationships section if no chunks have relationship data.
- For each relationship field, join entries with `, `. If the field is empty, print `(none)`.
- The four fields are: `Calls` (`chunk.Calls`), `Called by` (`chunk.CalledBy`), `Types used` (`chunk.TypesUsed`), `Implements` (`chunk.ImplementsIfaces`).

### `buildMessages(fileContent string, relationshipContext string) []Message` (unexported)

Assembles the system and user messages for the LLM chat completion request. Called internally by the `Describer` with the already-truncated file content and already-formatted relationship context string.

```go
type Message struct {
    Role    string `json:"role"`
    Content string `json:"content"`
}
```

**System message content (exact text):**

```
You are a code analysis assistant. For each function, method, or type in the provided source file, write a 1-2 sentence description of what it does and why. Focus on semantic purpose, not implementation details.

Return ONLY a JSON array with no additional text. Each element must have exactly two fields:
[{"name": "SymbolName", "description": "What this symbol does and why."}]

Do not include markdown code fences. Do not include any text before or after the JSON array.
```

**User message content:** The concatenation of the file content and the relationship context (if non-empty), separated by two newlines:

```
{fileContent}

{relationshipContext}
```

If `relationshipContext` is empty, the user message is just the file content with no trailing newlines.

## Acceptance Criteria

- [ ] `TruncateFileContent` (exported) returns content unchanged when at or below `maxLen`
- [ ] `TruncateFileContent` truncates at rune boundary and appends `"\n... (truncated)"` within budget
- [ ] `FormatRelationshipContext` (exported) produces the specified format with all four relationship fields
- [ ] `FormatRelationshipContext` returns empty string when no chunks have relationship data
- [ ] `FormatRelationshipContext` skips chunks where all four relationship slices are empty
- [ ] `buildMessages` (unexported) accepts `(fileContent string, relationshipContext string)` — no chunks parameter
- [ ] `buildMessages` returns exactly 2 messages: system (role `"system"`) and user (role `"user"`)
- [ ] System message matches the exact text specified above
- [ ] User message concatenates file content and relationship context correctly
