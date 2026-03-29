# Task 04: Output Truncation and Tool Executions Persistence

**Epic:** 01 — Tool Interface, Registry & Executor
**Status:** ⬚ Not started
**Dependencies:** Task 03, Layer 0 Epic 03 (config — `tool_output_max_tokens`)

---

## Description

Add output truncation logic to the executor so that tool results exceeding the configured token limit are truncated with a helpful notice. The truncation limit is sourced from config (`tool_output_max_tokens`) with the ability for individual tools to specify a per-tool override. This prevents oversized tool output from consuming the LLM's context window and guides the agent to use more targeted queries.

## Acceptance Criteria

- [ ] After each tool execution, the executor checks if `ToolResult.Content` exceeds the configured `tool_output_max_tokens` limit
- [ ] Token counting uses a simple heuristic (e.g., byte count / 4 or line count) — exact tokenization is not required, just a reasonable approximation
- [ ] Truncated results preserve the beginning of the output (first N lines) and append a notice: `[Output truncated — showing first N lines of M. Use file_read with line_start/line_end for specific sections.]`
- [ ] The truncation notice text is contextually appropriate — for search tools it might suggest narrowing the query, for git_diff it might suggest using a path filter
- [ ] Global truncation limit read from config `tool_output_max_tokens` with a sensible default (e.g., 20000 tokens)
- [ ] Per-tool override capability: the `Tool` interface or a wrapper allows a tool to declare a custom truncation limit (e.g., `git_diff` might want a higher limit than `file_read`)
- [ ] `tool_executions` row records `output_size` as the byte count of the full output (before truncation), enabling analytics on how often truncation occurs
- [ ] Unit tests: output below limit passes through unchanged, output above limit is truncated with notice, per-tool override respected
