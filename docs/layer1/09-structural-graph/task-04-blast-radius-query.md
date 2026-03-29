# Task 04: Blast Radius Query

**Epic:** 09 — Structural Graph
**Status:** ⬚ Not started
**Dependencies:** Task 01 (schema DDL), Task 02 (domain types), Task 03 (GraphStore CRUD)

---

## Description

Implement the `BlastRadius` method on the graph store -- the primary read-path query consumed by context assembly. Given a target symbol's qualified name, it traverses the graph to find upstream callers, downstream callees, type references, and interface relationships at configurable depth. The traversal uses SQLite recursive CTEs for efficient depth-limited BFS, avoiding N+1 query patterns.

## File Location

`internal/codeintel/graph/blast_radius.go`

## Function Signature

```go
// BlastRadius finds all symbols structurally related to the target within
// the configured depth and budget. Implements codeintel.GraphStore.BlastRadius.
//
// The target symbol is query.Symbol. Traversal depth is query.MaxDepth
// (0 defaults to 1). Budget cap is query.MaxNodes (0 means unlimited).
// Filters: query.IncludeKinds / query.ExcludeKinds restrict results by
// symbol kind.
//
// Returns a non-nil *codeintel.BlastRadiusResult with empty (not nil) slices if
// the symbol is not found, and nil error.
func (s *Store) BlastRadius(ctx context.Context, query codeintel.GraphQuery) (*codeintel.BlastRadiusResult, error)
```

The method internally uses `query.Symbol` as the target qualified name, `query.MaxDepth` as depth, `query.MaxNodes` as the budget, and `query.IncludeKinds`/`query.ExcludeKinds` as symbol type filters. Internal `graph.Symbol` results are converted to `codeintel.GraphNode` entries before populating the `codeintel.BlastRadiusResult` (see Task 07 for the field mapping).

## Algorithm

### Step 1: Resolve Target Symbol

```sql
SELECT id, project_id, file_path, name, qualified_name, symbol_type, language, line_start, line_end
FROM graph_symbols
WHERE qualified_name = ?;
```

If no row is found, return a non-nil `*codeintel.BlastRadiusResult` with empty (not nil) slices and nil error. The symbol may not have been indexed yet.

If multiple rows match (should not happen with the UNIQUE constraint, but defensively), use the first.

### Step 2: Apply Default Options

- If `query.MaxDepth == 0`, set to `1`.
- If `query.MaxNodes == 0`, treat as unlimited (no budget cap).

### Step 3: Upstream Traversal (Callers)

Find symbols that call the target, transitively up to `query.MaxDepth` hops. Uses a recursive CTE:

```sql
WITH RECURSIVE upstream(symbol_id, depth) AS (
    -- Base case: direct callers of the target
    SELECT gc.caller_id, 1
    FROM graph_calls gc
    WHERE gc.callee_id = ?  -- target symbol ID

    UNION ALL

    -- Recursive case: callers of callers
    SELECT gc.caller_id, u.depth + 1
    FROM graph_calls gc
    JOIN upstream u ON gc.callee_id = u.symbol_id
    WHERE u.depth < ?  -- max depth
)
SELECT DISTINCT gs.id, gs.project_id, gs.file_path, gs.name, gs.qualified_name,
       gs.symbol_type, gs.language, gs.line_start, gs.line_end,
       MIN(u.depth) as depth
FROM upstream u
JOIN graph_symbols gs ON gs.id = u.symbol_id
GROUP BY gs.id
ORDER BY depth ASC, gs.qualified_name ASC;
```

The `MIN(u.depth)` and `GROUP BY` ensure that if a symbol is reachable at multiple depths (e.g., A calls B and A also calls C which calls B), it appears at the shallowest depth.

### Step 4: Downstream Traversal (Callees)

Find symbols that the target calls, transitively:

```sql
WITH RECURSIVE downstream(symbol_id, depth) AS (
    -- Base case: direct callees of the target
    SELECT gc.callee_id, 1
    FROM graph_calls gc
    WHERE gc.caller_id = ?  -- target symbol ID

    UNION ALL

    -- Recursive case: callees of callees
    SELECT gc.callee_id, d.depth + 1
    FROM graph_calls gc
    JOIN downstream d ON gc.caller_id = d.symbol_id
    WHERE d.depth < ?  -- max depth
)
SELECT DISTINCT gs.id, gs.project_id, gs.file_path, gs.name, gs.qualified_name,
       gs.symbol_type, gs.language, gs.line_start, gs.line_end,
       MIN(d.depth) as depth
FROM downstream d
JOIN graph_symbols gs ON gs.id = d.symbol_id
GROUP BY gs.id
ORDER BY depth ASC, gs.qualified_name ASC;
```

### Step 5: Type Reference Traversal

Find types that reference or are referenced by the target (bidirectional, depth 1 only for v0.1):

