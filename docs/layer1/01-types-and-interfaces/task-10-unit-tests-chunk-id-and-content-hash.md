# Task 10: Unit Tests for Chunk ID and Content Hash

**Epic:** 01 — Types and Interfaces
**Status:** ⬚ Not started
**Dependencies:** Task 04

---

## Description

Write unit tests for the `ChunkID` and `ContentHash` functions defined in Task 04. These functions generate deterministic identifiers used throughout the indexing and change detection pipeline, so correctness is critical. Tests verify determinism, expected hash values, collision resistance for same-named symbols at different locations, and empty-input behavior.

## Acceptance Criteria

### ChunkID tests (in `internal/codeintel/hash_test.go`)

- [ ] **Determinism:** Calling `ChunkID("internal/auth/middleware.go", ChunkTypeFunction, "AuthMiddleware", 15)` twice returns the same string both times
- [ ] **Expected format:** The return value is exactly 64 lowercase hex characters (SHA-256 output)
- [ ] **Known value:** `ChunkID("main.go", ChunkTypeFunction, "main", 1)` produces the SHA-256 hex digest of the string `"main.gofunctionmain1"` — assert the exact expected hash value `"411fa97556c438d37bed36cad33112a3865f67223c04313ada02a2e10f3a524a"` in the test
- [ ] **Collision resistance — different line:** `ChunkID("a.go", ChunkTypeFunction, "Foo", 10)` and `ChunkID("a.go", ChunkTypeFunction, "Foo", 20)` produce different IDs (same-named function at different lines)
- [ ] **Collision resistance — different file:** `ChunkID("a.go", ChunkTypeFunction, "Foo", 10)` and `ChunkID("b.go", ChunkTypeFunction, "Foo", 10)` produce different IDs (same-named function in different files)
- [ ] **Collision resistance — different chunk type:** `ChunkID("a.go", ChunkTypeFunction, "Foo", 10)` and `ChunkID("a.go", ChunkTypeMethod, "Foo", 10)` produce different IDs (same name, different chunk type)
- [ ] **Empty name:** `ChunkID("a.go", ChunkTypeFallback, "", 1)` does not panic and returns a valid 64-character hex string (fallback chunks may have empty names)

### ContentHash tests (in `internal/codeintel/hash_test.go`)

- [ ] **Determinism:** Calling `ContentHash("func main() { fmt.Println(\"hello\") }")` twice returns the same string both times
- [ ] **Expected format:** The return value is exactly 64 lowercase hex characters
- [ ] **Known value:** `ContentHash("hello")` produces the SHA-256 hex digest of `"hello"` — assert the exact expected hash value `"2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"` in the test
- [ ] **Empty body:** `ContentHash("")` does not panic and returns the SHA-256 of the empty string: `"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"`
- [ ] **Change detection:** `ContentHash("version 1")` and `ContentHash("version 2")` produce different hashes (body change produces a different hash)

### General

- [ ] All tests pass: `go test ./internal/codeintel/...`
- [ ] Tests use table-driven test pattern where appropriate
