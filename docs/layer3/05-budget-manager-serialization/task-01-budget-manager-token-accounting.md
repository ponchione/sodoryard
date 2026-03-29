# Task 01: BudgetManager Struct and Token Accounting

**Epic:** 05 — Budget Manager & Context Serialization
**Status:** ⬚ Not started
**Dependencies:** Epic 01; Layer 2: Epic 01 (provider types for context window metadata)

---

## Description

Create the `BudgetManager` struct and implement the token accounting logic that computes the available assembled context budget. The budget calculation subtracts reserved allocations (base system prompt, tool schemas, response headroom) and current conversation history token count from the model's total context limit, then caps at `MaxAssembledTokens`. Token estimation uses the `len(text) / 4` character-based approximation — intentionally rough, favoring over-estimation to prevent context overflow.

## Acceptance Criteria

- [ ] `BudgetManager` struct defined with constructor accepting `RetrievalResults`, model context limit (`int`), current conversation history token count (`int`), and config
- [ ] Returns a `BudgetResult` struct containing selected content, budget metadata, and compression signal
- [ ] **Token accounting formula:**
  - Total budget = model context limit
  - Reserved (non-negotiable): base system prompt (~3k tokens), tool schemas (~3k tokens), response headroom (~16k tokens, configurable via `max_tokens`)
  - Available = Total - Reserved - Conversation history tokens - Response headroom
  - Assembled context budget = `min(Available, MaxAssembledTokens)` where `MaxAssembledTokens` defaults to 30,000
- [ ] Token estimation per content piece: `len(text) / 4` (character-based approximation)
- [ ] Package compiles with no errors
