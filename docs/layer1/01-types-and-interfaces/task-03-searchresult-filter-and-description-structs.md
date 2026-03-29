# Task 03: SearchResult, Filter, and Description Structs

**Epic:** 01 — Types and Interfaces
**Status:** ⬚ Not started
**Dependencies:** Task 02

---

## Description

Define the query-side and description-side data structures. `SearchResult` wraps a `Chunk` with retrieval metadata (similarity score, match info). `Filter` specifies metadata constraints for vector search queries. `Description` is the output format of the Describer — a name-description pair returned by the local LLM for each function/type in a file.

## Acceptance Criteria

- [ ] `SearchResult` struct defined in `internal/codeintel/types.go` with the following fields:
  - `Chunk      Chunk`   — the matched chunk (value, not pointer — avoid nil reference issues in result slices)
  - `Score      float64` — cosine similarity score (0.0 to 1.0)
  - `MatchedBy  string`  — which query produced this match (for multi-query deduplication diagnostics)
  - `HitCount   int`     — number of distinct queries that matched this chunk (higher = more relevant in multi-query re-ranking)
  - `FromHop    bool`    — true if this result came from one-hop call graph expansion rather than direct vector search
- [ ] `Filter` struct defined in `internal/codeintel/types.go` with the following fields:
  - `Language       string` — restrict results to a specific language (empty string means no filter)
  - `ChunkType      ChunkType` — restrict results to a specific chunk type (empty string means no filter)
  - `FilePathPrefix string` — restrict results to files under a directory prefix (empty string means no filter)
- [ ] `Description` struct defined in `internal/codeintel/types.go` with the following fields:
  - `Name        string` — symbol name that this description corresponds to
  - `Description string` — 1-2 sentence semantic summary of what the function/type does
- [ ] File compiles cleanly: `go build ./internal/codeintel/...`
