# Task 09: Unit Tests — Declaration Extraction

**Epic:** 03 — Go AST Parser
**Status:** ⬚ Not started
**Dependencies:** Task 02, Task 07

---

## Description

Create a test Go project fixture and unit tests that verify declaration extraction produces correct `RawChunk`s for all supported Go construct types. The test fixture is a small, self-contained Go module under `internal/codeintel/goparser/testdata/` that contains representative declarations.

## Acceptance Criteria

- [ ] Test fixture directory: `internal/codeintel/goparser/testdata/testproject/` containing a valid Go module with `go.mod` and at least two packages
- [ ] Test fixture file `testdata/testproject/pkg/declarations.go` containing at minimum:
  - A top-level function: `func PublicFunc(x int) string`
  - An unexported function: `func privateFunc() error`
  - A method with value receiver: `func (s SampleStruct) ValueMethod() string`
  - A method with pointer receiver: `func (s *SampleStruct) PointerMethod(n int) error`
  - A struct type: `type SampleStruct struct { Name string; Count int }`
  - An interface type: `type SampleInterface interface { DoSomething(ctx context.Context) error }`
  - A grouped type declaration block with at least two types
- [ ] Test: `TestExtractFunctionDeclaration` — verifies `PublicFunc` produces a `RawChunk` with:
  - `ChunkType` == `codeintel.ChunkTypeFunction` ("function")
  - `Name` == `"PublicFunc"`
  - `Signature` contains `"func PublicFunc(x int) string"`
  - `Body` contains the full function source
  - `LineStart` and `LineEnd` are correct (non-zero, `LineEnd >= LineStart`)
- [ ] Test: `TestExtractMethodDeclaration` — verifies `PointerMethod` produces a `RawChunk` with:
  - `ChunkType` == `codeintel.ChunkTypeMethod` ("method")
  - `Name` == `"*SampleStruct.PointerMethod"`
  - `Signature` contains the receiver in the signature text
- [ ] Test: `TestExtractStructDeclaration` — verifies `SampleStruct` produces a `RawChunk` with:
  - `ChunkType` == `codeintel.ChunkTypeType` ("type")
  - `Name` == `"SampleStruct"`
  - `Body` contains `"Name string"` and `"Count int"`
- [ ] Test: `TestExtractInterfaceDeclaration` — verifies `SampleInterface` produces a `RawChunk` with:
  - `ChunkType` == `codeintel.ChunkTypeInterface` ("interface")
  - `Name` == `"SampleInterface"`
  - `Body` contains `"DoSomething"`
- [ ] Test: `TestExtractGroupedTypeDeclarations` — verifies grouped `type ( ... )` block produces one `RawChunk` per type inside the group
- [ ] Test: `TestExtractUnexportedFunction` — verifies unexported functions are extracted (not skipped)
- [ ] Test: `TestBodyTruncation` — creates or uses a declaration whose body exceeds 2000 characters and verifies the `Body` field is truncated to `codeintel.MaxBodyLength`
- [ ] All tests use `GoParser` constructed against the testdata project (real `go/packages.Load`, not mocks)
