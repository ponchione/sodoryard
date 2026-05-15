import { act, renderHook, waitFor } from "@testing-library/react";
import type { ConnectionStatus } from "@shunter/client";
import { beforeEach, describe, expect, it, vi } from "vitest";

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

type MockDeclaredQueryResult = {
  tables: Array<{
    tableName: string;
    rows: readonly unknown[];
  }>;
};

type MockProjectMemoryEventRow = {
  id: string;
  chainId: string;
  stepId: string;
  sequence: bigint;
  eventType: string;
  createdAtUs: bigint;
  payloadJson: string;
};

type MockSubscriptionOptions = {
  onInitialRows?: (rows: readonly unknown[]) => void;
  onUpdate?: (update: { inserts: readonly unknown[]; deletes: readonly unknown[] }) => void;
};

const mocks = vi.hoisted(() => ({
  createProjectMemoryClientMock: vi.fn(),
  disposeMock: vi.fn(),
  queryRecentChainEventsDecodedMock: vi.fn(),
  queryRecentChainsDecodedMock: vi.fn(),
  subscribeLiveRecentChainEventsMock: vi.fn(),
  subscribeLiveRecentChainsMock: vi.fn(),
  unsubscribeMock: vi.fn(),
  verifyProjectMemoryRuntimeContractMock: vi.fn(),
}));

vi.mock("@/lib/project-memory/client", () => ({
  createProjectMemoryClient: mocks.createProjectMemoryClientMock,
  queryRecentChainEventsDecoded: mocks.queryRecentChainEventsDecodedMock,
  queryRecentChainsDecoded: mocks.queryRecentChainsDecodedMock,
  projectMemoryConnectionErrorMessage: (error: unknown) => (
    error instanceof Error ? error.message : "Project memory connection failed"
  ),
  subscribeLiveRecentChainEvents: mocks.subscribeLiveRecentChainEventsMock,
  subscribeLiveRecentChains: mocks.subscribeLiveRecentChainsMock,
  verifyProjectMemoryRuntimeContract: mocks.verifyProjectMemoryRuntimeContractMock,
}));

import { useProjectMemoryChains } from "./use-project-memory-chains";

let chainSubscriptionOptions: MockSubscriptionOptions | undefined;
let eventSubscriptionOptions: MockSubscriptionOptions | undefined;

function chainRows(count: number): MockDeclaredQueryResult {
  return {
    tables: [
      {
        tableName: "chains",
        rows: Array.from({ length: count }, (_, index) => ({ id: `chain-${index + 1}` })),
      },
    ],
  };
}

