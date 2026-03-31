import { useCallback, useEffect, useReducer } from "react";
import { useWebSocket } from "@/hooks/use-websocket";
import type {
  ServerMessage,
  ServerEventData,
  TokenEvent,
  StatusEvent,
  ErrorEvent,
  ConversationCreatedEvent,
  ThinkingDeltaEvent,
  ToolCallStartEvent,
  ToolCallOutputEvent,
  ToolCallEndEvent,
  TurnCompleteEvent,
  ContextDebugEvent,
  AgentState,
} from "@/types/events";

// ── Content blocks ───────────────────────────────────────────────────

export interface ThinkingBlock {
  kind: "thinking";
  text: string;
  done: boolean;
}

export interface ToolCallBlock {
  kind: "tool_call";
  toolCallId: string;
  toolName: string;
  args?: Record<string, unknown>;
  output: string;
  result?: string;
  duration?: number; // nanoseconds
  success?: boolean;
  done: boolean;
}

export interface TextBlock {
  kind: "text";
  text: string;
}

export type ContentBlock = ThinkingBlock | ToolCallBlock | TextBlock;

// ── Public types ─────────────────────────────────────────────────────

export interface ChatMessage {
  role: "user" | "assistant" | "system" | "tool";
  /** Plain text content — used for user messages and as fallback. */
  content: string;
  /** Structured content blocks for assistant messages. */
  blocks: ContentBlock[];
  /** Whether this message was compressed/summarized by context assembly. */
  isCompressed?: boolean;
  /** Whether this message is a summary marker. */
  isSummary?: boolean;
}

export interface TurnUsage {
  turnNumber: number;
  iterationCount: number;
  inputTokens?: number;
  outputTokens?: number;
  duration?: number; // nanoseconds
}

export interface ConversationState {
  conversationId: string | null;
  messages: ChatMessage[];
  /** Text being streamed for the current assistant text block. */
  streamingText: string;
  /** Is a turn currently in progress? */
  isStreaming: boolean;
  /** Agent state from status events. */
  agentState: AgentState | null;
  /** Last error from the backend. */
  error: string | null;
  /** Usage summary from the most recent turn. */
  lastTurnUsage: TurnUsage | null;
  /** Last context_debug event data (for the context inspector). */
  lastContextDebug: Record<string, unknown> | null;
}

// ── Reducer ──────────────────────────────────────────────────────────

type Action =
  | { type: "user_message"; content: string }
  | { type: "conversation_created"; conversationId: string }
  | { type: "token"; token: string }
  | { type: "thinking_start" }
  | { type: "thinking_delta"; delta: string }
  | { type: "thinking_end" }
  | { type: "tool_call_start"; toolCallId: string; toolName: string; args?: Record<string, unknown> }
  | { type: "tool_call_output"; toolCallId: string; output: string }
  | { type: "tool_call_end"; toolCallId: string; result?: string; duration?: number; success?: boolean }
  | { type: "status"; state: AgentState }
  | { type: "turn_complete"; usage: TurnUsage }
  | { type: "turn_cancelled" }
  | { type: "error"; message: string }
  | { type: "set_conversation_id"; conversationId: string }
  | { type: "load_history"; messages: ChatMessage[] }
  | { type: "context_debug"; report: Record<string, unknown> };

const initialState: ConversationState = {
  conversationId: null,
  messages: [],
  streamingText: "",
  isStreaming: false,
  agentState: null,
  error: null,
  lastTurnUsage: null,
  lastContextDebug: null,
};

/**
 * Get the current in-progress assistant message (last message if it's assistant),
 * or create a new one. Returns [updatedMessages, index].
 */
function ensureAssistantMessage(messages: ChatMessage[]): [ChatMessage[], number] {
  const last = messages[messages.length - 1];
  if (last && last.role === "assistant") {
    return [messages, messages.length - 1];
  }
  const newMsg: ChatMessage = { role: "assistant", content: "", blocks: [] };
  return [[...messages, newMsg], messages.length];
}

/**
 * Update the assistant message at `index` within `messages`, returning a new array.
 */
function updateAssistant(
  messages: ChatMessage[],
  index: number,
  updater: (msg: ChatMessage) => ChatMessage,
): ChatMessage[] {
  const copy = [...messages];
  copy[index] = updater(copy[index]);
  return copy;
}

