# Task 03: Go Tree-sitter Parser

**Epic:** 02 — Tree-sitter Parsers
**Status:** ⬚ Not started
**Dependencies:** Task 01, L1-E01 (RawChunk type, ChunkType enum)

---

## Description

Implement the Go language parser using tree-sitter CGo bindings. This parser extracts top-level declarations from Go source files and produces `RawChunk` structs for each declaration. This is the simpler Go parser used as a fallback when the full Go AST parser (L1-E03) fails. It does NOT produce call graphs, type usage, or interface implementation data -- only declaration-level chunks.

## Acceptance Criteria

- [ ] `GoParser` struct in `internal/rag/parser/go_parser.go` with method `Parse(filePath string, content []byte) ([]RawChunk, error)`
- [ ] Extracts `function_declaration` nodes from the tree-sitter AST. For each:
  - `Name`: the function name (child node with field name `name`)
  - `Signature`: everything from the start of the node up to (but not including) the opening `{` of the body, trimmed of trailing whitespace
  - `Body`: the full text of the entire node (declaration + body)
  - `ChunkType`: `Function`
  - `LineStart`: the 1-based starting line of the node
  - `LineEnd`: the 1-based ending line of the node
- [ ] Extracts `method_declaration` nodes from the tree-sitter AST. For each:
  - `Name`: the method name (child node with field name `name`)
  - `Signature`: everything from the start of the node up to (but not including) the opening `{` of the body, trimmed of trailing whitespace
  - `Body`: the full text of the entire node
  - `ChunkType`: `Method`
  - `LineStart` / `LineEnd`: 1-based line range
- [ ] Extracts `type_declaration` nodes from the tree-sitter AST. For each:
  - `Name`: the type name from the `type_spec` child (field name `name`)
  - `Signature`: the full text of the type declaration (for type declarations, signature = full body since there is no separate "body block")
  - `Body`: the full text of the entire node
  - `ChunkType`: `Type` for struct/basic types, `Interface` for interface types (determined by whether the `type_spec` contains an `interface_type` node)
  - `LineStart` / `LineEnd`: 1-based line range
- [ ] Nodes are returned in source order (by line number)
- [ ] The parser creates a new tree-sitter parser instance, sets the Go language, parses the content, and walks the root node's children
- [ ] If tree-sitter parsing returns a tree with errors (root node `HasError()`), the parser returns an error (allowing the dispatcher to fall back). The error is `fmt.Errorf("tree-sitter parse error in %s: syntax tree has errors", filePath)`
- [ ] Empty files (zero bytes or only whitespace) return an empty `[]RawChunk` with no error
