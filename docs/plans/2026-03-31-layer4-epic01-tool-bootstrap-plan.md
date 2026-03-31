# Layer 4 Epic 01: Tool Interface, Registry & Executor

> For Hermes: execute task-by-task using strict TDD, one local commit per task group, no push until all green.

**Goal:** Build the foundational tool infrastructure in `internal/tool/` — the Tool interface, purity classification, Registry, batch Executor with purity-based parallel/sequential dispatch, output truncation, and tool_executions persistence.

**Architecture:**
- New package `internal/tool/` — zero coupling to `internal/agent/` at this layer.
- Tool types are tool-layer types, NOT provider types. Conversion helpers bridge `provider.ToolCall`/`provider.ToolResult` to `tool.ToolCall`/`tool.ToolResult`.
- The Executor receives batches, partitions by purity, runs pure calls concurrently (goroutines + WaitGroup), runs mutating calls sequentially, returns results in original call order.
- After all tasks are done, an adapter satisfies the existing `agent.ToolExecutor` interface so the agent loop can use the new executor without refactoring.

**Tech stack:** Go, `internal/tool/`, `internal/db/` (sqlc for tool_executions), `internal/config/` (ToolOutputMaxTokens).

---

## Type boundary analysis

The agent loop currently defines:
```go
// internal/agent/loop.go
type ToolExecutor interface {
    Execute(ctx context.Context, call provider.ToolCall) (*provider.ToolResult, error)
}
```

The provider layer already defines:
```go
// internal/provider/types.go
type ToolCall struct {
    ID    string          `json:"id"`
    Name  string          `json:"name"`
    Input json.RawMessage `json:"input"`
}

type ToolResult struct {
    ToolUseID string `json:"tool_use_id"`
    Content   string `json:"content"`
    IsError   bool   `json:"is_error,omitempty"`
}
```

The epic spec calls for richer tool-layer types:
- `tool.ToolCall` — same shape as `provider.ToolCall`, aliased fields (Arguments vs Input)
- `tool.ToolResult` — richer: adds `Success bool`, `Error string`, `DurationMs int64`
- The executor's batch interface: `Execute(ctx, projectRoot, conversationID, turnNumber, iteration, calls []ToolCall) []ToolResult`

**Decision:** Define tool-layer types. Provide `ToolCallFromProvider(provider.ToolCall) ToolCall` and `(ToolResult).ToProvider() provider.ToolResult` conversion helpers. This keeps the tool layer self-contained and the agent loop unmodified until we're ready to refactor it for batch dispatch.

---

## Task 1: Purity enum and core types

**Objective:** Define `Purity`, `Tool` interface, `ToolCall`, `ToolResult`, and conversion helpers in `internal/tool/types.go`.

**Files:**
- Create: `internal/tool/types.go`

**What to implement:**

```go
package tool

import (
    "context"
    "encoding/json"

    "github.com/ponchione/sirtopham/internal/provider"
)

// Purity classifies a tool's side-effect behavior for dispatch purposes.
type Purity int

const (
    Pure     Purity = iota // Read-only, safe to run concurrently
    Mutating               // Has side effects, must run sequentially
)

func (p Purity) String() string {
    switch p {
    case Pure:
        return "pure"
    case Mutating:
        return "mutating"
    default:
        return "unknown"
    }
}

// Tool is the interface every tool implementation must satisfy.
type Tool interface {
    // Name returns the tool identifier (e.g., "file_read").
    Name() string

    // Description returns a one-line description for the LLM.
    Description() string

    // Purity declares whether the tool is Pure (read-only) or Mutating.
    Purity() Purity

    // Schema returns the JSON Schema definition of the tool's parameters.
    // Format follows the Anthropic/OpenAI function calling convention:
    //   {"name": "...", "description": "...", "input_schema": {...}}
    Schema() json.RawMessage

    // Execute runs the tool. projectRoot restricts file operations.
    // Returns a ToolResult on success or an error for infrastructure failures.
    // Tool-level failures (file not found, command failed) should be returned
    // as ToolResult with Success=false, NOT as Go errors.
    Execute(ctx context.Context, projectRoot string, input json.RawMessage) (*ToolResult, error)
}

// ToolCall represents an inbound tool invocation from the LLM.
type ToolCall struct {
    ID        string          `json:"id"`
    Name      string          `json:"name"`
    Arguments json.RawMessage `json:"arguments"`
}

// ToolCallFromProvider converts a provider.ToolCall to a tool.ToolCall.
func ToolCallFromProvider(pc provider.ToolCall) ToolCall {
    return ToolCall{
        ID:        pc.ID,
        Name:      pc.Name,
        Arguments: pc.Input,
    }
}

// ToolResult is the outcome of a tool execution.
type ToolResult struct {
    CallID     string `json:"call_id"`
    Content    string `json:"content"`
    Success    bool   `json:"success"`
    Error      string `json:"error,omitempty"`
    DurationMs int64  `json:"duration_ms"`
}

// ToProvider converts a tool.ToolResult to a provider.ToolResult.
func (r ToolResult) ToProvider() provider.ToolResult {
    return provider.ToolResult{
        ToolUseID: r.CallID,
        Content:   r.Content,
        IsError:   !r.Success,
    }
}
```

