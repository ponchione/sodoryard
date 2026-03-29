# Layer 3, Epic 01: Context Assembly Types & Interfaces

**Layer:** 3 — Context Assembly
**Epic:** 01 of 07
**Status:** ⬚ Not started
**Dependencies:** Layer 0: [[layer-0-epic-01-scaffolding]], [[layer-0-epic-03-config]]

---

## Description

Define all shared types, interfaces, and configuration structs for the context assembly layer. This is the foundation every other Layer 3 epic depends on. The types must capture the full information flow: what the turn analyzer produces (`ContextNeeds`, `Signal`), what retrieval returns (`RAGHit`, `BrainHit`, `GraphHit`, `FileResult`), what budget fitting outputs, what the final serialized package looks like (`FullContextPackage`), and what the observability report records (`ContextAssemblyReport`).

All types live in `internal/context/` (or `internal/assembly/` — the package name is an implementation-time decision). No logic beyond constructors and validation helpers. The `TurnAnalyzer` interface is defined here with the replaceability contract — the rule-based implementation lives in Epic 02, but the interface lives here so the pipeline (Epic 06) depends only on the abstraction.

---

## Definition of Done

- [ ] `ContextNeeds` struct defined with all fields: `SemanticQueries []string`, `ExplicitFiles []string`, `ExplicitSymbols []string`, `IncludeConventions bool`, `IncludeGitContext bool`, `GitContextDepth int`, `MomentumFiles []string`, `MomentumModule string`, `Signals []Signal`
- [ ] `Signal` struct defined: `Type string`, `Source string`, `Value string`
- [ ] `TurnAnalyzer` interface defined: `AnalyzeTurn(message string, recentHistory []Message) *ContextNeeds` — where `Message` is the type from the message storage model ([[08-data-model]])
- [ ] Retrieval result types defined:
  - `RAGHit`: chunk ID, file path, name, signature, description, body, similarity score, language, chunk type, line start, line end
  - `BrainHit`: document path, title, snippet, match score, match mode (keyword/semantic/graph), tags
  - `GraphHit`: symbol name, file path, relationship type (caller/callee/implements), depth
  - `FileResult`: file path, content, token count, truncated flag
- [ ] `RetrievalResults` aggregate struct containing slices of all hit types plus convention text and git context string
- [ ] `FullContextPackage` struct defined: serialized markdown string, token count, `ContextAssemblyReport`, frozen flag
- [ ] `ContextAssemblyReport` struct defined with all fields from [[06-context-assembly]]:
  - Latency fields: `AnalysisLatencyMs`, `RetrievalLatencyMs`, `TotalLatencyMs`
  - Analyzer output: `Needs ContextNeeds`
  - Pre-filtering results: `RAGResults`, `BrainResults`, `ExplicitFileResults`, `GraphResults`
  - Post-filtering: `IncludedChunks []string`, `ExcludedChunks []string`, `ExclusionReasons map[string]string`
  - Budget: `BudgetTotal int`, `BudgetUsed int`, `BudgetBreakdown map[string]int`
  - Quality signals (populated after turn completes): `AgentUsedSearchTool bool`, `AgentReadFiles []string`, `ContextHitRate float64`
- [ ] Context assembly config struct defined, loadable from the `context:` section of `sirtopham.yaml`:
  - Budget: `MaxAssembledTokens`, `MaxChunks`, `MaxExplicitFiles`, `ConventionBudgetTokens`, `GitContextBudgetTokens`
  - Quality: `RelevanceThreshold`, `StructuralHopDepth`, `StructuralHopBudget`, `MomentumLookbackTurns`
  - Compression: `CompressionThreshold`, `CompressionHeadPreserve`, `CompressionTailPreserve`, `CompressionModel`
  - Debug: `EmitContextDebug`, `StoreAssemblyReports`
- [ ] Package compiles with no errors
- [ ] All types have GoDoc comments explaining their role in the pipeline

---

## Architecture References

- [[06-context-assembly]] — "Component: Turn Analyzer" (Interface section), "Component: Context Assembly Report" (Structure section), "Configuration" section
- [[08-data-model]] — `context_reports` table schema (the report struct maps to these columns)
- [[04-code-intelligence-and-rag]] — `SearchResult` type from Layer 1 that `RAGHit` adapts from

---

## Implementation Notes

- `ContextAssemblyReport` quality fields (`AgentUsedSearchTool`, `ContextHitRate`, `AgentReadFiles`) are zero-valued at creation time and populated after the turn completes by the agent loop. The struct must support this two-phase lifecycle.
- The `Message` type referenced in `TurnAnalyzer.AnalyzeTurn` is the same type used for conversation history reconstruction. It comes from the data layer (Layer 0, Epic 06). Import it, don't redefine it.
- Config defaults should be set in the config loading path, not hardcoded in the type definition. The struct here just defines the shape.
- `FullContextPackage` is intentionally simple — a serialized string plus metadata. The serialization logic lives in Epic 05. The pipeline (Epic 06) calls the serializer and wraps the result.
