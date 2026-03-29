# Task 02: Declaration Extraction

**Epic:** 03 — Go AST Parser
**Status:** ⬚ Not started
**Dependencies:** Task 01, L1-E01 (RawChunk and ChunkType types)

---

## Description

Implement the core declaration extraction logic that walks a Go file's AST and produces a `RawChunk` for each top-level declaration. This covers functions, methods, and type declarations (structs and interfaces). Each chunk captures the declaration's name, signature, full body text, chunk type, and line range.

## Acceptance Criteria

- [ ] Method on `GoParser`: `extractDeclarations(filePath string, file *ast.File, pkg *packages.Package) ([]codeintel.RawChunk, error)`
- [ ] **Function declarations** (`*ast.FuncDecl` with nil receiver): extracted as `ChunkType = Function`
  - `Name`: the function name (e.g., `"ProcessFile"`)
  - `Signature`: text from the `func` keyword through the closing `)` of the return types (everything before the body block), e.g., `"func ProcessFile(path string) ([]Chunk, error)"`
  - `Body`: full source text of the entire declaration (signature + body block)
  - `LineStart`: 1-based line number of the `func` keyword
  - `LineEnd`: 1-based line number of the closing `}` of the body
- [ ] **Method declarations** (`*ast.FuncDecl` with non-nil receiver): extracted as `ChunkType = Method`
  - `Name`: receiver type + `.` + method name (e.g., `"Parser.Parse"` for a method `Parse` on `Parser`; `"*Parser.Parse"` for a pointer receiver)
  - `Signature`: full method signature including receiver, e.g., `"func (p *Parser) Parse(path string) error"`
  - `Body`: full source text of the entire declaration
  - `LineStart` / `LineEnd`: same rules as function declarations
- [ ] **Type declarations — struct** (`*ast.GenDecl` with `*ast.TypeSpec` where `.Type` is `*ast.StructType`): extracted as `ChunkType = Type`
  - `Name`: the type name (e.g., `"Config"`)
  - `Signature`: `"type Config struct"` (type keyword + name + kind)
  - `Body`: full source text from `type` through the closing `}`
  - `LineStart` / `LineEnd`: span of the full type declaration
- [ ] **Type declarations — interface** (`*ast.GenDecl` with `*ast.TypeSpec` where `.Type` is `*ast.InterfaceType`): extracted as `ChunkType = Interface`
  - `Name`: the type name (e.g., `"Store"`)
  - `Signature`: `"type Store interface"` (type keyword + name + kind)
  - `Body`: full source text from `type` through the closing `}`
  - `LineStart` / `LineEnd`: span of the full type declaration
- [ ] Grouped type declarations (`type ( ... )` blocks) are split into individual `RawChunk`s, one per `TypeSpec`
- [ ] Source text extraction uses `token.FileSet` positions to read the exact byte range from the original source (not AST pretty-printing)
- [ ] Bodies exceeding `codeintel.MaxBodyLength` (2000 characters) are truncated to that limit
- [ ] Declarations without a body (e.g., external function declarations with no block) are still extracted with an empty `Body`
- [ ] If `token.FileSet` position resolution fails for a single declaration (e.g., position is out of range), that declaration is skipped with a warning log and processing continues with remaining declarations. The function does not return an error for individual position resolution failures.
