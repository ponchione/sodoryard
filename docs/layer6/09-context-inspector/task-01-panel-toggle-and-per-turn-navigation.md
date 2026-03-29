# Task 01: Panel Toggle and Per-Turn Navigation

**Epic:** 09 — Context Inspector
**Status:** ⬚ Not started
**Dependencies:** Epic 07 (Conversation UI — panel lives within conversation view)

---

## Description

Create the context inspector panel shell with a toggle button to show/hide it and per-turn navigation to browse context reports across a conversation's turns. The panel appears as a side panel or bottom panel alongside the message thread when toggled open. It defaults to closed and is fully hidden when inactive. Per-turn navigation lets the user step through turns to compare context assembly decisions across the conversation.

## Acceptance Criteria

- [ ] **Toggle button:** A button (e.g., a debug/inspect icon) in the conversation view header or toolbar toggles the context inspector panel open/closed
- [ ] Panel defaults to closed on page load
- [ ] When closed, the panel is fully hidden — it does not consume screen space or push the message thread
- [ ] When open, the panel appears as a resizable side panel (right side) or bottom panel alongside the message thread
- [ ] The message thread adjusts its width or height to accommodate the open panel
- [ ] **Per-turn navigation:** The panel header shows the current turn number and total turns (e.g., "Turn 3 of 7")
- [ ] Previous/next buttons (or arrow icons) navigate between turns
- [ ] Selecting a turn fetches its context report via `GET /api/metrics/conversation/:id/context/:turn` if not already loaded
- [ ] **Real-time for current turn:** When a `context_debug` WebSocket event arrives during streaming, the panel automatically displays the current turn's data without requiring navigation
- [ ] The panel automatically navigates to the latest turn when a new `context_debug` event arrives (unless the user has manually navigated to a different turn)
- [ ] **Loading state:** Display a loading indicator while fetching a context report from the REST endpoint
- [ ] **No data state:** If a turn has no context report (e.g., the conversation just started), display "No context report for this turn"
- [ ] Panel state (open/closed) persists during navigation between conversations but resets to closed on page refresh
