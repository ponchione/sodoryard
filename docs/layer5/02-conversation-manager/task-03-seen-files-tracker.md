# Task 03: SeenFiles Tracker

**Epic:** 02 — Conversation Manager
**Status:** ⬚ Not started
**Dependencies:** L5-E01 (types)

---

## Description

Implement the `SeenFiles` tracker in `internal/conversation/seen.go`. This is a session-scoped, in-memory set that tracks which files have appeared in tool results during the current session. Context assembly uses it to annotate chunks from previously-viewed files with `[previously viewed in turn N]`, which tells the LLM it has already seen this file and reduces redundant tool calls. The tracker is not persisted to SQLite — it resets each time a new session starts.

## Acceptance Criteria

- [ ] `SeenFiles` struct with an internal `map[string]int` mapping file path to the turn number when it was first seen
- [ ] `NewSeenFiles() *SeenFiles` constructor initializing the empty map
- [ ] `Add(path string, turnNumber int)` — records a file path and the turn in which it was seen. If the path is already tracked, does not overwrite the original turn number (first-seen semantics)
- [ ] `Contains(path string) (bool, int)` — returns whether the path has been seen and the turn number. Returns `(false, 0)` if not seen
- [ ] `Paths() []string` — returns all tracked file paths (for diagnostics/logging)
- [ ] `Count() int` — returns the number of unique files tracked
- [ ] Thread-safe: `Add`, `Contains`, `Paths`, and `Count` can be called concurrently. Uses `sync.RWMutex` — reads use `RLock`, writes use `Lock`
- [ ] File paths are normalized before storage (cleaned via `filepath.Clean` to avoid duplicates from `./foo` vs `foo` vs `foo/`)
- [ ] Package compiles with `go build ./internal/conversation/...`
