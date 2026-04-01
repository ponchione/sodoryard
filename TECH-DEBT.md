# TECH-DEBT

Open issues that should be fixed in a later focused session or need closer investigation.

**Last sweep:** 2026-04-01

Items marked ✅ were resolved in the 2026-04-01 tech-debt session.


## Layer 2 — Provider Router

### ✅ Router Validate() uses generic Models() for all provider types
**Severity:** Medium | **Source:** Layer 2 audit (2026-03-31) | **Resolved:** 2026-04-01

Added `provider.Pinger` interface with `Ping(ctx) error` method. Anthropic implements
it with a credential auth check (5s timeout); OpenAI-compatible providers implement it
with HTTP HEAD to baseURL (2s timeout). The `TrackedProvider` wrapper delegates Ping()
to the inner provider. Router.Validate() now type-asserts for Pinger and falls back to
Models() only when the provider doesn't implement it. Tests added for all three paths
(Ping success, Ping failure → unregister, Models fallback).

---

### Codex integration tests gated behind build tag
**Severity:** Low | **Source:** Layer 2 audit (2026-03-31)

✅ **Resolved:** 2026-04-01 — Added `internal/provider/codex/httptest_integration_test.go`
(no build tag) with 14 test functions / 20 test cases covering Complete(), Stream(),
error handling, retry paths, tool call parsing, and context cancellation. Uses a
`newHTTPTestProvider()` helper that bypasses `exec.LookPath`. The original
`integration_test.go` (build-tagged) remains for CLI-dependent tests.


## Layer 3 — Context Assembly

**Audited:** 2026-04-01 | **Result:** Clean — no code defects. Two partial items noted below.

All 7 epics (42 checklist items) pass. Three test/doc gaps found during audit
were fixed in the same session:
1. GoDoc comments added to token approximation functions
2. Assembler tests added for error propagation and nil optional components
3. Cascading compression test added (two rounds)

Race detector clean. 43 tests pass across 9 test files.

### ✅ Turn Analyzer missing "question intent" and "debugging hints" signals
**Severity:** Low | **Source:** Layer 3 audit (2026-04-01) | **Resolved:** 2026-04-01

Added `applyQuestionIntent` and `applyDebuggingHints` signal extractors to
`analyzer.go`. Question patterns: "can you explain", "what does", "how does",
"how do", "what is", "explain", "why". Debugging patterns: "error", "panic",
"nil", "crash", "fail", "bug", "broken", "stack trace", "segfault", "exception".
Both wired into `AnalyzeTurn` and tested with 5 new test cases.

---

### Budget priority chain omits brain docs (v0.2 scope)

**Severity:** Info | **Source:** Layer 3 audit (2026-04-01)

The audit checklist listed the budget priority order as:
  explicit files > **brain docs** > top RAG > graph > conventions > git > lower RAG

The epic spec (`docs/layer3/05-budget-manager-serialization/epic-05-budget-manager-serialization.md`)
explicitly defers brain docs to v0.2 and lists 6 priority tiers without brain.
The implementation matches the v0.1 spec exactly.

**Fix direction:** When v0.2 proactive brain retrieval lands, add a `BrainHit`
priority tier in `budget.go`'s `Fit()` method between explicit files and top RAG
hits. The `BrainHit` type already exists in `types.go` and `RetrievalResults`
already has a `BrainHits` field — only the budget allocation logic needs updating.


## Layer 4 — Tool System

**Audited:** 2026-04-01 | **Result:** Clean — 5 code issues fixed, 2 informational items deferred.

All 6 epics (168 checklist items) pass. Five issues found during audit were fixed
in the same session.

Race detector clean. 207 tool tests + 9 brain client tests pass across 18 test files.

### Executor.Execute signature differs from spec
**Severity:** Info | **Source:** Layer 4 audit (2026-04-01)

The spec defines `Execute(ctx, projectRoot, conversationID, turnNumber, iteration,
calls) []ToolResult`. The implementation splits this into `Execute(ctx, calls)` and
`ExecuteWithMeta(ctx, calls, meta)`, with `projectRoot` on `ExecutorConfig`.

All data reaches the same destination. The refactored design is arguably cleaner
(separating per-executor config from per-call metadata). **No change needed — spec
should be updated to reflect the cleaner design.**

---

### Tool interface method named ToolPurity() instead of Purity()
**Severity:** Info | **Source:** Layer 4 audit (2026-04-01)

The spec defines the method as `Purity() Purity`. The implementation uses
`ToolPurity() Purity` to avoid the type/method name collision in Go.

**No change needed — intentional Go idiom. Spec should use `ToolPurity()`.**


## Layer 5 — Agent Loop

**Audited:** 2026-04-01 | **Result:** Clean — 3 code issues fixed, 5 informational items deferred.

All 4 epics pass. Three issues found during audit were fixed in the same session.
Race detector clean. All tests pass across 13 test files (internal/agent) + 3 test files (internal/conversation).

### ✅ Block 3 cache marker on conversation history is a placeholder
**Severity:** Medium | **Source:** Layer 5 audit (2026-04-01) | **Resolved:** 2026-04-01

Added `CacheControl *CacheControl` field to `provider.Message`. In `buildMessages`,
the last history message now gets `CacheControl: ephemeral` when `wantCache` is true.
The Anthropic request translator (`request.go`) reads `msg.CacheControl` and maps it
to `apiCacheControl` in the API request body. Other providers ignore the field.
Updated `TestBuildPromptHistoryGrowthStability` to verify the cache marker moves
with the history prefix boundary.

