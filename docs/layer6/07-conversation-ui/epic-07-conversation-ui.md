# Layer 6, Epic 07: Conversation UI — Chat & Streaming

**Layer:** 6 — Web Interface & Streaming
**Status:** ⬚ Not started
**Dependencies:** [[layer-6-epic-06-react-scaffolding]] (app shell, types, API client), [[layer-6-epic-04-websocket-handler]] (backend WebSocket protocol)

---

## Description

Build the core conversation view — the main content area where the user interacts with the agent. This is the heart of sirtopham's frontend. It includes the WebSocket client that maintains a connection to the backend, the message thread that renders user messages, assistant text (with streaming token display), thinking blocks (collapsible), and tool call blocks (collapsible with input/output). It also includes the message input bar with send and cancel buttons, and an agent status indicator.

v0.1 scope: functional streaming chat that correctly handles all event types from [[05-agent-loop]]. Syntax highlighting of code in assistant responses and tool outputs is included. Live streaming diffs, interactive file trees, and RAG visualizations are v0.5+ polish.

---

## Definition of Done

- [ ] **WebSocket client hook** (`useWebSocket` or similar): connects to `/api/ws`, handles reconnection on disconnect, exposes `sendMessage(text, conversationId?)`, `cancel()`, `setModelOverride(provider, model)`, and an event stream. Cleans up on unmount
- [ ] **Message thread:** Renders conversation messages in a scrollable container. Auto-scrolls to bottom on new content. Three message types rendered:
  - **User messages:** Simple text bubbles/blocks
  - **Assistant messages:** Parsed from the JSON content block array. Text blocks rendered as markdown with syntax-highlighted code fences. Thinking blocks rendered as collapsible sections (collapsed by default). Tool use blocks rendered as tool call components (see below)
  - **Tool result messages:** Displayed inline after their corresponding tool call block, matched by `tool_use_id`
- [ ] **Streaming display:** Token deltas (`token` events) are appended to the current assistant message in real-time. The message visibly grows as tokens arrive. No jank, no flicker — smooth incremental rendering
- [ ] **Thinking blocks:** `thinking_start` → `thinking_delta` → `thinking_end` events render a collapsible "Thinking..." section. Thinking content streams in real-time (collapsed by default, expandable). Thinking blocks from completed turns also rendered as collapsible
- [ ] **Tool call blocks:** Each tool call renders as a collapsible card showing:
  - Tool name and a summary of arguments (e.g., "file_read: internal/auth/middleware.go")
  - While running: a spinner or "executing" indicator
  - `tool_call_output` events: incremental output displayed inside the card (for streaming shell output)
  - On completion (`tool_call_end`): output/result displayed, duration shown, success/failure status
  - Tool output that contains code is syntax-highlighted
  - Multiple concurrent tool calls render as separate cards (matched by tool call ID)
- [ ] **Compressed messages:** Messages with `is_compressed=true` rendered as greyed-out/collapsed with a "[compressed]" indicator. Summary messages (`is_summary=true`) rendered with a "[summary]" prefix. Per [[08-data-model]]: the UI shows ALL messages
- [ ] **Message input:** Text input area at the bottom of the conversation view. Submit on Enter (Shift+Enter for newline). Send button. Disabled while a turn is running
- [ ] **Cancel button:** Visible while a turn is in progress. Sends `cancel` client event. Transitions to disabled/hidden after `turn_cancelled` or `turn_complete` event
- [ ] **Agent status indicator:** Displays current agent state from `status` events: idle, thinking, executing_tools. Visually distinct states (e.g., pulsing dot, text label)
- [ ] **Error display:** `error` events render as error banners in the message thread. Recoverable errors are dismissible; non-recoverable errors show a "Conversation ended" state
- [ ] **Turn complete:** `turn_complete` event hides the cancel button, re-enables message input, and optionally displays a subtle usage summary (token count, iteration count)
- [ ] **Loading existing conversation:** When navigating to `/c/:id`, load messages via REST (`GET /api/conversations/:id/messages`) and render the full history. Then connect WebSocket for new turns
- [ ] **Syntax highlighting:** Code blocks in assistant responses and tool outputs use a syntax highlighting library (e.g., `react-syntax-highlighter` with Prism, or shiki). Language detection from code fence tags
- [ ] **Markdown rendering:** Assistant text blocks rendered as markdown (headings, lists, bold, italic, links, inline code, code fences). Use a markdown renderer that supports GFM (e.g., `react-markdown` with `remark-gfm`)

---

## Architecture References

- [[07-web-interface-and-streaming]] — §UI Components: "Conversation view: Chat-style message thread with streaming token display", "Tool call visualization: Inline syntax-highlighted diffs, command output, search results"
- [[05-agent-loop]] — §Streaming to the Web UI: all event types with fields — the definitive spec for what this component consumes
- [[05-agent-loop]] — §Response Handling: three content block types (text, thinking, tool_use) and their streaming behavior
- [[05-agent-loop]] — §Tool Dispatch: pure vs mutating execution, concurrent tool calls — the UI must display parallel execution correctly
- [[05-agent-loop]] — §Cancellation: post-cancellation state — partially streamed content remains visible
- [[08-data-model]] — §Message Storage Model: how messages are stored and retrieved. Assistant `content` is a JSON array of content blocks. §Compression Model: `is_compressed` and `is_summary` flags
- [[01-project-vision-and-principles]] — §What Success Looks Like v0.1: "have a multi-turn conversation where the agent reads files, runs commands, and edits code"

---

## Notes for the Implementing Agent

This is the largest frontend epic. The key complexity is handling all event types correctly with smooth streaming.

**Streaming architecture:** Use React state for the current message being streamed. Token deltas append to an accumulating string. When `turn_complete` fires, the accumulated message moves to the "completed messages" array. This avoids re-rendering the entire message list on every token delta.

**Tool call matching:** Tool call events carry IDs (`id` field). `tool_call_start` creates a card with that ID. `tool_call_output` and `tool_call_end` update the card matched by ID. Multiple tool calls can be in-flight simultaneously — the UI must track each independently.

**Markdown + syntax highlighting:** `react-markdown` with `remark-gfm` for markdown rendering. Override the `code` element to use a syntax highlighter for fenced code blocks. This is a well-trodden React pattern.

**Performance concern:** Token-by-token streaming means many small state updates. Batch token events (e.g., accumulate tokens for 16ms, then update state once per animation frame) if naive rendering causes jank. Start without batching — optimize only if needed.

**Compressed messages** from older turns are loaded via REST and rendered statically. They don't participate in streaming — streaming only applies to the current turn. The visual treatment should make it clear these are historical: greyed out, collapsed, with a note about compression.