**Verify:**
```bash
go build ./internal/tool/...
```

---

## Task 2: Registry

**Objective:** Implement `Registry` with Register, Get, All, Schemas methods in `internal/tool/registry.go`.

**Files:**
- Create: `internal/tool/registry.go`
- Create: `internal/tool/registry_test.go`

**What to implement:**

```go
package tool

import (
    "encoding/json"
    "fmt"
    "sort"
)

// Registry holds all registered tools and provides lookup and enumeration.
// Tools are registered at startup (single-threaded). After initialization,
// the registry is read-only — no mutex needed for concurrent reads.
type Registry struct {
    tools map[string]Tool
    order []string // insertion order for stable All()
}

// NewRegistry creates an empty tool registry.
func NewRegistry() *Registry {
    return &Registry{
        tools: make(map[string]Tool),
    }
}

// Register adds a tool to the registry. Panics on duplicate names
// (fail-fast at startup, not runtime).
func (r *Registry) Register(t Tool) {
    name := t.Name()
    if _, exists := r.tools[name]; exists {
        panic(fmt.Sprintf("tool: duplicate registration for %q", name))
    }
    r.tools[name] = t
    r.order = append(r.order, name)
}

// Get returns the tool with the given name, or (nil, false) if not found.
func (r *Registry) Get(name string) (Tool, bool) {
    t, ok := r.tools[name]
    return t, ok
}

// All returns all registered tools in insertion order.
func (r *Registry) All() []Tool {
    result := make([]Tool, 0, len(r.order))
    for _, name := range r.order {
        result = append(result, r.tools[name])
    }
    return result
}

// Names returns all registered tool names sorted alphabetically.
func (r *Registry) Names() []string {
    names := make([]string, len(r.order))
    copy(names, r.order)
    sort.Strings(names)
    return names
}

// Schemas returns the JSON Schema definitions from all registered tools,
// suitable for injection into LLM API requests.
func (r *Registry) Schemas() []json.RawMessage {
    result := make([]json.RawMessage, 0, len(r.order))
    for _, name := range r.order {
        result = append(result, r.tools[name].Schema())
    }
    return result
}
```

**Test cases (registry_test.go):**
1. Register and Get — register a mock tool, retrieve by name, verify identity
2. Duplicate panics — register two tools with the same name, assert panic
3. Get unknown — `Get("nonexistent")` returns `(nil, false)`
4. All — register multiple tools, verify All() returns all in insertion order
5. Schemas — register multiple tools, verify Schemas() returns all schemas
6. Names — verify Names() returns sorted list

**Verify:**
```bash
go test ./internal/tool/... -v -run TestRegistry
go build ./internal/tool/...
```

---

## Task 3: Executor core — purity-based batch dispatch

**Objective:** Implement the `Executor` that receives a batch of ToolCalls, partitions by purity, runs pure calls concurrently and mutating calls sequentially, returns results in original order.

**Files:**
- Create: `internal/tool/executor.go`

**What to implement:**

