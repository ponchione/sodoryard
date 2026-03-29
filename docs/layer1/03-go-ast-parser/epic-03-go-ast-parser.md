# L1-E03 — Go AST Parser

**Layer:** 1 — Code Intelligence
**Epic:** 03
**Status:** ⬜ Not Started
**Dependencies:** L1-E01 (types & interfaces)

---

## Description

Implement the enhanced Go parser using `go/packages` and `go/types` for full type-resolved analysis of Go source files. This is the primary parsing path for Go code — richer than tree-sitter because it produces call graph data, type usage tracking, interface implementation detection, and import tracking alongside the standard declaration extraction. This metadata powers both the reverse call graph construction in the indexing pipeline ([[L1-E07-indexing-pipeline]]) and the structural graph's blast radius analysis ([[L1-E09-structural-graph]]).

The Go AST parser is initialized once per indexing run (loads all packages via `go/packages` — takes seconds) and is then cheap per-file. Ports from topham's `internal/rag/goparser.go`.

---

## Package

`internal/codeintel/goparser/` — Go AST parser with call graph extraction.

---

## Definition of Done

- [ ] Parser initializes via `go/packages.Load()` with `NeedTypes`, `NeedSyntax`, `NeedTypesInfo`, `NeedDeps` modes against the project root
- [ ] Extracts `function_declaration`, `method_declaration`, `type_declaration` (struct, interface) from Go source files
- [ ] Produces `RawChunk` per declaration with: name, signature, body, chunk type, line range
- [ ] **Call graph extraction:** for each function/method, walks call expressions in the AST to produce a `Calls []string` list (qualified names: `"pkg.FuncName"`)
- [ ] **Type usage tracking:** for each type declaration, identifies referenced types in field definitions and method signatures
- [ ] **Interface implementation detection:** uses `go/types` to determine which interfaces each concrete type satisfies
- [ ] **Import tracking:** extracts import paths per file
- [ ] Relationship metadata (Calls, TypesUsed, ImplementsIfaces, Imports) attached to `RawChunk` or returned alongside it for the indexer to merge into `Chunk`
- [ ] Graceful degradation: if `go/packages` fails to load a specific file (syntax errors, missing dependencies), fall back to tree-sitter parsing for that file via the parser dispatcher. Note: the parser dispatcher fallback logic is implemented in L1-E02 (Tree-sitter Parsers). This epic (L1-E03) defines sentinel errors that the dispatcher uses to trigger fallback. End-to-end fallback is verified when both epics are integrated.
- [ ] Implements the `Parser` interface from [[L1-E01-types-and-interfaces]] (or a richer variant that also returns relationship metadata)
- [ ] Unit tests against a test Go project with: functions, methods, types, interfaces, cross-package calls, interface implementations
- [ ] Unit tests for graceful fallback on unparseable files
- [ ] Integration test: load a real Go project, verify call graph connectivity (function A calls function B → B appears in A's Calls list)

---

## Architecture References

- [[04-code-intelligence-and-rag]] — "Component: Parsing Pipeline" → "Go AST Parser (Enhanced)" section
- [[04-code-intelligence-and-rag]] — "Component: Indexing Pipeline" → Pass 2 (Reverse Call Graph) depends on forward call references from this parser
- topham source: `internal/rag/goparser.go`

---

## Notes

- The `go/packages` loader is expensive to initialize (loads all packages in the project). It should be created once and reused across all files in an indexing run. The parser's constructor should accept the project root and return a stateful parser object.
- The call graph extraction walks `*ast.CallExpr` nodes. Resolving the callee to a qualified name requires the `go/types.Info` from the loaded packages. Unresolved calls (to external packages not loaded) should be recorded as-is, not silently dropped.
- Interface implementation detection uses `types.Implements()`. This requires iterating all concrete types against all interface types found during loading. The implementation should cache this computation.
- This epic's output (relationship metadata) is a critical input to both [[L1-E07-indexing-pipeline]] (Pass 2: reverse call graph) and [[L1-E09-structural-graph]] (blast radius analysis). The relationship data format must be consistent.
