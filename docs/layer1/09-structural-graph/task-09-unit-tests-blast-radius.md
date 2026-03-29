# Task 09: Unit Tests for Blast Radius Query

**Epic:** 09 — Structural Graph
**Status:** ⬚ Not started
**Dependencies:** Task 04 (blast radius implementation), Task 08 (CRUD tests, for test helpers)

---

## Description

Write unit tests for the `BlastRadius` method covering depth traversal, interface relationships, filtering, budget limiting, cycle safety, and edge cases. Each test seeds a known graph topology and verifies exact expected results. These tests are the primary correctness guarantee for the blast radius feature used by context assembly.

## File Location

`internal/graph/blast_radius_test.go`

## Test Helper

```go
// seedGraph inserts symbols and relationships for a test scenario.
// Returns a map of qualified name -> symbol ID for verification.
func seedGraph(t *testing.T, store *Store, symbols []Symbol, calls []Call, typeRefs []TypeRef, implements []Implements) map[string]int64
```

## Test Scenarios

### Linear Call Chain (A -> B -> C)

**TestBlastRadius_LinearChain_Depth1:**
- Seed: `pkg.A` calls `pkg.B`, `pkg.B` calls `pkg.C`. All functions.
- Query: BlastRadius for `pkg.B`, depth=1.
- Expected Upstream: `[{pkg.A, RelCaller, depth=1}]`
- Expected Downstream: `[{pkg.C, RelCallee, depth=1}]`
- Expected Interfaces: `[]` (empty)

**TestBlastRadius_LinearChain_Depth2:**
- Seed: same as above.
- Query: BlastRadius for `pkg.B`, depth=2.
- Expected Upstream: `[{pkg.A, RelCaller, depth=1}]` (no depth-2 callers of A exist)
- Expected Downstream: `[{pkg.C, RelCallee, depth=1}]` (no depth-2 callees of C exist)

**TestBlastRadius_LinearChain_FromLeaf:**
- Query: BlastRadius for `pkg.C`, depth=1.
- Expected Upstream: `[{pkg.B, RelCaller, depth=1}]`
- Expected Downstream: `[]` (C calls nothing)

**TestBlastRadius_LinearChain_FromRoot:**
- Query: BlastRadius for `pkg.A`, depth=1.
- Expected Upstream: `[]` (nothing calls A)
- Expected Downstream: `[{pkg.B, RelCallee, depth=1}]`

### Diamond Call Graph (A -> B, A -> C, B -> D, C -> D)

**TestBlastRadius_Diamond_Depth1:**
- Query: BlastRadius for `pkg.D`, depth=1.
- Expected Upstream: `[{pkg.B, depth=1}, {pkg.C, depth=1}]` (both call D)

**TestBlastRadius_Diamond_Depth2:**
- Query: BlastRadius for `pkg.D`, depth=2.
- Expected Upstream: `[{pkg.A, depth=2}, {pkg.B, depth=1}, {pkg.C, depth=1}]` (A is 2 hops from D via both B and C; appears at depth=2)

### Cyclic Call Graph (A -> B -> C -> A)

**TestBlastRadius_Cycle_Depth1:**
- Seed: `pkg.A` calls `pkg.B`, `pkg.B` calls `pkg.C`, `pkg.C` calls `pkg.A`.
- Query: BlastRadius for `pkg.A`, depth=1.
- Expected Upstream: `[{pkg.C, RelCaller, depth=1}]` (C calls A)
- Expected Downstream: `[{pkg.B, RelCallee, depth=1}]` (A calls B)

**TestBlastRadius_Cycle_Depth3:**
- Query: BlastRadius for `pkg.A`, depth=3.
- Verify: terminates without infinite loop.
- Expected Upstream should include B (depth=2, via C->...->B->...->A path) and C (depth=1).
- Expected Downstream should include B (depth=1) and C (depth=2).
- Key assertion: the function returns without hanging.

### Interface Implementation

**TestBlastRadius_InterfaceImplementation:**
- Seed: `auth.TokenService` (type) implements `auth.Validator` (interface).
- Query: BlastRadius for `auth.TokenService`, depth=1.
- Expected Interfaces: `[{auth.Validator, RelImplements, depth=1}]`

**TestBlastRadius_InterfaceImplementedBy:**
- Query: BlastRadius for `auth.Validator` (the interface), depth=1.
- Expected Interfaces: `[{auth.TokenService, RelImplementedBy, depth=1}]`