```go
package tool

import (
    "context"
    "fmt"
    "log/slog"
    "strings"
    "sync"
    "time"
)

// ExecutorConfig carries executor-level configuration.
type ExecutorConfig struct {
    // MaxOutputTokens is the global truncation limit (chars / 4 approximation).
    // Default: 50000.
    MaxOutputTokens int

    // ProjectRoot restricts file tool operations to this directory.
    ProjectRoot string
}

// Executor dispatches tool call batches with purity-based execution strategy.
// It is the single entry point for all tool dispatch — the agent loop never
// calls tools directly.
type Executor struct {
    registry *Registry
    config   ExecutorConfig
    logger   *slog.Logger
    nowFn    func() time.Time // injectable for testing
}

// NewExecutor creates an executor backed by the given registry.
func NewExecutor(registry *Registry, config ExecutorConfig, logger *slog.Logger) *Executor {
    if logger == nil {
        logger = slog.Default()
    }
    return &Executor{
        registry: registry,
        config:   config,
        logger:   logger,
        nowFn:    time.Now,
    }
}

// Execute dispatches a batch of tool calls with purity-based strategy:
// 1. Partition calls into pure and mutating.
// 2. Execute all pure calls concurrently (goroutines + WaitGroup).
// 3. Execute mutating calls sequentially in input order.
// 4. Return results in the original call order.
//
// Tool execution errors are caught and returned as failed ToolResult values —
// they never propagate as Go errors. Only infrastructure failures (e.g., panic
// recovery) produce error results.
func (e *Executor) Execute(ctx context.Context, calls []ToolCall) []ToolResult {
    results := make([]ToolResult, len(calls))

    // Index calls by position and partition by purity.
    type indexedCall struct {
        index int
        call  ToolCall
        tool  Tool
    }
    var pureCalls, mutatingCalls []indexedCall

    for i, call := range calls {
        t, ok := e.registry.Get(call.Name)
        if !ok {
            results[i] = ToolResult{
                CallID:  call.ID,
                Content: fmt.Sprintf("Unknown tool: %q. Available tools: %s", call.Name, strings.Join(e.registry.Names(), ", ")),
                Success: false,
                Error:   "unknown tool",
            }
            continue
        }
        ic := indexedCall{index: i, call: call, tool: t}
        if t.Purity() == Pure {
            pureCalls = append(pureCalls, ic)
        } else {
            mutatingCalls = append(mutatingCalls, ic)
        }
    }

    // Execute pure calls concurrently.
    var wg sync.WaitGroup
    for _, ic := range pureCalls {
        wg.Add(1)
        go func(ic indexedCall) {
            defer wg.Done()
            results[ic.index] = e.executeSingle(ctx, ic.call, ic.tool)
        }(ic)
    }
    wg.Wait()

    // Execute mutating calls sequentially.
    for _, ic := range mutatingCalls {
        if ctx.Err() != nil {
            results[ic.index] = ToolResult{
                CallID:  ic.call.ID,
                Content: "Tool execution cancelled",
                Success: false,
                Error:   ctx.Err().Error(),
            }
            continue
        }
        results[ic.index] = e.executeSingle(ctx, ic.call, ic.tool)
    }

    return results
}

// executeSingle runs a single tool call with panic recovery and timing.
func (e *Executor) executeSingle(ctx context.Context, call ToolCall, t Tool) (result ToolResult) {
    start := e.nowFn()

    // Panic recovery — tool panics become failed results, not crashes.
    defer func() {
        if r := recover(); r != nil {
            result = ToolResult{
                CallID:     call.ID,
                Content:    fmt.Sprintf("Tool %q panicked: %v", call.Name, r),
                Success:    false,
                Error:      fmt.Sprintf("panic: %v", r),
                DurationMs: e.nowFn().Sub(start).Milliseconds(),
            }
            e.logger.Error("tool panic recovered",
                "tool", call.Name,
                "call_id", call.ID,
                "panic", r,
            )
        }
    }()

    tr, err := t.Execute(ctx, e.config.ProjectRoot, call.Arguments)
    duration := e.nowFn().Sub(start)

    if err != nil {
        return ToolResult{
            CallID:     call.ID,
            Content:    fmt.Sprintf("Tool %q failed: %v", call.Name, err),
            Success:    false,
            Error:      err.Error(),
            DurationMs: duration.Milliseconds(),
        }
    }

    tr.CallID = call.ID
    tr.DurationMs = duration.Milliseconds()
    return *tr
}
```

**Key design notes:**
- Pure calls: goroutines write to pre-allocated `results[i]` — no slice append race
- Mutating calls: check `ctx.Err()` before each — stops the chain on cancellation
- Unknown tools: result with Success=false and available tool list
- Panics: caught via `defer recover()`, returned as failed ToolResult
- Results always in original call order (indexed writes)

**Verify:**
```bash
go build ./internal/tool/...
```

---

## Task 4: Output truncation

**Objective:** Add post-execution output truncation to the executor. Results exceeding the configured limit are truncated with a helpful notice.

