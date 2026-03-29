# Task 01: Tool Interface, Purity Enum, ToolCall and ToolResult Types

**Epic:** 01 — Tool Interface, Registry & Executor
**Status:** ⬚ Not started
**Dependencies:** Layer 0 Epic 02 (logging), Layer 0 Epic 03 (config)

---

## Description

Define the core type system for the tool layer in `internal/tool/`. This includes the `Tool` interface that all tools implement, the `Purity` enum that classifies tools as `Pure` or `Mutating` for dispatch purposes, the `ToolCall` type representing an inbound tool invocation from the LLM, and the `ToolResult` type representing the outcome. These types are the contract between the executor, the registry, and every individual tool implementation.

## Acceptance Criteria

- [ ] `Purity` type defined as an enum with two values: `Pure` and `Mutating`. Includes a `String()` method for logging and display.
- [ ] `Tool` interface defined with methods:
  - `Name() string` — tool identifier (e.g., `"file_read"`)
  - `Description() string` — one-line description for the LLM
  - `Purity() Purity` — declares whether the tool is Pure or Mutating
  - `Schema() json.RawMessage` — returns the JSON Schema definition of the tool's parameters
  - `Execute(ctx context.Context, projectRoot string, input json.RawMessage) (*ToolResult, error)` — runs the tool
- [ ] `ToolCall` struct defined with fields: `ID string` (LLM-generated tool_use_id), `Name string`, `Arguments json.RawMessage`
- [ ] `ToolResult` struct defined with fields: `CallID string`, `Content string`, `Success bool`, `Error string`, `DurationMs int64`
- [ ] All types exported from `internal/tool/` package
- [ ] JSON Schema format follows the Anthropic/OpenAI function calling convention: `{"name": "...", "description": "...", "input_schema": {...}}` — documented in a comment on the `Schema()` method
- [ ] Compiles cleanly: `go build ./internal/tool/...`
