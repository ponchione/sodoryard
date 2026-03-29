# Task 03: Structural Graph Path and v0.2 Brain Handoff Note

**Epic:** 04 — Retrieval Orchestrator
**Status:** ⬚ Not started
**Dependencies:** Task 01; Layer 1: Epic 09 (structural graph)

---

## Description

Implement the structural graph retrieval path. The structural graph path queries Layer 1 for upstream callers, downstream callees, and interface implementations of each symbol in `ContextNeeds.ExplicitSymbols`, with configurable depth and budget. Also document the explicit v0.2 handoff: proactive project-brain retrieval is not part of v0.1 context assembly and remains a future extension.

## Acceptance Criteria

- [ ] **Structural graph path:** For each symbol in `ContextNeeds.ExplicitSymbols`, queries Layer 1 structural graph for callers, callees, and interface implementations
- [ ] Configurable depth (`StructuralHopDepth`, default 1) and budget (`StructuralHopBudget`, default 10)
- [ ] Returns `[]GraphHit`
- [ ] Epic/task notes make the v0.1 boundary explicit: project-brain retrieval stays reactive-only in v0.1 and is not called from context assembly
- [ ] Package compiles with no errors
