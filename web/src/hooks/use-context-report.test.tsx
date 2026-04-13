import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ContextReport } from "@/types/metrics";

const { apiGetMock } = vi.hoisted(() => ({
  apiGetMock: vi.fn(),
}));

vi.mock("@/lib/api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api")>("@/lib/api");
  return {
    ...actual,
    api: {
      ...actual.api,
      get: apiGetMock,
    },
  };
});

import { useContextReport } from "./use-context-report";

const liveReport = (turnNumber: number): ContextReport => ({
  conversation_id: "conv-1",
  turn_number: turnNumber,
  created_at: "2026-04-13T00:00:00Z",
  signal_stream: [],
});

describe("useContextReport", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    apiGetMock.mockReset();
    apiGetMock.mockResolvedValue({ stream: [] });
  });

  it("does not eagerly fetch the latest turn report before the defer window elapses", () => {
    const { result } = renderHook(() => useContextReport("conv-1"));

    act(() => {
      result.current.setHistoryTurns(1);
    });

    expect(apiGetMock).not.toHaveBeenCalled();
  });

  it("fetches the latest turn report after the defer window when no live report arrives", () => {
    const { result } = renderHook(() => useContextReport("conv-1"));

    act(() => {
      result.current.setHistoryTurns(1);
    });

    act(() => {
      vi.advanceTimersByTime(250);
    });

    expect(apiGetMock).toHaveBeenNthCalledWith(1, "/api/metrics/conversation/conv-1/context/1");
    expect(apiGetMock).toHaveBeenNthCalledWith(2, "/api/metrics/conversation/conv-1/context/1/signals");
  });

  it("cancels the deferred latest-turn fetch when a live context report arrives first", () => {
    const { result } = renderHook(() => useContextReport("conv-1"));

    act(() => {
      result.current.setHistoryTurns(1);
    });

    act(() => {
      result.current.setLiveReport(liveReport(1));
    });

    act(() => {
      vi.runAllTimers();
    });

    expect(apiGetMock).not.toHaveBeenCalled();
  });
});
