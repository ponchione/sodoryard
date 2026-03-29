# Task 10: Unit Tests for Go Analyzer

**Epic:** 09 — Structural Graph
**Status:** ⬚ Not started
**Dependencies:** Task 05 (Go analyzer implementation)

---

## Description

Write unit tests for the `AnalyzeGoFile` function that transforms Go AST parser output into graph domain types. Tests verify correct symbol construction, qualified name generation, relationship mapping, and error handling for invalid input. These are pure transformation tests -- no database or file system access required.

## File Location

`internal/graph/analyzer_go_test.go`

## Test Cases

### Symbol Construction

**TestAnalyzeGoFile_FunctionSymbol:**
- Input:
  ```go
  GoAnalyzerInput{
      ProjectID:  "proj1",
      FilePath:   "internal/auth/middleware.go",
      PackageDir: "auth",
      Chunks: []GoChunkMeta{{
          Name:      "ValidateToken",
          ChunkType: "function",
          LineStart: 15,
          LineEnd:   42,
      }},
  }
  ```
- Expected output: 1 symbol with:
  - `QualifiedName = "auth.ValidateToken"`
  - `SymbolType = SymbolFunction`
  - `Language = "go"`
  - `FilePath = "internal/auth/middleware.go"`
  - `LineStart = 15`, `LineEnd = 42`

**TestAnalyzeGoFile_MethodSymbol:**
- Input: chunk with `ChunkType: "method"`, Name: `"ServeHTTP"`, PackageDir: `"server"`.
- Expected: `QualifiedName = "server.ServeHTTP"`, `SymbolType = SymbolMethod`.

**TestAnalyzeGoFile_TypeSymbol:**
- Input: chunk with `ChunkType: "type"`, Name: `"Claims"`, PackageDir: `"auth"`.
- Expected: `QualifiedName = "auth.Claims"`, `SymbolType = SymbolType_`.

**TestAnalyzeGoFile_InterfaceSymbol:**
- Input: chunk with `ChunkType: "interface"`, Name: `"Validator"`, PackageDir: `"auth"`.
- Expected: `QualifiedName = "auth.Validator"`, `SymbolType = SymbolInterface`.

### Multiple Chunks

**TestAnalyzeGoFile_MultipleChunks:**
- Input: 3 chunks in one file: function `HandleLogin`, method `ServeHTTP`, type `Server`.
- Expected: 3 symbols, each with correct qualified names and types.
- Verify all share the same `ProjectID`, `FilePath`, and `Language`.

### Call Relationship Mapping

**TestAnalyzeGoFile_CallRelationships:**
- Input:
  ```go
  GoChunkMeta{
      Name:      "HandleLogin",
      ChunkType: "function",
      Calls:     []string{"auth.ValidateToken", "db.GetUser", "errors.New"},
  }
  ```
  PackageDir: `"server"`.
- Expected: 3 `Call` entries:
  - `{CallerQName: "server.HandleLogin", CalleeQName: "auth.ValidateToken"}`
  - `{CallerQName: "server.HandleLogin", CalleeQName: "db.GetUser"}`
  - `{CallerQName: "server.HandleLogin", CalleeQName: "errors.New"}`
- All with `ProjectID = "proj1"`.

### Type Reference Mapping

**TestAnalyzeGoFile_TypeRefs:**
- Input:
  ```go
  GoChunkMeta{
      Name:      "HandleLogin",
      ChunkType: "function",
      TypesUsed: []string{"http.Request", "http.ResponseWriter", "auth.Claims"},
  }
  ```
- Expected: 3 `TypeRef` entries with `SourceQName = "server.HandleLogin"`, `RefType = RefField`.

### Interface Implementation Mapping

**TestAnalyzeGoFile_Implements:**
- Input:
  ```go
  GoChunkMeta{
      Name:             "TokenService",
      ChunkType:        "type",
      ImplementsIfaces: []string{"auth.Validator", "io.Closer"},
  }
  ```
  PackageDir: `"auth"`.
