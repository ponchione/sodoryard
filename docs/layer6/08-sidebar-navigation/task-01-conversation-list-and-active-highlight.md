# Task 01: Conversation List and Active Highlight

**Epic:** 08 — Sidebar Navigation
**Status:** ⬚ Not started
**Dependencies:** Epic 06 Task 04 (app shell), Epic 06 Task 05 (API client)

---

## Description

Build the conversation list component that populates the sidebar with past conversations fetched from the REST API. Each item displays the conversation title (or "Untitled" for null titles) and a relative timestamp. The list is sorted by most recently updated. The currently active conversation (matching the URL parameter) is visually highlighted. The list auto-refreshes when a turn completes to reflect updated timestamps.

## Acceptance Criteria

- [ ] Conversation list fetched via `GET /api/conversations` using the API client utility from Epic 06
- [ ] Each list item displays the conversation title. Null titles render as "Untitled" (or the first few words of the first user message if available)
- [ ] Each list item displays a relative timestamp based on `updated_at` (e.g., "2 hours ago", "yesterday", "Mar 15")
- [ ] List is sorted by `updated_at` descending — most recent conversation appears at the top
- [ ] Clicking a conversation item navigates to `/c/:id` using React Router's `<Link>` or `useNavigate`
- [ ] **Active highlight:** The conversation matching the current URL parameter (`:id`) is visually highlighted (e.g., different background color, border, or font weight)
- [ ] Highlight updates immediately when navigating between conversations
- [ ] **Auto-refresh:** The list updates when a `turn_complete` WebSocket event fires on the active conversation — the conversation moves to the top of the list. Implementation can be optimistic (move the item locally) or re-fetch the list
- [ ] **Loading state:** Display a skeleton/spinner while the initial conversation list is loading
- [ ] **Empty state:** When no conversations exist, display a message like "No conversations yet" with a visual prompt to start one
- [ ] The list scrolls if there are more conversations than fit in the sidebar