### Type References

**TestBlastRadius_TypeReferences:**
- Seed: `auth.Handler` (function) has TypeRef to `http.Request` (type) with RefField.
- Query: BlastRadius for `auth.Handler`, depth=1.
- Expected Downstream should include `http.Request` with `RelTypeRef`.

**TestBlastRadius_TypeReferencedBy:**
- Query: BlastRadius for `http.Request`, depth=1.
- Expected Upstream should include `auth.Handler` with `RelTypeRef`.

### Unknown Target

**TestBlastRadius_UnknownSymbol:**
- Query: BlastRadius for `nonexistent.Symbol`, depth=1.
- Expected: non-nil `*rag.BlastRadiusResult` with empty (not nil) slices, and nil error.

### Default Options

**TestBlastRadius_DefaultDepth:**
- Query: BlastRadius for `pkg.B` (in A->B->C chain) with `Depth=0`.
- Expected: same results as `Depth=1` (0 defaults to 1).

### IncludeKinds Filter

**TestBlastRadius_IncludeFilter:**
- Seed: `pkg.A` (function) calls `pkg.B` (function), `pkg.B` calls `pkg.C` (method).
- Query: BlastRadius for `pkg.B`, depth=1, IncludeKinds=`["function"]`.
- Expected Upstream: `[{pkg.A}]` (function, included)
- Expected Downstream: `[]` (pkg.C is a method, excluded by filter)

### ExcludeKinds Filter

**TestBlastRadius_ExcludeFilter:**
- Same seed as above.
- Query: BlastRadius for `pkg.B`, depth=1, ExcludeKinds=`["method"]`.
- Expected Upstream: `[{pkg.A}]`
- Expected Downstream: `[]` (pkg.C excluded)

### MaxNodes Budget

**TestBlastRadius_MaxNodes:**
- Seed: `pkg.Target` called by `pkg.C1`, `pkg.C2`, `pkg.C3`, `pkg.C4`, `pkg.C5` (5 callers). `pkg.Target` calls `pkg.D1`, `pkg.D2` (2 callees).
- Query: BlastRadius for `pkg.Target`, depth=1, MaxNodes=4.
- Expected: total entries across Upstream + Downstream + Interfaces <= 4.
- Verify truncation removes entries deterministically (deepest first; at same depth, by category priority then qualified name).

### Shallowest Depth Wins

**TestBlastRadius_ShallowDepthWins:**
- Seed: `pkg.A` calls `pkg.B`, `pkg.B` calls `pkg.C`, `pkg.A` also calls `pkg.C` (direct).
- Query: BlastRadius for `pkg.A`, depth=2.
- Expected Downstream: `pkg.B` at depth=1, `pkg.C` at depth=1 (NOT depth=2, because A calls C directly).

### Empty Graph

**TestBlastRadius_EmptyGraph:**
- No symbols seeded.
- Query: BlastRadius for any name.
- Expected: non-nil `*rag.BlastRadiusResult` with empty (not nil) slices, nil error.

## Acceptance Criteria

- [ ] All tests pass with `go test ./internal/graph/... -v -run TestBlastRadius`
- [ ] `TestBlastRadius_LinearChain_Depth1` verifies exact upstream/downstream entries for B in A->B->C
- [ ] `TestBlastRadius_Diamond_Depth2` verifies A appears at depth=2 (not depth=1) when querying D
- [ ] `TestBlastRadius_Cycle_Depth3` completes without hanging (timeout safety)
- [ ] `TestBlastRadius_InterfaceImplementation` verifies Interfaces field is populated
- [ ] `TestBlastRadius_UnknownSymbol` returns non-nil result with empty slices, nil error
- [ ] `TestBlastRadius_DefaultDepth` confirms depth=0 behaves as depth=1
- [ ] `TestBlastRadius_IncludeFilter` confirms only specified symbol kinds appear in results
- [ ] `TestBlastRadius_MaxNodes` confirms budget cap is respected with deterministic truncation
- [ ] `TestBlastRadius_ShallowDepthWins` confirms symbols reachable at multiple depths appear at shallowest
- [ ] `TestBlastRadius_Cycle_Depth3` has an explicit test timeout (e.g., `t.Parallel()` with 5-second deadline) to catch infinite loops
- [ ] `MaxNodes=0` returns unlimited results (all reachable nodes within MaxDepth)
