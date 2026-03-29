# Layer 3, Epic 05: Budget Manager & Context Serialization

**Layer:** 3 — Context Assembly
**Epic:** 05 of 07
**Status:** ⬚ Not started
**Dependencies:** [[layer-3-epic-01-context-assembly-types]]; Layer 2: [[layer-2-epic-01-provider-types]] (context window metadata per model)

---

## Description

Implement the budget manager that allocates the available token budget across context sources by priority, and the serializer that formats the selected content into the markdown block injected as system prompt cache block 2.

The budget manager performs token accounting (computing available budget from model context limit minus reserved blocks minus conversation history), priority-based filling (explicit files → top RAG → structural → conventions → git → lower RAG), and tracks what was included vs excluded and why. The serializer produces the markdown format specified in [[06-context-assembly]] with code grouping, description-before-code, conventions/git sections, and previously-viewed annotations. Proactive project-brain content is deferred to v0.2.

Both are pure computation — no I/O, no LLM calls. They operate on `RetrievalResults` (from Epic 04) and produce a serialized string plus budget metadata for the `ContextAssemblyReport`.

---

## Definition of Done

### Budget Manager

- [ ] `BudgetManager` struct that takes `RetrievalResults`, model context limit, current conversation history token count, and config, and returns `BudgetResult`
- [ ] **Token accounting:**
  - Total budget = model context limit
  - Reserved (non-negotiable): base system prompt (~3k tokens), tool schemas (~3k tokens), response headroom (~16k tokens, configurable via `max_tokens`)
  - Available = Total - Reserved - Conversation history tokens - Response headroom
  - Assembled context budget = `min(Available, MaxAssembledTokens)` where `MaxAssembledTokens` defaults to 30,000
- [ ] **Priority-based filling** — iterate through sources in priority order, adding content until budget is exhausted:
  1. Explicit files (highest signal — user mentioned them directly)
  2. Top RAG code hits (above threshold, de-duped, re-ranked by hit count)
  3. Structural graph results (callers/callees of identified symbols)
  4. Conventions (derived from code analysis)
  5. Git context (recent commits)
  6. Lower-ranked RAG code hits (fill remaining budget)
- [ ] **Token estimation:** Estimate token count per content piece as `len(text) / 4` (character-based approximation). Sufficient for budget fitting — exact counts come from the API response.
- [ ] **Sub-budgets:** Conventions capped at `ConventionBudgetTokens` (default 3000). Git context capped at `GitContextBudgetTokens` (default 2000). These are soft caps within the overall budget.
- [ ] **Tracking:** Record which chunks/documents were included, which were excluded, and the exclusion reason (`"below_threshold"` or `"budget_exceeded"`). This feeds `ContextAssemblyReport.IncludedChunks`, `ExcludedChunks`, `ExclusionReasons`.
- [ ] **Budget breakdown:** Produce `map[string]int` showing tokens consumed per source category: `"explicit_files"`, `"rag"`, `"structural"`, `"conventions"`, `"git"`. This feeds `ContextAssemblyReport.BudgetBreakdown`.
- [ ] **History compression trigger:** If conversation history exceeds `CompressionThreshold` (default 50%) of the total context window, set a `CompressionNeeded` flag on the result. The budget manager does not perform compression — it signals the agent loop.

### Context Serialization

- [ ] `ContextSerializer` function or struct that takes `BudgetResult` (selected content) and a `seenFiles` set, and produces a markdown-formatted string
- [ ] **Code chunks grouped by file:** `## Relevant Code` section with chunks organized by file path. Multiple chunks from the same file grouped under a single file-level header.
- [ ] **Description before code:** Each code chunk has: header (`### file/path.go (lines X-Y)`), 1-2 sentence description, then the code fence with language tag
- [ ] **Previously-viewed annotation:** Chunks from files in the `seenFiles` set are annotated with `[previously viewed in turn N]` in the header
- [ ] **Conventions section:** `## Project Conventions` with 5-10 bullet points
- [ ] **Git context section:** `## Recent Changes (last N commits)` with one-line commit summaries
- [ ] **Language-tagged code fences:** Go code in `` ```go ``, TypeScript in `` ```typescript ``, Python in `` ```python ``, etc.
- [ ] Output is a single markdown string ready for injection as system prompt block 2

### Tests

- [ ] Budget: model with 200k context, 50k history → available budget computed correctly
- [ ] Budget: explicit files fill first, then RAG, then structural / conventions / git, in correct priority order
- [ ] Budget: budget exhausted mid-priority-2 → remaining lower-priority items in ExcludedChunks with reason "budget_exceeded"
- [ ] Budget: history exceeds 50% of context window → CompressionNeeded flag set
- [ ] Budget breakdown: correct token counts per category
- [ ] Serialization: two chunks from same file grouped under one file header
- [ ] Serialization: chunk from a seen file gets `[previously viewed]` annotation
- [ ] Serialization: output is valid markdown (code fences properly closed, headers properly nested)
- [ ] Serialization: empty retrieval results → empty or minimal markdown output (no crash)

---

## Architecture References

- [[06-context-assembly]] — "Component: Budget Manager" section (Token Accounting, MAX_CONTEXT_BUDGET, Priority Allocation, History Compression Trigger), "Component: Context Serialization" section (Format, Format Principles, What Is NOT Included)
- [[03-provider-architecture]] — Context window limits per model (the budget manager needs to know the model's total context limit)

---

## Implementation Notes

- The token estimation (`len/4`) is intentionally rough. Over-estimating is better than under-estimating — the assembled context block being slightly smaller than budget is fine. Going over budget risks context overflow.
- The `seenFiles` set is maintained at the session level by the agent loop. This epic receives it as input, doesn't maintain it. The pipeline (Epic 06) passes it through.
- Priority order is a design guess from [[06-context-assembly]]. The context inspector debug panel (Layer 6) is the mechanism for validating whether this ordering produces good hit rates. The priority list should be configurable or at least easy to reorder in code.
- Sub-budgets (conventions, git) are soft caps. The implementation should not pre-reserve them — just enforce caps as each category is processed.
- The serialization format must be stable across all iterations within a turn (it's cache block 2). No randomization, no timestamp-dependent content, no non-deterministic ordering. Given the same input, the serializer must produce the same output.