function eventRow(overrides: Partial<MockProjectMemoryEventRow> = {}): MockProjectMemoryEventRow {
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

function eventRows(rows: MockProjectMemoryEventRow[]): MockDeclaredQueryResult {
  return {
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
  const runDeclaredQueryMock = vi.fn();
  const subscribeDeclaredViewMock = vi.fn();
  const connectMock = vi.fn(async () => {
    if (connectError) throw connectError;
    const previous = state;
    state = { status: "connected" };
    onStateChange?.({ previous, current: state });
    return {};
  });
  const client = {
    get state() {
      return state;
    },
    connect: connectMock,
    dispose: mocks.disposeMock,
    runDeclaredQuery: runDeclaredQueryMock,
    subscribeDeclaredView: subscribeDeclaredViewMock,
  };
  mocks.createProjectMemoryClientMock.mockImplementation((options: MockClientOptions = {}) => {
    onStateChange = options.onStateChange;
    return client;
  });
}

describe("useProjectMemoryChains", () => {
  beforeEach(() => {
    chainSubscriptionOptions = undefined;
    eventSubscriptionOptions = undefined;
    mocks.createProjectMemoryClientMock.mockReset();
    mocks.disposeMock.mockReset().mockResolvedValue(undefined);
    mocks.queryRecentChainEventsDecodedMock.mockReset().mockResolvedValue(eventRows([
      eventRow({ id: "event-1", sequence: 1n, createdAtUs: 1_700_000_000_000_000n }),
      eventRow({ id: "event-2", sequence: 2n, eventType: "step_completed", createdAtUs: 1_700_000_100_000_000n }),
    ]));
    mocks.queryRecentChainsDecodedMock.mockReset().mockResolvedValue(chainRows(2));
    mocks.subscribeLiveRecentChainsMock.mockReset().mockImplementation((
      _subscribeDeclaredView: unknown,
      options: MockSubscriptionOptions,
    ) => {
      chainSubscriptionOptions = options;
      return Promise.resolve(mocks.unsubscribeMock);
    });
    mocks.subscribeLiveRecentChainEventsMock.mockReset().mockImplementation((
      _subscribeDeclaredView: unknown,
      options: MockSubscriptionOptions,
    ) => {
      eventSubscriptionOptions = options;
      return Promise.resolve(mocks.unsubscribeMock);
    });
    mocks.unsubscribeMock.mockReset().mockResolvedValue(undefined);
    mocks.verifyProjectMemoryRuntimeContractMock.mockReset().mockResolvedValue({
      module: { name: "yard_project_memory", version: "0.13.0" },
    });
  });

  it("does not connect while disabled", () => {
    const { result } = renderHook(() => useProjectMemoryChains({ enabled: false }));

    expect(result.current.status).toBe("idle");
    expect(result.current.rowCount).toBeNull();
    expect(result.current.eventCount).toBeNull();
    expect(result.current.recentEvents).toEqual([]);
    expect(result.current.error).toBeNull();
    expect(mocks.createProjectMemoryClientMock).not.toHaveBeenCalled();
    expect(mocks.queryRecentChainEventsDecodedMock).not.toHaveBeenCalled();
    expect(mocks.verifyProjectMemoryRuntimeContractMock).not.toHaveBeenCalled();
  });

  it("loads snapshots, exposes recent events, and invalidates REST on live updates", async () => {
    installClient();
    const onChanged = vi.fn();
    const { result, unmount } = renderHook(() => useProjectMemoryChains({ onChanged }));

    await waitFor(() => expect(result.current.status).toBe("connected"));
    await waitFor(() => expect(result.current.rowCount).toBe(2));
    await waitFor(() => expect(result.current.eventCount).toBe(2));

    expect(mocks.verifyProjectMemoryRuntimeContractMock).toHaveBeenCalledTimes(1);
    expect(mocks.createProjectMemoryClientMock).toHaveBeenCalledTimes(1);
    expect(mocks.queryRecentChainsDecodedMock).toHaveBeenCalledTimes(1);
    expect(mocks.queryRecentChainEventsDecodedMock).toHaveBeenCalledTimes(1);
    expect(mocks.subscribeLiveRecentChainsMock).toHaveBeenCalledTimes(1);
    expect(mocks.subscribeLiveRecentChainEventsMock).toHaveBeenCalledTimes(1);
    expect(result.current.recentEvents.map((event) => event.id)).toEqual(["event-2", "event-1"]);
    expect(result.current.recentEvents[0]).toMatchObject({
      chainId: "chain-1",
      eventType: "step_completed",
      payloadJson: "{\"role\":\"coder\"}",
      sequence: "2",
    });

    act(() => {
      chainSubscriptionOptions?.onUpdate?.({ inserts: [{ id: "chain-3" }], deletes: [] });
    });

    expect(result.current.rowCount).toBe(3);
    expect(onChanged).toHaveBeenCalledTimes(1);

    act(() => {
      eventSubscriptionOptions?.onUpdate?.({
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

    expect(result.current.eventCount).toBe(3);
    expect(result.current.recentEvents[0]).toMatchObject({
      id: "event-3",
      eventType: "approval_required",
      sequence: "3",
    });
    expect(onChanged).toHaveBeenCalledTimes(2);

    await act(async () => {
      await result.current.refresh();
    });

    expect(mocks.queryRecentChainsDecodedMock).toHaveBeenCalledTimes(2);
    expect(mocks.queryRecentChainEventsDecodedMock).toHaveBeenCalledTimes(2);
    expect(result.current.rowCount).toBe(2);
    expect(result.current.eventCount).toBe(2);

    unmount();
    expect(mocks.unsubscribeMock).toHaveBeenCalledTimes(2);
    expect(mocks.disposeMock).toHaveBeenCalledTimes(1);
  });

  it("reports connection failures without subscribing", async () => {
    installClient(new Error("project memory socket unavailable"));

    const { result } = renderHook(() => useProjectMemoryChains());

    await waitFor(() => expect(result.current.status).toBe("failed"));

    expect(result.current.error).toBe("project memory socket unavailable");
    expect(mocks.subscribeLiveRecentChainsMock).not.toHaveBeenCalled();
    expect(mocks.subscribeLiveRecentChainEventsMock).not.toHaveBeenCalled();
  });

  it("reports contract mismatches before opening the socket", async () => {
    installClient();
    mocks.verifyProjectMemoryRuntimeContractMock.mockRejectedValue(new Error("project memory contract mismatch"));

    const { result } = renderHook(() => useProjectMemoryChains());

    await waitFor(() => expect(result.current.status).toBe("failed"));

    expect(result.current.error).toBe("project memory contract mismatch");
    expect(mocks.createProjectMemoryClientMock).not.toHaveBeenCalled();
    expect(mocks.queryRecentChainEventsDecodedMock).not.toHaveBeenCalled();
  });
});
