import { useCallback, useEffect, useRef, useState } from "react";
import type {
  ClientMessage,
  ServerMessage,
  ServerEventData,
} from "@/types/events";

export type ConnectionStatus = "connecting" | "connected" | "disconnected";

export interface UseWebSocketReturn {
  /** Current connection status. */
  status: ConnectionStatus;
  /** Last event received from the server. Consumers should watch this via useEffect. */
  lastEvent: ServerMessage<ServerEventData> | null;
  /** Send a user message. Creates a new conversation if conversationId is omitted. */
  sendMessage: (content: string, conversationId?: string) => void;
  /** Cancel the in-progress turn. */
  cancel: () => void;
}

/**
 * Manages a single WebSocket connection to the sirtopham backend.
 *
 * Reconnects automatically on disconnect with exponential backoff.
 * The hook connects on mount and disconnects on unmount.
 */
export function useWebSocket(): UseWebSocketReturn {
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const reconnectDelay = useRef(1000);

  const [status, setStatus] = useState<ConnectionStatus>("disconnected");
  const [lastEvent, setLastEvent] = useState<ServerMessage<ServerEventData> | null>(null);

  const connect = useCallback(() => {
    // Build WS URL relative to the current page origin.
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const url = `${protocol}//${window.location.host}/api/ws`;

    setStatus("connecting");
    const ws = new WebSocket(url);
    wsRef.current = ws;

    ws.onopen = () => {
      setStatus("connected");
      reconnectDelay.current = 1000; // reset backoff
    };

    ws.onmessage = (ev: MessageEvent) => {
      try {
        const msg = JSON.parse(ev.data as string) as ServerMessage<ServerEventData>;
        setLastEvent(msg);
      } catch {
        // Ignore malformed messages.
      }
    };

    ws.onclose = () => {
      setStatus("disconnected");
      wsRef.current = null;
      // Reconnect with exponential backoff (max 30s).
      const delay = reconnectDelay.current;
      reconnectDelay.current = Math.min(delay * 2, 30_000);
      reconnectTimer.current = setTimeout(connect, delay);
    };

    ws.onerror = () => {
      // onclose will fire after onerror, triggering reconnect.
    };
  }, []);

  useEffect(() => {
    connect();
    return () => {
      clearTimeout(reconnectTimer.current);
      wsRef.current?.close();
      wsRef.current = null;
    };
  }, [connect]);

  const send = useCallback((msg: ClientMessage) => {
    const ws = wsRef.current;
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify(msg));
    }
  }, []);

  const sendMessage = useCallback(
    (content: string, conversationId?: string) => {
      send({
        type: "message",
        content,
        conversation_id: conversationId,
      });
    },
    [send],
  );

  const cancel = useCallback(() => {
    send({ type: "cancel" });
  }, [send]);

  return { status, lastEvent, sendMessage, cancel };
}
