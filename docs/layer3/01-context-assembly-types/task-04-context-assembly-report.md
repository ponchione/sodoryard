# Task 04: ContextAssemblyReport Struct

**Epic:** 01 — Context Assembly Types & Interfaces
**Status:** ⬚ Not started
**Dependencies:** Task 01, Task 02

---

## Description

Define the `ContextAssemblyReport` struct that captures full observability data for each context assembly invocation. This struct maps to the `context_reports` SQLite table and supports a two-phase lifecycle: latency, analyzer output, retrieval results, and budget data are populated during assembly, while quality signals are populated after the turn completes when the agent loop reports which files the agent actually read reactively.

## Acceptance Criteria

- [ ] Latency fields defined: `AnalysisLatencyMs int64`, `RetrievalLatencyMs int64`, `TotalLatencyMs int64`
- [ ] Analyzer output field: `Needs ContextNeeds`
- [ ] Pre-filtering result fields: `RAGResults []RAGHit`, `BrainResults []BrainHit`, `ExplicitFileResults []FileResult`, `GraphResults []GraphHit`
- [ ] Post-filtering fields: `IncludedChunks []string`, `ExcludedChunks []string`, `ExclusionReasons map[string]string`
- [ ] Budget fields: `BudgetTotal int`, `BudgetUsed int`, `BudgetBreakdown map[string]int`
- [ ] Quality signal fields (zero-valued at creation, populated post-turn): `AgentUsedSearchTool bool`, `AgentReadFiles []string`, `ContextHitRate float64`
- [ ] GoDoc comment explains the two-phase lifecycle: quality fields are zero-valued at creation time and populated after the turn completes via the agent loop
- [ ] Package compiles with no errors
