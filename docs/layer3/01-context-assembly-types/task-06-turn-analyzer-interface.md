# Task 06: TurnAnalyzer Interface and Compilation Verification

**Epic:** 01 — Context Assembly Types & Interfaces
**Status:** ⬚ Not started
**Dependencies:** Task 01

---

## Description

Define the `TurnAnalyzer` interface that all turn analyzer implementations must satisfy. The interface is defined here in the types package so that the pipeline (Epic 06) depends only on the abstraction, not the concrete `RuleBasedAnalyzer` from Epic 02. The `Message` type referenced in the method signature is imported from the data layer (Layer 0, Epic 06), not redefined. This task also verifies that the entire package compiles cleanly with all types from Tasks 01-05.

## Acceptance Criteria

- [ ] `TurnAnalyzer` interface defined with method: `AnalyzeTurn(message string, recentHistory []Message) *ContextNeeds`
- [ ] `Message` type is imported from the data layer package, not redefined in this package
- [ ] GoDoc comment on the interface explains the replaceability contract: the rule-based implementation lives in Epic 02, but the interface is defined here so downstream consumers depend only on the abstraction
- [ ] GoDoc comment lists the planned implementations: `RuleBasedAnalyzer` (v1, this layer) and future LLM-assisted analyzer
- [ ] Full package compiles with no errors: `go build ./internal/context/...`
- [ ] All types across Tasks 01-06 have GoDoc comments explaining their role in the pipeline
