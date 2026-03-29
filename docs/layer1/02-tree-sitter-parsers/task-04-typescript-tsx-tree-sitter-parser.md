# Task 04: TypeScript/TSX Tree-sitter Parser

**Epic:** 02 — Tree-sitter Parsers
**Status:** ⬚ Not started
**Dependencies:** Task 01, L1-E01 (RawChunk type, ChunkType enum)

---

## Description

Implement the TypeScript and TSX language parsers using tree-sitter CGo bindings. TypeScript and TSX use different tree-sitter grammars but share extraction logic. Both are primary parsers (not fallbacks) since there is no richer TypeScript parser equivalent to the Go AST parser. The parser extracts top-level declarations: functions, classes, interfaces, type aliases, enums, and exported arrow functions.

## Acceptance Criteria

- [ ] `TypeScriptParser` struct in `internal/rag/parser/typescript_parser.go` with method `Parse(filePath string, content []byte) ([]RawChunk, error)`
- [ ] `TSXParser` struct in the same file (or separate `tsx_parser.go`), identical extraction logic but configured with the TSX tree-sitter grammar instead of the TypeScript grammar
- [ ] Extracts `function_declaration` nodes:
  - `Name`: the function name (field name `name`)
  - `Signature`: everything from the start of the node up to (but not including) the opening `{` of the body, trimmed of trailing whitespace
  - `Body`: full text of the entire node
  - `ChunkType`: `Function`
  - `LineStart` / `LineEnd`: 1-based line range
- [ ] Extracts `class_declaration` nodes:
  - `Name`: the class name (field name `name`)
  - `Signature`: everything from `class` keyword through the class name and any `extends`/`implements` clauses, up to the opening `{`
  - `Body`: full text of the entire node
  - `ChunkType`: `Class`
  - `LineStart` / `LineEnd`: 1-based line range
- [ ] Extracts `interface_declaration` nodes:
  - `Name`: the interface name (field name `name`)
  - `Signature`: everything from `interface` keyword through the interface name and any `extends` clause, up to the opening `{`
  - `Body`: full text of the entire node
  - `ChunkType`: `Interface`
  - `LineStart` / `LineEnd`: 1-based line range
- [ ] Extracts `type_alias_declaration` nodes:
  - `Name`: the type alias name (field name `name`)
  - `Signature`: the full text of the type alias (e.g., `type Foo = string | number`)
  - `Body`: full text of the entire node (same as signature for type aliases)
  - `ChunkType`: `Type`
  - `LineStart` / `LineEnd`: 1-based line range
- [ ] Extracts `enum_declaration` nodes:
  - `Name`: the enum name (field name `name`)
  - `Signature`: everything from `enum` keyword through the enum name, up to the opening `{`
  - `Body`: full text of the entire node
  - `ChunkType`: `Type`
  - `LineStart` / `LineEnd`: 1-based line range
- [ ] Extracts exported arrow functions: `export_statement` nodes containing a `lexical_declaration` with an `arrow_function` initializer:
  - `Name`: the variable name from the `variable_declarator` (e.g., for `export const foo = () => {}`, the name is `foo`)
  - `Signature`: everything from `export` through the arrow function parameters and return type annotation (if present), up to the `=>` token (inclusive)
  - `Body`: full text of the entire export statement
  - `ChunkType`: `Function`
  - `LineStart` / `LineEnd`: 1-based line range
- [ ] Non-exported arrow functions assigned to `const`/`let`/`var` at the top level are NOT extracted (only exported ones)
- [ ] Nodes are returned in source order (by line number)
- [ ] If tree-sitter parsing returns a tree with errors (root node `HasError()`), the parser returns an error. The error is `fmt.Errorf("tree-sitter parse error in %s: syntax tree has errors", filePath)`
- [ ] Empty files return an empty `[]RawChunk` with no error

## Work Breakdown

**Part A (~2h):** Five direct node type extractions: function_declaration, class_declaration, interface_declaration, type_alias_declaration, enum_declaration. These follow the same pattern as the Go parser.

**Part B (~2h):** Exported arrow function detection (AC7: export_statement → lexical_declaration → variable_declarator → arrow_function) plus TSX parser setup and dual-grammar testing.

This task should be worked in two sessions to stay within the 4-hour budget.
