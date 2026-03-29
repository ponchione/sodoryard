# Task 11: Compilation and Package Verification

**Epic:** 01 — Types and Interfaces
**Status:** ⬚ Not started
**Dependencies:** Task 01, Task 02, Task 03, Task 04, Task 05, Task 06, Task 07, Task 08, Task 09, Task 10

---

## Description

Final verification that all types, interfaces, constants, and functions defined in Tasks 01-10 compile cleanly as a cohesive package, that tests pass, and that the package structure is correct. This task catches any cross-task issues — missing imports, circular references, naming conflicts, or forgotten `time` import for `Chunk.IndexedAt`.

## Acceptance Criteria

- [ ] `go build ./internal/codeintel/...` succeeds with zero errors and zero warnings
- [ ] `go vet ./internal/codeintel/...` reports no issues
- [ ] `go test ./internal/codeintel/...` passes all tests (from Task 10)
- [ ] The package contains exactly two source files: `types.go` (all types, structs, constants) and `interfaces.go` (all interfaces), plus `hash.go` (ID/hash functions) and `hash_test.go` (tests)
- [ ] No circular imports — `internal/codeintel/` depends only on standard library packages (`context`, `crypto/sha256`, `encoding/hex`, `strconv`, `time`)
- [ ] Every exported type, interface, function, and constant has a doc comment
