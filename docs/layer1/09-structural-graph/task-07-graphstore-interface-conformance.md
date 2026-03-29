# Task 07: GraphStore Interface Conformance

**Epic:** 09 — Structural Graph
**Status:** ⬚ Not started
**Dependencies:** Task 03 (CRUD), Task 04 (blast radius), L1-E01 (GraphStore interface)

---

## Description

Wire the graph `Store` to satisfy the `rag.GraphStore` interface defined in L1-E01-T09. The `GraphStore` interface is declared in `internal/rag/interfaces.go` and consumed by context assembly. The graph `Store` in `internal/graph/` implements it. This task ensures the interface contract is met with a compile-time check, implements `Close`, and adds the type conversion layer from internal graph types to the public rag types.

This is a thin wiring task -- the actual logic is already implemented in Tasks 03 and 04. The work here is:
1. Add the compile-time interface assertion.
2. Implement the `Close() error` method (required by the interface) — closes the underlying SQLite connection or delegates to the DB closer.
3. Ensure `BlastRadius` method signature matches the interface exactly: `BlastRadius(ctx context.Context, query rag.GraphQuery) (*rag.BlastRadiusResult, error)`.
4. Implement type conversion from internal `graph.Symbol`/`graph.BlastRadiusEntry` results to `rag.GraphNode` entries.

## File Location

`internal/graph/store.go` (same file as Task 03, additions only)

## Compile-Time Check

```go
// Verify Store implements the GraphStore interface at compile time.
var _ rag.GraphStore = (*Store)(nil)
```

This line goes at package level in `store.go`. If the interface changes or a method signature drifts, compilation fails with a clear error.

## Required Interface Methods

The `rag.GraphStore` interface from L1-E01-T09 requires exactly two methods:

- `BlastRadius(ctx context.Context, query rag.GraphQuery) (*rag.BlastRadiusResult, error)` — implemented in Task 04 (internal logic), wired here to match the interface signature
- `Close() error` — closes the underlying SQLite connection or delegates to the DB closer

```go
// Close releases resources held by the graph store.
// Closes the underlying SQLite database connection.
func (s *Store) Close() error {
    return s.db.Close()
}
```

## Type Conversion: Internal to Public

The `rag.GraphStore` interface uses types from `internal/rag/` (`rag.GraphQuery`, `rag.BlastRadiusResult`, `rag.GraphNode`), while the internal graph logic uses types from `internal/graph/` (`graph.Symbol`, `graph.BlastRadiusEntry`). The `BlastRadius` method must convert internal results to public types.

**Field mapping from `graph.Symbol` to `rag.GraphNode`:**

| `rag.GraphNode` field | Source |
|---|---|
| `Symbol` | `graph.Symbol.QualifiedName` |
| `FilePath` | `graph.Symbol.FilePath` |
| `Kind` | `string(graph.Symbol.SymbolType)` |
| `Depth` | computed during traversal (from `graph.BlastRadiusEntry.Depth`) |
| `LineStart` | `graph.Symbol.LineStart` |
| `LineEnd` | `graph.Symbol.LineEnd` |

**Query parameter mapping from `rag.GraphQuery` to internal options:**

| `rag.GraphQuery` field | Internal usage |
|---|---|
| `Symbol` | target qualified name for lookup |
| `MaxDepth` | `BlastRadiusOptions.Depth` |
| `MaxNodes` | `BlastRadiusOptions.MaxResults` |
| `IncludeKinds` | `BlastRadiusOptions.IncludeSymbolTypes` (converted from `[]string` to `[]SymbolType`) |
| `ExcludeKinds` | `BlastRadiusOptions.ExcludeSymbolTypes` (converted from `[]string` to `[]SymbolType`) |

## Acceptance Criteria

- [ ] `var _ rag.GraphStore = (*Store)(nil)` compiles without error
- [ ] Both methods required by the `GraphStore` interface (`BlastRadius` and `Close`) are implemented on `*Store`
- [ ] `BlastRadius` signature matches the interface exactly: `BlastRadius(ctx context.Context, query rag.GraphQuery) (*rag.BlastRadiusResult, error)`
- [ ] `Close() error` is implemented — closes the underlying SQLite connection via `s.db.Close()`
- [ ] Type conversion from `graph.Symbol` to `rag.GraphNode` is implemented with the field mapping: `Symbol` <- `QualifiedName`, `FilePath` <- `FilePath`, `Kind` <- `SymbolType`, `Depth` <- computed, `LineStart` <- `LineStart`, `LineEnd` <- `LineEnd`
- [ ] `rag.GraphQuery.IncludeKinds`/`ExcludeKinds` (`[]string`) are converted to `[]SymbolType` for internal filtering
- [ ] `go build ./internal/graph/...` and `go build ./internal/rag/...` both succeed
