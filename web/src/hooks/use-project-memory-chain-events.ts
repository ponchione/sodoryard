import { useCallback, useEffect, useRef, useState } from "react";
import type { ConnectionStatus, SubscriptionUnsubscribe } from "@shunter/client";

import {
  createProjectMemoryClient,
  projectMemoryConnectionErrorMessage,
  queryChainEventsDecoded,
  subscribeLiveChainEvents,
  type EventsRow,
  type ProjectMemoryClient,
  verifyProjectMemoryRuntimeContract,
} from "@/lib/project-memory/client";
import type { ChainEvent } from "@/types/chains";

export interface UseProjectMemoryChainEventsOptions {
  enabled?: boolean;
  limit?: number;
  onApprovalChanged?: () => void | Promise<void>;
  onChanged?: () => void | Promise<void>;
  verifyContract?: boolean;
}

export interface UseProjectMemoryChainEventsReturn {
  status: ConnectionStatus;
  error: string | null;
  events: ChainEvent[];
  refresh: () => Promise<void>;
}

interface ProjectMemoryChainEventsSnapshot {
  events: ChainEvent[];
}

const approvalEventTypes = new Set(["approval_required", "approval_decision"]);

function projectMemoryTimeToISO(value: bigint): string {
  if (value === 0n) return "";
  return new Date(Number(value / 1000n)).toISOString();
}

function mapChainEvent(row: EventsRow): ChainEvent {
  return {
    id: Number(row.sequence),
    chain_id: row.chainId,
    step_id: row.stepId,
    event_type: row.eventType,
    event_data: row.payloadJson,
    created_at: projectMemoryTimeToISO(row.createdAtUs),
  };
}

function sortChainEvents(events: ChainEvent[]): ChainEvent[] {
  return [...events].sort((a, b) => {
    if (a.id !== b.id) return a.id - b.id;
    return a.event_type.localeCompare(b.event_type);
  });
}

function normalizeEventLimit(limit: number | undefined): number {
  const requestedLimit = Number.isFinite(limit) ? Math.trunc(limit ?? 500) : 500;
  return Math.max(1, Math.min(requestedLimit, 500));
}

function limitChainEvents(events: ChainEvent[], limit: number): ChainEvent[] {
  const sorted = sortChainEvents(events);
  return sorted.length > limit ? sorted.slice(-limit) : sorted;
}

function mergeChainEventRows(
  current: ChainEvent[],
  inserts: readonly EventsRow[],
  deletes: readonly EventsRow[],
  limit: number,
): ChainEvent[] {
  const byID = new Map(current.map((event) => [event.id, event]));
  for (const row of deletes) byID.delete(Number(row.sequence));
  for (const row of inserts) byID.set(Number(row.sequence), mapChainEvent(row));
  return limitChainEvents(Array.from(byID.values()), limit);
}

function hasApprovalEvent(rows: readonly EventsRow[]): boolean {
  return rows.some((row) => approvalEventTypes.has(row.eventType));
}

export function useProjectMemoryChainEvents(
  chainID: string,
  options: UseProjectMemoryChainEventsOptions = {},
): UseProjectMemoryChainEventsReturn {
  const enabled = (options.enabled ?? true) && chainID.trim() !== "";
  const limit = normalizeEventLimit(options.limit);
  const onApprovalChanged = options.onApprovalChanged;
  const onChanged = options.onChanged;
  const verifyContract = options.verifyContract ?? true;
  const clientRef = useRef<ProjectMemoryClient | null>(null);
  const onApprovalChangedRef = useRef(onApprovalChanged);
  const onChangedRef = useRef(onChanged);
  const [status, setStatus] = useState<ConnectionStatus>("idle");
  const [error, setError] = useState<string | null>(null);
  const [events, setEvents] = useState<ChainEvent[]>([]);

  useEffect(() => {
    onApprovalChangedRef.current = onApprovalChanged;
    onChangedRef.current = onChanged;
  }, [onApprovalChanged, onChanged]);

  const loadSnapshot = useCallback(async (client: ProjectMemoryClient): Promise<ProjectMemoryChainEventsSnapshot> => {
    const result = await queryChainEventsDecoded(client.runDeclaredQuery, { chainId: chainID });
    const eventsTable = result.tables.find((table) => table.tableName === "events");
    return { events: limitChainEvents((eventsTable?.rows ?? []).map(mapChainEvent), limit) };
  }, [chainID, limit]);

  const refresh = useCallback(async () => {
    const client = clientRef.current;
    if (client?.state.status !== "connected") return;
    const snapshot = await loadSnapshot(client);
    setEvents(snapshot.events);
  }, [loadSnapshot]);

  useEffect(() => {
    if (!enabled) {
      setStatus("idle");
      setError(null);
      setEvents([]);
      return;
    }

    let cancelled = false;
    let client: ProjectMemoryClient | undefined;
    const unsubscribes: SubscriptionUnsubscribe[] = [];
    setStatus("connecting");
    setError(null);

    const cleanupResources = () => {
      for (const unsubscribe of unsubscribes.splice(0)) void unsubscribe();
      if (clientRef.current === client) clientRef.current = null;
      void client?.dispose();
    };

    const start = async () => {
      try {
        if (verifyContract) {
          await verifyProjectMemoryRuntimeContract();
          if (cancelled) return;
        }
        client = createProjectMemoryClient({
          onStateChange: ({ current }) => {
            if (!cancelled) setStatus(current.status);
          },
        });
        clientRef.current = client;
        setStatus(client.state.status);
        await client.connect();
        if (cancelled) return;
        const snapshot = await loadSnapshot(client);
        if (cancelled) return;
        setEvents(snapshot.events);
        const unsubscribe = await subscribeLiveChainEvents(client.subscribeDeclaredView, { chainId: chainID }, {
          onInitialRows: (rows) => {
            if (!cancelled) setEvents(limitChainEvents(rows.map(mapChainEvent), limit));
          },
          onUpdate: (update) => {
            if (cancelled) return;
            setEvents((current) => mergeChainEventRows(current, update.inserts, update.deletes, limit));
            void onChangedRef.current?.();
            if (hasApprovalEvent(update.inserts) || hasApprovalEvent(update.deletes)) {
              void onApprovalChangedRef.current?.();
            }
          },
        });
        if (cancelled) {
          void unsubscribe();
          return;
        }
        unsubscribes.push(unsubscribe);
      } catch (err) {
        if (cancelled) return;
        cleanupResources();
        setStatus("failed");
        setError(projectMemoryConnectionErrorMessage(err));
      }
    };

    void start();

    return () => {
      cancelled = true;
      cleanupResources();
    };
  }, [chainID, enabled, limit, loadSnapshot, verifyContract]);

  return { status, error, events, refresh };
}
