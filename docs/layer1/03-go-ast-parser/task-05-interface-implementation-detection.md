# Task 05: Interface Implementation Detection

**Epic:** 03 — Go AST Parser
**Status:** ⬚ Not started
**Dependencies:** Task 01

---

## Description

Implement interface implementation detection using `go/types`. After packages are loaded, iterate all concrete types and all interface types to determine which interfaces each concrete type satisfies. The results are cached on the `GoParser` struct for efficient lookup during per-file parsing. This metadata powers the structural graph — when an interface changes, the graph knows which concrete types are affected.

## Acceptance Criteria

- [ ] Method on `GoParser`: `buildIfaceMap()` — called during or after construction, populates the cached interface implementation mapping
- [ ] Iterates all loaded packages, collecting:
  - All named interface types (where the underlying type is `*types.Interface`)
  - All named concrete types (structs, named non-interface types)
- [ ] For each concrete type `T`, checks `types.Implements(T, iface)` and `types.Implements(*T, iface)` (pointer receiver methods count) against every discovered interface
- [ ] The empty interface (`interface{}` / `any`) is excluded from results — every type implements it, so it carries no useful information
- [ ] Single-method standard library interfaces (`error`, `fmt.Stringer`, `io.Reader`, `io.Writer`, `io.Closer`, `sort.Interface`) are included if satisfied — these are structurally meaningful
- [ ] Results stored as a `map[string][]string` keyed by concrete type qualified name (e.g., `"goparser.GoParser"`), value is a sorted list of interface qualified names (e.g., `["rag.Parser", "io.Closer"]`)
- [ ] Lookup method: `getImplementedIfaces(typeName string) []string` — returns the cached list for a given type, or nil if the type implements no interfaces
- [ ] The computation is done once and cached. Subsequent calls to `getImplementedIfaces` do not recompute.
- [ ] If the interface map computation encounters errors (e.g., incomplete type info for a package), it logs the error and continues with partial results rather than failing entirely
