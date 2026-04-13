/**
 * WebSocket event types — mirrors the Go agent event types in
 * internal/agent/events.go and the ServerMessage/ClientMessage
 * envelopes in internal/server/websocket.go.
 */

// ── Server → Client events ─────────────────────────────────────────

/** Envelope wrapping every server→client WebSocket message. */
export interface ServerMessage<T = unknown> {
  type: ServerEventType;
  timestamp: string; // ISO 8601
  data: T;
}

export type ServerEventType =
  | "token"
  | "thinking_start"
  | "thinking_delta"
  | "thinking_end"
  | "tool_call_start"
  | "tool_call_output"
  | "tool_call_end"
  | "turn_complete"
  | "turn_cancelled"
  | "error"
  | "status"
  | "context_debug"
  | "conversation_created";

/** Streamed visible text delta from the LLM. */
export interface TokenEvent {
  type: "token";
  token: string;
  time: string;
}

/** Beginning of a streamed thinking block. */
export interface ThinkingStartEvent {
  type: "thinking_start";
  time: string;
}

/** Incremental thinking delta. */
export interface ThinkingDeltaEvent {
  type: "thinking_delta";
  delta: string;
  time: string;
}

/** End of a streamed thinking block. */
export interface ThinkingEndEvent {
  type: "thinking_end";
  time: string;
}

/** A tool call is beginning. */
export interface ToolCallStartEvent {
  type: "tool_call_start";
  tool_call_id: string;
  tool_name: string;
  arguments?: Record<string, unknown>;
  time: string;
}

/** Incremental tool output (e.g. streaming shell stdout). */
export interface ToolCallOutputEvent {
  type: "tool_call_output";
  tool_call_id: string;
  output?: string;
  time: string;
}

/** Tool execution complete. */
export interface ToolCallEndEvent {
  type: "tool_call_end";
  tool_call_id: string;
  result?: string;
  duration?: number; // nanoseconds (Go time.Duration)
  success?: boolean;
  time: string;
}

/** Turn finished — usage summary. */
export interface TurnCompleteEvent {
  type: "turn_complete";
  turn_number: number;
  iteration_count: number;
  total_input_tokens?: number;
  total_output_tokens?: number;
  duration?: number;
  time: string;
}

/** Turn was cancelled or interrupted before completion. */
export interface TurnCancelledEvent {
  type: "turn_cancelled";
  turn_number: number;
  completed_iterations?: number;
  reason?: string;
  time: string;
}

/** Recoverable or terminal agent-loop error. */
export interface ErrorEvent {
  type: "error";
  error_code?: string;
  message?: string;
  recoverable?: boolean;
  time: string;
}

/** Agent state machine transition. */
export type AgentState =
  | "idle"
  | "assembling_context"
  | "waiting_for_llm"
  | "executing_tools"
  | "compressing";

export interface StatusEvent {
  type: "status";
  state: AgentState;
  time: string;
}

/** Context assembly debug report. */
export interface ContextDebugEvent {
  type: "context_debug";
  report?: Record<string, unknown>; // ContextAssemblyReport shape
  time: string;
}

/** Emitted when a new conversation is created via WS "message" with no conversation_id. */
export interface ConversationCreatedEvent {
  conversation_id: string;
}

/** Union of all possible event data types. */
export type ServerEventData =
  | TokenEvent
  | ThinkingStartEvent
  | ThinkingDeltaEvent
  | ThinkingEndEvent
  | ToolCallStartEvent
  | ToolCallOutputEvent
  | ToolCallEndEvent
  | TurnCompleteEvent
  | TurnCancelledEvent
  | ErrorEvent
  | StatusEvent
  | ContextDebugEvent
  | ConversationCreatedEvent;

// ── Client → Server messages ────────────────────────────────────────

export type ClientMessageType = "message" | "model_override" | "cancel";

/** A message sent from the client to the server via WebSocket. */
export interface ClientMessage {
  type: ClientMessageType;
  conversation_id?: string;
  content?: string;
  model?: string;
  provider?: string;
}
