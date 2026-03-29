# Task 05: Unit Tests and Integration Test

**Epic:** 01 — Tool Interface, Registry & Executor
**Status:** ⬚ Not started
**Dependencies:** Task 01, Task 02, Task 03, Task 04

---

## Description

Write comprehensive unit tests for the registry and executor, plus an integration test that registers mock tools and dispatches a mixed batch of pure and mutating calls to verify the full dispatch pipeline end-to-end. These tests validate the correctness of purity-based partitioning, concurrent execution, result ordering, output truncation, cancellation, and error handling.

## Acceptance Criteria

- [ ] All tests in `internal/tool/` package (e.g., `registry_test.go`, `executor_test.go`)
- [ ] All tests pass via `go test ./internal/tool/...`
- [ ] **Registry — register and get:** Register a mock tool, retrieve by name, verify identity
- [ ] **Registry — duplicate panics:** Assert that registering two tools with the same name panics
- [ ] **Registry — get unknown:** Verify `Get("nonexistent")` returns `(nil, false)`
- [ ] **Registry — All and Schemas:** Register multiple tools, verify `All()` returns all and `Schemas()` returns all schemas
- [ ] **Executor — pure calls run concurrently:** Register two mock pure tools that record their execution goroutine ID or use a shared channel to prove they overlap in time. Dispatch both, verify both executed.
- [ ] **Executor — mutating calls run sequentially:** Register two mock mutating tools that record their execution order via a shared slice. Dispatch both, verify they executed in input order (second started after first finished).
- [ ] **Executor — mixed batch ordering:** Dispatch a batch with 2 pure and 2 mutating calls. Verify results are returned in the original call order (not grouped by purity).
- [ ] **Executor — unknown tool:** Dispatch a call for a tool not in the registry. Verify result has `Success=false` and lists available tools.
- [ ] **Executor — tool panic recovery:** Register a mock tool whose `Execute` panics. Dispatch it. Verify the executor returns a `ToolResult` with `Success=false` and an error message mentioning the panic, without crashing.
- [ ] **Executor — context cancellation:** Create a context that is cancelled after a short delay. Dispatch a slow mock tool. Verify the tool receives the cancelled context and the executor returns without hanging.
- [ ] **Executor — output truncation:** Register a mock tool that returns a very large string. Verify the result is truncated with the notice message appended.
- [ ] **Integration test:** Register one mock pure tool and one mock mutating tool. Dispatch a mixed batch of 3 calls (2 pure, 1 mutating). Verify execution order, result order, and `tool_executions` rows written to an in-memory SQLite database.
