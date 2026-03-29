# L1-E02 — Tree-sitter Parsers

**Layer:** 1 — Code Intelligence
**Epic:** 02
**Status:** ⬜ Not Started
**Dependencies:** L1-E01 (types & interfaces)

---

## Description

Integrate tree-sitter via CGo bindings and implement the multi-language parser dispatcher. This covers the tree-sitter grammars for Go, TypeScript/TSX, and Python, plus the Markdown section splitter and the fallback chunker for unsupported languages. Each parser extracts top-level declarations into `RawChunk` structs at semantic boundaries (function/method/type/class/interface declarations), not arbitrary text windows.

The tree-sitter parsers are the primary parsing path for TypeScript/TSX and Python, and serve as a fallback for Go when the Go AST parser ([[L1-E03-go-ast-parser]]) fails (e.g., unparseable files). This ports directly from topham's `internal/rag/parser.go`.

---

## Package

`internal/rag/parser/` — parser dispatcher, language-specific extractors, tree-sitter CGo bindings.

---

## Definition of Done

- [ ] tree-sitter CGo bindings compile successfully with `CGO_ENABLED=1`
- [ ] tree-sitter grammars vendored and pinned for: Go, TypeScript, TSX, Python
- [ ] Parser dispatcher: given a file path and content, selects the correct parser by file extension
- [ ] **Go parser:** extracts `function_declaration`, `method_declaration`, `type_declaration` nodes. Produces `RawChunk` with name, signature (everything before body), full body text, chunk type, line range
- [ ] **TypeScript/TSX parser:** extracts function/class/interface/type_alias/enum declarations and exported arrow functions. Produces `RawChunk` per declaration
- [ ] **Python parser:** extracts function and class definitions. Produces `RawChunk` per definition
- [ ] **Markdown parser:** splits on `##` headers into sections. Each section produces a `RawChunk` with the heading as the name
- [ ] **Fallback parser:** 40-line sliding windows with 20-line overlap for unsupported file types. Produces `RawChunk` per window
- [ ] Body truncation enforced: bodies exceeding `MaxBodyLength` (2000 chars) are truncated before `RawChunk` creation
- [ ] Parser returns `[]RawChunk` matching the `Parser` interface from [[L1-E01-types-and-interfaces]]
- [ ] Unit tests for each language parser against representative source files
- [ ] Unit tests for dispatcher routing (correct parser selected per extension)
- [ ] Unit tests for edge cases: empty files, files with no declarations, very long function bodies, syntax errors in source files
- [ ] Makefile updated with CGo flags for tree-sitter compilation if needed (verified by successful `go build` with `CGO_ENABLED=1`)

---

## Architecture References

- [[04-code-intelligence-and-rag]] — "Component: Parsing Pipeline" → "Tree-sitter Parsers" section
- [[02-tech-stack-decisions]] — "Code Parsing: tree-sitter (CGo)" and "CGo: Accepted"
- topham source: `internal/rag/parser.go`

---

## Notes

- The Go tree-sitter parser here is a simpler extraction than the Go AST parser in [[L1-E03-go-ast-parser]]. It extracts declarations but does NOT produce call graphs, type usage, or interface implementation data. It's the fallback path when `go/packages` analysis fails on a Go file.
- Tree-sitter grammars should be pinned to specific versions in `go.mod` or vendored directly. Grammar updates can change node types and break extraction logic.
- The Makefile already handles CGo compilation from Layer 0 (L0-E01). This epic adds tree-sitter-specific linker flags if needed.
- Signature extraction ("everything before the body") is language-specific. For Go, it's the function/method declaration up to the opening `{`. For Python, it's up to the `:`. The extraction logic per language needs careful attention.