---

### ✅ Retry-After header not respected
**Severity:** Low | **Source:** Layer 5 audit (2026-04-01) | **Resolved:** 2026-04-01

Added `RetryAfter time.Duration` to `ProviderError` with `ParseRetryAfter()` helper.
Anthropic and OpenAI HTTP response handlers now parse the `Retry-After` header
(seconds or HTTP-date) and populate the field on retriable errors. `streamWithRetry`
uses `max(backoff, retryAfter)` as the sleep duration. 9 new tests added.

---

### Provider fallback not implemented
**Severity:** Low | **Source:** Layer 5 audit (2026-04-01)

The spec mentions "optionally fall back to configured fallback provider" when retries
are exhausted. The router already implements fallback in `handleCompleteError` and
`handleStreamError` for retriable errors. The agent loop's `streamWithRetry` does not
trigger a separate fallback — it relies on the router's built-in fallback mechanism.

**Status:** The router-level fallback covers most cases. Agent-level fallback (e.g.,
rebuilding the prompt with a different model) would require `FallbackModel` on
`AgentLoopConfig`. Deferred — low practical impact since the router handles it.

---

### Iteration analytics persistence is still non-atomic relative to messages
**Severity:** Low | **Source:** Layer 5 audit (2026-04-01), revisited 2026-04-01

The current contract is now explicit: `PersistIteration` is atomic for `messages`
rows only. `tool_executions` and `sub_calls` are persisted on separate best-effort
paths (`tool.Executor` and `provider/tracking.TrackedProvider`) and may be missing if
an analytics write fails after message persistence succeeds.

This is currently tolerated because:
- the user-visible source of truth is the `messages` table
- cancellation cleanup still deletes `messages`, `tool_executions`, and `sub_calls`
  together for an in-flight iteration
- missing analytics rows are recoverable and far lower severity than losing the
  canonical conversation history

**Future fix direction:** If stronger guarantees become necessary, extend the
iteration persistence contract so the agent loop can hand `PersistIteration`
optional tool-execution and sub-call payloads and commit all three record classes in a
single transaction.

---

### Interrupted assistant/tool tombstones still reuse existing message schemas
**Severity:** Low | **Source:** Claude-handoff cancellation cleanup follow-up (2026-04-01)

Cancellation cleanup now persists two distinct durable markers inside existing message content:
- assistant tombstones: `[interrupted_assistant]` or `[failed_assistant]`
- tool tombstones: `[interrupted_tool_result]`

This is good enough to preserve transcript truthfulness today, but it still has follow-up debt:
- no first-class DB/message type distinguishes tombstones from ordinary assistant/tool payloads
- frontend/UI rendering may eventually want special treatment rather than showing raw marker text
- compression now collapses assistant tombstones to compact summaries, but other downstream consumers (search, title-adjacent transcript utilities, any future transcript exports) may still need explicit rules for these markers

**Future fix direction:** If interrupted-state UX or analytics become important, introduce a
first-class durable representation (schema field, content-block type, or explicit metadata)
for interrupted assistant/tool records and teach the web UI plus transcript utilities to render,
filter, or down-rank them intentionally.

---

### Executor.Execute signature differs from spec (agent loop interface)
**Severity:** Info | **Source:** Layer 5 audit (2026-04-01)

The agent loop's `ToolExecutor` interface uses `Execute(ctx, call) (*ToolResult, error)`
(single call). The batch dispatch logic lives in the agent loop itself. **No change
needed — documented for spec reconciliation.**


## Layer 6 — Web Interface & Streaming

**Audited:** 2026-04-01 | **Result:** Clean — 2 code defects fixed, 4 informational items deferred.

All 10 epics pass. Frontend builds without errors. TypeScript type-checks cleanly.

### ✅ model_override WebSocket event not implemented
**Severity:** Medium | **Source:** Layer 6 audit (2026-04-01) | **Resolved:** 2026-04-01

Added `model_override` case to the WebSocket `readLoop` switch. Per-connection
`connOverride` struct stores model/provider overrides (guarded by mutex for goroutine
safety). `handleMessage` consumes the override and passes it to `RunTurnRequest` via
new `Model` and `Provider` fields. Overrides can also be sent inline on the `"message"`
event. The agent loop resolves effective model/provider from request overrides vs config
defaults before the iteration loop.

---

### ✅ WS client model/provider fields not forwarded to RunTurnRequest
**Severity:** Low | **Source:** Layer 6 audit (2026-04-01) | **Resolved:** 2026-04-01

Resolved as part of the model_override implementation above. `ClientMessage.Model` and
`ClientMessage.Provider` are now forwarded to `RunTurnRequest.Model` and
`RunTurnRequest.Provider`.

---

### Metrics endpoint paths differ from spec
**Severity:** Info | **Source:** Layer 6 audit (2026-04-01)

The spec defines `/api/conversations/:id/metrics` and `/api/conversations/:id/context-reports`.
The implementation uses `/api/metrics/conversation/:id` and
`/api/metrics/conversation/:id/context/:turn`. **No change needed — implementation
paths are cleaner. Spec should be updated.**

---

### Conversation list page is a landing page, not a dedicated list view
**Severity:** Info | **Source:** Layer 6 audit (2026-04-01)

The spec mentions a conversation list page at `/`. The implementation uses root as a
landing page with quick-start input; the actual list lives in the sidebar. **No change
needed — reasonable UX choice. Documented for spec reconciliation.**
