# Task 05: Go Analyzer

**Epic:** 09 — Structural Graph
**Status:** ⬚ Not started
**Dependencies:** Task 02 (domain types), L1-E03 (Go AST parser output)

---

## Description

Implement the Go analyzer that transforms the Go AST parser's relationship metadata into graph symbols and relationships for storage. The Go analyzer does NOT re-run `go/packages` analysis -- it consumes the output of the Go AST parser from L1-E03, which already extracts call graph data, type usage, and interface implementations. The analyzer's job is to transform that parser output into the graph domain types (`Symbol`, `Call`, `TypeRef`, `Implements`) for the graph store to persist.

## File Location

`internal/codeintel/graph/analyzer_go.go`

## Input: Go AST Parser Output

The Go AST parser (L1-E03) produces `Chunk` objects (as defined in L1-E01-T02) with relationship metadata. The specific fields consumed by the graph analyzer from the `Chunk` struct's Relationships fields:

From each `Chunk`:
- `Name`: unqualified symbol name (e.g., `ValidateToken`)
- `ChunkType`: one of the `ChunkType` enum values (`Function`, `Method`, `Type`, `Interface`)
- `LineStart`, `LineEnd`: source line range
- `Calls []string`: qualified names of functions/methods this chunk calls (e.g., `"auth.HashPassword"`)
- `CalledBy []string`: qualified names of functions that call this chunk (populated in Pass 2 of indexing; not used by the graph analyzer directly)
- `TypesUsed []string`: qualified names of types referenced in this chunk
- `ImplementsIfaces []string`: qualified names of interfaces this chunk's receiver type satisfies

The analyzer also needs the file path, language (`"go"`), and a way to construct qualified names. Qualified name format: `"packageDir.SymbolName"` where `packageDir` is the Go package's directory name (e.g., `"auth.ValidateToken"`, `"server.HandleLogin"`).

## Interface

```go
package graph

// GoAnalyzerInput represents the parsed output from L1-E03 for a single file.
type GoAnalyzerInput struct {
    ProjectID  string
    FilePath   string     // relative to project root, e.g., "internal/auth/middleware.go"
    PackageDir string     // Go package directory, e.g., "auth"
    Chunks     []GoChunkMeta
}

// GoChunkMeta is the relationship metadata from a parsed Go chunk.
type GoChunkMeta struct {
    Name             string     // "ValidateToken"
    ChunkType        string     // "function", "method", "type", "interface"
    LineStart        int
    LineEnd          int
    Calls            []string   // qualified names: ["auth.HashPassword", "errors.New"]
    TypesUsed        []string   // qualified names: ["auth.Claims", "http.Request"]
    ImplementsIfaces []string   // qualified names: ["io.Reader", "auth.Validator"]
}

// GoAnalyzerOutput contains the graph data extracted from a single file.
type GoAnalyzerOutput struct {
    Symbols    []Symbol
    Calls      []Call
    TypeRefs   []TypeRef
    Implements []Implements
}

// AnalyzeGoFile transforms Go AST parser output into graph domain objects.
func AnalyzeGoFile(input GoAnalyzerInput) (*GoAnalyzerOutput, error)
```

## Transformation Logic

### Symbol Construction

For each `GoChunkMeta` in the input:

1. Map `ChunkType` string to `SymbolType`:
   - `"function"` -> `SymbolFunction`
   - `"method"` -> `SymbolMethod`
   - `"type"` -> `SymbolType_`
   - `"interface"` -> `SymbolInterface`
   - Unknown -> return error

2. Construct qualified name: `input.PackageDir + "." + chunk.Name`
   - Example: PackageDir=`"auth"`, Name=`"ValidateToken"` -> `"auth.ValidateToken"`

3. Build `Symbol`:
   ```go
   Symbol{
       ProjectID:     input.ProjectID,
       FilePath:      input.FilePath,
       Name:          chunk.Name,
       QualifiedName: input.PackageDir + "." + chunk.Name,
       SymbolType:    mappedType,
       Language:      "go",
       LineStart:     chunk.LineStart,
       LineEnd:       chunk.LineEnd,
   }
   ```

