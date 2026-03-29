# L1-E09 — Structural Graph

**Layer:** 1 — Code Intelligence
**Epic:** 09
**Status:** ⬜ Not Started
**Dependencies:** L1-E03 (Go AST parser), L0-E04 (sqlite), L0-E06 (schema/sqlc)

---

## Description

Implement the structural graph system — separate from and complementary to RAG. Where RAG answers "what code is _about_ this concept," the structural graph answers "what code is _connected to_ this symbol." It stores call relationships, type references, and interface implementations in SQLite and supports blast radius queries: given a target symbol, find upstream callers, downstream callees, and implemented interfaces at configurable depth.

The structural graph has its own analyzers (Go, Python, TypeScript) and its own SQLite tables distinct from the RAG pipeline's LanceDB store. It's consumed by [[06-context-assembly]] for impact analysis when the user is modifying specific symbols. Ports from topham's `internal/graph/`.

---

## Package

`internal/codeintel/graph/` — structural graph store, analyzers, and blast radius queries.

---

## Definition of Done

### Schema

- [ ] Structural graph SQLite tables defined and created (these are NOT in the main `schema.sql` from L0-E06 — the graph has its own schema, co-located in the same SQLite database but managed separately):
  - `graph_symbols`: id, project_id, file_path, name, qualified_name, symbol_type (function/method/type/interface), language, line_start, line_end
  - `graph_calls`: id, project_id, caller_id (FK → graph_symbols), callee_id (FK → graph_symbols)
  - `graph_type_refs`: id, project_id, source_id (FK → graph_symbols), target_id (FK → graph_symbols), ref_type (field, parameter, return, embedding)
  - `graph_implements`: id, project_id, type_id (FK → graph_symbols), interface_id (FK → graph_symbols)
- [ ] Indexes on foreign keys and qualified_name for efficient traversal
- [ ] Schema versioning: version constant checked on open, full rebuild if mismatch

### Graph Store

- [ ] Implements the `codeintel.GraphStore` interface from L1-E01-T09 (`BlastRadius` and `Close` methods)
- [ ] `UpsertSymbols(ctx, symbols []Symbol) error` — insert or update symbols for a file
- [ ] `UpsertRelationships(ctx, calls []Call, typeRefs []TypeRef, implements []Implements) error` — insert relationships between symbols
- [ ] `DeleteByFilePath(ctx, projectID, filePath string) error` — remove all symbols and relationships for a file (used during re-indexing)
- [ ] `BlastRadius(ctx context.Context, query codeintel.GraphQuery) (*codeintel.BlastRadiusResult, error)` — the primary query (see below), matching the `codeintel.GraphStore` interface from L1-E01-T09

### Blast Radius Query

- [ ] Given a target symbol (qualified name), returns:
  - **Upstream:** functions/types that call or reference the target (callers, at configurable depth)
  - **Downstream:** functions/types the target calls or references (callees, at configurable depth)
  - **Interfaces:** interfaces the target type implements
- [ ] Query parameters come from `codeintel.GraphQuery` fields: `MaxDepth` (default 1), `MaxNodes` (budget cap), `IncludeKinds`/`ExcludeKinds` (symbol type filters)
- [ ] Results include: symbol name, qualified name, file path, line range, relationship type (caller/callee/implements), depth from target
- [ ] Efficient traversal: uses recursive CTEs or iterative BFS in SQLite, not N+1 queries

### Go Analyzer

- [ ] Consumes the relationship metadata from [[L1-E03-go-ast-parser]] (Calls, TypesUsed, ImplementsIfaces)
- [ ] Transforms parser output into graph symbols and relationships for storage
- [ ] Handles qualified names: `"package.FuncName"` format for cross-package resolution

### Python Analyzer (deferred to v0.2 — stubs returning empty results for v0.1)

- [ ] AST-based analysis using tree-sitter Python parser output
- [ ] Extracts: imports, function/method calls, class hierarchies (inheritance)
- [ ] Transforms into graph symbols and relationships

### TypeScript Analyzer (deferred to v0.2 — stubs returning empty results for v0.1)

- [ ] Extracts: imports, function calls, type references from tree-sitter TypeScript parser output
- [ ] Handles: ES6 imports, class inheritance, interface implementations
- [ ] Note: topham uses an external Node.js analyzer script (`ts-analyzer/analyze.ts`). Evaluate whether to port this to Go or keep the external script approach. If external script, document the dependency

### Testing

- [ ] Unit tests for blast radius query: seed graph with known call chain (A → B → C), verify depth-1 query from B returns A (upstream) and C (downstream)
- [ ] Unit tests for interface implementation: seed graph with type T implementing interface I, verify blast radius from T includes I
- [ ] Unit tests for Go analyzer: parse a test Go file with cross-function calls, verify correct symbols and relationships are produced
- [ ] Unit tests for depth limiting: verify depth=1 doesn't return depth-2 results
- [ ] Unit tests for budget limiting: verify max results cap is respected
- [ ] Integration test: analyze a multi-file Go project, populate graph, run blast radius queries, verify completeness

---

## Architecture References

- [[04-code-intelligence-and-rag]] — "Architecture: Two Complementary Systems" → "Structural Graph (Blast Radius)", "Component: Structural Graph" (analyzers, blast radius query, use in sirtopham)
- [[06-context-assembly]] — "Component: Retrieval Execution" → "Structural graph: Blast radius for identified symbols"
- [[06-context-assembly]] — "Component: Turn Analyzer" → "Modification intent" signals trigger structural graph lookups
- topham source: `internal/graph/`

---

## Notes

- The structural graph tables live in the same SQLite database as everything else (conversations, messages, index_state) but are managed by the graph package, not the main schema. The graph package creates its own tables on initialization if they don't exist. This keeps the graph self-contained while sharing the database connection from L0-E04.
- The Go analyzer has significant overlap with the Go AST parser ([[L1-E03-go-ast-parser]]) — both use `go/packages` and extract call relationships. The graph analyzer should consume the Go AST parser's output rather than re-running `go/packages` analysis independently. The parser produces relationship metadata; the graph analyzer transforms it into the graph store's format.
- The Python and TypeScript analyzers are lower priority than the Go analyzer. Go is the primary language for sirtopham's target use case. Python and TypeScript analyzers can be stubbed (return empty results) for v0.1 if time is constrained — the blast radius feature still works for Go codebases.
- Recursive CTEs in SQLite are well-supported and efficient for graph traversal. A depth-limited BFS query: `WITH RECURSIVE reachable(id, depth) AS (SELECT callee_id, 1 FROM graph_calls WHERE caller_id = ? UNION ALL SELECT gc.callee_id, r.depth+1 FROM graph_calls gc JOIN reachable r ON gc.caller_id = r.id WHERE r.depth < ?) SELECT ...`
- The structural graph is populated during indexing ([[L1-E07-indexing-pipeline]]) or as a separate analysis pass. The indexing pipeline epic doesn't explicitly include graph population — this epic should define whether the graph is populated as part of the indexing pipeline or as a separate command. Architecturally, it makes sense to populate the graph after Pass 1 (parsing) since the graph uses the same relationship data as Pass 2 (reverse call graph).
