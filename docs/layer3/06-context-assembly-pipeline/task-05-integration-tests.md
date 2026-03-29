# Task 05: Integration Tests

**Epic:** 06 — Context Assembly Pipeline
**Status:** ⬚ Not started
**Dependencies:** Task 01, Task 02, Task 03, Task 04

---

## Description

Integration tests for the full context assembly pipeline. Tests exercise the complete flow from user message to `FullContextPackage` with mock component implementations, verify report persistence to SQLite, quality metric updates, the frozen package invariant, and edge cases like empty retrieval results.

## Acceptance Criteria

- [ ] Integration test: full pipeline with mock components — message in, `FullContextPackage` out, report persisted to SQLite
- [ ] Test: assembly with no relevant code (all below threshold) produces empty assembled context — agent will rely on reactive tools
- [ ] Test: assembly with explicit file reference — file content appears in serialized output at highest priority
- [ ] Test: quality update after turn — `context_hit_rate` correctly computed and persisted to `context_reports` table
- [ ] Test: `FullContextPackage` frozen flag prevents modification after creation
- [ ] Test: pipeline latency tracked correctly across all stages (`AnalysisLatencyMs`, `RetrievalLatencyMs`, `TotalLatencyMs`)
- [ ] Test: report JSON fields serialize and deserialize correctly (`ContextNeeds` to JSON and back)
- [ ] All tests pass: `go test ./internal/context/...`
