# Task 05: Message Input, Cancel Button, and Agent Status Indicator

**Epic:** 07 — Conversation UI
**Status:** ⬚ Not started
**Dependencies:** Task 01, Task 03

---

## Description

Build the message input area at the bottom of the conversation view, the cancel button for aborting in-progress turns, and the agent status indicator that shows the current agent state. The input handles text entry with Enter to submit and Shift+Enter for newlines. The cancel button appears during active turns and sends the cancel event via WebSocket. The status indicator reflects the agent's current state (idle, thinking, executing tools).

## Acceptance Criteria

- [ ] **Message input:** Text area at the bottom of the conversation view, fixed position so it's always visible
- [ ] Submit on Enter key (sends the message via WebSocket). Shift+Enter inserts a newline in the input
- [ ] Send button adjacent to the input area — clicking sends the message
- [ ] Input is disabled while a turn is running (between sending a message and receiving `turn_complete` or `turn_cancelled`)
- [ ] Input is cleared after a message is sent successfully
- [ ] Input supports multi-line text (textarea, not single-line input)
- [ ] Empty messages cannot be sent — the send button is disabled and Enter does nothing when the input is empty
- [ ] **Cancel button:** Visible while a turn is in progress (replaces or appears alongside the send button)
- [ ] Clicking cancel sends the `{type: "cancel"}` WebSocket event
- [ ] Cancel button transitions to disabled/hidden after `turn_cancelled` or `turn_complete` event is received
- [ ] **Agent status indicator:** Displays the current agent state from `status` events
- [ ] Three visually distinct states: `idle` (no indicator or subtle "Ready"), `thinking` (e.g., pulsing dot with "Thinking..."), `executing_tools` (e.g., spinning icon with "Running tools...")
- [ ] Status indicator is visible near the message input or in the conversation header
- [ ] **Turn complete:** `turn_complete` event re-enables the input, hides the cancel button, and resets the status to idle
- [ ] Optionally displays a subtle usage summary after turn completion (token count, iteration count) — dismissable or auto-hiding
- [ ] **Error display:** `error` events render as error banners in the message thread. Recoverable errors include a dismiss button. Non-recoverable errors show "Conversation ended" and disable the input
