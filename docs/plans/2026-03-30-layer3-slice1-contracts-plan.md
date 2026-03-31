# Layer 3 Slice 1: Contracts and Boundaries Plan

> For Hermes: implement only this slice, commit locally, then stop.

Goal: create the foundational `internal/context` contracts so every later Layer 3 slice can compile against stable types and narrow interfaces.

Architecture:
- Keep everything in `internal/context`.
- Reuse existing repo types where they already exist:
  - `internal/db.Message` for persisted history input
  - `internal/config.ContextConfig` for config shape
  - `internal/codeintel` types as upstream retrieval sources, but define Layer 3-facing result structs in `internal/context`
- Do not import any future Layer 5 runtime/session package.
- Define only the minimal Layer 3 boundary needed right now: `SeenFileLookup` and `AssemblyScope`.

Tech stack: Go, existing `internal/db`, `internal/config`, `internal/codeintel`

---

## Scope of this single problem

Deliver only these artifacts:
- `ContextNeeds`
- `Signal`
- `RAGHit`
- `BrainHit`
- `GraphHit`
- `FileResult`
- `RetrievalResults`
- `BudgetResult`
- `FullContextPackage`
- `ContextAssemblyReport`
- `SeenFileLookup`
- `AssemblyScope`
- narrow interfaces for future components (`TurnAnalyzer`, `QueryExtractor`, `MomentumTracker`, `ConventionSource`, `Retriever`, `BudgetManager`, `Serializer`)

Do not implement any real retrieval, analysis, serialization, DB writes, or compression logic in this slice.

---

## Files

Create:
- `internal/context/types.go`
- `internal/context/interfaces.go`
- `internal/context/scope.go`
- `internal/context/types_test.go`

Modify:
- `internal/context/doc.go`

---

## Task 1: Define the shared data structs

Objective: establish the Layer 3 data model.

Add to `internal/context/types.go`:
- `Signal`
- `ContextNeeds`
- `RAGHit`
- `BrainHit`
- `GraphHit`
- `FileResult`
- `RetrievalResults`
- `BudgetResult`
- `ContextAssemblyReport`
- `FullContextPackage`

Required design choices:
- `ContextNeeds` includes `SemanticQueries`, `ExplicitFiles`, `ExplicitSymbols`, `IncludeConventions`, `IncludeGitContext`, `GitContextDepth`, `MomentumFiles`, `MomentumModule`, `Signals`
- `ContextAssemblyReport` includes both assembly-time and post-turn quality fields
- `BrainHit` exists but is v0.1-placeholder-friendly
- `FullContextPackage` keeps `Frozen bool`; no fake deep immutability mechanism
- include enough inclusion/exclusion metadata inside result structs so Layer 6 can later render “included vs excluded” without schema changes

Verification:
- package compiles

---

## Task 2: Define the boundary/scope types

Objective: prevent Layer 3 from coupling to unfinished Layer 5 runtime types.

Add to `internal/context/scope.go`:
- `type SeenFileLookup interface { Contains(path string) (bool, int) }`
- `type AssemblyScope struct { ConversationID string; TurnNumber int; SeenFiles SeenFileLookup }`

Rules:
- no `SessionState`
- no `internal/agent` import
- no `internal/conversation` import

Verification:
- package compiles

---

## Task 3: Define the future component interfaces

Objective: give later slices stable interfaces to implement against.

Add to `internal/context/interfaces.go`:
- `TurnAnalyzer`
- `QueryExtractor`
- `MomentumTracker`
- `ConventionSource`
- `Retriever`
- `BudgetManager`
- `Serializer`

Guidance:
- keep signatures narrow and obvious
- use `context.Context` only where future implementations will need it
- use `[]db.Message` for history-bearing interfaces
- reuse `config.ContextConfig` rather than inventing a duplicate config type here

Verification:
- package compiles

---

## Task 4: Add GoDoc and package-level clarity

Objective: make the contracts self-explanatory.

Update `internal/context/doc.go` and add comments on all exported types/interfaces.

Must explicitly document:
- Layer 3 owns context assembly contracts
- Layer 5 consumes outputs but is not imported here
- project brain fields are reserved for v0.2 proactive retrieval

Verification:
- `go test ./internal/context/...`

---

## Task 5: Write focused unit tests for contract behavior

Objective: lock down the intended shapes and zero-value expectations.

Add to `internal/context/types_test.go` tests for:
- zero-value `ContextNeeds` is usable
- zero-value `RetrievalResults` is usable
- `AssemblyScope` can hold a stub `SeenFileLookup`
- `FullContextPackage` preserves `Frozen` and report pointer as expected
- interfaces compile against tiny stub implementations

Keep tests structural and cheap. No mocks for real dependencies.

Verification:
- `go test ./internal/context/... -run 'TestContext|TestAssembly|TestInterfaces|TestFullContextPackage'`

---

## Task 6: Final verification and commit

Objective: prove the slice is complete and isolated.

Run:
- `go test ./internal/context/...`
- optionally: `make test` if you want whole-repo confidence after the Makefile fix

Commit:
- `git add internal/context`
- `git commit -m "feat(context): add layer3 contracts"`

Stop after commit and ask for approval before Slice 2.

---

## Success criteria

This slice is done when:
- `internal/context` contains the full Layer 3 contract surface
- later slices can compile against these types/interfaces without redefining them
- no Layer 5 runtime dependency leaked in
- tests for `internal/context` pass
- the change is small enough for one clean local commit
