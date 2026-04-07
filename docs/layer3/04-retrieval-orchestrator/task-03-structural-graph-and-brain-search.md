# Task 03: Structural Graph Path and v0.2 Brain Handoff Note

**Epic:** 04 — Retrieval Orchestrator
**Status:** ⬚ Not started
**Dependencies:** Task 01; Layer 1: Epic 09 (structural graph)

---

## Description

Implement the structural graph retrieval path. The structural graph path queries Layer 1 for upstream callers, downstream callees, and interface implementations of each symbol in `ContextNeeds.ExplicitSymbols`, with configurable depth and budget. Historical note: the old v0.1 handoff on this page said proactive brain retrieval was future work; current runtime has already landed MCP/vault-backed proactive keyword brain retrieval.

## Acceptance Criteria

- [ ] **Structural graph path:** For each symbol in `ContextNeeds.ExplicitSymbols`, queries Layer 1 structural graph for callers, callees, and interface implementations
- [ ] Configurable depth (`StructuralHopDepth`, default 1) and budget (`StructuralHopBudget`, default 10)
- [ ] Returns `[]GraphHit`
- [ ] Historical notes no longer present proactive brain retrieval as future work; current runtime truth is referenced instead
- [ ] Package compiles with no errors
