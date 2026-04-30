import { useCallback, useEffect, useRef, useState } from "react";
import type {
  ClientMessage,
  ServerMessage,
  ServerEventData,
} from "@/types/events";

export type ConnectionStatus = "connecting" | "connected" | "disconnected";

export type ServerEvent = ServerMessage<ServerEventData>;

export interface UseWebSocketReturn {
  /** Current connection status. */
  status: ConnectionStatus;
  /**
   * Ref-backed queue of server events received in arrival order. Consumers
   * should drain and clear the queue whenever `eventTick` changes. Using a
   * ref + tick pattern instead of a single
   * `lastEvent` state avoids losing events when React 18's automatic batching
   * coalesces rapid-fire WebSocket frames into a single render — the prior
   * design silently dropped all but the last frame in a batch (B1 fix).
   */
  eventQueue: React.MutableRefObject<ServerEvent[]>;
  /** Increments once per received event. Use as a useEffect dependency. */
  eventTick: number;
  /** Send a user message. Creates a new conversation if conversationId is omitted. */
  sendMessage: (content: string, conversationId?: string) => void;
  /** Cancel the in-progress turn. */
  cancel: () => void;
}

/**
 * Manages a single WebSocket connection to the Sodoryard backend.
 *
 * Reconnects automatically on disconnect with exponential backoff.
 * The hook connects on mount and disconnects on unmount.
 */
export function useWebSocket(): UseWebSocketReturn {
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const reconnectDelay = useRef(1000);
  const shouldReconnect = useRef(true);
  const connectRef = useRef<() => void>(() => {});

  const [status, setStatus] = useState<ConnectionStatus>("disconnected");
  // Ref-backed queue: ref writes are synchronous and never batched, so every
  // WebSocket frame is preserved in arrival order until the consumer drains
  // it. `eventTick` is a cheap counter that nudges React to re-render and fire
  // consumer effects.
  const eventQueue = useRef<ServerEvent[]>([]);
  const [eventTick, setEventTick] = useState(0);

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
        const msg = JSON.parse(ev.data as string) as ServerEvent;
        eventQueue.current.push(msg);
        setEventTick((t) => t + 1);
      } catch {
        // Ignore malformed messages.
      }
    };

    ws.onclose = () => {
      if (wsRef.current === ws) {
        wsRef.current = null;
      }
      setStatus("disconnected");
      if (!shouldReconnect.current) {
        return;
      }
      // Reconnect with exponential backoff (max 30s).
      const delay = reconnectDelay.current;
      reconnectDelay.current = Math.min(delay * 2, 30_000);
      reconnectTimer.current = setTimeout(() => connectRef.current(), delay);
    };

    ws.onerror = () => {
      // onclose will fire after onerror, triggering reconnect.
    };
  }, []);
  useEffect(() => {
    connectRef.current = connect;
    shouldReconnect.current = true;
    connect();
    return () => {
      shouldReconnect.current = false;
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

  return { status, eventQueue, eventTick, sendMessage, cancel };
}
