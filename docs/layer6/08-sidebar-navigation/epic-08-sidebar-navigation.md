# Layer 6, Epic 08: Sidebar, Navigation & Search

**Layer:** 6 — Web Interface & Streaming
**Status:** ⬚ Not started
**Dependencies:** [[layer-6-epic-06-react-scaffolding]] (app shell, routing, API client), [[layer-6-epic-02-rest-api-conversations]] (REST conversation endpoints)

---

## Description

Build the conversation sidebar and SPA navigation. The sidebar occupies the left panel of the app layout and shows the list of past conversations (sorted by recency), a search input for FTS5 full-text search across conversation content, and a "New conversation" button. Clicking a conversation navigates to `/c/:id` and loads it in the main content area. The sidebar also supports conversation deletion. SPA routing ensures that URLs are deep-linkable — opening `http://localhost:3000/c/<uuid>` loads that conversation directly.

---

## Definition of Done

- [ ] **Conversation list:** Fetched via `GET /api/conversations`. Displays conversation title (or "Untitled" if null) and relative timestamp ("2 hours ago", "yesterday"). Sorted by `updated_at` descending (most recent first)
- [ ] **Active conversation highlight:** The currently viewed conversation is visually highlighted in the sidebar
- [ ] **New conversation button:** Creates a new conversation and navigates to it. The conversation is created either eagerly via `POST /api/conversations` or lazily on first message send (agent's choice — lazy is simpler)
- [ ] **Delete conversation:** Delete button (with confirmation) on each sidebar item. Calls `DELETE /api/conversations/:id`. Removes from list. If the deleted conversation is currently active, navigate to the conversation list or the most recent remaining conversation
- [ ] **Search input:** Text input at the top of the sidebar. Queries `GET /api/conversations/search?q=<query>` on input (debounced, ~300ms). Results replace the conversation list while searching, showing matching conversations with content snippets (from FTS5 `snippet()`)
- [ ] **SPA routing:** `react-router-dom` routes:
  - `/` — Shows the sidebar with conversation list, main area shows a "New conversation" prompt or the most recent conversation
  - `/c/:id` — Loads conversation `:id` in the main content area, highlights it in the sidebar
  - Navigation between conversations does NOT full-page reload — the sidebar stays rendered, only the main content area transitions
- [ ] **Deep linking:** Opening `http://localhost:3000/c/<uuid>` in a new browser tab loads the app, fetches the conversation, and renders it. The Go server's SPA fallback from [[layer-6-epic-01-http-server-foundation]] serves `index.html` for this path
- [ ] **Sidebar collapsible:** Optional: the sidebar can be collapsed to give the conversation view full width. A toggle button expands/collapses. Not strictly required for v0.1 but improves usability
- [ ] **Auto-refresh:** The conversation list updates when a turn completes (the current conversation's `updated_at` changes, moving it to the top). Either poll periodically or update optimistically when a `turn_complete` event fires on the active conversation
- [ ] **Empty state:** When there are no conversations, show a clear empty state with a prompt to start a new conversation

---

## Architecture References

- [[07-web-interface-and-streaming]] — §UI Components: "Conversation sidebar: Past conversations with search"
- [[08-data-model]] — §Key Query Patterns: conversation list query (`ORDER BY updated_at DESC LIMIT ? OFFSET ?`), FTS5 search query with `snippet()`
- [[08-data-model]] — §Full-Text Search: FTS5 virtual table, search query pattern
- [[01-project-vision-and-principles]] — §Design Principles: "Conversation as the unit of work. Everything is organized around conversations"

---

## Notes for the Implementing Agent

The sidebar is a persistent layout component — it renders regardless of which route is active. Use React Router's layout routes or a simple layout component that wraps all routes.

For conversation titles: the data model allows null titles. The agent loop's title generation (via a lightweight LLM call after the first assistant response, per [[08-data-model]]) populates this asynchronously. The sidebar should handle null gracefully — show "Untitled" or the first few words of the first user message as a fallback.

For search, debounce the input to avoid hammering the API. `useDeferredValue` from React 18+ or a simple `setTimeout` debounce both work. Clear search results when the search input is emptied.

The conversation list will be short initially — a personal tool might have dozens to low hundreds of conversations. Pagination is not critical for v0.1. Simple "load all" is fine. If performance becomes an issue later, add cursor-based pagination.

Consider using `react-router-dom`'s `useParams` to get the conversation ID from the URL, and `useNavigate` for programmatic navigation. The sidebar's conversation items are `<Link to={`/c/${conv.id}`}>` elements.
