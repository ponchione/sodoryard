# Task 04: SPA Routing and Deep Linking

**Epic:** 08 — Sidebar Navigation
**Status:** ⬚ Not started
**Dependencies:** Task 01, Epic 06 Task 04 (client-side routing), Epic 01 Task 04 (SPA fallback)

---

## Description

Ensure SPA routing works correctly for all navigation patterns: sidebar clicks, browser back/forward, direct URL entry, and page refresh. Deep linking must work — opening `http://localhost:3000/c/<uuid>` in a new browser tab loads the app, fetches the conversation, and renders it. The sidebar remains a persistent layout element across all navigations. Optionally implement sidebar collapse for full-width conversation viewing.

## Acceptance Criteria

- [ ] **SPA navigation:** Clicking conversations in the sidebar navigates to `/c/:id` without a full page reload. Only the main content area transitions — the sidebar stays rendered
- [ ] **Browser navigation:** Back and forward buttons navigate between previously viewed conversations correctly. The sidebar highlight updates to match
- [ ] **Deep linking:** Opening `http://localhost:3000/c/<uuid>` in a new browser tab:
  1. The Go server's SPA fallback serves `index.html`
  2. React Router parses the URL and renders the conversation view
  3. The conversation is fetched via REST and displayed
  4. The sidebar loads the conversation list and highlights the active conversation
- [ ] **Page refresh:** Refreshing the page on `/c/:id` reloads the app and restores the conversation view correctly (same behavior as deep linking)
- [ ] **URL sync:** The browser URL always reflects the currently displayed conversation. Navigating programmatically (e.g., after creating or deleting a conversation) updates the URL
- [ ] **404 handling:** If a deep-linked conversation ID does not exist (REST returns 404), display a "Conversation not found" message with a link back to the home view
- [ ] **Sidebar collapse (optional for v0.1):** A toggle button collapses/expands the sidebar. When collapsed, the main content area takes full width. Collapse state persists across navigation (e.g., stored in local state or localStorage)
- [ ] Transitions between conversations are fast — the sidebar does not re-fetch its list on every navigation (cached or persistent state)
