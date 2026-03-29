# Task 13: Unit Tests — Reverse Call Graph Construction

**Epic:** 07 — Indexing Pipeline
**Status:** ⬚ Not started
**Dependencies:** Task 06

---

## Description

Write unit tests for the reverse call graph construction in Pass 2. Tests create chunks with known `Calls` lists and verify that `CalledBy` is correctly populated after the pass. These are pure in-memory tests — no parsers, no I/O.

## Acceptance Criteria

### Basic Resolution

- [ ] Test: chunk A calls chunk B. After Pass 2, B's `CalledBy` contains A's identifier. Setup:
  - Chunk A: `FilePath="internal/auth/handler.go"`, `Name="HandleLogin"`, `Calls=["auth.ValidateToken"]`
  - Chunk B: `FilePath="internal/auth/token.go"`, `Name="ValidateToken"`, `Calls=[]`
  - Assert: B's `CalledBy` contains `"internal/auth.HandleLogin"`

### Multi-caller

- [ ] Test: chunk B is called by both chunk A and chunk C. After Pass 2, B's `CalledBy` contains both callers
  - Chunk A: calls `["util.Format"]`
  - Chunk C: calls `["util.Format"]`
  - Chunk B (Format): `CalledBy` = `["pkg1.A", "pkg2.C"]` (both present, order irrelevant)

### Cross-package Resolution

- [ ] Test: caller in `internal/api/` calls function in `internal/auth/`. The suffix matching resolves `"auth.ValidateToken"` to `"internal/auth.ValidateToken"` via `suffixToDir`

### Unresolved Calls

- [ ] Test: chunk A calls a function in an external package not in the chunk list. The call is silently skipped, A's `Calls` is unchanged, no chunk's `CalledBy` is affected

### Deduplication

- [ ] Test: chunk A calls chunk B twice in its `Calls` list (e.g., two call sites). B's `CalledBy` contains A only once

### Non-Go Chunks

- [ ] Test: TypeScript and Python chunks (empty `Calls`) pass through with empty `CalledBy`. No errors, no modifications

### Empty Input

- [ ] Test: empty chunk list — Pass 2 completes without error

### Suffix Collision

- [ ] Test: two directories share the same suffix (e.g., `pkg/auth` and `internal/auth`). First one wins in `suffixToDir`. Verify that calls to the ambiguous suffix resolve to the first directory's chunks
