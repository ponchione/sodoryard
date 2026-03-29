# Task 02: Message Thread and User/Assistant Message Rendering

**Epic:** 07 — Conversation UI
**Status:** ⬚ Not started
**Dependencies:** Task 01, Epic 06 Task 04 (app shell)

---

## Description

Build the message thread component that renders the conversation as a scrollable list of messages. Implement rendering for user messages (plain text blocks) and assistant messages (parsed from JSON content block arrays with markdown rendering and syntax-highlighted code fences). The thread auto-scrolls to the bottom when new content arrives. Compressed messages are rendered with visual distinction.

## Acceptance Criteria

- [ ] Message thread component renders inside the main content area of the app shell
- [ ] Messages displayed in a scrollable container that fills the available vertical space
- [ ] **Auto-scroll:** Thread automatically scrolls to the bottom when new messages are added or when streaming content grows. Auto-scroll pauses if the user manually scrolls up (to allow reading earlier messages) and resumes when they scroll back to the bottom
- [ ] **User messages:** Rendered as plain text blocks with a visual treatment distinguishing them from assistant messages (e.g., different background color, alignment, or label)
- [ ] **Assistant messages:** Content is parsed from the JSON content block array. Text blocks are rendered as markdown using a GFM-compatible renderer (e.g., `react-markdown` with `remark-gfm`)
- [ ] **Markdown rendering:** Supports headings, lists (ordered and unordered), bold, italic, links, inline code, code fences, blockquotes, tables
- [ ] **Syntax highlighting:** Fenced code blocks in assistant text use a syntax highlighting library (e.g., `react-syntax-highlighter` with Prism, or shiki). Language is detected from the code fence tag (e.g., ` ```go `)
- [ ] Code blocks include a "copy to clipboard" button
- [ ] **Compressed messages:** Messages with `is_compressed=true` rendered as collapsed/greyed-out with a "[compressed]" indicator. Expandable to view content
- [ ] **Summary messages:** Messages with `is_summary=true` rendered with a "[summary]" prefix/badge to distinguish them from regular messages
- [ ] Messages are keyed by their `id` for efficient React reconciliation
- [ ] Empty conversation state renders a welcoming prompt (e.g., "Send a message to start a conversation")
