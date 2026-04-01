# Layer 5 Audit: Agent Loop

## Scope

Layer 5 is the orchestration engine: the turn state machine that receives a user
message, assembles context via Layer 3, calls the LLM via Layer 2, dispatches
tools via Layer 4, iterates until the turn is complete, and persists everything.
It owns the agent loop runtime, event system, conversation persistence, and
system prompt construction.

## Spec References

- `docs/specs/05-agent-loop.md` — Full architecture
- `docs/layer5/layer-5-overview.md` — Epic index (4 epics: 01, 02, 05, 06)
- `docs/layer5/01-event-system-session-types/` — Event system specs
- `docs/layer5/02-conversation-manager/` — Conversation persistence specs
- `docs/layer5/05-system-prompt-builder/` — System prompt specs
- `docs/layer5/06-agent-loop-core/` — Core loop specs

## Packages to Audit

| Package | Src | Test | Purpose |
|---------|-----|------|---------| 
| `internal/agent` | 12 | 13 | Agent loop, events, state, prompt, stream |
| `internal/conversation` | 4 | 3 | Conversation/history manager, title gen, seen tracking |

Key source files in `internal/agent/`:
- `types.go`, `events.go`, `eventsink.go`, `state.go` — Event system (Epic 01)
- `loop.go` — Core agent loop (Epic 06, 1043 LOC — largest file in the project)
- `prompt.go` — System prompt builder (Epic 05, 307 LOC)
- `stream.go` — Streaming response handler
- `compression.go` — Delegates to Layer 3 compression
- `retry.go` — Retry logic with backoff
- `loopdetect.go` — Detects repetitive tool call patterns
- `errors.go` — Error classification

Key source files in `internal/conversation/`:
- `manager.go` — Conversation CRUD, project association
- `history.go` — Message persistence, iteration management, history reconstruction
- `title.go` — Async conversation title generation
- `seen.go` — Seen/unseen tracking for UI

## Test Commands

```bash
CGO_ENABLED=1 CGO_LDFLAGS="-L$(pwd)/lib/linux_amd64 -llancedb_go -lm -ldl -lpthread" \
  LD_LIBRARY_PATH="$(pwd)/lib/linux_amd64" \
  go test -tags 'sqlite_fts5' ./internal/agent/...

CGO_ENABLED=1 CGO_LDFLAGS="..." go test -tags 'sqlite_fts5' ./internal/conversation/...
```

## Audit Checklist

**Audited:** 2026-04-01 | **Result:** Clean — 3 code issues fixed, 5 informational items deferred to TECH-DEBT.

All 4 epics pass. Three issues found during audit were fixed in the same session:
1. ExtendedThinking wired through BuildPrompt to provider.Request ProviderOptions
2. MaxIterations=0 now means unlimited (for testing) per spec
3. Nil-checks added for ProviderRouter and ToolExecutor in RunTurn

Race detector clean. All tests pass across 13 test files (internal/agent) + 3 test files (internal/conversation).

### Epic 01: Event System & Session Types
- [x] `events.go` — Event types enumerated (12 types per spec):
  - token, thinking_start, thinking_delta, thinking_end, tool_call_start,
    tool_call_output, tool_call_end, turn_complete, turn_cancelled, error,
    status, context_debug
- [x] Event interface: `EventType() string`, `Timestamp() time.Time`
  - Concrete event structs carry type-specific fields (not a flat Event struct)
- [x] `eventsink.go` — `ChannelSink` for non-blocking event emission
  - Emit is non-blocking (drops if channel full, logs warning)
  - Close is safe to call multiple times (mutex + closed bool)
  - Default buffer size: 256
- [x] `MultiSink` fans out events to multiple sinks
  - Add, Remove, Emit (thread-safe via sync.RWMutex)
  - Dedup on Add, auto-close on post-Close Add
- [x] `state.go` — Agent states: idle, assembling_context, waiting_for_llm,
  executing_tools, compressing
- [x] State transitions documented via godoc (no runtime enforcement — spec
  does not require it)
- [x] Test covers: event creation (compile-time interface checks), sink
  emit/close/drop, multi-sink fanout/remove

### Epic 02: Conversation Manager
- [x] `internal/conversation/manager.go` — `Manager` struct
  - Create, Get, List, Delete conversations
  - Conversations scoped to projects
  - UpdatedAt timestamp maintained
- [x] `internal/conversation/history.go` — `HistoryManager`
  - `PersistUserMessage(ctx, convID, turn, content)` — hardcodes iteration=1
  - `PersistIteration(ctx, convID, turn, iteration, messages)` — stores assistant+tool messages
  - `CancelIteration(ctx, convID, turn, iteration)` — deletes iteration messages
  - **CRITICAL (TECH-DEBT item 3 — resolved)**: DeleteIterationMessages SQL has
    `AND role != 'user'` guard so CancelIteration never deletes user messages
  - `ReconstructHistory(ctx, convID)` — rebuilds message list excluding compressed rows
  - **NOTE**: PersistIteration inserts message rows only — tool_execution and
    sub_call records persisted separately by agent loop (see TECH-DEBT)