```sql
-- Types the target references
SELECT gs.id, gs.project_id, gs.file_path, gs.name, gs.qualified_name,
       gs.symbol_type, gs.language, gs.line_start, gs.line_end,
       gtr.ref_type
FROM graph_type_refs gtr
JOIN graph_symbols gs ON gs.id = gtr.target_id
WHERE gtr.source_id = ?;

-- Types that reference the target
SELECT gs.id, gs.project_id, gs.file_path, gs.name, gs.qualified_name,
       gs.symbol_type, gs.language, gs.line_start, gs.line_end,
       gtr.ref_type
FROM graph_type_refs gtr
JOIN graph_symbols gs ON gs.id = gtr.source_id
WHERE gtr.target_id = ?;
```

Type references are included in Upstream (for types referencing the target) and Downstream (for types the target references), both at depth 1, with `RelationshipKind = RelTypeRef`.

### Step 6: Interface Relationships

```sql
-- Interfaces the target type implements
SELECT gs.id, gs.project_id, gs.file_path, gs.name, gs.qualified_name,
       gs.symbol_type, gs.language, gs.line_start, gs.line_end
FROM graph_implements gi
JOIN graph_symbols gs ON gs.id = gi.interface_id
WHERE gi.type_id = ?;

-- Types that implement the target interface
SELECT gs.id, gs.project_id, gs.file_path, gs.name, gs.qualified_name,
       gs.symbol_type, gs.language, gs.line_start, gs.line_end
FROM graph_implements gi
JOIN graph_symbols gs ON gs.id = gi.type_id
WHERE gi.interface_id = ?;
```

Interface relationships go into `BlastRadiusResult.Interfaces` at depth 1.

### Step 7: Apply Filters and Budget

After collecting all results:
1. Apply `query.IncludeKinds` filter: if non-empty, remove entries whose Kind is not in the list.
2. Apply `query.ExcludeKinds` filter: if non-empty, remove entries whose Kind is in the list. (IncludeKinds takes precedence -- if both are set, only IncludeKinds is applied.)
3. Deduplicate: a symbol appearing in both upstream and downstream (cycle in call graph) should appear in both lists.
4. Apply `query.MaxNodes` budget: if the total count across Upstream + Downstream + Interfaces exceeds `MaxNodes`, truncate by removing the deepest entries first. Within the same depth, prefer Upstream over Downstream over Interfaces. Within the same category and depth, order by qualified name for determinism.

### Cycle Safety

The recursive CTEs use `UNION ALL` which can loop on cycles. The `WHERE depth < ?` clause is the cycle breaker -- it guarantees termination. Additionally, the `GROUP BY gs.id` in the outer query deduplicates symbols that appear at multiple depths. For extra safety, consider using `UNION` (which deduplicates) instead of `UNION ALL` in the CTE, but this has a performance cost. The depth limit is sufficient for correctness.

## Acceptance Criteria

- [ ] `BlastRadius` returns a non-nil `*codeintel.BlastRadiusResult` with empty (not nil) slices if the symbol is not found, and nil error
- [ ] `BlastRadius` with depth=1 returns direct callers in Upstream and direct callees in Downstream
- [ ] `BlastRadius` with depth=2 returns transitive callers/callees up to 2 hops
- [ ] `BlastRadius` with depth=0 defaults to depth=1 behavior
- [ ] Upstream entries have `Relationship = RelCaller` and correct `Depth` values
- [ ] Downstream entries have `Relationship = RelCallee` and correct `Depth` values
- [ ] Interface entries have `Relationship = RelImplements` or `RelImplementedBy` and `Depth = 1`
- [ ] Type reference results are included in Upstream/Downstream with `Relationship = RelTypeRef`
- [ ] Symbols reachable at multiple depths appear at the shallowest depth
- [ ] `IncludeKinds` filter correctly restricts results to specified types
- [ ] `ExcludeKinds` filter correctly removes specified types from results
- [ ] `MaxNodes` budget truncates deepest entries first, maintaining deterministic ordering
- [ ] Query does not loop on cyclic call graphs (A calls B, B calls A)
- [ ] Results are ordered by depth ascending, then qualified name ascending within each category
- [ ] If the SQLite query fails (e.g., database locked, corrupted), `BlastRadius` returns a wrapped error with context about which query phase failed (upstream, downstream, or interfaces)

## Work Breakdown

**Part A (~2-3h):** Upstream and downstream CTE traversal queries. Implement the recursive CTEs for caller/callee traversal with depth limiting and cycle detection.

**Part B (~2-3h):** Type reference and interface queries, filtering (IncludeKinds/ExcludeKinds), MaxNodes budget enforcement, deduplication, and internal-to-codeintel type conversion.

This task should be worked in two sessions to stay within the 4-hour budget.
