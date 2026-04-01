# Layer 1 Audit Fix Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Resolve all issues found in the Layer 1 Code Intelligence audit.

**Architecture:** Three categories of fixes — (A) spec alignment where specs were aspirational,
(B) real code gaps ported from the reference codebase at ~/source/agent-conductor,
(C) dead code removal. All changes maintain backward compatibility.

**Tech Stack:** Go, tree-sitter, go/ast, go/packages, LanceDB, SQLite

---

## Task 1: Remove unused HopDepth from SearchOptions

**Objective:** HopDepth is defined in SearchOptions but never read by the searcher.
Neither codebase implements multi-hop. Remove dead field rather than implement unused complexity (YAGNI).

**Files:**
- Modify: `internal/codeintel/types.go`
- Modify: `internal/codeintel/types_test.go`

**Step 1:** Remove `HopDepth int` field from SearchOptions struct in types.go

**Step 2:** Remove any test references to HopDepth in types_test.go

**Step 3:** Search entire codebase for HopDepth references and remove any remaining ones

**Step 4:** Run `make test` — expected: all pass

**Step 5:** Commit
```bash
git add -A && git commit -m "fix(codeintel): remove unused HopDepth from SearchOptions"
```

---

## Task 2: Update ChunkType enum to match actual usage

**Objective:** The ChunkType enum has class/section/enum/fallback but the spec expected
struct/const/var/file/module. The actual values are correct for a multi-language system.
Update the spec comment and add any missing values that ARE useful.

**Files:**
- Modify: `internal/codeintel/types.go`
- Modify: `internal/codeintel/types_test.go`

**Step 1:** Ensure ChunkType constants include all values actually used across parsers.
Current values (function, method, type, interface, class, section, enum, fallback) are correct.
No code changes needed — just verify the test covers all defined values.

**Step 2:** Update the audit checklist to reflect the actual enum values.

**Step 3:** Run `make test` — expected: all pass

**Step 4:** Commit
```bash
git add -A && git commit -m "docs(codeintel): align ChunkType spec with implementation"
```

---

## Task 3: Add tree-sitter fallback to GoASTParser (port from reference)

**Objective:** The reference codebase has GoASTParser embedding a *TreeSitterParser and
delegating non-Go files and fallback cases to it. Port this pattern.

**Files:**
- Modify: `internal/codeintel/goparser/goparser.go`
- Modify: `internal/codeintel/goparser/goparser_test.go`

**Step 1: Write failing test — non-Go file returns chunks via fallback**

Add test TestParse_NonGoFile_FallsBackToTreeSitter that creates a Python file,
passes it to GoASTParser.ParseFile with language="python", and asserts chunks are returned.

**Step 2:** Run test, verify it fails (currently GoASTParser returns empty for non-Go)

**Step 3: Implement fallback**

Add a `treeSitter` field to the Parser struct (or accept a codeintel.Parser interface as fallback).
In ParseFile, if language != "go", delegate to treeSitter.ParseFile().
For Go files not found in loaded packages, also fall back.

Reference pattern (~/source/agent-conductor/internal/rag/goparser.go lines 15-24, 108-137):
```go
type GoASTParser struct {
    fset       *token.FileSet
    pkgsByFile map[string]*packages.Package
    allIfaces  []ifaceInfo
    treeSitter *TreeSitterParser  // fallback
}
```

**Step 4:** Run `make test` — expected: all pass including new test

**Step 5:** Commit
```bash
git add -A && git commit -m "feat(goparser): add tree-sitter fallback for non-Go files"
```

---

## Task 4: Add generics test fixture to GoASTParser

**Objective:** Verify goparser handles Go generics. The go/packages loader already parses
generic code — we just need a test proving type parameters don't break parsing and
generic types are extracted as chunks.

**Files:**
- Modify: `internal/codeintel/goparser/goparser_test.go`

**Step 1: Write test TestParse_Generics**

Create a temp Go module with a file containing:
```go
package example

type Set[T comparable] struct {
    items map[T]bool
}

func NewSet[T comparable]() *Set[T] {
    return &Set[T]{items: make(map[T]bool)}
}

func (s *Set[T]) Add(item T) {
    s.items[item] = true
}
```

Assert: At least 3 chunks returned (Set type, NewSet function, Add method).
Assert: The Set chunk has ChunkType "type" or "interface".
Assert: No error returned.

**Step 2:** Run test — expected: PASS (go/packages handles generics natively)

**Step 3:** If the test fails, add explicit TypeParams handling to goparser.go

