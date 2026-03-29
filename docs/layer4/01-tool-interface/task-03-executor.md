# Task 03: Executor — Purity-Based Dispatch

**Epic:** 01 — Tool Interface, Registry & Executor
**Status:** ⬚ Not started
**Dependencies:** Task 01, Task 02, Layer 0 Epic 04 (SQLite), Layer 0 Epic 06 (sqlc queries)

---

## Description

Implement the `Executor` in `internal/tool/` that receives a batch of `ToolCall` values from the agent loop, partitions them by purity, executes pure calls concurrently and mutating calls sequentially, records analytics in the `tool_executions` table, and returns results in the original call order. The executor is the single entry point for all tool dispatch — Layer 5 never calls tools directly.

## Acceptance Criteria

- [ ] `Executor` struct with constructor accepting a `*Registry`, a database connection (for `tool_executions` persistence), and config (for truncation limits)
- [ ] `Execute(ctx context.Context, projectRoot string, conversationID string, turnNumber int, iteration int, calls []ToolCall) []ToolResult` method
- [ ] Partitions incoming calls by purity: looks up each tool in the registry, groups into pure and mutating batches
- [ ] Pure calls execute concurrently using goroutines and a `sync.WaitGroup`. Each goroutine calls `tool.Execute()` and stores the result.
- [ ] Mutating calls execute sequentially in the order they appear in the input `calls` slice
- [ ] Results are returned in the original call order (matching the order of the input `calls` slice), regardless of which calls were pure vs mutating or which goroutines finished first
- [ ] If a tool name is not found in the registry, the result for that call is a `ToolResult` with `Success=false` and content `"Unknown tool: <name>. Available tools: <list>"`
- [ ] Tool execution errors (panics, unexpected errors from `Execute()`) are caught via recover and returned as `ToolResult` with `Success=false` — never propagated as Go errors to the caller
- [ ] Context cancellation is propagated: when `ctx` is cancelled, in-flight pure goroutines receive the cancelled context, and the sequential mutating chain stops before starting the next call
- [ ] Each tool call's duration is measured (start/end timestamps) and recorded in `DurationMs` on the result
- [ ] `tool_executions` row inserted for every call: tool_use_id, tool_name, input JSON, output_size, error, success, duration_ms, conversation_id, turn_number, iteration
- [ ] Database write failures are logged but do not fail the tool execution (analytics are best-effort)
