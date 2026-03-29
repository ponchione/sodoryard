# Layer 3, Epic 06: Context Assembly Pipeline

**Layer:** 3 — Context Assembly
**Epic:** 06 of 07
**Status:** ⬚ Not started
**Dependencies:** [[layer-3-epic-02-turn-analyzer]], [[layer-3-epic-03-query-extraction-momentum]], [[layer-3-epic-04-retrieval-orchestrator]], [[layer-3-epic-05-budget-manager-serialization]], [[layer-3-epic-07-compression-engine]]; Layer 0: [[layer-0-epic-04-sqlite]], [[layer-0-epic-06-schema-sqlc]]

---

## Description

The capstone epic. Wire the turn analyzer, query extractor, momentum tracker, retrieval orchestrator, budget manager, serializer, and compression-needed handoff into a single `ContextAssembler` that the agent loop calls once at the start of each turn. This is the orchestrator that implements the full 9-step assembly flow from [[06-context-assembly]].

The pipeline takes a user message, recent conversation history, and session state, and produces a `FullContextPackage` — the frozen, immutable context block that becomes system prompt cache block 2 for all iterations within the turn. It also persists a `ContextAssemblyReport` to SQLite for observability and provides a method for the agent loop to update quality metrics after the turn completes.

---

## Definition of Done

### Pipeline Orchestration

- [ ] `ContextAssembler` struct with a constructor accepting all component dependencies needed for assembly orchestration (analyzer, query extractor, momentum tracker, retrieval orchestrator, budget manager, serializer, config, DB handle)
- [ ] `Assemble(ctx context.Context, message string, history []Message, sessionState *SessionState) (*FullContextPackage, error)` method implementing the full flow:
  1. **Turn Analyzer** (E02): Extract signals, produce `ContextNeeds` — record `AnalysisLatencyMs`
  2. **Momentum** (E03): If continuation detected or weak signals, scan recent history for momentum — update `ContextNeeds.MomentumFiles` and `MomentumModule`
  3. **Query Extraction** (E03): Translate `ContextNeeds` into 1-3 semantic search queries
  4. **Parallel Retrieval** (E04): Execute all retrieval paths concurrently — record `RetrievalLatencyMs`
  5. **Relevance Filtering** (E04): Apply thresholds, merge, dedup
  6. **Budget Fitting** (E05): Allocate tokens by priority, track inclusions/exclusions
  7. **Serialization** (E05): Format selected content into markdown — pass `seenFiles` from session state
  8. **Freeze**: Wrap serialized markdown + token count + report into `FullContextPackage`, mark as frozen
  9. **Persist report**: Write `ContextAssemblyReport` to `context_reports` table
- [ ] `FullContextPackage` is immutable after creation — the `Frozen` flag prevents modification
- [ ] Total pipeline wall-clock time recorded as `TotalLatencyMs` on the report
- [ ] `seenFiles` set maintained on `SessionState` — updated by the agent loop when tool results contain file paths, passed to the serializer for previously-viewed annotations

### Report Persistence

- [ ] `ContextAssemblyReport` persisted to the `context_reports` SQLite table at step 9
- [ ] INSERT includes: `conversation_id`, `turn_number`, latency fields, `needs_json`, `signals_json`, `rag_results_json`, `brain_results_json`, `graph_results_json`, `explicit_files_json`, `budget_total`, `budget_used`, `budget_breakdown_json`, `included_count`, `excluded_count`
- [ ] Quality fields (`agent_used_search_tool`, `agent_read_files_json`, `context_hit_rate`) are zero-valued on INSERT — updated after the turn completes

### Post-Turn Quality Update

- [ ] `UpdateQuality(ctx context.Context, conversationID string, turnNumber int, usedSearchTool bool, readFiles []string) error` method
- [ ] Computes `ContextHitRate`: intersection of `readFiles` with `IncludedChunks` file paths, divided by `len(readFiles)`. If `readFiles` is empty, hit rate is 1.0 (no reactive reads needed = perfect).
- [ ] UPDATE the `context_reports` row with the computed quality fields
- [ ] Called by the agent loop after a turn completes (after the final LLM response)

### Compression Integration

- [ ] Before returning the `FullContextPackage`, check `BudgetResult.CompressionNeeded` flag
- [ ] If compression is needed, return a `CompressionNeeded` signal alongside the package so the agent loop can invoke the compression engine (E07) on conversation history before the first LLM call of the turn
- [ ] After agent-loop-triggered compression, history can be reloaded or re-measured for reporting if needed
- [ ] Compression does not re-trigger assembly — the assembled context is already frozen. Compression only affects the conversation history that will be sent alongside the assembled context.

### Tests

- [ ] Integration test: full pipeline with mock components — message in, `FullContextPackage` out, report persisted to SQLite
- [ ] Test: assembly with no relevant code (all below threshold) → empty assembled context, agent relies on reactive tools
- [ ] Test: assembly with explicit file reference → file content appears in serialized output, highest priority
- [ ] Test: quality update after turn → `context_hit_rate` correctly computed and persisted
- [ ] Test: `FullContextPackage` frozen flag prevents modification after creation
- [ ] Test: pipeline latency tracked correctly across all stages
- [ ] Test: report JSON fields serialize/deserialize correctly (ContextNeeds → JSON → back)

---

## Architecture References

- [[06-context-assembly]] — "The Full Assembly Flow" section (steps 1-9), "Component: Context Assembly Report" (Storage, Quality Metrics), "Interaction with Conversation History" (The Annotation Approach)
- [[08-data-model]] — `context_reports` table schema, INSERT/UPDATE lifecycle
- [[05-agent-loop]] — Steps 2-3 (Context Assembly, System Prompt Construction) — the agent loop calls this pipeline at turn start

---

## Implementation Notes

- This epic is primarily wiring — it calls into the components built in E02-E05 and E07. The logic here is orchestration: calling things in order, passing outputs as inputs, timing each stage, building the report.
- The `SessionState` struct (which tracks `seenFiles`, current turn number, conversation ID) is owned by Layer 5's session/conversation runtime and passed into Layer 3 as input. Layer 3 consumes it for serialization hints; it does not own the type.
- The compression integration point is subtle. The pipeline detects that compression is needed (via the budget manager's flag). But compression modifies conversation history, which the agent loop owns. The cleanest approach is the canonical one here: the pipeline returns a `CompressionNeeded` signal alongside the `FullContextPackage`, and the agent loop invokes the compression engine before proceeding to the LLM call. The pipeline doesn't directly mutate history.
- JSON serialization for report fields (`needs_json`, `signals_json`, etc.) should use `json.Marshal` → store as TEXT in SQLite. The web UI's context inspector parses these for display.
- The pipeline should log (via structured logging) the assembly summary: total latency, chunks included/excluded, budget usage. This is the turn-level diagnostic line in the logs.
