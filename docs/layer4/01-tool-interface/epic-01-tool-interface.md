# Layer 4 — Epic 01: Tool Interface, Registry & Executor

**Layer:** 4 (Tool System)
**Package:** `internal/tool/`
**Status:** ⬚ Not Started
**Dependencies:**
- Layer 0 Epic 02: Structured Logging (`internal/logging/`)
- Layer 0 Epic 03: Configuration (`internal/config/` — `tool_output_max_tokens`)
- Layer 0 Epic 04: SQLite Connection (`internal/db/`)
- Layer 0 Epic 06: Schema & sqlc (`tool_executions` table, generated queries)

**Architecture Refs:**
- [[05-agent-loop]] §Tool Dispatch — purity classification, parallel/sequential execution strategy, output truncation
- [[05-agent-loop]] §Tool Set — eight tools, purity assignments
- [[08-data-model]] §tool_executions — analytics table schema

---

## What This Epic Covers

The foundational infrastructure for the entire tool system. Defines the `Tool` interface that all tools implement, the purity classification enum (`Pure` vs `Mutating`), the `ToolCall` and `ToolResult` types, the `Registry` for tool registration and lookup, and the `Executor` that handles purity-based dispatch.

The executor is the primary entry point for Layer 5 (Agent Loop). It receives a batch of tool calls from an LLM response, partitions them by purity, runs pure calls concurrently via goroutines, runs mutating calls sequentially in LLM-specified order, applies output truncation, records `tool_executions` rows, and returns ordered results.

This epic also includes JSON Schema generation — each tool must produce a schema definition of its parameters so the LLM knows what tools are available and what arguments they accept. The registry collects these for injection into LLM requests.

---

## Definition of Done

- [ ] `Tool` interface defined with methods: `Name() string`, `Description() string`, `Purity() Purity`, `Schema() json.RawMessage`, `Execute(ctx context.Context, projectRoot string, input json.RawMessage) (*ToolResult, error)`
- [ ] `Purity` enum with values `Pure` and `Mutating`
- [ ] `ToolCall` type with fields: `ID string` (LLM-generated tool_use_id), `Name string`, `Arguments json.RawMessage`
- [ ] `ToolResult` type with fields: `CallID string`, `Content string`, `Success bool`, `Error string`, `DurationMs int64`
- [ ] `Registry` with `Register(tool Tool)`, `Get(name string) (Tool, bool)`, `All() []Tool`, `Schemas() []json.RawMessage`
- [ ] Duplicate registration panics (fail-fast at startup, not runtime)
- [ ] `Executor` with `Execute(ctx context.Context, projectRoot string, conversationID string, turnNumber int, iteration int, calls []ToolCall) []ToolResult`
- [ ] Executor partitions calls by purity: pure calls execute concurrently (goroutines + WaitGroup), mutating calls execute sequentially in input order
- [ ] Executor returns results in the original call order (matching call IDs), regardless of execution order
- [ ] Output truncation: results exceeding the configured token limit are truncated with a notice message (e.g., `[Output truncated — showing first N lines of M. Use file_read with line_start/line_end for specific sections.]`)
- [ ] Truncation limit sourced from config (`tool_output_max_tokens`), with per-tool override capability
- [ ] `tool_executions` row inserted for every tool call: tool_use_id, tool_name, input JSON, output_size (byte count), error, success, duration_ms, conversation_id, turn_number, iteration
- [ ] Tool execution errors are caught and returned as failed `ToolResult` values (content contains the error message), never propagated as Go errors — the agent loop feeds errors back to the LLM
- [ ] Context cancellation propagated to all in-flight tool executions (pure goroutines cancelled, sequential chain stops)
- [ ] Unit tests: registry CRUD, executor purity partitioning, parallel execution of pure calls, sequential execution of mutating calls, result ordering, output truncation, cancellation
- [ ] Integration test: register a mock pure tool and a mock mutating tool, dispatch a mixed batch, verify execution order and result assembly

---

## Key Design Notes

**Error philosophy:** Tool execution failures are NOT Go errors. They are `ToolResult` values with `Success=false` and the error in `Content`. The agent loop feeds these back to the LLM as `role=tool` messages. The LLM self-corrects. Only infrastructure failures (database write errors, context cancelled) propagate as Go errors.

**Enriched error messages:** Per [[05-agent-loop]] §Error Recovery, tool errors should include helpful context. For example, a "file not found" error should list available files in the same directory. This enrichment is the responsibility of each tool's `Execute` implementation (Epics 02–06), not the executor. The executor just passes through whatever the tool returns.

**JSON Schema format:** Schemas follow the JSON Schema spec as used by Anthropic's tool_use and OpenAI's function calling — an object with `name`, `description`, `input_schema` (JSON Schema for parameters). The exact format should match what the provider layer (Layer 2) injects into LLM requests.

**Project root sandboxing:** The executor passes `projectRoot` to every tool call. File and git tools use this to restrict operations to the project directory. The executor itself does not enforce sandboxing — each tool does.

---

## Consumed By

- [[layer4-epic02-file-tools]] — implements `Tool` interface
- [[layer4-epic03-search-tools]] — implements `Tool` interface
- [[layer4-epic04-git-tools]] — implements `Tool` interface
- [[layer4-epic05-shell-tool]] — implements `Tool` interface
- [[layer4-epic06-obsidian-client-brain-tools]] — implements `Tool` interface
- Layer 5 (Agent Loop) — calls `Registry.Schemas()` for system prompt, calls `Executor.Execute()` for dispatch
