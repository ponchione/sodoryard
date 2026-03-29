# Task 02: Client-to-Server Message Handling (message, cancel, model_override)

**Epic:** 04 — WebSocket Handler
**Status:** ⬚ Not started
**Dependencies:** Task 01, Layer 5 Epic 02 (conversation manager), Layer 5 Epic 06 (agent loop)

---

## Description

Implement the three client-to-server message types: `message` (send a user message to start a turn), `cancel` (abort the in-flight turn), and `model_override` (change the model for subsequent turns). The `message` handler is the most complex: it creates or resumes a conversation and calls `AgentLoop.RunTurn()` in a goroutine while the connection's already-registered EventSink forwards events through the write loop. The `cancel` handler calls the agent loop's cancel method. The `model_override` handler updates the conversation's model setting.

## Acceptance Criteria

- [ ] **`message` type:** Receives `{type: "message", data: {text: string, conversation_id?: string}}`
- [ ] If `conversation_id` is provided, resumes that conversation; if absent, creates a new conversation via the conversation manager
- [ ] For new conversations, sends a `conversation_id` event to the client: `{"type": "conversation_id", "data": {"id": "<uuid>"}}`
- [ ] Calls `AgentLoop.RunTurn(ctx, conversationID, text)` in a separate goroutine so the write loop can forward events concurrently
- [ ] Reuses the connection's existing subscribed `EventSink`; subscription happens once during connection setup, not once per turn
- [ ] Missing `text` field returns an error event: `{"type": "error", "data": {"message": "text is required"}}`
- [ ] **`cancel` type:** Receives `{type: "cancel"}` and calls `AgentLoop.Cancel()` for the in-flight turn
- [ ] Cancel is a no-op if no turn is currently running (does not send an error)
- [ ] **`model_override` type:** Receives `{type: "model_override", data: {provider: string, model: string}}`
- [ ] Updates the conversation's model setting via the conversation manager for subsequent turns (does not affect the currently running turn)
- [ ] Invalid provider or model names return an error event with a descriptive message
- [ ] Unknown message types are logged and an error event is sent: `{"type": "error", "data": {"message": "unknown message type: <type>"}}`
