# Task 02: Registry Implementation

**Epic:** 01 — Tool Interface, Registry & Executor
**Status:** ⬚ Not started
**Dependencies:** Task 01

---

## Description

Implement the `Registry` in `internal/tool/` that holds all registered tools and provides lookup, enumeration, and schema collection. Tools register themselves at startup (typically from a `RegisterAll` function). The registry is the single source of truth for what tools are available and is consumed by the executor for dispatch and by Layer 5 for injecting tool schemas into LLM requests.

## Acceptance Criteria

- [ ] `Registry` struct with a `Register(tool Tool)` method that adds a tool by its `Name()`
- [ ] Duplicate registration (same tool name registered twice) panics with a descriptive message — this catches wiring bugs at startup, not at runtime
- [ ] `Get(name string) (Tool, bool)` returns the tool with the given name, or `(nil, false)` if not found
- [ ] `All() []Tool` returns all registered tools in a stable order (sorted by name or insertion order)
- [ ] `Schemas() []json.RawMessage` returns the JSON Schema definitions from all registered tools, suitable for injection into LLM API requests
- [ ] Registry is safe for concurrent reads after initialization (tools are registered at startup, then read-only during execution — no mutex needed if registration is single-threaded, but document this invariant)
- [ ] Constructor function `NewRegistry() *Registry`
- [ ] Compiles cleanly: `go build ./internal/tool/...`