- Expected: 2 `Implements` entries:
  - `{TypeQName: "auth.TokenService", InterfaceQName: "auth.Validator"}`
  - `{TypeQName: "auth.TokenService", InterfaceQName: "io.Closer"}`

### Combined Output

**TestAnalyzeGoFile_CombinedOutput:**
- Input: file with 3 functions where `A` calls `B` and `C`, `B` calls `C`, and `C` uses type `T`.
  ```go
  GoAnalyzerInput{
      ProjectID:  "proj1",
      FilePath:   "internal/pkg/logic.go",
      PackageDir: "pkg",
      Chunks: []GoChunkMeta{
          {Name: "A", ChunkType: "function", LineStart: 1, LineEnd: 10, Calls: []string{"pkg.B", "pkg.C"}},
          {Name: "B", ChunkType: "function", LineStart: 12, LineEnd: 20, Calls: []string{"pkg.C"}},
          {Name: "C", ChunkType: "function", LineStart: 22, LineEnd: 30, TypesUsed: []string{"pkg.T"}},
          {Name: "T", ChunkType: "type", LineStart: 32, LineEnd: 35},
      },
  }
  ```
- Expected:
  - 4 symbols: `pkg.A`, `pkg.B`, `pkg.C`, `pkg.T`
  - 3 calls: `(A->B)`, `(A->C)`, `(B->C)`
  - 1 type ref: `(C->T)`
  - 0 implements

### Error Cases

**TestAnalyzeGoFile_EmptyProjectID:**
- Input: `ProjectID = ""`.
- Expected: non-nil error.

**TestAnalyzeGoFile_EmptyFilePath:**
- Input: `FilePath = ""`.
- Expected: non-nil error.

**TestAnalyzeGoFile_EmptyPackageDir:**
- Input: `PackageDir = ""`.
- Expected: non-nil error.

**TestAnalyzeGoFile_UnknownChunkType:**
- Input: chunk with `ChunkType: "unknown_type"`.
- Expected: non-nil error (contract violation with L1-E03).

**TestAnalyzeGoFile_EmptyChunkName:**
- Input: chunk with `Name: ""`.
- Expected: no error (skipped), but the output should contain no symbol for this chunk and no relationships referencing it.

### Edge Cases

**TestAnalyzeGoFile_EmptyChunks:**
- Input: valid ProjectID, FilePath, PackageDir, but `Chunks: nil`.
- Expected: no error, output has empty slices for all fields.

**TestAnalyzeGoFile_NoRelationships:**
- Input: chunk with no Calls, TypesUsed, or ImplementsIfaces.
- Expected: 1 symbol, 0 calls, 0 type refs, 0 implements.

**TestAnalyzeGoFile_EmptyCallsSlice:**
- Input: chunk with `Calls: []string{}` (explicitly empty, not nil).
- Expected: no Call entries produced.

## Acceptance Criteria

- [ ] All tests pass with `go test ./internal/graph/... -v -run TestAnalyzeGoFile`
- [ ] `TestAnalyzeGoFile_FunctionSymbol` verifies all Symbol fields for a function chunk
- [ ] `TestAnalyzeGoFile_CallRelationships` verifies 3 Call entries with correct caller/callee QNames
- [ ] `TestAnalyzeGoFile_Implements` verifies 2 Implements entries with correct type/interface QNames
- [ ] `TestAnalyzeGoFile_CombinedOutput` verifies 4 symbols, 3 calls, 1 type ref, 0 implements
- [ ] `TestAnalyzeGoFile_EmptyProjectID` returns error
- [ ] `TestAnalyzeGoFile_UnknownChunkType` returns error
- [ ] `TestAnalyzeGoFile_EmptyChunkName` skips the chunk without error
- [ ] No test requires database access or file system access (pure transformation tests)
