import { useCallback, useEffect, useReducer } from "react";
import { useWebSocket } from "@/hooks/use-websocket";
import type {
  ServerMessage,
  ServerEventData,
  TokenEvent,
  StatusEvent,
  ErrorEvent,
  ConversationCreatedEvent,
  AgentState,
} from "@/types/events";

// ── Public types ────────────────────────────────────────────────────

export interface ChatMessage {
  role: "user" | "assistant";
  content: string;
}

export interface ConversationState {
  conversationId: string | null;
  messages: ChatMessage[];
  /** Text being streamed for the current assistant response. */
  streamingText: string;
  /** Is a turn currently in progress? */
  isStreaming: boolean;
  /** Agent state from status events. */
  agentState: AgentState | null;
  /** Last error from the backend. */
  error: string | null;
}

// ── Reducer ─────────────────────────────────────────────────────────

type Action =
  | { type: "user_message"; content: string }
  | { type: "conversation_created"; conversationId: string }
  | { type: "token"; token: string }
  | { type: "status"; state: AgentState }
  | { type: "turn_complete" }
  | { type: "turn_cancelled" }
  | { type: "error"; message: string }
  | { type: "set_conversation_id"; conversationId: string };

const initialState: ConversationState = {
  conversationId: null,
  messages: [],
  streamingText: "",
  isStreaming: false,
  agentState: null,
  error: null,
};

function reducer(state: ConversationState, action: Action): ConversationState {
  switch (action.type) {
    case "user_message":
      return {
        ...state,
        messages: [...state.messages, { role: "user", content: action.content }],
        streamingText: "",
        isStreaming: true,
        error: null,
      };

    case "conversation_created":
      return {
        ...state,
        conversationId: action.conversationId,
      };

    case "token":
      return {
        ...state,
        streamingText: state.streamingText + action.token,
      };

    case "status":
      return {
        ...state,
        agentState: action.state,
      };

    case "turn_complete":
    case "turn_cancelled": {
      const newMessages = [...state.messages];
      if (state.streamingText.length > 0) {
        newMessages.push({ role: "assistant", content: state.streamingText });
      }
      return {
        ...state,
        messages: newMessages,
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

    default:
      return state;
  }
}

// ── Hook ────────────────────────────────────────────────────────────

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
      case "status": {
        const data = msg.data as StatusEvent;
        dispatch({ type: "status", state: data.state });
        break;
      }
      case "turn_complete": {
        dispatch({ type: "turn_complete" });
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

  return {
    ...state,
    connectionStatus: status,
    sendMessage,
    cancel,
  };
}