function reducer(state: ConversationState, action: Action): ConversationState {
  switch (action.type) {
    case "user_message":
      return {
        ...state,
        messages: [...state.messages, { role: "user", content: action.content, blocks: [] }],
        streamingText: "",
        isStreaming: true,
        error: null,
        lastTurnUsage: null,
      };

    case "conversation_created":
      return {
        ...state,
        conversationId: action.conversationId,
      };

    case "token": {
      const [msgs, idx] = ensureAssistantMessage(state.messages);
      const updated = updateAssistant(msgs, idx, (msg) => {
        const blocks = [...msg.blocks];
        const last = blocks[blocks.length - 1];
        if (last && last.kind === "text") {
          blocks[blocks.length - 1] = { ...last, text: last.text + action.token };
        } else {
          blocks.push({ kind: "text", text: action.token });
        }
        return { ...msg, blocks, content: flattenText(blocks) };
      });
      return { ...state, messages: updated, streamingText: state.streamingText + action.token };
    }

    case "thinking_start": {
      const [msgs, idx] = ensureAssistantMessage(state.messages);
      const updated = updateAssistant(msgs, idx, (msg) => ({
        ...msg,
        blocks: [...msg.blocks, { kind: "thinking" as const, text: "", done: false }],
      }));
      return { ...state, messages: updated };
    }

    case "thinking_delta": {
      const [msgs, idx] = ensureAssistantMessage(state.messages);
      const updated = updateAssistant(msgs, idx, (msg) => {
        const blocks = [...msg.blocks];
        // Find the last thinking block that isn't done.
        for (let i = blocks.length - 1; i >= 0; i--) {
          const b = blocks[i];
          if (b.kind === "thinking" && !b.done) {
            blocks[i] = { ...b, text: b.text + action.delta };
            break;
          }
        }
        return { ...msg, blocks };
      });
      return { ...state, messages: updated };
    }

    case "thinking_end": {
      const [msgs, idx] = ensureAssistantMessage(state.messages);
      const updated = updateAssistant(msgs, idx, (msg) => {
        const blocks = [...msg.blocks];
        for (let i = blocks.length - 1; i >= 0; i--) {
          const b = blocks[i];
          if (b.kind === "thinking" && !b.done) {
            blocks[i] = { ...b, done: true };
            break;
          }
        }
        return { ...msg, blocks };
      });
      return { ...state, messages: updated };
    }

    case "tool_call_start": {
      const [msgs, idx] = ensureAssistantMessage(state.messages);
      const updated = updateAssistant(msgs, idx, (msg) => ({
        ...msg,
        blocks: [
          ...msg.blocks,
          {
            kind: "tool_call" as const,
            toolCallId: action.toolCallId,
            toolName: action.toolName,
            args: action.args,
            output: "",
            done: false,
          },
        ],
      }));
      return { ...state, messages: updated };
    }

    case "tool_call_output": {
      const [msgs, idx] = ensureAssistantMessage(state.messages);
      const updated = updateAssistant(msgs, idx, (msg) => {
        const blocks = [...msg.blocks];
        for (let i = blocks.length - 1; i >= 0; i--) {
          const b = blocks[i];
          if (b.kind === "tool_call" && b.toolCallId === action.toolCallId) {
            blocks[i] = { ...b, output: b.output + (action.output ?? "") };
            break;
          }
        }
        return { ...msg, blocks };
      });
      return { ...state, messages: updated };
    }

    case "tool_call_end": {
      const [msgs, idx] = ensureAssistantMessage(state.messages);
      const updated = updateAssistant(msgs, idx, (msg) => {
        const blocks = [...msg.blocks];
        for (let i = blocks.length - 1; i >= 0; i--) {
          const b = blocks[i];
          if (b.kind === "tool_call" && b.toolCallId === action.toolCallId) {
            blocks[i] = {
              ...b,
              result: action.result,
              duration: action.duration,
              success: action.success,
              done: true,
            };
            break;
          }
        }
        return { ...msg, blocks };
      });
      return { ...state, messages: updated };
    }

    case "status":
      return {
        ...state,
        agentState: action.state,
      };

    case "turn_complete": {
      // Finalize: make sure the last assistant message has its content populated.
      const msgs = [...state.messages];
      const last = msgs[msgs.length - 1];
      if (last && last.role === "assistant") {
        msgs[msgs.length - 1] = { ...last, content: flattenText(last.blocks) };
      }
      return {
        ...state,
        messages: msgs,
        streamingText: "",
        isStreaming: false,
        agentState: "idle",
        lastTurnUsage: action.usage,
      };
    }

    case "turn_cancelled": {
      const msgs = [...state.messages];
      const last = msgs[msgs.length - 1];
      if (last && last.role === "assistant") {
        msgs[msgs.length - 1] = { ...last, content: flattenText(last.blocks) };
      }
      return {
        ...state,
        messages: msgs,
        streamingText: "",
        isStreaming: false,
        agentState: "idle",
      };
    }

    case "error":
      return {
        ...state,
        error: action.message,
        isStreaming: false,
      };

    case "set_conversation_id":
      return {
        ...state,
        conversationId: action.conversationId,
      };

    case "load_history":
      return {
        ...state,
        messages: action.messages,
      };

    case "context_debug":
      return {
        ...state,
        lastContextDebug: action.report,
      };

    default:
      return state;
  }
}