**Files:**
- Create: `internal/tool/truncate.go`
- Modify: `internal/tool/executor.go` (call truncation after executeSingle)

**What to implement in truncate.go:**

```go
package tool

import (
    "fmt"
    "strings"
)

const defaultMaxOutputTokens = 50000

// truncateResult checks if a tool result's content exceeds the token limit
// and truncates it with a helpful notice if so.
// Token estimation uses chars/4 as a rough heuristic.
func truncateResult(result *ToolResult, maxTokens int, toolName string) {
    if maxTokens <= 0 {
        maxTokens = defaultMaxOutputTokens
    }
    maxChars := maxTokens * 4

    if len(result.Content) <= maxChars {
        return
    }

    // Count total lines for the notice.
    totalLines := strings.Count(result.Content, "\n") + 1

    // Truncate to maxChars, then find the last newline to avoid mid-line cut.
    truncated := result.Content[:maxChars]
    if lastNL := strings.LastIndex(truncated, "\n"); lastNL > 0 {
        truncated = truncated[:lastNL]
    }
    shownLines := strings.Count(truncated, "\n") + 1

    notice := truncationNotice(toolName, shownLines, totalLines)
    result.Content = truncated + "\n" + notice
}

// truncationNotice returns a contextually appropriate truncation message.
func truncationNotice(toolName string, shownLines, totalLines int) string {
    switch toolName {
    case "file_read":
        return fmt.Sprintf("[Output truncated — showing first %d lines of %d. Use file_read with line_start/line_end for specific sections.]", shownLines, totalLines)
    case "search_text", "search_semantic":
        return fmt.Sprintf("[Output truncated — showing first %d lines of %d. Try a more specific query to narrow results.]", shownLines, totalLines)
    case "git_diff":
        return fmt.Sprintf("[Output truncated — showing first %d lines of %d. Use a path filter to narrow the diff.]", shownLines, totalLines)
    case "shell":
        return fmt.Sprintf("[Output truncated — showing first %d lines of %d. Consider piping to head/tail or grep for specific output.]", shownLines, totalLines)
    default:
        return fmt.Sprintf("[Output truncated — showing first %d lines of %d.]", shownLines, totalLines)
    }
}
```

**Modification to executor.go — add truncation call after executeSingle:**

In `Execute()`, after both pure and mutating calls complete, loop through results and apply truncation:

```go
// Apply output truncation.
for i := range results {
    if results[i].Success {
        truncateResult(&results[i], e.config.MaxOutputTokens, calls[i].Name)
    }
}
```

**Verify:**
```bash
go build ./internal/tool/...
```

---

## Task 5: sqlc query for InsertToolExecution

**Objective:** Add the `InsertToolExecution` sqlc query so the executor can persist analytics rows.

**Files:**
- Modify: `internal/db/query/analytics.sql` (add InsertToolExecution query)
- Regenerate: `internal/db/analytics.sql.go` (via `sqlc generate`)

**SQL to add to analytics.sql:**

```sql
-- name: InsertToolExecution :exec
INSERT INTO tool_executions (
    conversation_id, turn_number, iteration,
    tool_use_id, tool_name, input,
    output_size, error, success,
    duration_ms, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
```

**Steps:**
1. Add the query to `internal/db/query/analytics.sql`
2. Run `sqlc generate` from the project root
3. Verify the generated `InsertToolExecution` function exists in `internal/db/analytics.sql.go`

**Verify:**
```bash
sqlc generate
go build ./internal/db/...
```

---

## Task 6: tool_executions persistence in executor

**Objective:** After each tool execution, insert a `tool_executions` row for analytics. Database write failures are logged but do not fail the tool execution.

**Files:**
- Modify: `internal/tool/executor.go` (add DB dependency, persist after each call)

**Changes to Executor:**

