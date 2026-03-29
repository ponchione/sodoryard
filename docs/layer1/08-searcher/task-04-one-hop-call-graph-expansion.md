# Task 04: One-Hop Call Graph Expansion

**Epic:** 08 — Searcher
**Status:** ⬚ Not started
**Dependencies:** Task 03, L1-E05 (Store.GetByName)

---

## Description

Implement one-hop call graph expansion for the top-ranked direct hits. For each direct hit chunk, look up the functions it calls (from the chunk's `Calls` field) and the functions that call it (from its `CalledBy` field) via the store's `GetByName` method. These hop results are tagged with their relationship to the direct hit and merged into the final result set. Budget allocation controls how many slots go to direct hits vs. hop results. This expansion surfaces structurally related code that the user didn't explicitly ask about but is likely relevant.

## Acceptance Criteria

- [ ] `(s *Searcher) expandHops(ctx context.Context, directHits []rankedResult, opts SearchOptions) ([]rankedResult, error)` method (unexported) that:
  1. Calculates the budget split from `opts.MaxResults` and `opts.HopBudgetRatio`:
     - `directBudget = int(float64(opts.MaxResults) * (1.0 - opts.HopBudgetRatio))` — with default 0.4 ratio and MaxResults 30, this is 18
     - `hopBudget = opts.MaxResults - directBudget` — with defaults, this is 12
  2. Truncates `directHits` to `directBudget` (take the top-ranked direct hits only)
  3. For each direct hit in the truncated list, iterates over its `Chunk.Calls` and `Chunk.CalledBy` string slices:
     - For each name in `Calls`: calls `s.store.GetByName(ctx, name)` and tags the returned chunk(s) as `HopCallee` with `HopSource` set to the direct hit's chunk ID
     - For each name in `CalledBy`: calls `s.store.GetByName(ctx, name)` and tags the returned chunk(s) as `HopCaller` with `HopSource` set to the direct hit's chunk ID
  4. Deduplicates hop results against direct hits — if a chunk ID already exists in the direct hits, the hop result is dropped (direct hit takes precedence)
  5. Deduplicates hop results against each other — if the same chunk appears as a hop from multiple direct hits, keep the first occurrence
  6. Truncates the hop result list to `hopBudget`
  7. Returns a combined slice: truncated direct hits followed by truncated hop results
- [ ] Hop results have `HitCount` set to 0 and `BestScore` set to 0.0 (they are not vector search results; they are structural expansions)
- [ ] If `GetByName` returns no chunks for a given name, that name is silently skipped (not an error)
- [ ] If `GetByName` returns an error, log the error but continue processing remaining names (do not fail the entire expansion for one lookup failure)
- [ ] If a direct hit has empty `Calls` and `CalledBy` slices, no hop lookups are performed for that hit
- [ ] If `opts.ExpandHops` is false, this method is not called (caller responsibility, but document the contract)
- [ ] If `opts.HopBudgetRatio` is 0.0, `hopBudget` is 0 and no hops are looked up — all budget goes to direct hits
- [ ] If `opts.HopBudgetRatio` is 1.0, `directBudget` is 0 and only hops are returned (degenerate case, but should not panic)
- [ ] Example expected behavior:
  - MaxResults=10, HopBudgetRatio=0.4 -> directBudget=6, hopBudget=4
  - Direct hit chunk X has Calls=["FuncA", "FuncB"], CalledBy=["FuncC"]
  - GetByName("FuncA") returns chunk for FuncA -> tagged HopCallee, HopSource=X.ID
  - GetByName("FuncB") returns chunk for FuncB -> tagged HopCallee, HopSource=X.ID
  - GetByName("FuncC") returns chunk for FuncC -> tagged HopCaller, HopSource=X.ID
  - If FuncA's chunk ID matches a direct hit, FuncA is dropped from hops
