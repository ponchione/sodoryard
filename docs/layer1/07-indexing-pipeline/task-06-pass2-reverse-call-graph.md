# Task 06: Pass 2 — Reverse Call Graph Construction

**Epic:** 07 — Indexing Pipeline
**Status:** ⬚ Not started
**Dependencies:** Task 01, Task 05, L1-E01 (types)

---

## Description

Implement the second pass of the indexing pipeline: build the reverse call graph from forward call references. After Pass 1 produces chunks with `Calls` lists (populated by the Go AST parser), Pass 2 constructs lookup indexes and populates every chunk's `CalledBy` list. This creates the bidirectional call graph needed for one-hop expansion in search results and for relationship context in description generation.

## Function Signature

```go
// pass2ReverseCallGraph builds the reverse call graph in place,
// populating CalledBy on each chunk.
func (idx *Indexer) pass2ReverseCallGraph(chunks []rag.Chunk)
```

Modifies chunks in place. No error return — this is a pure in-memory computation.

## Acceptance Criteria

- [ ] **Build `pkgIndex`:** a `map[string]*rag.Chunk` that maps `"dir.FuncName"` to the chunk reference. The key format is `filepath.Dir(chunk.FilePath) + "." + chunk.Name`. For example, a function `HandleAuth` in file `internal/auth/handler.go` maps to `"internal/auth.HandleAuth"`
- [ ] **Build `suffixToDir`:** a `map[string]string` that maps the last path component (package directory name) to the full directory path. For example, `"auth"` → `"internal/auth"`. This enables resolving import paths like `"github.com/user/project/internal/auth"` to directory `"internal/auth"` by matching the suffix
- [ ] **Handle suffix collisions:** if multiple directories share the same suffix (e.g., `pkg/auth` and `internal/auth`), `suffixToDir` stores the first one encountered. Log a debug warning for collisions: `"suffix collision in package index" suffix=<suffix> existing=<dir1> new=<dir2>`
- [ ] **Resolve forward calls to targets:** for each chunk with a non-empty `Calls` list:
  1. For each call target string (e.g., `"auth.HandleAuth"`), parse it into `pkg.Name` components
  2. Look up the package prefix in `suffixToDir` to resolve to a full directory path
  3. Construct the full key (`dir.FuncName`) and look up in `pkgIndex`
  4. If found: append the caller chunk's `dir.Name` identifier to the target chunk's `CalledBy` list
  5. If not found: skip silently (the target may be in an external package not indexed)
- [ ] **Deduplication:** `CalledBy` entries are deduplicated per chunk — no duplicate caller entries
- [ ] After this pass, every chunk has both `Calls` (from Pass 1) and `CalledBy` (from Pass 2) fully populated
- [ ] Emits progress: `"pass 2 complete" total_chunks=<N> call_edges_resolved=<N> unresolved_calls=<N>`
- [ ] Non-Go chunks (which have empty `Calls` lists) pass through unchanged
