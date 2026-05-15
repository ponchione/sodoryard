import { useCallback, useEffect, useRef, useState } from "react";
import type { ConnectionStatus, SubscriptionUnsubscribe } from "@shunter/client";

import {
  createProjectMemoryClient,
  projectMemoryConnectionErrorMessage,
  queryRecentChainEventsDecoded,
  queryRecentChainsDecoded,
  subscribeLiveRecentChainEvents,
  subscribeLiveRecentChains,
  type LiveRecentChainEventsViewRow,
  type ProjectMemoryClient,
  type RecentChainEventsQueryRow,
  verifyProjectMemoryRuntimeContract,
} from "@/lib/project-memory/client";

export interface UseProjectMemoryChainsOptions {
  enabled?: boolean;
  onChanged?: () => void | Promise<void>;
  verifyContract?: boolean;
}

export interface ProjectMemoryRecentEvent {
  id: string;
  chainId: string;
  stepId: string;
  sequence: string;
  eventType: string;
  createdAt: string;
  createdAtUs: bigint;
  payloadJson: string;
}

export interface UseProjectMemoryChainsReturn {
  status: ConnectionStatus;
  error: string | null;
  rowCount: number | null;
  eventCount: number | null;
  recentEvents: ProjectMemoryRecentEvent[];
  refresh: () => Promise<void>;
}

interface ProjectMemorySnapshot {
  chainRowCount: number;
  eventRowCount: number;
  recentEvents: ProjectMemoryRecentEvent[];
}

type ProjectMemoryEventRow = RecentChainEventsQueryRow | LiveRecentChainEventsViewRow;

const recentEventLimit = 10;

function changedRowCount(current: number | null, inserts: number, deletes: number): number | null {
  if (current === null) return null;
  return Math.max(0, current + inserts - deletes);
}

function projectMemoryTimeToISO(value: bigint): string {
  if (value === 0n) return "";
  return new Date(Number(value / 1000n)).toISOString();
}

function mapRecentEvent(row: ProjectMemoryEventRow): ProjectMemoryRecentEvent {
  return {
    id: row.id,
    chainId: row.chainId,
    stepId: row.stepId,
    sequence: row.sequence.toString(),
    eventType: row.eventType,
    createdAt: projectMemoryTimeToISO(row.createdAtUs),
    createdAtUs: row.createdAtUs,
    payloadJson: row.payloadJson,
  };
}

function sortRecentEvents(events: ProjectMemoryRecentEvent[]): ProjectMemoryRecentEvent[] {
  return [...events].sort((a, b) => {
    if (a.createdAtUs !== b.createdAtUs) return a.createdAtUs > b.createdAtUs ? -1 : 1;
    const aSequence = BigInt(a.sequence || "0");
    const bSequence = BigInt(b.sequence || "0");
    if (aSequence !== bSequence) return aSequence > bSequence ? -1 : 1;
    return a.id.localeCompare(b.id);
  });
}

function mergeRecentEvents(
  current: ProjectMemoryRecentEvent[],
  inserts: readonly ProjectMemoryEventRow[],
  deletes: readonly ProjectMemoryEventRow[],
): ProjectMemoryRecentEvent[] {
  const byID = new Map(current.map((event) => [event.id, event]));
  for (const row of deletes) byID.delete(row.id);
  for (const row of inserts) byID.set(row.id, mapRecentEvent(row));
  return sortRecentEvents(Array.from(byID.values())).slice(0, recentEventLimit);
}

export function useProjectMemoryChains(
  options: UseProjectMemoryChainsOptions = {},
): UseProjectMemoryChainsReturn {
  const enabled = options.enabled ?? true;
  const onChanged = options.onChanged;
  const verifyContract = options.verifyContract ?? true;
  const onChangedRef = useRef(onChanged);
  const clientRef = useRef<ProjectMemoryClient | null>(null);
  const [status, setStatus] = useState<ConnectionStatus>("idle");
  const [error, setError] = useState<string | null>(null);
  const [rowCount, setRowCount] = useState<number | null>(null);
  const [eventCount, setEventCount] = useState<number | null>(null);
  const [recentEvents, setRecentEvents] = useState<ProjectMemoryRecentEvent[]>([]);

  useEffect(() => {
    onChangedRef.current = onChanged;
  }, [onChanged]);

  const loadSnapshot = useCallback(async (client: ProjectMemoryClient): Promise<ProjectMemorySnapshot> => {
    const [chainsResult, eventsResult] = await Promise.all([
      queryRecentChainsDecoded(client.runDeclaredQuery),
      queryRecentChainEventsDecoded(client.runDeclaredQuery),
    ]);
    const chainsTable = chainsResult.tables.find((table) => table.tableName === "chains");
    const eventsTable = eventsResult.tables.find((table) => table.tableName === "events");
    const eventRows = eventsTable?.rows ?? [];
    return {
      chainRowCount: chainsTable?.rows.length ?? 0,
      eventRowCount: eventRows.length,
      recentEvents: sortRecentEvents(eventRows.map(mapRecentEvent)).slice(0, recentEventLimit),
    };
  }, []);

  const refresh = useCallback(async () => {
    const client = clientRef.current;
    if (client?.state.status !== "connected") return;
    const snapshot = await loadSnapshot(client);
    setRowCount(snapshot.chainRowCount);
    setEventCount(snapshot.eventRowCount);
    setRecentEvents(snapshot.recentEvents);
  }, [loadSnapshot]);

  useEffect(() => {
    if (!enabled) {
      setStatus("idle");
      setError(null);
      setRowCount(null);
      setEventCount(null);
      setRecentEvents([]);
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
        setRowCount(snapshot.chainRowCount);
        setEventCount(snapshot.eventRowCount);
        setRecentEvents(snapshot.recentEvents);
        const chainsUnsubscribe = await subscribeLiveRecentChains(client.subscribeDeclaredView, {
          onInitialRows: (rows) => {
            if (!cancelled) setRowCount(rows.length);
          },
          onUpdate: (update) => {
            if (cancelled) return;
            setRowCount((current) => changedRowCount(current, update.inserts.length, update.deletes.length));
            void onChangedRef.current?.();
          },
        });
        if (cancelled) {
          void chainsUnsubscribe();
          return;
        }
        unsubscribes.push(chainsUnsubscribe);
        const eventsUnsubscribe = await subscribeLiveRecentChainEvents(client.subscribeDeclaredView, {
          onInitialRows: (rows) => {
            if (cancelled) return;
            const events = sortRecentEvents(rows.map(mapRecentEvent)).slice(0, recentEventLimit);
            setRecentEvents(events);
            setEventCount(rows.length);
          },
          onUpdate: (update) => {
            if (cancelled) return;
            setRecentEvents((current) => mergeRecentEvents(current, update.inserts, update.deletes));
            setEventCount((current) => changedRowCount(current, update.inserts.length, update.deletes.length));
            void onChangedRef.current?.();
          },
        });
        if (cancelled) {
          void eventsUnsubscribe();
          return;
        }
        unsubscribes.push(eventsUnsubscribe);
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
  }, [enabled, loadSnapshot, verifyContract]);

  return { status, error, rowCount, eventCount, recentEvents, refresh };
}
