# Task 02: Graph Domain Types

**Epic:** 09 — Structural Graph
**Status:** ⬚ Not started
**Dependencies:** L1-E01 (types & interfaces -- GraphStore interface definition)

---

## Description

Define the Go structs and enums for the structural graph domain: symbols, relationships, blast radius options, and blast radius results. These are INTERNAL types in `internal/codeintel/graph/` (`Symbol`, `Call`, `TypeRef`, `Implements`, `BlastRadiusOptions`, `BlastRadiusEntry`) used for SQLite storage and internal query logic. They are distinct from the PUBLIC types in `internal/codeintel/` (`GraphNode`, `GraphQuery`, `BlastRadiusResult`) defined in L1-E01-T09 which form the `codeintel.GraphStore` interface contract. The blast radius query (Task 04) converts FROM these internal types TO the E01 `codeintel.GraphNode`/`codeintel.BlastRadiusResult` types before returning results through the interface.

## File Location

`internal/codeintel/graph/types.go`

## Types

```go
package graph

// SymbolType classifies a graph symbol.
type SymbolType string

const (
    SymbolFunction  SymbolType = "function"
    SymbolMethod    SymbolType = "method"
    SymbolType_     SymbolType = "type"      // trailing underscore to avoid collision with keyword
    SymbolInterface SymbolType = "interface"
)

// RefType classifies a type reference relationship.
type RefType string

const (
    RefField     RefType = "field"
    RefParameter RefType = "parameter"
    RefReturn    RefType = "return"
    RefEmbedding RefType = "embedding"
)

// RelationshipKind describes how a symbol relates to the blast radius target.
type RelationshipKind string

const (
    RelCaller     RelationshipKind = "caller"      // upstream: calls the target
    RelCallee     RelationshipKind = "callee"       // downstream: called by the target
    RelTypeRef    RelationshipKind = "type_ref"     // references or is referenced by the target type
    RelImplements RelationshipKind = "implements"   // target type implements this interface
    RelImplementedBy RelationshipKind = "implemented_by" // this type implements the target interface
)

// Symbol represents a code symbol in the structural graph.
type Symbol struct {
    ID            int64      // 0 for new symbols (assigned by DB on insert)
    ProjectID     string
    FilePath      string
    Name          string     // unqualified: "ValidateToken"
    QualifiedName string     // qualified: "auth.ValidateToken"
    SymbolType    SymbolType
    Language      string     // "go", "python", "typescript"
    LineStart     int
    LineEnd       int
}

// Call represents a caller-callee relationship.
type Call struct {
    ProjectID     string
    CallerQName   string  // qualified name of the calling symbol
    CalleeQName   string  // qualified name of the called symbol
}

// TypeRef represents a type reference relationship.
type TypeRef struct {
    ProjectID     string
    SourceQName   string  // qualified name of the referencing symbol
    TargetQName   string  // qualified name of the referenced type
    RefType       RefType
}

// Implements represents an interface implementation relationship.
type Implements struct {
    ProjectID      string
    TypeQName      string  // qualified name of the concrete type
    InterfaceQName string  // qualified name of the interface
}

// BlastRadiusOptions configures a blast radius query.
type BlastRadiusOptions struct {
    // Depth controls how many hops to traverse. Default: 1.
    // Depth 1 returns direct callers/callees. Depth 2 includes callers-of-callers, etc.
    Depth int

    // MaxResults caps the total number of symbols returned across all categories.
    // Default: 50. A value of 0 means no limit.
    MaxResults int

    // IncludeSymbolTypes filters results to only these symbol types.
    // Empty means include all types.
    IncludeSymbolTypes []SymbolType

    // ExcludeSymbolTypes filters out these symbol types from results.
    // Empty means exclude nothing. If both Include and Exclude are set, Include takes precedence.
    ExcludeSymbolTypes []SymbolType
}

// BlastRadiusResult contains the full blast radius for a target symbol.
type BlastRadiusResult struct {
    // Target is the symbol the query was centered on.
    Target *Symbol

    // Upstream contains symbols that call or reference the target, ordered by depth.
    Upstream []BlastRadiusEntry

    // Downstream contains symbols the target calls or references, ordered by depth.
    Downstream []BlastRadiusEntry

    // Interfaces contains interfaces the target type implements (depth is always 1).
    Interfaces []BlastRadiusEntry
}

// BlastRadiusEntry is a single symbol in the blast radius result.
type BlastRadiusEntry struct {
    Symbol       Symbol
    Relationship RelationshipKind
    Depth        int  // 1 = direct, 2 = one hop away from direct, etc.
}
```

## Design Decisions

- `Call`, `TypeRef`, and `Implements` use qualified names (strings) rather than database IDs. The graph store resolves qualified names to IDs internally during upsert. This keeps the analyzer decoupled from database internals.
- `BlastRadiusOptions.Depth` defaults to 1 if zero-valued. The store implementation must treat `Depth == 0` as `Depth = 1`.
- `BlastRadiusOptions.MaxResults` defaults to 50 if zero-valued. The store implementation must treat `MaxResults == 0` as unlimited.
- The `SymbolType_` constant uses a trailing underscore to avoid collision with Go's `type` keyword. Consider an alternative naming convention if the team prefers (e.g., `SymbolTypeDecl`).

## Acceptance Criteria

- [ ] All types compile cleanly with `go build ./internal/codeintel/graph/...`
- [ ] `SymbolType` constants match the CHECK constraint values in the graph schema DDL from Task 01: `function`, `method`, `type`, `interface`
- [ ] `RefType` constants match the CHECK constraint values: `field`, `parameter`, `return`, `embedding`
- [ ] `BlastRadiusOptions` has `Depth`, `MaxResults`, `IncludeSymbolTypes`, `ExcludeSymbolTypes` fields
- [ ] `BlastRadiusResult` has `Target`, `Upstream`, `Downstream`, `Interfaces` fields
- [ ] `BlastRadiusEntry` includes `Symbol`, `Relationship` (RelationshipKind), and `Depth`
- [ ] `Call`, `TypeRef`, `Implements` structs use qualified name strings, not database IDs
- [ ] Zero-value semantics are documented in comments: `Depth=0` means 1, `MaxResults=0` means unlimited
- [ ] These types live in `internal/codeintel/graph/` and are INTERNAL to the graph package — they are distinct from the public `codeintel.GraphNode`, `codeintel.GraphQuery`, and `codeintel.BlastRadiusResult` types defined in L1-E01-T09 (`internal/codeintel/`)
- [ ] BlastRadius query results are converted FROM internal types (`graph.Symbol`, `graph.BlastRadiusEntry`) TO E01's `codeintel.GraphNode` entries before returning through the `codeintel.GraphStore` interface (see Task 07 for the field mapping)
