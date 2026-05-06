import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ConnectionStatus, ServerEvent } from "@/hooks/use-websocket";

const { wsState } = vi.hoisted(() => ({
  wsState: {
    status: "connected" as ConnectionStatus,
    eventQueue: { current: [] as ServerEvent[] },
    eventTick: 0,
    sendMessage: vi.fn(),
    cancel: vi.fn(),
  },
}));

vi.mock("@/hooks/use-websocket", () => ({
  useWebSocket: () => wsState,
}));

import { useConversation } from "./use-conversation";

function tokenEvent(token: string): ServerEvent {
  return {
    type: "token",
    timestamp: new Date().toISOString(),
    data: {
      type: "token",
      token,
      time: new Date().toISOString(),
    },
  } as ServerEvent;
}

describe("useConversation", () => {
  beforeEach(() => {
    wsState.status = "connected";
    wsState.eventQueue.current = [];
    wsState.eventTick = 0;
    wsState.sendMessage.mockReset().mockReturnValue(true);
    wsState.cancel.mockReset();
  });

  it("drains processed websocket events and keeps streaming text bounded", () => {
    const { result, rerender } = renderHook(() => useConversation("conv-1"));

    act(() => {
      wsState.eventQueue.current.push(tokenEvent("a"), tokenEvent("b"), tokenEvent("c"));
      wsState.eventTick += 3;
      rerender();
    });

    expect(wsState.eventQueue.current).toHaveLength(0);
    expect(result.current.messages[0].content).toBe("abc");
    expect(result.current.streamingText).toBe("1");

    act(() => {
      wsState.eventQueue.current.push(tokenEvent("d"));
      wsState.eventTick += 1;
      rerender();
    });

    expect(wsState.eventQueue.current).toHaveLength(0);
    expect(result.current.messages[0].content).toBe("abcd");
    expect(result.current.streamingText).toBe("1");
  });

  it("does not append a local user message when websocket send fails", () => {
    wsState.status = "disconnected";
    wsState.sendMessage.mockReturnValue(false);

    const { result } = renderHook(() => useConversation("conv-1"));

    act(() => {
      expect(result.current.sendMessage("hello")).toBe(false);
    });

    expect(wsState.sendMessage).toHaveBeenCalledWith("hello", "conv-1");
    expect(result.current.messages).toHaveLength(0);
    expect(result.current.isStreaming).toBe(false);
    expect(result.current.error).toBe("Disconnected. Reconnecting to the server.");
  });
});
