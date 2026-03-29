# Task 03: Post-Turn Quality Update Method

**Epic:** 06 — Context Assembly Pipeline
**Status:** ⬚ Not started
**Dependencies:** Task 02

---

## Description

Implement the `UpdateQuality` method that the agent loop calls after a turn completes to populate the quality signal fields on the persisted `ContextAssemblyReport`. The method computes `ContextHitRate` by measuring the overlap between files the agent reactively read (via search/read tools) and the files that were proactively included in the assembled context. A high hit rate means the assembled context anticipated the agent's needs; a low hit rate means the agent had to search for information that should have been included.

## Acceptance Criteria

- [ ] `UpdateQuality(ctx context.Context, conversationID string, turnNumber int, usedSearchTool bool, readFiles []string) error` method on `ContextAssembler`
- [ ] Computes `ContextHitRate`: intersection of `readFiles` with `IncludedChunks` file paths, divided by `len(readFiles)`
- [ ] If `readFiles` is empty, hit rate is 1.0 (no reactive reads needed = perfect proactive context)
- [ ] UPDATE the `context_reports` row with: `agent_used_search_tool`, `agent_read_files_json` (JSON-serialized), `context_hit_rate`
- [ ] Called by the agent loop after a turn completes (after the final LLM response)
- [ ] Package compiles with no errors
