# Task 11: Integration Tests and Graceful Degradation Tests

**Epic:** 03 — Go AST Parser
**Status:** ⬚ Not started
**Dependencies:** Task 07, Task 08, Task 09, Task 10

---

## Description

Integration tests that verify the full end-to-end flow: construct a `GoParser` against a test project, parse files, and verify that cross-cutting concerns work together — call graph connectivity across declarations, `ParseWithRelationships` returning coherent data, and graceful degradation on unparseable files. These are the final verification that the parser produces correct, connected output.

## Acceptance Criteria

- [ ] **Integration test: call graph connectivity**
  - `TestCallGraphConnectivity` — loads the test project, parses `caller.go`, and verifies:
    - Function `ProcessOrder` that calls function `ValidateInput` has `ValidateInput` in `ProcessOrder`'s `Calls` list
    - The relationship data is accessible from `ParseWithRelationships` return value
    - Call targets use consistent naming (the same qualified name format used in chunk `Name` fields), enabling the indexer to match callers to callees
- [ ] **Integration test: full file parse**
  - `TestParseFileEndToEnd` — parses `declarations.go` through `ParseWithRelationships` and verifies:
    - Returns exactly 7 `RawChunk`s for the test file: 2 functions (`ProcessOrder`, `ValidateInput`), 2 methods (`*OrderService.Create`, `*OrderService.Get`), 1 struct (`OrderService`), 1 interface (`Validator`), 1 type alias (`OrderID`)
    - `FileRelationships.Imports` is populated
    - Each chunk with a type declaration has `ChunkRelationships` entries with populated `TypesUsed` or `ImplementsIfaces`
    - Each chunk with a function/method declaration has `ChunkRelationships` entries with populated `Calls`
- [ ] **Integration test: Parser interface compliance**
  - `TestParserInterfaceCompliance` — verifies `GoParser` can be assigned to a `rag.Parser` variable (compile-time check via assignment: `var _ rag.Parser = (*goparser.GoParser)(nil)`). The `rag.Parser` interface (from L1-E01) requires: `Parse(filePath string, content []byte) ([]RawChunk, error)`
- [ ] **Graceful degradation test: file not loaded**
  - `TestParseUnknownFile` — calls `Parse("/nonexistent/file.go", []byte("package foo"))` and verifies:
    - Error is returned
    - `errors.Is(err, goparser.ErrFileNotLoaded)` is true
- [ ] **Graceful degradation test: syntax error file**
  - `TestParseSyntaxErrorFile` — adds a `testdata/testproject/pkg/broken.go` file with deliberate syntax errors (or constructs one in a temp directory for the test)
  - Verifies the parser either returns `ErrFileNotLoaded` (if `go/packages` excluded it) or returns partial results with `ErrPackageErrors`
  - Verifies no panic occurs
- [ ] **Graceful degradation test: nil safety**
  - `TestNilSafety` — exercises edge cases: function with no body (`*ast.FuncDecl` where `Body` is nil — e.g., CGo extern declarations), type spec with nil type expression
  - Verifies no panic and graceful skip of the malformed declaration
- [ ] All tests run with `go test ./internal/rag/goparser/...` and pass
