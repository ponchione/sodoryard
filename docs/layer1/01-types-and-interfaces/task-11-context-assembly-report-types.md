# Task 11: Context Assembly Report and Retrieval Result Types

**Epic:** 01 — Types and Interfaces
**Status:** ⬚ Not started
**Dependencies:** Task 10

---

## Description

Define the `ContextAssemblyReport` struct and its supporting retrieval result types. The report is the observability backbone of the context assembly layer — every turn produces one, recording what was analyzed, retrieved, filtered, included, and excluded. The retrieval result types (`RAGHit`, `BrainHit`, `FileResult`, `GraphHit`) capture pre-filtering results from each parallel retrieval path. These types are critical for the context inspector debug panel in the web UI and for systematic tuning of relevance thresholds and budget allocation.

## Acceptance Criteria

- [ ] `RAGHit` struct defined in `internal/rag/types.go` with the following fields:
  - `ChunkID    string`  — ID of the matched chunk
  - `FilePath   string`  — file path of the chunk
  - `Name       string`  — symbol name
  - `Score      float64` — cosine similarity score
- [ ] `BrainHit` struct defined in `internal/rag/types.go` with the following fields:
  - `DocumentPath string`  — path to the brain document (Obsidian note)
  - `Title        string`  — document title
  - `Score        float64` — match score (keyword or semantic)
  - `MatchMode    string`  — how this result was found: `"keyword"`, `"semantic"`, or `"graph_hop"`
- [ ] `FileResult` struct defined in `internal/rag/types.go` with the following fields:
  - `FilePath   string` — path to the explicitly requested file
  - `SizeTokens int`    — approximate token count of the file content
  - `Truncated  bool`   — whether the file was truncated to fit budget
- [ ] `GraphHit` struct defined in `internal/rag/types.go` with the following fields:
  - `Symbol           string` — qualified symbol name
  - `RelationshipType string` — `"upstream"` (caller), `"downstream"` (callee), or `"interface"`
  - `Depth            int`    — hop distance from the query target
- [ ] `ContextAssemblyReport` struct defined in `internal/rag/types.go` with the following fields:
  - **Timing:**
    - `TurnNumber         int`   — which turn in the session this report is for
    - `AnalysisLatencyMs  int64` — milliseconds spent in the turn analyzer
    - `RetrievalLatencyMs int64` — milliseconds spent in parallel retrieval
    - `TotalLatencyMs     int64` — total wall-clock milliseconds for the full assembly pipeline
  - **Analyzer output:**
    - `Needs ContextNeeds` — the full ContextNeeds produced by the turn analyzer
  - **Pre-filtering retrieval results:**
    - `RAGResults          []RAGHit`    — all RAG results before relevance threshold
    - `BrainResults        []BrainHit`  — all brain results before relevance threshold
    - `ExplicitFileResults []FileResult` — results of direct file reads
    - `GraphResults        []GraphHit`  — structural graph results
  - **Post-filtering (budget fitting output):**
    - `IncludedChunks   []string`          — chunk IDs that made it into the final context
    - `ExcludedChunks   []string`          — chunk IDs that were cut
    - `ExclusionReasons map[string]string` — chunk ID to reason: `"below_threshold"` or `"budget_exceeded"`
  - **Budget accounting:**
    - `BudgetTotal     int`            — tokens available for assembled context
    - `BudgetUsed      int`            — tokens actually consumed
    - `BudgetBreakdown map[string]int` — per-category breakdown (e.g., `"rag": 15000`, `"brain": 5000`, `"conventions": 2500`, `"git": 500`, `"explicit_files": 8000`)
  - **Quality signals (computed after the turn completes):**
    - `AgentUsedSearchTool bool`     — true if the agent invoked `search_semantic` during this turn (indicates assembled context was insufficient)
    - `AgentReadFiles      []string` — file paths the agent read via tool calls during this turn
    - `ContextHitRate      float64`  — fraction of `AgentReadFiles` that were present in the assembled context (0.0 to 1.0)
- [ ] File compiles cleanly: `go build ./internal/rag/...`
