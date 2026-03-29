# Task 01: WebSocket Client Hook

**Epic:** 07 — Conversation UI
**Status:** ⬚ Not started
**Dependencies:** Epic 06 Task 05 (TypeScript event types)

---

## Description

Create a React hook (`useWebSocket` or similar) that manages the WebSocket connection to the backend at `/api/ws`. The hook handles connection establishment, reconnection on disconnect, message sending (user messages, cancel, model override), and provides an event stream that components can subscribe to. It cleans up the connection on component unmount to prevent leaks.

## Acceptance Criteria

- [ ] `useWebSocket` hook exported from a shared hooks file (e.g., `web/src/hooks/useWebSocket.ts`)
- [ ] Connects to `/api/ws` using the browser's native `WebSocket` API (relative URL works in both dev proxy and production modes)
- [ ] Exposes `sendMessage(text: string, conversationId?: string)` — sends a `{type: "message", data: {text, conversation_id?}}` frame
- [ ] Exposes `cancel()` — sends a `{type: "cancel"}` frame
- [ ] Exposes `setModelOverride(provider: string, model: string)` — sends a `{type: "model_override", data: {provider, model}}` frame
- [ ] Exposes a `connectionStatus` field: `"connecting" | "connected" | "disconnected" | "reconnecting"`
- [ ] Provides an event callback mechanism (e.g., `onEvent: (event: ServerEvent) => void`) or a ref-based event stream that components can consume
- [ ] **Reconnection:** On unexpected disconnect, attempts to reconnect with exponential backoff (e.g., 1s, 2s, 4s, up to 30s max). Stops reconnecting after a configurable number of attempts or when the component unmounts
- [ ] **Cleanup:** Closes the WebSocket connection when the component using the hook unmounts — no orphaned connections or event listeners
- [ ] Handles the `conversation_id` server event for new conversations — exposes the received conversation ID to the consuming component
- [ ] Incoming messages are parsed as JSON and typed as `ServerEvent` (using the types from Epic 06 Task 05) — malformed messages are logged and ignored
- [ ] Does not connect until explicitly requested or until the hook is mounted in a component that needs it (lazy connection is acceptable)