- [x] `internal/conversation/title.go` — async title generation via LLM
  - Fires on first turn only
  - Non-blocking (goroutine-safe design, caller does `go gen.GenerateTitle(...)`)
  - TitleGen struct-based (better encapsulation than spec's standalone function)
- [x] `internal/conversation/seen.go` — message seen tracking for UI
  - Thread-safe via sync.RWMutex, path normalization via filepath.Clean
  - Implements contextpkg.SeenFileLookup interface
- [x] Test covers:
  - Conversation CRUD lifecycle
  - PersistUserMessage + PersistIteration + CancelIteration
  - CancelIteration preserves user messages (iteration namespace fix verified)
  - CancelIteration is no-op for nonexistent iterations
  - CancelIteration deletes tool_executions and sub_calls too
  - History reconstruction with compressed/summary messages

### Epic 05: System Prompt Builder
- [x] `internal/agent/prompt.go` — builds the system prompt from:
  - Base persona/instructions (Block 1)
  - Assembled context from Layer 3 FullContextPackage (Block 2)
  - Conversation history prefix (Block 3)
  - Current turn messages (fresh, uncached)
- [x] Tool definitions passed through to Request.Tools (omitted when DisableTools=true)
- [x] Context package content serialized as system block text
- [x] Cache markers (ephemeral) placed on Anthropic system blocks
  - Block 3 cache marker on history messages is a **design placeholder** — see TECH-DEBT
- [x] Extended thinking wired through ProviderOptions for Anthropic (**fixed in this audit**)
- [x] Phase 2 history compression (CompressHistoricalResults) supported
- [x] Test covers: prompt with tools, prompt with context, prompt without context,
  cache marker behavior per provider, tool disable, history stability, Phase 2
  compression, extended thinking wiring

### Epic 06: Agent Loop Core
- [x] `internal/agent/loop.go` — `AgentLoop` struct (1043 LOC)
  - `RunTurn(ctx, request)` — main entry point
  - Turn lifecycle: persist user msg → assemble context → build prompt → stream LLM → dispatch tools → iterate → persist
- [x] Iteration logic:
  - If LLM returns tool calls: execute tools, add results, call LLM again
  - If LLM returns text (end_turn): turn is complete
  - Max iterations enforced (configurable, default 50)
  - MaxIterations=0 means unlimited (**fixed in this audit**)
  - Final iteration injects directive to conclude (tools disabled)
- [x] Compression integration:
  - Preflight compression check before LLM call
  - Post-response compression check after LLM response
  - Emergency compression on context_length_exceeded (413/400) errors
  - Compression failure is non-fatal (logs warning, continues)
- [x] Loop detection (`loopdetect.go`):
  - Detects repetitive tool call patterns (JSON-canonicalized comparison)
  - Injects nudge message to break out of loops
  - Threshold-based (default 3, configurable)
- [x] Error classification (`errors.go`):
  - Rate limit (429), auth failure (401/403), context overflow (400), server error (5xx)
  - Context overflow triggers compression retry
  - Malformed tool calls get synthetic error result with correction guidance
  - Tool errors enriched with contextual hints
- [x] Streaming (`stream.go`):
  - Parses all stream event types from provider
  - Emits content_delta/thinking events to sink (no buffering)
  - Assembles complete response with ordered ContentBlocks
- [x] Retry (`retry.go`):
  - Retries on transient errors (rate limit, server error)
  - Exponential backoff (1s, 2s, 4s — 3 attempts)
  - No retry on auth failures or permanent errors
  - **NOTE**: Retry-After header not respected — see TECH-DEBT
- [x] Nil-checks for ProviderRouter and ToolExecutor in RunTurn (**fixed in this audit**)
- [x] Event emission throughout the loop:
  - StatusEvent at each state transition (assembling_context, waiting_for_llm,
    executing_tools, compressing, idle)
  - TokenEvent for each text chunk
  - ThinkingStart/Delta/End events for thinking blocks
  - ToolCallStart/Output/End events with ToolCallID
  - ContextDebugEvent with ContextAssemblyReport (conditional on EmitContextDebug)
  - TurnCompleteEvent with turn number, iteration count, tokens, duration
  - TurnCancelledEvent with reason
  - ErrorEvent for all error paths with Recoverable flag
  - StatusEvent(StateIdle) after completion or cancellation
- [x] Test covers (13 test files — comprehensive):
  - Text-only response, tool use response, multi-iteration
  - Stream error handling
  - Compression triggers (preflight, post-response, emergency, graceful failure)
  - Loop detection and nudge injection
  - Event ordering verification
  - Cancellation handling (during stream, during tools, ctx deadline, idempotent,
    preserves completed iterations)
  - Context debug emission (enabled/disabled)
  - Title generation on first turn (and not on subsequent turns)
  - Real conversation history manager integration (sqlite_fts5 build tag)
  - Nil dependency checks (provider router, tool executor)
  - MaxIterations=0 unlimited mode

### Cross-cutting
- [x] `go test -race ./internal/agent/...` — no data races
- [x] `go test -race ./internal/conversation/...` — no data races
- [x] Context cancellation stops the loop promptly
- [x] No goroutine leaks after turn completion
- [x] Event sink never blocks the loop (non-blocking emit)
- [x] All database operations use transactions where appropriate
