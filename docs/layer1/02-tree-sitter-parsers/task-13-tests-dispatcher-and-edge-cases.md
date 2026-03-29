# Task 13: Unit Tests — Dispatcher Routing and Edge Cases

**Epic:** 02 — Tree-sitter Parsers
**Status:** ⬚ Not started
**Dependencies:** Task 02, Task 08

---

## Description

Write unit tests for the parser dispatcher's extension routing logic, the fallback-on-error behavior, and edge cases that span all parsers. These tests validate the integration layer that ties all parsers together.

## Acceptance Criteria

### Dispatcher routing tests

- [ ] Test file at `internal/codeintel/treesitter/dispatcher_test.go`
- [ ] **Go routing test:** `Parse("main.go", goSource)` routes to the Go parser and returns function/method/type chunks
- [ ] **TypeScript routing test:** `Parse("app.ts", tsSource)` routes to the TypeScript parser
- [ ] **TSX routing test:** `Parse("component.tsx", tsxSource)` routes to the TSX parser
- [ ] **Python routing test:** `Parse("script.py", pySource)` routes to the Python parser
- [ ] **Markdown routing test (`.md`):** `Parse("README.md", mdSource)` routes to the Markdown splitter
- [ ] **Markdown routing test (`.markdown`):** `Parse("notes.markdown", mdSource)` routes to the Markdown splitter
- [ ] **Unknown extension routing test:** `Parse("data.json", jsonContent)` routes to the fallback chunker, returns `Fallback` chunk type
- [ ] **No extension routing test:** `Parse("Makefile", content)` routes to the fallback chunker
- [ ] **Case-insensitive extension test:** `Parse("Main.GO", goSource)` routes to the Go parser (not the fallback)

### Fallback-on-error tests

- [ ] **Syntax error fallback test:** `Parse("broken.go", invalidGoSource)` where `invalidGoSource` is syntactically invalid Go (e.g., `func {{{`) falls back to the fallback chunker and returns `Fallback` chunks instead of an error
- [ ] **Fallback error propagation test:** construct a Dispatcher with a mock fallback parser (a `codeintel.Parser` implementation that always returns an error). Call `Parse` with an unknown extension. Verify the mock's error is propagated to the caller.

### Edge case tests

- [ ] **Empty file test:** `Parse("empty.go", []byte(""))` returns an empty `[]RawChunk` with no error
- [ ] **Whitespace-only file test:** `Parse("blank.py", []byte("   \n\n  \n"))` returns an empty `[]RawChunk` with no error
- [ ] **Body truncation integration test:** `Parse("big.go", sourceWithHugeFunctionBody)` where the function body is 3000 characters returns a chunk with `Body` truncated to 2000 characters
- [ ] **Very long file test:** a file with 1000 lines routed to the fallback chunker produces the expected number of chunks (49 chunks: starting positions 1, 21, 41, ..., 961, plus a final chunk if lines remain)
- [ ] **File with only comments test:** `Parse("comments.go", []byte("package main\n// just a comment\n"))` returns an empty `[]RawChunk` (no declarations extracted, but no error)