/** Extract plain text from blocks (for content fallback). */
function flattenText(blocks: ContentBlock[]): string {
  return blocks
    .filter((b): b is TextBlock => b.kind === "text")
    .map((b) => b.text)
    .join("");
}

// ── Hook ─────────────────────────────────────────────────────────────

export function useConversation(conversationId?: string) {
  const [state, dispatch] = useReducer(reducer, initialState);
  const { status, lastEvent, sendMessage: wsSend, cancel: wsCancel } = useWebSocket();

  // Set conversation ID from route param.
  useEffect(() => {
    if (conversationId) {
      dispatch({ type: "set_conversation_id", conversationId });
    }
  }, [conversationId]);

  // Dispatch server events into reducer.
  useEffect(() => {
    if (!lastEvent) return;

    const msg = lastEvent as ServerMessage<ServerEventData>;

    switch (msg.type) {
      case "token": {
        const data = msg.data as TokenEvent;
        dispatch({ type: "token", token: data.token });
        break;
      }
      case "thinking_start": {
        dispatch({ type: "thinking_start" });
        break;
      }
      case "thinking_delta": {
        const data = msg.data as ThinkingDeltaEvent;
        dispatch({ type: "thinking_delta", delta: data.delta });
        break;
      }
      case "thinking_end": {
        dispatch({ type: "thinking_end" });
        break;
      }
      case "tool_call_start": {
        const data = msg.data as ToolCallStartEvent;
        dispatch({
          type: "tool_call_start",
          toolCallId: data.tool_call_id,
          toolName: data.tool_name,
          args: data.arguments,
        });
        break;
      }
      case "tool_call_output": {
        const data = msg.data as ToolCallOutputEvent;
        dispatch({
          type: "tool_call_output",
          toolCallId: data.tool_call_id,
          output: data.output ?? "",
        });
        break;
      }
      case "tool_call_end": {
        const data = msg.data as ToolCallEndEvent;
        dispatch({
          type: "tool_call_end",
          toolCallId: data.tool_call_id,
          result: data.result,
          duration: data.duration,
          success: data.success,
        });
        break;
      }
      case "status": {
        const data = msg.data as StatusEvent;
        dispatch({ type: "status", state: data.state });
        break;
      }
      case "turn_complete": {
        const data = msg.data as TurnCompleteEvent;
        dispatch({
          type: "turn_complete",
          usage: {
            turnNumber: data.turn_number,
            iterationCount: data.iteration_count,
            inputTokens: data.total_input_tokens,
            outputTokens: data.total_output_tokens,
            duration: data.duration,
          },
        });
        break;
      }
      case "turn_cancelled": {
        dispatch({ type: "turn_cancelled" });
        break;
      }
      case "error": {
        const data = msg.data as ErrorEvent;
        dispatch({ type: "error", message: data.message ?? "Unknown error" });
        break;
      }
      case "conversation_created": {
        const data = msg.data as ConversationCreatedEvent;
        dispatch({ type: "conversation_created", conversationId: data.conversation_id });
        break;
      }
      case "context_debug": {
        const data = msg.data as ContextDebugEvent;
        if (data.report) {
          dispatch({ type: "context_debug", report: data.report as Record<string, unknown> });
        }
        break;
      }
    }
  }, [lastEvent]);

  const sendMessage = useCallback(
    (content: string) => {
      dispatch({ type: "user_message", content });
      wsSend(content, state.conversationId ?? undefined);
    },
    [wsSend, state.conversationId],
  );

  const cancel = useCallback(() => {
    wsCancel();
  }, [wsCancel]);

  const loadHistory = useCallback((messages: ChatMessage[]) => {
    dispatch({ type: "load_history", messages });
  }, []);

  return {
    ...state,
    connectionStatus: status,
    sendMessage,
    cancel,
    loadHistory,
  };
}
