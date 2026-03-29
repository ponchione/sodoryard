# Task 01: ContextAssembler Struct and Assemble Method Orchestration

**Epic:** 06 — Context Assembly Pipeline
**Status:** ⬚ Not started
**Dependencies:** Epics 02, 03, 04, 05

---

## Description

Create the `ContextAssembler` struct and implement the `Assemble` method that wires all context assembly components into the full 9-step pipeline. The constructor accepts the component dependencies needed for assembly orchestration (analyzer, query extractor, momentum tracker, retrieval orchestrator, budget manager, serializer, config, DB handle). The `Assemble` method orchestrates the flow: turn analysis, momentum computation, query extraction, parallel retrieval, relevance filtering, budget fitting, serialization, freezing, and report persistence. Each stage's latency is recorded for the assembly report.

## Acceptance Criteria

- [ ] `ContextAssembler` struct with constructor accepting all component dependencies
- [ ] `Assemble(ctx context.Context, message string, history []Message, sessionState *SessionState) (*FullContextPackage, error)` method implementing the full 9-step flow:
  1. Turn Analyzer (E02): extract signals, produce `ContextNeeds` — record `AnalysisLatencyMs`
  2. Momentum (E03): if continuation or weak signals, scan history for momentum — update `MomentumFiles` and `MomentumModule`
  3. Query Extraction (E03): translate `ContextNeeds` into 1-3 semantic search queries
  4. Parallel Retrieval (E04): execute all retrieval paths concurrently — record `RetrievalLatencyMs`
  5. Relevance Filtering (E04): apply thresholds, merge, dedup
  6. Budget Fitting (E05): allocate tokens by priority, track inclusions/exclusions
  7. Serialization (E05): format selected content into markdown, passing `seenFiles` from session state
  8. Freeze: wrap serialized markdown + token count + report into `FullContextPackage`, mark as frozen
  9. Persist report: write `ContextAssemblyReport` to `context_reports` table
- [ ] `FullContextPackage` is immutable after creation — the `Frozen` flag prevents modification
- [ ] Total pipeline wall-clock time recorded as `TotalLatencyMs` on the report
- [ ] `seenFiles` set from `SessionState` passed through to the serializer for previously-viewed annotations
- [ ] Pipeline logs assembly summary via structured logging: total latency, chunks included/excluded, budget usage
- [ ] Package compiles with no errors
