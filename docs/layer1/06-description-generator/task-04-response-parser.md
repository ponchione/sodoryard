# Task 04: Response Parser

**Epic:** 06 — Description Generator
**Status:** ⬚ Not started
**Dependencies:** L1-E01 (types — `rag.Description` struct used as return type)

---

## Description

Implement the response parser that extracts a JSON array of `{name, description}` pairs from raw LLM output. Local models frequently include preamble text, markdown code fences, or trailing commentary around the JSON. The parser must handle all of these by locating and extracting the JSON array from anywhere in the response string. This is a pure function — no I/O, no side effects.

## Package

`internal/rag/describer/parse.go`

## Types

```go
type descriptionEntry struct {
    Name        string `json:"name"`
    Description string `json:"description"`
}
```

## Functions

### `extractJSON(raw string) (string, error)`

Extracts a JSON array from the raw LLM response. Handles these cases:

1. **Clean JSON:** Response is already a valid JSON array (starts with `[`, ends with `]`). Return as-is after trimming whitespace.

2. **Markdown code fences:** Response contains ` ```json\n[...]\n``` ` or ` ```\n[...]\n``` `. Extract the content between the fences.

3. **Preamble/postscript text:** Response has text before the `[` or after the final `]`. Find the first `[` and the last `]`, extract that substring.

4. **No JSON array found:** Response contains no `[` or the `[` appears after the last `]`. Return error: `"no JSON array found in LLM response"`.

Implementation strategy:
- Trim leading/trailing whitespace.
- Check for markdown fences first: find ` ``` ` markers, extract inner content, recurse/retry extraction on inner content.
- If no fences, find first `[` and last `]`. If first `[` index < last `]` index, extract that substring.
- Otherwise, return error.

### `parseDescriptions(raw string) ([]rag.Description, error)`

Parses the raw LLM response into a `[]rag.Description`.

1. Call `extractJSON(raw)`. If error, return `nil, err`.
2. Unmarshal extracted JSON into `[]descriptionEntry`. If unmarshal fails, return `nil, fmt.Errorf("failed to parse description JSON: %w", err)`.
3. Build `[]rag.Description` from the entries. Each `descriptionEntry` maps to a `rag.Description` with `Name` and `Description` fields (see E01-T03 for the `Description` struct).
4. Skip entries where `Name` is empty (defensive — LLM might produce a malformed entry).
5. If the resulting slice is empty (zero valid entries), return `nil, fmt.Errorf("LLM returned no valid descriptions")`.
6. Return the slice.

## Edge Cases to Handle

| LLM output | Expected behavior |
|---|---|
| `[{"name":"Foo","description":"Does foo"}]` | Parses normally |
| `` ```json\n[{"name":"Foo","description":"Does foo"}]\n``` `` | Strips fences, parses normally |
| `Here are the descriptions:\n[{"name":"Foo","description":"Does foo"}]\nHope that helps!` | Extracts array between first `[` and last `]` |
| `{"name":"Foo","description":"Does foo"}` (object, not array) | Error: `"no JSON array found in LLM response"` |
| Empty string | Error: `"no JSON array found in LLM response"` |
| `[{"name":"","description":"something"}]` | Skipped (empty name), then error: `"LLM returned no valid descriptions"` |
| `[{"name":"Foo","description":"Does foo"},{"name":"Foo","description":"Also foo"}]` | Both entries included in the slice — duplicates acceptable, unlikely in practice |
| `[{"name":"Foo","description":"Does foo"},{}]` | Entry with empty name skipped; `Foo` entry kept |

## Acceptance Criteria

- [ ] `extractJSON` extracts JSON array from clean response
- [ ] `extractJSON` strips markdown code fences (both ` ```json ` and bare ` ``` `)
- [ ] `extractJSON` handles preamble and postscript text around JSON array
- [ ] `extractJSON` returns error when no JSON array is present
- [ ] `parseDescriptions` returns `[]rag.Description` with `Name` and `Description` fields populated
- [ ] `parseDescriptions` skips entries with empty `Name`
- [ ] `parseDescriptions` returns error when LLM response contains no valid descriptions
- [ ] `parseDescriptions` returns error when JSON unmarshal fails (malformed JSON)
- [ ] Both functions are unexported (package-internal), tested via same-package `_test.go`
