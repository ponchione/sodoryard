# Task 10: Unit Tests — Relationship Extraction

**Epic:** 03 — Go AST Parser
**Status:** ⬚ Not started
**Dependencies:** Task 03, Task 04, Task 05, Task 07, Task 09 (reuses test fixture)

---

## Description

Unit tests that verify all four relationship extraction features: call graph, type usage, interface implementation detection, and import tracking. These tests extend the test fixture from Task 09 with additional files that exercise cross-package calls and interface satisfaction.

## Acceptance Criteria

- [ ] Test fixture extended with `testdata/testproject/pkg/caller.go` containing:
  - A function that calls `PublicFunc` from the same package
  - A function that calls `fmt.Sprintf` (cross-package, stdlib)
  - A function that calls a method on `SampleStruct` (method call)
  - A function that contains a type conversion (e.g., `int64(x)`) that should NOT appear in calls
  - A function that calls `len()` and `make()` (builtins that should NOT appear in calls)
- [ ] Test fixture extended with `testdata/testproject/pkg/types.go` containing:
  - A struct with fields referencing `SampleStruct`, `time.Duration`, `*SampleInterface`
  - A struct with an embedded type
  - A struct with a map field: `map[string]SampleStruct`
- [ ] Test fixture includes a concrete type that satisfies `SampleInterface` (has the `DoSomething(ctx context.Context) error` method)
- [ ] **Call graph tests:**
  - `TestCallGraphSamePackage` — function calling `PublicFunc` has `"PublicFunc"` in its `Calls` list
  - `TestCallGraphCrossPackage` — function calling `fmt.Sprintf` has `"fmt.Sprintf"` in its `Calls` list
  - `TestCallGraphMethodCall` — function calling `s.ValueMethod()` has `"SampleStruct.ValueMethod"` in its `Calls` list
  - `TestCallGraphExcludesTypeConversions` — `int64(x)` does NOT appear in calls
  - `TestCallGraphExcludesBuiltins` — `len`, `make` do NOT appear in calls
  - `TestCallGraphDeduplicated` — a function calling `fmt.Println` twice lists it only once
  - `TestCallGraphSorted` — the calls list is in alphabetical order
- [ ] **Type usage tests:**
  - `TestTypeUsageStructFields` — struct referencing `SampleStruct` and `time.Duration` has both in `TypesUsed`
  - `TestTypeUsageExcludesBuiltins` — `string`, `int`, `bool` do NOT appear in `TypesUsed`
  - `TestTypeUsageEmbeddedType` — embedded type appears in `TypesUsed`
  - `TestTypeUsageMapField` — `map[string]SampleStruct` records `"SampleStruct"`, does not record `"string"`
- [ ] **Interface implementation tests:**
  - `TestIfaceDetection` — the concrete type satisfying `SampleInterface` has `"pkg.SampleInterface"` (or equivalent qualified name) in its `ImplementsIfaces` list
  - `TestIfaceDetectionExcludesEmptyInterface` — no type lists `"interface{}"` or `"any"` in its implements list
- [ ] **Import tracking tests:**
  - `TestImportTracking` — a file importing `"fmt"`, `"context"`, and `"time"` returns all three paths, sorted
  - `TestImportTrackingNoImports` — a file with no imports returns an empty non-nil slice
  - `TestImportTrackingNamedImport` — `import myfmt "fmt"` records `"fmt"` (the path, not the alias)
