# Task 11: Integration Test — End-to-End Graph Pipeline

**Epic:** 09 — Structural Graph
**Status:** ⬚ Not started
**Dependencies:** Task 03 (CRUD), Task 04 (blast radius), Task 05 (Go analyzer), Task 08 (CRUD tests), Task 09 (blast radius tests)

---

## Description

Write an integration test that exercises the full graph pipeline: analyze multiple Go files, populate the graph store, and run blast radius queries to verify correctness across file boundaries. This test validates that the analyzer, store, and query components work together as a system -- not just in isolation. The test uses synthetic Go file metadata (not actual `go/packages` parsing) to keep the test fast and self-contained.

## File Location

`internal/codeintel/graph/integration_test.go`

## Test Scenario: Multi-File Go Project

Simulate a small Go project with 3 packages and 8 symbols across 4 files:

### File 1: `internal/auth/service.go`

Package: `auth`

| Symbol | Type | Calls | TypesUsed | Implements |
|--------|------|-------|-----------|------------|
| `auth.ValidateToken` | function | `auth.parseJWT`, `errors.New` | `auth.Claims` | — |
| `auth.parseJWT` | function | — | `auth.Claims` | — |
| `auth.Claims` | type | — | — | — |

### File 2: `internal/auth/validator.go`

Package: `auth`

| Symbol | Type | Calls | TypesUsed | Implements |
|--------|------|-------|-----------|------------|
| `auth.Validator` | interface | — | — | — |
| `auth.TokenValidator` | type | — | — | `auth.Validator` |

### File 3: `internal/server/handler.go`

Package: `server`

| Symbol | Type | Calls | TypesUsed | Implements |
|--------|------|-------|-----------|------------|
| `server.HandleLogin` | function | `auth.ValidateToken`, `server.writeJSON` | `http.Request` | — |
| `server.writeJSON` | function | — | `http.ResponseWriter` | — |

### File 4: `internal/server/middleware.go`

Package: `server`

| Symbol | Type | Calls | TypesUsed | Implements |
|--------|------|-------|-----------|------------|
| `server.AuthMiddleware` | function | `auth.ValidateToken` | `http.Request` | — |

### Expected Call Graph

```
server.HandleLogin ──→ auth.ValidateToken ──→ auth.parseJWT
     │                        ↑
     ↓                        │
server.writeJSON     server.AuthMiddleware
```

## Test Steps

### Step 1: Analyze All Files

Call `AnalyzeGoFile` for each of the 4 files above. Verify each produces the expected symbols and relationships.

### Step 2: Populate the Graph

For each file's analyzer output:
1. Call `store.UpsertSymbols(ctx, output.Symbols)`
2. After all symbols are inserted (so cross-file references resolve), call `store.UpsertRelationships(ctx, output.Calls, output.TypeRefs, output.Implements)`.

**Important ordering:** All symbols must be inserted before any relationships, because relationships reference symbols by qualified name. Inserting file 3's relationships before file 1's symbols would cause the `auth.ValidateToken` callee to be silently skipped.

### Step 3: Verify Symbol Count

Query `graph_symbols` and verify 8 symbols exist (excluding external types like `http.Request`, `http.ResponseWriter`, `errors.New` which are not indexed as symbols).

### Step 4: Blast Radius Queries

**Query A: BlastRadius for `auth.ValidateToken`, depth=1**
- Expected Target: `auth.ValidateToken` (function)
- Expected Upstream: `server.HandleLogin` (caller, depth=1), `server.AuthMiddleware` (caller, depth=1)
- Expected Downstream: `auth.parseJWT` (callee, depth=1)
- Expected Interfaces: `[]` (empty)
- Rationale: ValidateToken is called by two server functions and calls one internal helper.

**Query B: BlastRadius for `auth.ValidateToken`, depth=2**
- Expected Upstream: same as Query A (no callers of HandleLogin or AuthMiddleware in the graph)
- Expected Downstream: `auth.parseJWT` (callee, depth=1). No further callees.

**Query C: BlastRadius for `server.HandleLogin`, depth=1**
- Expected Upstream: `[]` (nothing calls HandleLogin in this graph)
- Expected Downstream: `auth.ValidateToken` (callee, depth=1), `server.writeJSON` (callee, depth=1)

**Query D: BlastRadius for `server.HandleLogin`, depth=2**
- Expected Downstream: `auth.ValidateToken` (depth=1), `server.writeJSON` (depth=1), `auth.parseJWT` (depth=2)
- Rationale: HandleLogin calls ValidateToken, which calls parseJWT. At depth=2, parseJWT appears.

**Query E: BlastRadius for `auth.TokenValidator`, depth=1**
- Expected Interfaces: `auth.Validator` (implements, depth=1)
- Rationale: TokenValidator implements the Validator interface.

**Query F: BlastRadius for `auth.Validator` (interface), depth=1**
- Expected Interfaces: `auth.TokenValidator` (implemented_by, depth=1)
- Rationale: Querying an interface should show types that implement it.

**Query G: BlastRadius for `auth.parseJWT`, depth=1**
- Expected Upstream: `auth.ValidateToken` (caller, depth=1)
- Expected Downstream: `[]` (parseJWT calls nothing in the graph)

### Step 5: Re-indexing Simulation

1. Call `store.DeleteByFilePath(ctx, projectID, "internal/auth/service.go")`.
2. Verify `auth.ValidateToken`, `auth.parseJWT`, and `auth.Claims` are gone from `graph_symbols`.
3. Verify relationships involving deleted symbols are also gone (cascade).
4. Run BlastRadius for `server.HandleLogin`, depth=1.
5. Expected Downstream: only `server.writeJSON` remains (the call to `auth.ValidateToken` was deleted with the symbol).

### Step 6: Re-insert After Re-index

1. Re-analyze `internal/auth/service.go` and re-populate symbols + relationships.
2. Verify BlastRadius for `auth.ValidateToken` returns the same results as Query A.

## Acceptance Criteria

- [ ] Test creates an in-memory SQLite database with a fresh graph store
- [ ] All 4 files are analyzed via `AnalyzeGoFile` and produce expected symbol/relationship counts
- [ ] 8 project symbols are stored (external package references like `http.Request` are not stored as symbols)
- [ ] Query A: `auth.ValidateToken` depth=1 returns 2 upstream callers and 1 downstream callee
- [ ] Query D: `server.HandleLogin` depth=2 returns `auth.parseJWT` at depth=2
- [ ] Query E: `auth.TokenValidator` depth=1 shows `auth.Validator` in interfaces
- [ ] Query F: `auth.Validator` depth=1 shows `auth.TokenValidator` as implemented_by
- [ ] Re-indexing simulation: delete + re-query confirms cascade works and re-insert restores full graph
- [ ] Test completes in under 2 seconds (all in-memory, no external dependencies)
- [ ] Cross-file call relationships resolve correctly (server package calls into auth package)
- [ ] The test file includes `var _ codeintel.GraphStore = (*graph.Store)(nil)` to verify compile-time interface satisfaction
