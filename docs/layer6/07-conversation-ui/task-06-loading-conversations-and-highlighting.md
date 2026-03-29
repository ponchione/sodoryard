# Task 06: Loading Existing Conversations and Markdown/Syntax Highlighting

**Epic:** 07 — Conversation UI
**Status:** ⬚ Not started
**Dependencies:** Task 02, Task 04, Epic 06 Task 05 (API client)

---

## Description

Implement loading existing conversations from the REST API when navigating to `/c/:id` and rendering the full message history. This includes fetching messages via the REST endpoint, parsing assistant content blocks (text, thinking, tool_use), reconstructing the visual state (collapsed thinking blocks, finalized tool call cards), and connecting the WebSocket for new turns. Also finalize the markdown rendering and syntax highlighting integration for both loaded and streamed content.

## Acceptance Criteria

- [ ] When navigating to `/c/:id`, fetch the conversation metadata via `GET /api/conversations/:id` and messages via `GET /api/conversations/:id/messages`
- [ ] Display a loading state (spinner or skeleton) while fetching
- [ ] If the conversation ID does not exist (404), display a "Conversation not found" message and offer navigation back to the home view
- [ ] **Message history reconstruction:** Parse each message's `content` field based on `role`:
  - `user` messages: render as plain text
  - `assistant` messages: parse the JSON content block array and render text blocks as markdown, thinking blocks as collapsed sections, tool_use blocks as finalized tool call cards
  - `tool` result messages: match to their corresponding tool_use block by `tool_use_id` and display inline
- [ ] Tool call cards from history render in their completed state (result displayed, duration shown, no spinners)
- [ ] Thinking blocks from history render as collapsed sections (expandable)
- [ ] After loading history, connect the WebSocket so the user can send new messages and start streaming turns
- [ ] New streamed content appends below the loaded history
- [ ] **Markdown rendering:** `react-markdown` (or equivalent) with `remark-gfm` plugin for GitHub Flavored Markdown support
- [ ] **Syntax highlighting:** Code fences use `react-syntax-highlighter` (Prism backend) or shiki. Language detection from code fence tags. Falls back to plain text for unrecognized languages
- [ ] Syntax highlighting applies consistently to both loaded messages and streamed content
- [ ] Long code blocks are scrollable horizontally (no layout overflow)
- [ ] Syntax highlighting theme matches the dark theme (e.g., One Dark, Dracula, or VS Code Dark+)
