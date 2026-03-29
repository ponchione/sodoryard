# Task 05: Python Tree-sitter Parser

**Epic:** 02 — Tree-sitter Parsers
**Status:** ⬚ Not started
**Dependencies:** Task 01, L1-E01 (RawChunk type, ChunkType enum)

---

## Description

Implement the Python language parser using tree-sitter CGo bindings. This is the primary parser for Python files. It extracts top-level function definitions and class definitions. Python's signature ends at the `:` character (before the body block).

## Acceptance Criteria

- [ ] `PythonParser` struct in `internal/rag/parser/python_parser.go` with method `Parse(filePath string, content []byte) ([]RawChunk, error)`
- [ ] Extracts `function_definition` nodes from the tree-sitter AST. For each:
  - `Name`: the function name (field name `name`)
  - `Signature`: everything from the start of the node up to and including the `:` that ends the function header (before the body block), trimmed of trailing whitespace. This includes decorators if they are siblings preceding the node. Example: `def foo(x: int, y: str) -> bool:` or `async def bar():` for async functions
  - `Body`: full text of the entire node (including decorators if captured)
  - `ChunkType`: `Function`
  - `LineStart` / `LineEnd`: 1-based line range
- [ ] Extracts `class_definition` nodes from the tree-sitter AST. For each:
  - `Name`: the class name (field name `name`)
  - `Signature`: everything from `class` keyword through the class name and any base classes/arguments, up to and including the `:`. Example: `class MyClass(BaseClass, metaclass=ABCMeta):`
  - `Body`: full text of the entire node
  - `ChunkType`: `Class`
  - `LineStart` / `LineEnd`: 1-based line range
- [ ] Only top-level declarations are extracted (direct children of the `module` root node). Nested functions inside classes or other functions are NOT extracted as separate chunks.
- [ ] `decorated_definition` nodes are handled: if a `function_definition` or `class_definition` is wrapped in a `decorated_definition`, the decorator text is included in both the `Signature` and `Body`, and the `LineStart` reflects the first decorator line
- [ ] Nodes are returned in source order (by line number)
- [ ] If tree-sitter parsing returns a tree with errors (root node `HasError()`), the parser returns an error. The error is `fmt.Errorf("tree-sitter parse error in %s: syntax tree has errors", filePath)`
- [ ] Empty files return an empty `[]RawChunk` with no error
