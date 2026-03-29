# Task 11: Unit Tests — Python Tree-sitter Parser

**Epic:** 02 — Tree-sitter Parsers
**Status:** ⬚ Not started
**Dependencies:** Task 05

---

## Description

Write comprehensive unit tests for the Python tree-sitter parser. Tests use inline Python source strings and assert exact `RawChunk` fields. Tests cover standard functions, classes, decorated definitions, async functions, and edge cases.

## Acceptance Criteria

- [ ] Test file at `internal/rag/parser/python_parser_test.go`
- [ ] **Function extraction test:** parsing `def add(x: int, y: int) -> int:\n    return x + y` produces:
  - `Name` = `"add"`
  - `Signature` = `"def add(x: int, y: int) -> int:"`
  - `ChunkType` = `Function`
  - `Body` contains the full function text including the body
  - `LineStart` = 1
- [ ] **Class extraction test:** parsing `class MyService(BaseService):\n    def run(self):\n        pass` produces:
  - `Name` = `"MyService"`
  - `Signature` = `"class MyService(BaseService):"`
  - `ChunkType` = `Class`
  - `Body` contains the full class text including methods
- [ ] **Decorated function test:** parsing `@app.route("/api")\ndef handler():\n    pass` produces:
  - `Name` = `"handler"`
  - `Signature` includes the decorator: starts with `@app.route("/api")`
  - `LineStart` = 1 (line of the decorator, not the def)
  - `Body` includes the decorator text
- [ ] **Multiple decorators test:** a function with two decorators (`@staticmethod\n@cache\ndef compute():`) includes both decorators in the body and signature, `LineStart` is the first decorator's line
- [ ] **Async function test:** parsing `async def fetch(url: str) -> bytes:\n    pass` produces:
  - `Name` = `"fetch"`
  - `Signature` = `"async def fetch(url: str) -> bytes:"`
  - `ChunkType` = `Function`
- [ ] **Class with no bases test:** parsing `class Empty:\n    pass` produces `Name` = `"Empty"`, `Signature` = `"class Empty:"`
- [ ] **Multiple declarations test:** a source with 2 functions and 1 class returns exactly 3 chunks in source order
- [ ] **Nested function ignored test:** parsing `def outer():\n    def inner():\n        pass\n    return inner()` returns exactly 1 chunk (only `outer`; `inner` is not a top-level declaration)
- [ ] **No declarations test:** parsing `import os\nprint("hello")` returns an empty `[]RawChunk`
- [ ] **Empty file test:** parsing an empty string returns an empty `[]RawChunk` with no error
