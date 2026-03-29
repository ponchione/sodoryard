# Task 05: TypeScript Event Types and API Client Utility

**Epic:** 06 — React Scaffolding
**Status:** ⬚ Not started
**Dependencies:** Task 01

---

## Description

Define the TypeScript type definitions for the WebSocket protocol (all server-to-client and client-to-server event types) and create a thin API client utility for REST calls. The event types serve as the contract between the frontend and backend — they are the single source of truth for the frontend's understanding of the WebSocket protocol. The API client provides a convenience wrapper around `fetch` for REST endpoints with consistent error handling and JSON parsing.

## Acceptance Criteria

- [ ] TypeScript event types defined in a shared file (e.g., `web/src/types/events.ts`)
- [ ] **Server-to-client event types:** `TokenEvent`, `ThinkingStartEvent`, `ThinkingDeltaEvent`, `ThinkingEndEvent`, `ToolCallStartEvent`, `ToolCallOutputEvent`, `ToolCallEndEvent`, `TurnCompleteEvent`, `TurnCancelledEvent`, `ErrorEvent`, `StatusEvent`, `ContextDebugEvent`, `ConversationIdEvent`
- [ ] Each event type includes a `type` discriminant field and a `data` field with the appropriate shape
- [ ] A union type `ServerEvent` covers all server-to-client event types, enabling exhaustive type checking with `switch` on `event.type`
- [ ] **Client-to-server event types:** `SendMessageEvent`, `CancelEvent`, `ModelOverrideEvent`
- [ ] A union type `ClientEvent` covers all client-to-server event types
- [ ] **API client utility** in `web/src/lib/api.ts` (or similar): a thin wrapper around `fetch`
- [ ] API client exports functions: `api.get<T>(path)`, `api.post<T>(path, body)`, `api.put<T>(path, body)`, `api.delete(path)`
- [ ] API client automatically sets `Content-Type: application/json` for requests with bodies
- [ ] API client parses JSON responses and throws a typed error for non-2xx status codes, including the error message from the server's `{"error": "message"}` format
- [ ] API client base URL is relative (`/api/...`) so it works in both development (Vite proxy) and production (same origin)
- [ ] All types and the API client compile with zero TypeScript errors
