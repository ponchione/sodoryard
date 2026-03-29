# Task 09: GraphStore Interface and Blast Radius Types

**Epic:** 01 — Types and Interfaces
**Status:** ⬚ Not started
**Dependencies:** Task 01

---

## Description

Define the `GraphStore` interface and its associated result types for structural (non-semantic) code analysis. The GraphStore is separate from the vector `Store` — structural graph data lives in SQLite, not LanceDB. It provides blast radius queries: given a symbol name, it returns upstream callers, downstream callees, and interface relationships. This powers impact analysis ("what would break if I change this function") and is used by the context assembly layer to pull in structurally related code when the user is modifying specific symbols.

## Acceptance Criteria

- [ ] `BlastRadiusResult` struct defined in `internal/rag/types.go` with the following fields:
  - `Upstream   []GraphNode` — functions/types that call or reference the target (callers)
  - `Downstream []GraphNode` — functions/types the target calls or references (callees)
  - `Interfaces []GraphNode` — interfaces that the target type implements
- [ ] `GraphNode` struct defined in `internal/rag/types.go` with the following fields:
  - `Symbol    string` — qualified symbol name (e.g., `"pkg.FunctionName"`)
  - `FilePath  string` — file containing this symbol (relative to project root)
  - `Kind      string` — node type: `"function"`, `"method"`, `"type"`, `"interface"`
  - `Depth     int`    — distance from the query target (1 = direct caller/callee, 2 = two hops away)
  - `LineStart int`    — start line of the symbol in its file
  - `LineEnd   int`    — end line of the symbol in its file
- [ ] `GraphQuery` struct defined in `internal/rag/types.go` with the following fields:
  - `Symbol    string` — the target symbol to query
  - `MaxDepth  int`    — maximum traversal depth (default: 1 for one-hop expansion)
  - `MaxNodes  int`    — maximum total nodes to return across all categories (budget cap)
  - `IncludeKinds []string` — if non-empty, only return nodes whose Kind is in this list
  - `ExcludeKinds []string` — if non-empty, exclude nodes whose Kind is in this list
- [ ] `GraphStore` interface defined in `internal/rag/interfaces.go` with the following method:
  ```go
  type GraphStore interface {
      // BlastRadius returns the upstream callers, downstream callees, and
      // interface implementations for the given symbol.
      //
      // The query's MaxDepth controls how many hops to traverse (1 = direct
      // callers/callees only). MaxNodes caps the total results to prevent
      // explosion on highly-connected symbols.
      //
      // Returns an empty BlastRadiusResult (not nil slices) if the symbol
      // is not found in the graph.
      BlastRadius(ctx context.Context, query GraphQuery) (*BlastRadiusResult, error)

      // Close releases resources held by the graph store (SQLite connection, etc.).
      Close() error
  }
  ```
- [ ] `BlastRadius` returns a pointer to `BlastRadiusResult` (the result struct is large enough to warrant avoiding copies)
- [ ] File compiles cleanly: `go build ./internal/rag/...`
