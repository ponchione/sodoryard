# Task 01: ContextNeeds and Signal Structs

**Epic:** 01 — Context Assembly Types & Interfaces
**Status:** ⬚ Not started
**Dependencies:** Layer 0: Epic 01 (scaffolding)

---

## Description

Define the `ContextNeeds` and `Signal` structs in `internal/context/`. `ContextNeeds` captures everything the turn analyzer determines about what context should be retrieved for a given turn — explicit file references, symbol references, flags for conventions and git context, momentum state, and a trace of all signals detected. `Signal` records individual extraction decisions for observability and debugging.

## Acceptance Criteria

- [ ] `ContextNeeds` struct defined with all fields: `SemanticQueries []string`, `ExplicitFiles []string`, `ExplicitSymbols []string`, `IncludeConventions bool`, `IncludeGitContext bool`, `GitContextDepth int`, `MomentumFiles []string`, `MomentumModule string`, `Signals []Signal`
- [ ] `Signal` struct defined with fields: `Type string`, `Source string`, `Value string`
- [ ] Both structs have GoDoc comments explaining their role in the context assembly pipeline
- [ ] `Signal.Type` values documented in the GoDoc: `"file_ref"`, `"symbol_ref"`, `"modification_intent"`, `"creation_intent"`, `"git_context"`, `"continuation"`
- [ ] Package compiles with no errors: `go build ./internal/context/...`
