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
  details?: Record<string, unknown>;
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
  /** Non-empty once visible text has streamed for the current turn. */
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
  | { type: "tool_call_end"; toolCallId: string; result?: string; details?: Record<string, unknown>; duration?: number; success?: boolean }
  | { type: "status"; state: AgentState }
  | { type: "turn_complete"; usage: TurnUsage }
  | { type: "turn_cancelled" }
  | { type: "error"; message: string }
  | { type: "route_conversation_changed"; conversationId: string | null }
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

function updateLastMatchingBlock(
  blocks: ContentBlock[],
  predicate: (block: ContentBlock) => boolean,
  updater: (block: ContentBlock) => ContentBlock,
): ContentBlock[] {
  const copy = [...blocks];
  for (let i = copy.length - 1; i >= 0; i--) {
    const block = copy[i];
    if (predicate(block)) {
      copy[i] = updater(block);
      break;
    }
  }
  return copy;
}

function updateToolCallBlock(
  blocks: ContentBlock[],
  toolCallId: string,
  updater: (block: ToolCallBlock) => ToolCallBlock,
): ContentBlock[] {
  return updateLastMatchingBlock(
    blocks,
    (block) => block.kind === "tool_call" && block.toolCallId === toolCallId,
    (block) => block.kind === "tool_call" ? updater(block) : block,
  );
}

function finalizeAssistantMessages(messages: ChatMessage[]): ChatMessage[] {
  const copy = [...messages];
  const last = copy[copy.length - 1];
  if (last && last.role === "assistant") {
    copy[copy.length - 1] = { ...last, content: flattenText(last.blocks) };
  }
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
        return { ...msg, blocks, content: msg.content + action.token };
      });
      return { ...state, messages: updated, streamingText: state.streamingText || "1" };
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
        const blocks = updateLastMatchingBlock(
          msg.blocks,
          (block) => block.kind === "thinking" && !block.done,
          (block) => block.kind === "thinking" ? { ...block, text: block.text + action.delta } : block,
        );
        return { ...msg, blocks };
      });
      return { ...state, messages: updated };
    }

    case "thinking_end": {
      const [msgs, idx] = ensureAssistantMessage(state.messages);
      const updated = updateAssistant(msgs, idx, (msg) => {
        const blocks = updateLastMatchingBlock(
          msg.blocks,
          (block) => block.kind === "thinking" && !block.done,
          (block) => block.kind === "thinking" ? { ...block, done: true } : block,
        );
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
        const blocks = updateToolCallBlock(
          msg.blocks,
          action.toolCallId,
          (block) => ({ ...block, output: block.output + (action.output ?? "") }),
        );
        return { ...msg, blocks };
      });
      return { ...state, messages: updated };
    }

    case "tool_call_end": {
      const [msgs, idx] = ensureAssistantMessage(state.messages);
      const updated = updateAssistant(msgs, idx, (msg) => {
        const blocks = updateToolCallBlock(
          msg.blocks,
          action.toolCallId,
          (block) => ({
            ...block,
            result: action.result,
            details: action.details,
            duration: action.duration,
            success: action.success,
            done: true,
          }),
        );
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
      return {
        ...state,
        messages: finalizeAssistantMessages(state.messages),
        streamingText: "",
        isStreaming: false,
        agentState: "idle",
        lastTurnUsage: action.usage,
      };
    }

    case "turn_cancelled": {
      return {
        ...state,
        messages: finalizeAssistantMessages(state.messages),
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

    case "route_conversation_changed":
      if (state.conversationId === action.conversationId) {
        return state;
      }
      return {
        ...initialState,
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
  const { status, eventQueue, eventTick, sendMessage: wsSend, cancel: wsCancel } = useWebSocket();

  useEffect(() => {
    dispatch({ type: "route_conversation_changed", conversationId: conversationId ?? null });
  }, [conversationId]);

  // Drain every queued WebSocket event in arrival order. `splice(0)` releases
  // processed event objects so long sessions do not retain the full stream.
  useEffect(() => {
    const queue = eventQueue.current.splice(0) as ServerMessage<ServerEventData>[];
    let tokenBuffer = "";
    const flushTokens = () => {
      if (!tokenBuffer) {
        return;
      }
      dispatch({ type: "token", token: tokenBuffer });
      tokenBuffer = "";
    };

    for (const msg of queue) {
      switch (msg.type) {
      case "token": {
        const data = msg.data as TokenEvent;
        tokenBuffer += data.token;
        break;
      }
      case "thinking_start": {
        flushTokens();
        dispatch({ type: "thinking_start" });
        break;
      }
      case "thinking_delta": {
        flushTokens();
        const data = msg.data as ThinkingDeltaEvent;
        dispatch({ type: "thinking_delta", delta: data.delta });
        break;
      }
      case "thinking_end": {
        flushTokens();
        dispatch({ type: "thinking_end" });
        break;
      }
      case "tool_call_start": {
        flushTokens();
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
        flushTokens();
        const data = msg.data as ToolCallOutputEvent;
        dispatch({
          type: "tool_call_output",
          toolCallId: data.tool_call_id,
          output: data.output ?? "",
        });
        break;
      }
      case "tool_call_end": {
        flushTokens();
        const data = msg.data as ToolCallEndEvent;
        dispatch({
          type: "tool_call_end",
          toolCallId: data.tool_call_id,
          result: data.result,
          details: data.details,
          duration: data.duration,
          success: data.success,
        });
        break;
      }
      case "status": {
        flushTokens();
        const data = msg.data as StatusEvent;
        dispatch({ type: "status", state: data.state });
        break;
      }
      case "turn_complete": {
        flushTokens();
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
        flushTokens();
        dispatch({ type: "turn_cancelled" });
        break;
      }
      case "error": {
        flushTokens();
        const data = msg.data as ErrorEvent;
        dispatch({ type: "error", message: data.message ?? "Unknown error" });
        break;
      }
      case "conversation_created": {
        flushTokens();
        const data = msg.data as ConversationCreatedEvent;
        dispatch({ type: "conversation_created", conversationId: data.conversation_id });
        break;
      }
      case "context_debug": {
        flushTokens();
        const data = msg.data as ContextDebugEvent;
        if (data.report) {
          dispatch({ type: "context_debug", report: data.report as Record<string, unknown> });
        }
        break;
      }
      }
    }
    flushTokens();
  }, [eventTick, eventQueue]);

  const sendMessage = useCallback(
    (content: string) => {
      if (!wsSend(content, state.conversationId ?? undefined)) {
        dispatch({
          type: "error",
          message: status === "connecting"
            ? "Still connecting. Try again when the connection is ready."
            : "Disconnected. Reconnecting to the server.",
        });
        return false;
      }
      dispatch({ type: "user_message", content });
      return true;
    },
    [wsSend, state.conversationId, status],
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
