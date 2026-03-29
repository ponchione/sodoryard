# Task 03: Search Input with FTS5

**Epic:** 08 — Sidebar Navigation
**Status:** ⬚ Not started
**Dependencies:** Task 01, Epic 02 Task 04 (FTS5 search endpoint)

---

## Description

Add a search input at the top of the sidebar that queries the FTS5 search endpoint as the user types. Search results replace the normal conversation list while a search is active, showing matching conversations with content snippets from the FTS5 `snippet()` function. The search is debounced to avoid excessive API calls, and clearing the search input restores the normal conversation list.

## Acceptance Criteria

- [ ] Search input field placed at the top of the sidebar, above the conversation list
- [ ] As the user types, queries `GET /api/conversations/search?q=<query>` with the input value
- [ ] Search is debounced with a ~300ms delay — API is not called on every keystroke
- [ ] **Search results:** Replace the normal conversation list while a search query is active
- [ ] Each search result shows: conversation title (or "Untitled"), relative timestamp, and a content snippet with match highlighting
- [ ] Snippets display the FTS5 `snippet()` output — matched terms are visually highlighted (bold, background color, or underline)
- [ ] Clicking a search result navigates to `/c/:id` and loads that conversation
- [ ] **Clear search:** Clearing the search input (backspace to empty, or a clear/X button) restores the normal conversation list
- [ ] **No results state:** When a search returns no results, display "No conversations match your search"
- [ ] **Loading state:** Display a subtle loading indicator while a search request is in flight
- [ ] Search input has a placeholder text like "Search conversations..."
- [ ] FTS5 query syntax errors (if the user types special characters) are handled gracefully — display a message rather than an error state
