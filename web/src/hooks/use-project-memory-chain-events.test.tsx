import { act, renderHook, waitFor } from "@testing-library/react";
import type { ConnectionStatus } from "@shunter/client";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { EventsRow } from "@/lib/project-memory/client";

type MockConnectionState = {
  status: ConnectionStatus;
};

type MockConnectionStateChange = {
  previous: MockConnectionState;
  current: MockConnectionState;
};

type MockClientOptions = {
  onStateChange?: (change: MockConnectionStateChange) => void;
};

type MockSubscriptionOptions = {
  onInitialRows?: (rows: EventsRow[]) => void;
  onUpdate?: (update: { inserts: EventsRow[]; deletes: EventsRow[] }) => void;
};

const mocks = vi.hoisted(() => ({
  createProjectMemoryClientMock: vi.fn(),
  disposeMock: vi.fn(),
  queryChainEventsDecodedMock: vi.fn(),
  subscribeChainEventsMock: vi.fn(),
  unsubscribeMock: vi.fn(),
  verifyProjectMemoryRuntimeContractMock: vi.fn(),
}));

vi.mock("@/lib/project-memory/client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/project-memory/client")>()),
  createProjectMemoryClient: mocks.createProjectMemoryClientMock,
  queryChainEventsDecoded: mocks.queryChainEventsDecodedMock,
  subscribeChainEvents: mocks.subscribeChainEventsMock,
  verifyProjectMemoryRuntimeContract: mocks.verifyProjectMemoryRuntimeContractMock,
}));

import { useProjectMemoryChainEvents } from "./use-project-memory-chain-events";

let subscriptionOptions: MockSubscriptionOptions | undefined;

function eventRow(overrides: Partial<EventsRow> = {}): EventsRow {
  return {
    id: "event-1",
    chainId: "chain-1",
    stepId: "step-1",
    sequence: 1n,
    eventType: "step_started",
    createdAtUs: 1_700_000_000_000_000n,
    payloadJson: "{\"role\":\"coder\"}",
    ...overrides,
  };
}

function eventResult(rows: EventsRow[]) {
  return {
    queryName: "chain_events",
    tables: [
      {
        tableName: "events",
        rows,
      },
    ],
  };
}

function installClient(connectError?: Error) {
  let state: MockConnectionState = { status: "idle" };
  let onStateChange: MockClientOptions["onStateChange"];
  const runQueryMock = vi.fn();
  const subscribeViewMock = vi.fn();
  const connectMock = vi.fn(async () => {
    if (connectError) throw connectError;
    const previous = state;
    state = { status: "connected" };
    onStateChange?.({ previous, current: state });
  });
  const client = {
    get state() {
      return state;
    },
    connect: connectMock,
    dispose: mocks.disposeMock,
    runQuery: runQueryMock,
    subscribeView: subscribeViewMock,
  };
  mocks.createProjectMemoryClientMock.mockImplementation((options: MockClientOptions = {}) => {
    onStateChange = options.onStateChange;
    return client;
  });
}

describe("useProjectMemoryChainEvents", () => {
  beforeEach(() => {
    subscriptionOptions = undefined;
    mocks.createProjectMemoryClientMock.mockReset();
    mocks.disposeMock.mockReset().mockResolvedValue(undefined);
    mocks.queryChainEventsDecodedMock.mockReset().mockResolvedValue(eventResult([
      eventRow({ id: "event-2", sequence: 2n, eventType: "step_completed", createdAtUs: 1_700_000_100_000_000n }),
      eventRow({ id: "event-1", sequence: 1n, eventType: "step_started", createdAtUs: 1_700_000_000_000_000n }),
    ]));
    mocks.subscribeChainEventsMock.mockReset().mockImplementation((
      _subscribeView: unknown,
      _chainID: string,
      options: MockSubscriptionOptions,
    ) => {
      subscriptionOptions = options;
      return Promise.resolve(mocks.unsubscribeMock);
    });
    mocks.unsubscribeMock.mockReset().mockResolvedValue(undefined);
    mocks.verifyProjectMemoryRuntimeContractMock.mockReset().mockResolvedValue({
      module: { name: "yard_project_memory", version: "0.13.0" },
    });
  });

  it("does not connect while disabled", () => {
    const { result } = renderHook(() => useProjectMemoryChainEvents("chain-1", { enabled: false }));

    expect(result.current.status).toBe("idle");
    expect(result.current.events).toEqual([]);
    expect(result.current.error).toBeNull();
    expect(mocks.createProjectMemoryClientMock).not.toHaveBeenCalled();
    expect(mocks.queryChainEventsDecodedMock).not.toHaveBeenCalled();
  });

  it("loads SDK-decoded chain events and merges live updates", async () => {
    installClient();
    const onChanged = vi.fn();
    const onApprovalChanged = vi.fn();
    const { result, unmount } = renderHook(() => useProjectMemoryChainEvents("chain-1", {
      onApprovalChanged,
      onChanged,
    }));

    await waitFor(() => expect(result.current.status).toBe("connected"));
    await waitFor(() => expect(result.current.events.map((event) => event.id)).toEqual([1, 2]));

    expect(mocks.verifyProjectMemoryRuntimeContractMock).toHaveBeenCalledTimes(1);
    expect(mocks.createProjectMemoryClientMock).toHaveBeenCalledTimes(1);
    expect(mocks.queryChainEventsDecodedMock).toHaveBeenCalledTimes(1);
    expect(mocks.subscribeChainEventsMock).toHaveBeenCalledTimes(1);

    act(() => {
      subscriptionOptions?.onUpdate?.({
        inserts: [eventRow({
          id: "event-3",
          sequence: 3n,
          eventType: "approval_required",
          createdAtUs: 1_700_000_200_000_000n,
          payloadJson: "{\"tool\":\"shell\"}",
        })],
        deletes: [],
      });
    });

    expect(result.current.events.map((event) => event.id)).toEqual([1, 2, 3]);
    expect(result.current.events[2]).toMatchObject({
      event_type: "approval_required",
      event_data: "{\"tool\":\"shell\"}",
      created_at: "2023-11-14T22:16:40.000Z",
    });
    expect(onChanged).toHaveBeenCalledTimes(1);
    expect(onApprovalChanged).toHaveBeenCalledTimes(1);

    await act(async () => {
      await result.current.refresh();
    });

    expect(mocks.queryChainEventsDecodedMock).toHaveBeenCalledTimes(2);

    unmount();
    expect(mocks.unsubscribeMock).toHaveBeenCalledTimes(1);
    expect(mocks.disposeMock).toHaveBeenCalledTimes(1);
  });

  it("reports connection failures without subscribing", async () => {
    installClient(new Error("project memory socket unavailable"));

    const { result } = renderHook(() => useProjectMemoryChainEvents("chain-1"));

    await waitFor(() => expect(result.current.status).toBe("failed"));

    expect(result.current.error).toBe("project memory socket unavailable");
    expect(mocks.subscribeChainEventsMock).not.toHaveBeenCalled();
  });

  it("reports contract mismatches before opening the socket", async () => {
    installClient();
    mocks.verifyProjectMemoryRuntimeContractMock.mockRejectedValue(new Error("project memory contract mismatch"));

    const { result } = renderHook(() => useProjectMemoryChainEvents("chain-1"));

    await waitFor(() => expect(result.current.status).toBe("failed"));

    expect(result.current.error).toBe("project memory contract mismatch");
    expect(mocks.createProjectMemoryClientMock).not.toHaveBeenCalled();
    expect(mocks.queryChainEventsDecodedMock).not.toHaveBeenCalled();
  });
});