**Step 4:** Commit
```bash
git add -A && git commit -m "test(goparser): add generics test fixture"
```

---

## Task 5: Tag Go interfaces as ChunkTypeInterface in tree-sitter parser

**Objective:** The tree-sitter Go parser tags all type_declaration nodes as ChunkTypeType,
but interfaces should be ChunkTypeInterface for consistency with goparser.

**Files:**
- Modify: `internal/codeintel/treesitter/parser.go`
- Modify: `internal/codeintel/treesitter/parser_test.go`

**Step 1: Write failing test**

Add TestParseGo_Interface that parses:
```go
package example

type Reader interface {
    Read(p []byte) (n int, err error)
}
```
Assert the chunk has ChunkType == codeintel.ChunkTypeInterface.

**Step 2:** Run test, verify it fails (currently returns ChunkTypeType)

**Step 3: Implement**

In parseGo(), after identifying a type_declaration, check if the type spec body
contains an interface_type node. If so, set chunkType to ChunkTypeInterface.

Tree-sitter Go grammar: type_declaration → type_spec → type (field_declaration_list for struct,
interface_type for interface). Check the child node kind.

**Step 4:** Run `make test` — all pass

**Step 5:** Commit
```bash
git add -A && git commit -m "fix(treesitter): tag Go interfaces as ChunkTypeInterface"
```

---

## Task 6: Make tree-sitter parse failure return empty slice instead of error

**Objective:** When tree-sitter fails to parse a file, return an empty slice gracefully
instead of propagating an error. The caller can still index the file via fallback.

**Files:**
- Modify: `internal/codeintel/treesitter/parser.go`
- Modify: `internal/codeintel/treesitter/pyparser.go`
- Modify: `internal/codeintel/treesitter/tsparser.go`
- Modify: `internal/codeintel/treesitter/parser_test.go`

**Step 1: Write test**

Add TestParseGo_InvalidSyntax that parses garbage content with language "go".
Assert: no error returned, empty or fallback chunks returned.

**Step 2:** Run test, verify it fails (currently returns error)

**Step 3: Implement**

In parseGo, parsePython, parseTypeScript — change the nil tree check from:
```go
if tree == nil {
    return nil, fmt.Errorf("tree-sitter returned nil tree")
}
```
to:
```go
if tree == nil {
    slog.Warn("tree-sitter parse failed, returning empty", "language", lang)
    return nil, nil
}
```

**Step 4:** Run `make test` — all pass

**Step 5:** Commit
```bash
git add -A && git commit -m "fix(treesitter): graceful empty return on parse failure"
```

---

## Task 7: Add filter passthrough test to searcher

**Objective:** The searcher's fakeStore ignores the Filter parameter. Add a test that
verifies filters are passed through to the vector store.

**Files:**
- Modify: `internal/codeintel/searcher/searcher_test.go`

**Step 1: Write test TestSearch_FilterPassthrough**

Enhance the fakeStore to capture the filter argument. Run a search with
SearchOptions{Filter: &codeintel.SearchFilter{Language: "go"}}.
Assert: fakeStore received the filter with Language == "go".

**Step 2:** Run test — expected: PASS (filters ARE passed through, we just verify it)

**Step 3:** Commit
```bash
git add -A && git commit -m "test(searcher): add filter passthrough verification"
```

---

## Task 8: Add interface/generics/embedded-type tests to GoASTParser

**Objective:** The goparser test suite is missing coverage for interface extraction,
embedded types, and generics (Task 4 handles generics separately).

**Files:**
- Modify: `internal/codeintel/goparser/goparser_test.go`

**Step 1: Write TestParse_InterfaceExtraction**

Create a Go file with an interface, parse it, assert a chunk with ChunkTypeInterface is found.

**Step 2: Write TestParse_EmbeddedTypes**

Create a Go file with an embedded struct:
```go
type Base struct { ID int }
type Child struct { Base; Name string }
```
Parse it, assert both types are extracted as separate chunks.

**Step 3:** Run `make test` — all pass

**Step 4:** Commit
```bash
git add -A && git commit -m "test(goparser): add interface and embedded type test coverage"
```

---

## Task 9: Update audit report with resolutions

**Objective:** Update the audit checklist to reflect all fixes made.

**Files:**
- Modify: `audit/layer1-code-intelligence.md`

**Step 1:** Mark resolved items with [x], update DIVERGED items with notes on resolution,
update the summary table with new pass counts.

**Step 2:** Commit
```bash
git add -A && git commit -m "docs(audit): update layer1 audit with fix resolutions"
```
