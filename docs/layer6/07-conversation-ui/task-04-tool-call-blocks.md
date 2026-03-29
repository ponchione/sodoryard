# Task 04: Tool Call Blocks (Start, Output, End, Concurrent)

**Epic:** 07 — Conversation UI
**Status:** ⬚ Not started
**Dependencies:** Task 01, Task 02

---

## Description

Implement the tool call block components that render tool execution lifecycle events. Each tool call is displayed as a collapsible card showing the tool name, arguments summary, execution status (running/completed/failed), incremental output, final result, and duration. Multiple concurrent tool calls render as separate cards matched by their tool call ID. Tool output containing code is syntax-highlighted.

## Acceptance Criteria

- [ ] **`tool_call_start`** creates a new collapsible card component for the tool call, identified by the tool call `id`
- [ ] Card header shows the tool name and a summary of arguments (e.g., "file_read: internal/auth/middleware.go", "shell: go test ./...")
- [ ] While the tool is running, the card displays a spinner or "Executing..." indicator
- [ ] **`tool_call_output`** events append incremental output to the card's content area (matched by tool call `id`). This handles streaming shell stdout in real-time
- [ ] **`tool_call_end`** finalizes the card: removes the spinner, displays the final result/output, shows execution duration (e.g., "1.2s"), and indicates success or failure (e.g., green checkmark vs red X)
- [ ] Failed tool calls display the error message prominently within the card
- [ ] Tool output containing code is syntax-highlighted using the same highlighting library from Task 02
- [ ] Cards are collapsible — collapsed by default after completion (showing just the header with tool name and status), expandable to view full output
- [ ] **Concurrent tool calls:** Multiple tool calls in-flight simultaneously render as separate cards. Events from different tool calls are routed to the correct card by matching on the tool call `id`
- [ ] Tool call cards appear inline in the message thread at the position where the tool_use content block occurs in the assistant's response
- [ ] **Tool result messages** (from REST-loaded history) are displayed inline after their corresponding tool call block, matched by `tool_use_id`
- [ ] Tool call blocks from completed turns (loaded via REST) render in their finalized state (no spinners, just result and duration)
