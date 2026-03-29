# Task 02: New Conversation and Delete Conversation

**Epic:** 08 — Sidebar Navigation
**Status:** ⬚ Not started
**Dependencies:** Task 01

---

## Description

Implement the "New conversation" button and the delete conversation action in the sidebar. The new conversation action either creates a conversation eagerly via the REST API or navigates to an empty conversation view that creates one lazily on first message send. The delete action removes a conversation with a confirmation dialog, updates the sidebar list, and handles the case where the deleted conversation is currently active.

## Acceptance Criteria

- [ ] **New conversation button:** Prominently placed at the top of the sidebar (above the conversation list)
- [ ] Clicking the button either: (a) calls `POST /api/conversations` to create an empty conversation and navigates to `/c/:new_id`, or (b) navigates to `/` which renders an empty conversation view — agent's choice, with lazy creation preferred for simplicity
- [ ] New conversation appears in the sidebar list immediately (optimistic update or list re-fetch)
- [ ] **Delete conversation:** Each sidebar item has a delete button (visible on hover or as a persistent icon)
- [ ] Clicking delete shows a confirmation dialog: "Delete this conversation? This cannot be undone."
- [ ] On confirmation, calls `DELETE /api/conversations/:id` and removes the item from the sidebar list
- [ ] If the deleted conversation is currently active (displayed in the main content area), navigate to `/` or to the most recent remaining conversation
- [ ] If deletion fails (network error, server error), show an error notification and keep the item in the list
- [ ] Delete button is not visible or is disabled while a turn is running in that conversation (to prevent deleting mid-turn)
- [ ] The sidebar list re-renders smoothly after deletion — no layout jump or flicker
