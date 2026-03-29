# Task 04: App Shell and Client-Side Routing

**Epic:** 06 — React Scaffolding
**Status:** ⬚ Not started
**Dependencies:** Task 02

---

## Description

Create the app shell layout and configure client-side routing with `react-router-dom`. The app shell defines the top-level layout: a sidebar panel on the left and a main content area on the right. Client-side routing maps URL paths to components: `/` for the conversation list/home view and `/c/:id` for individual conversation views. Routes render placeholder components that subsequent epics will replace with full implementations.

## Acceptance Criteria

- [ ] App shell layout component renders a two-panel layout: sidebar (left, fixed width) and main content area (right, fills remaining space)
- [ ] Layout is responsive — sidebar has a reasonable fixed width (e.g., 280px) and the main content area fills the rest of the viewport
- [ ] `react-router-dom` v6+ configured with `BrowserRouter` (or `createBrowserRouter` for data router pattern)
- [ ] Route `/` renders a placeholder home/conversation list component
- [ ] Route `/c/:id` renders a placeholder conversation view component that displays the conversation ID from the URL parameter
- [ ] The sidebar is a persistent layout element — it renders on all routes (using a layout route or wrapper component)
- [ ] Navigation between routes does not cause a full page reload — SPA behavior is working
- [ ] The Go server's SPA fallback (from Epic 01) correctly serves `index.html` for paths like `/c/some-uuid`, enabling deep linking
- [ ] A 404/catch-all route exists for unmatched paths, rendering a "Page not found" placeholder
- [ ] The app renders correctly with the dark theme from Task 02 — no unstyled flashes or theme mismatches