```go
// Add to ExecutorConfig or Executor fields:
type ExecutorDeps struct {
    Registry *Registry
    DB       *db.Queries      // nil means no persistence (testing)
    Config   ExecutorConfig
    Logger   *slog.Logger
}

// In executeSingle, after computing the result:
func (e *Executor) recordExecution(ctx context.Context, call ToolCall, result ToolResult, meta ExecutionMeta) {
    if e.db == nil {
        return
    }
    err := e.db.InsertToolExecution(ctx, db.InsertToolExecutionParams{
        ConversationID: meta.ConversationID,
        TurnNumber:     int64(meta.TurnNumber),
        Iteration:      int64(meta.Iteration),
        ToolUseID:      call.ID,
        ToolName:       call.Name,
        Input:          string(call.Arguments),
        OutputSize:     sql.NullInt64{Int64: int64(len(result.Content)), Valid: true},
        Error:          nullString(result.Error),
        Success:        boolToInt(result.Success),
        DurationMs:     result.DurationMs,
        CreatedAt:      e.nowFn().UTC().Format(time.RFC3339),
    })
    if err != nil {
        e.logger.Warn("failed to record tool execution",
            "tool", call.Name,
            "call_id", call.ID,
            "error", err,
        )
    }
}

// ExecutionMeta carries the conversation context for persistence.
type ExecutionMeta struct {
    ConversationID string
    TurnNumber     int
    Iteration      int
}
```

**Update Execute signature to accept meta:**

The batch `Execute` gains an optional `ExecutionMeta` parameter. To avoid breaking the core dispatch interface, add a separate method:

```go
// ExecuteWithMeta dispatches tools and records analytics.
func (e *Executor) ExecuteWithMeta(ctx context.Context, calls []ToolCall, meta ExecutionMeta) []ToolResult {
    results := e.Execute(ctx, calls)
    for i, call := range calls {
        e.recordExecution(ctx, call, results[i], meta)
    }
    return results
}
```

**Verify:**
```bash
go build ./internal/tool/...
```

---

## Task 7: Agent loop adapter

**Objective:** Build an adapter that wraps `tool.Executor` to satisfy the existing `agent.ToolExecutor` interface, so the agent loop can use the new tool system without refactoring its dispatch loop.

**Files:**
- Create: `internal/tool/adapter.go`

**What to implement:**

```go
package tool

import (
    "context"

    "github.com/ponchione/sirtopham/internal/provider"
)

// AgentLoopAdapter wraps an Executor to satisfy the agent.ToolExecutor interface.
// It dispatches a single tool call at a time (the agent loop handles batching
// and event emission). This adapter exists as a bridge until the agent loop
// is refactored to use batch dispatch.
type AgentLoopAdapter struct {
    executor *Executor
}

// NewAgentLoopAdapter creates an adapter for the given executor.
func NewAgentLoopAdapter(executor *Executor) *AgentLoopAdapter {
    return &AgentLoopAdapter{executor: executor}
}

// Execute satisfies the agent.ToolExecutor interface.
// It converts the provider types, dispatches a single-element batch,
// and converts the result back.
func (a *AgentLoopAdapter) Execute(ctx context.Context, call provider.ToolCall) (*provider.ToolResult, error) {
    toolCall := ToolCallFromProvider(call)
    results := a.executor.Execute(ctx, []ToolCall{toolCall})
    if len(results) == 0 {
        return nil, fmt.Errorf("tool executor returned no results for %q", call.Name)
    }
    pr := results[0].ToProvider()
    return &pr, nil
}
```

**Verify:**
```bash
go build ./internal/tool/...
```

---

## Task 8: Comprehensive tests

**Objective:** Write full test coverage for registry, executor, truncation, and adapter. Include an integration test with mock tools.

**Files:**
- Create or extend: `internal/tool/registry_test.go`
- Create: `internal/tool/executor_test.go`
- Create: `internal/tool/truncate_test.go`
- Create: `internal/tool/adapter_test.go`

**Mock tool for tests:**

```go
// mockTool is a configurable test double.
type mockTool struct {
    name        string
    description string
    purity      Purity
    schema      json.RawMessage
    executeFn   func(ctx context.Context, projectRoot string, input json.RawMessage) (*ToolResult, error)
}

func (m *mockTool) Name() string                { return m.name }
func (m *mockTool) Description() string          { return m.description }
func (m *mockTool) Purity() Purity               { return m.purity }
func (m *mockTool) Schema() json.RawMessage       { return m.schema }
func (m *mockTool) Execute(ctx context.Context, projectRoot string, input json.RawMessage) (*ToolResult, error) {
    return m.executeFn(ctx, projectRoot, input)
}
```

**Registry test cases (registry_test.go):**
1. `TestRegistryRegisterAndGet` — register, retrieve, verify identity
2. `TestRegistryDuplicatePanics` — assert panic on duplicate name
3. `TestRegistryGetUnknown` — returns (nil, false)
4. `TestRegistryAll` — multiple tools, insertion order preserved
5. `TestRegistrySchemas` — returns all schemas
6. `TestRegistryNames` — sorted alphabetically