### Call Relationship Construction

For each `GoChunkMeta` with non-empty `Calls`:

For each callee qualified name in `chunk.Calls`:
```go
Call{
    ProjectID:   input.ProjectID,
    CallerQName: qualifiedName, // the caller's qualified name (constructed above)
    CalleeQName: calleeName,    // from chunk.Calls, already qualified
}
```

### Type Reference Construction

For each `GoChunkMeta` with non-empty `TypesUsed`:

For each type qualified name in `chunk.TypesUsed`:
```go
TypeRef{
    ProjectID:   input.ProjectID,
    SourceQName: qualifiedName, // the referencing symbol
    TargetQName: typeName,      // from chunk.TypesUsed, already qualified
    RefType:     RefField,      // default; the Go AST parser does not currently distinguish ref types
}
```

**Note on RefType:** The Go AST parser from L1-E03 produces a flat `TypesUsed` list without distinguishing field vs. parameter vs. return vs. embedding usage. Default all to `RefField` for v0.1. If L1-E03 is later enhanced to classify type usage, update this mapping.

### Interface Implementation Construction

For each `GoChunkMeta` with non-empty `ImplementsIfaces`:

For each interface qualified name in `chunk.ImplementsIfaces`:
```go
Implements{
    ProjectID:      input.ProjectID,
    TypeQName:      qualifiedName, // the concrete type
    InterfaceQName: ifaceName,     // from chunk.ImplementsIfaces, already qualified
}
```

### Validation

- Return an error if `input.ProjectID` is empty.
- Return an error if `input.FilePath` is empty.
- Return an error if `input.PackageDir` is empty.
- Skip chunks with empty `Name` (log a warning, do not error).
- Skip chunks with unknown `ChunkType` (return an error -- this indicates a contract mismatch with L1-E03).

## Acceptance Criteria

- [ ] `AnalyzeGoFile` produces correct `Symbol` for each chunk type: function, method, type, interface
- [ ] Qualified names follow the `"packageDir.Name"` format
- [ ] `AnalyzeGoFile` produces `Call` entries for each item in `chunk.Calls`
- [ ] `AnalyzeGoFile` produces `TypeRef` entries for each item in `chunk.TypesUsed` with `RefType = RefField`
- [ ] `AnalyzeGoFile` produces `Implements` entries for each item in `chunk.ImplementsIfaces`
- [ ] Returns error for empty ProjectID, FilePath, or PackageDir
- [ ] Returns error for unknown ChunkType (contract violation)
- [ ] Skips chunks with empty Name without error
- [ ] Output symbols, calls, type refs, and implements all have the correct ProjectID
- [ ] A file with 3 functions where A calls B and C, B calls C, produces 3 symbols and 3 Call entries: (A->B), (A->C), (B->C)
- [ ] A file with type `Server` implementing `http.Handler` produces 1 Symbol + 1 Implements entry

### Python and TypeScript Analyzer Stubs (v0.1)

- [ ] `AnalyzePythonFile` stub defined in `internal/codeintel/graph/analyzer_python.go` — compiles and returns `(&GoAnalyzerOutput{}, nil)` (empty symbols, calls, type refs, implements)
- [ ] `AnalyzeTypeScriptFile` stub defined in `internal/codeintel/graph/analyzer_typescript.go` — compiles and returns `(&GoAnalyzerOutput{}, nil)` (empty symbols, calls, type refs, implements)
- [ ] Both stubs accept their respective input structs (`PythonAnalyzerInput`, `TypeScriptAnalyzerInput`) with `ProjectID` and `FilePath` fields
- [ ] Both files contain a doc comment explaining that the analyzer is a stub for v0.1
- [ ] TypeScript stub file contains a TODO comment about the external script decision (port to Go vs. keep Node.js `ts-analyzer/analyze.ts`)
- [ ] Both stubs compile cleanly with `go build ./internal/codeintel/graph/...`
