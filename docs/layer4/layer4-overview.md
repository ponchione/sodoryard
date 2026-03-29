# Layer 4 — Tool System: Build Phase Overview

**Layer:** 4 (Shell, file, search, git tools)
**Primary Source:** [[05-agent-loop]] §Tool Set, §Tool Dispatch
**Supporting Docs:** [[04-code-intelligence-and-rag]], [[08-data-model]], [[09-project-brain]]
**Last Updated:** 2026-03-28

---

## Scope

Layer 4 implements the tool system that the agent loop (Layer 5) dispatches into. It covers:

- The generic `Tool` interface, purity classification, tool registry, and the executor that handles purity-based parallel/sequential dispatch
- Eight core tools: `file_read`, `file_write`, `file_edit`, `search_text`, `search_semantic`, `git_status`, `git_diff`, `shell`
- Four brain tools: `brain_read`, `brain_write`, `brain_update`, `brain_search` (v0.1 keyword-only via Obsidian REST API)
- Tool result formatting, output truncation, and `tool_executions` persistence

Layer 4 depends on Layer 1 (for `search_semantic`'s RAG backend) but NOT on Layer 2 (tools don't make LLM calls). Layer 4's sole consumer is Layer 5 (Agent Loop), which dispatches tool calls and collects results.

---

## Epic Index

| #   | Epic                                           | Status | Dependencies                     |
| --- | ---------------------------------------------- | ------ | -------------------------------- |
| 01  | [[layer4-epic01-tool-interface]]               | ⬚      | Layer 0: Epics 02, 03, 04, 06   |
| 02  | [[layer4-epic02-file-tools]]                   | ⬚      | Epic 01                          |
| 03  | [[layer4-epic03-search-tools]]                 | ⬚      | Epic 01, Layer 1: Epic 08        |
| 04  | [[layer4-epic04-git-tools]]                    | ⬚      | Epic 01                          |
| 05  | [[layer4-epic05-shell-tool]]                   | ⬚      | Epic 01                          |
| 06  | [[layer4-epic06-obsidian-client-brain-tools]]  | ⬚      | Epic 01                          |

---

## Dependency Graph

```
Layer 0 (Foundation)
  ├─ Epic 02: Structured Logging
  ├─ Epic 03: Configuration ──────── (shell_timeout, shell_denylist,
  │                                    tool_output_max_tokens, brain config)
  ├─ Epic 04: SQLite Connection
  └─ Epic 06: Schema & sqlc ──────── (tool_executions table, brain tables)

Layer 1 (Code Intelligence)
  └─ Epic 08: Searcher ───────────── (multi-query expansion, dependency hops)

                    ┌─────────────────────────────────┐
                    │  Layer 4 Epic 01                 │
                    │  Tool Interface, Registry        │
                    │  & Executor                      │
                    └────────┬────────────────────────-┘
                             │
          ┌──────────┬───────┼───────┬──────────┐
          │          │       │       │          │
          ▼          ▼       ▼       ▼          ▼
      Epic 02    Epic 03  Epic 04  Epic 05   Epic 06
      File       Search   Git      Shell     Obsidian
      Tools      Tools    Tools    Tool      Client &
                                             Brain Tools
```

Epics 02–06 are independent of each other and can be built in parallel once Epic 01 is complete.

---

## Cross-Layer Dependencies

| Layer 4 Epic | Depends On (Layer 0) | Depends On (Layer 1) | Depends On (Layer 2) |
|---|---|---|---|
| Epic 01: Tool Interface | Epics 02 (logging), 03 (config), 04 (SQLite), 06 (sqlc) | — | — |
| Epic 02: File Tools | — | — | — |
| Epic 03: Search Tools | — | Epic 08 (Searcher) | — |
| Epic 04: Git Tools | — | — | — |
| Epic 05: Shell Tool | — | — | — |
| Epic 06: Brain Tools | — | — | — |

---

## Key Design Decisions

**Tool interface carries purity classification.** Each tool declares itself `Pure` or `Mutating` at registration. The executor uses this to partition batches — pure calls run concurrently via goroutines, mutating calls run sequentially in LLM-specified order. This is core to the dispatch strategy defined in [[05-agent-loop]] §Tool Dispatch.

**Output truncation is executor-level.** Each tool has a configurable output size limit. The executor applies truncation after execution, appending a notice with guidance (e.g., "Use file_read with line_start/line_end for specific sections"). The global default comes from `tool_output_max_tokens` in config.

**Tool results become `role=tool` messages.** The executor returns structured `ToolResult` values. The agent loop (Layer 5) converts these to API messages with `tool_use_id` linkage. Layer 4 does not know about the message format — it returns data, Layer 5 formats it.

**Brain tools are v0.1 scope but keyword-only.** Per [[09-project-brain]] build phases, v0.1 includes the four brain tools with keyword search via the Obsidian REST API. No vector search, no context assembly integration, no wikilink graph traversal. Those are v0.2. The Obsidian REST API client is the only external HTTP dependency in this layer.

**JSON Schema generation per tool.** Each tool produces a JSON Schema definition of its parameters. The registry collects these for injection into LLM requests. This is how the LLM knows what tools are available and what arguments they accept.

---

## What Feeds Into Layer 5

Layer 5 (Agent Loop) consumes Layer 4 via three interfaces:

1. **Registry** — enumerate available tools, get JSON Schema definitions for the system prompt
2. **Executor** — dispatch a batch of `ToolCall` values, receive `ToolResult` values
3. **ToolResult** — structured result with content string, success/failure, duration, error message

Layer 5 handles iteration logic, loop detection, cancellation, and message persistence. Layer 4 handles tool execution mechanics only.