**Executor test cases (executor_test.go):**
1. `TestExecutorPureCallsConcurrent` — two pure tools that signal via channels to prove concurrent execution
2. `TestExecutorMutatingCallsSequential` — two mutating tools recording execution order via shared slice
3. `TestExecutorMixedBatchOrdering` — 2 pure + 2 mutating, verify results in original call order
4. `TestExecutorUnknownTool` — dispatches unknown tool, verifies Success=false with available tools list
5. `TestExecutorPanicRecovery` — tool that panics, verify recovered result with Success=false
6. `TestExecutorContextCancellation` — cancelled context, verify mutating chain stops
7. `TestExecutorEmptyBatch` — empty calls slice returns empty results

**Truncation test cases (truncate_test.go):**
1. `TestTruncateResultBelowLimit` — content below limit passes through unchanged
2. `TestTruncateResultAboveLimit` — content above limit is truncated with notice
3. `TestTruncateResultToolSpecificNotice` — file_read, search_text, shell, git_diff each get correct notice text
4. `TestTruncateResultZeroLimit` — zero limit uses default

**Adapter test cases (adapter_test.go):**
1. `TestAdapterConvertsTypes` — verify provider.ToolCall -> tool.ToolCall conversion and result back
2. `TestAdapterUnknownTool` — unknown tool returns provider.ToolResult with IsError=true

**Integration test (executor_integration_test.go):**
1. Register one pure and one mutating mock tool
2. Dispatch a mixed batch of 3 calls (2 pure, 1 mutating)
3. Verify execution order: pure calls overlapped, mutating ran last
4. Verify result order matches input order
5. Verify all ToolResult fields populated correctly

**Verify:**
```bash
go test ./internal/tool/... -v
go test -race ./internal/tool/...
go build ./internal/tool/...
```

---

## Deliberate non-goals

- No concrete tool implementations (file_read, shell, etc.) — those are Epics 02-06
- No Phase 1 tool result normalization (JSON minification, ANSI stripping) — that comes with Epic 05
- No agent loop refactor for batch dispatch — the adapter bridges the gap
- No tool_executions persistence integration test (requires SQLite test harness — deferred)
- No Retry-After or fallback routing
- No changes to `internal/agent/` in this epic

---

## Commit strategy

Given this is infrastructure, two commits make sense:

1. `feat(tool): add tool types, registry, executor with purity dispatch`
   - types.go, registry.go, executor.go, truncate.go, adapter.go
   - registry_test.go, executor_test.go, truncate_test.go, adapter_test.go
   - All tests green

2. `feat(tool): add InsertToolExecution sqlc query and executor persistence`
   - analytics.sql update, sqlc regeneration, executor persistence wiring
   - Only if sqlc is available and the generated code compiles cleanly

If sqlc is not installed, Task 5+6 are deferred to a follow-up session. Tasks 1-4 + 7-8 are the core deliverable.

---

## Verification checklist

```bash
go test ./internal/tool/... -v                                    # all tests pass
go test -race ./internal/tool/...                                 # no races
go build ./internal/tool/...                                      # compiles
go build ./internal/agent/...                                     # agent still compiles (no changes)
go test ./internal/agent/...                                      # agent tests still green
go vet ./internal/tool/...                                        # no vet issues
```

---

## Files created/modified summary

| File | Action | Purpose |
|------|--------|---------|
| `internal/tool/types.go` | Create | Purity, Tool, ToolCall, ToolResult, conversions |
| `internal/tool/registry.go` | Create | Registry with Register/Get/All/Schemas |
| `internal/tool/executor.go` | Create | Batch executor with purity dispatch |
| `internal/tool/truncate.go` | Create | Output truncation with tool-specific notices |
| `internal/tool/adapter.go` | Create | AgentLoopAdapter bridging to agent.ToolExecutor |
| `internal/tool/registry_test.go` | Create | 6 registry tests |
| `internal/tool/executor_test.go` | Create | 7+ executor tests + integration |
| `internal/tool/truncate_test.go` | Create | 4 truncation tests |
| `internal/tool/adapter_test.go` | Create | 2 adapter tests |
| `internal/db/query/analytics.sql` | Modify | Add InsertToolExecution query |
| `internal/db/analytics.sql.go` | Regenerate | sqlc output |
