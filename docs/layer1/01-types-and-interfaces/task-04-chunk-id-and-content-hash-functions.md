# Task 04: Chunk ID and Content Hash Functions

**Epic:** 01 — Types and Interfaces
**Status:** ⬚ Not started
**Dependencies:** Task 01

---

## Description

Implement the two deterministic hashing functions used throughout the indexing pipeline. `ChunkID` generates a unique, deterministic identifier for a chunk based on its location and identity. `ContentHash` generates a hash of the chunk body for change detection during incremental indexing. Both use SHA-256 and return hex-encoded strings.

## Acceptance Criteria

- [ ] Function `ChunkID(filePath string, chunkType ChunkType, name string, lineStart int) string` defined in `internal/rag/hash.go`
  - Computes `sha256(filePath + string(chunkType) + name + strconv.Itoa(lineStart))`
  - Returns the lowercase hex-encoded hash string (64 characters)
  - Input concatenation uses no separator (matches topham behavior: direct concatenation)
- [ ] Function `ContentHash(body string) string` defined in `internal/rag/hash.go`
  - Computes `sha256(body)`
  - Returns the lowercase hex-encoded hash string (64 characters)
- [ ] Both functions are pure (no side effects, no I/O)
- [ ] Both functions are deterministic (same inputs always produce the same output)
- [ ] File compiles cleanly: `go build ./internal/rag/...`
