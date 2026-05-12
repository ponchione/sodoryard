import { useCallback, useEffect, useRef, useState } from "react";
import type { ConnectionStatus, SubscriptionUnsubscribe } from "@shunter/client";

import {
  createProjectMemoryClient,
  queryRecentChainsDecoded,
  subscribeLiveRecentChains,
  type ProjectMemoryClient,
} from "@/lib/project-memory/client";

export interface UseProjectMemoryChainsOptions {
  enabled?: boolean;
  onChanged?: () => void | Promise<void>;
}

export interface UseProjectMemoryChainsReturn {
  status: ConnectionStatus;
  error: string | null;
  rowCount: number | null;
  refresh: () => Promise<void>;
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : "Project memory connection failed";
}

function changedRowCount(current: number | null, inserts: number, deletes: number): number | null {
  if (current === null) return null;
  return Math.max(0, current + inserts - deletes);
}

export function useProjectMemoryChains(
  options: UseProjectMemoryChainsOptions = {},
): UseProjectMemoryChainsReturn {
  const enabled = options.enabled ?? true;
  const onChanged = options.onChanged;
  const onChangedRef = useRef(onChanged);
  const clientRef = useRef<ProjectMemoryClient | null>(null);
  const [status, setStatus] = useState<ConnectionStatus>("idle");
  const [error, setError] = useState<string | null>(null);
  const [rowCount, setRowCount] = useState<number | null>(null);

  useEffect(() => {
    onChangedRef.current = onChanged;
  }, [onChanged]);

  const loadSnapshotRowCount = useCallback(async (client: ProjectMemoryClient): Promise<number> => {
    const result = await queryRecentChainsDecoded(client.runDeclaredQuery);
    const chainsTable = result.tables.find((table) => table.tableName === "chains");
    return chainsTable?.rows.length ?? 0;
  }, []);

  const refresh = useCallback(async () => {
    const client = clientRef.current;
    if (client?.state.status !== "connected") return;
    setRowCount(await loadSnapshotRowCount(client));
  }, [loadSnapshotRowCount]);

  useEffect(() => {
    if (!enabled) {
      setStatus("idle");
      setError(null);
      setRowCount(null);
      return;
    }

    let cancelled = false;
    let unsubscribe: SubscriptionUnsubscribe | undefined;
    const client = createProjectMemoryClient({
      onStateChange: ({ current }) => {
        if (!cancelled) setStatus(current.status);
      },
    });
    clientRef.current = client;
    setStatus(client.state.status);
    setError(null);

    const start = async () => {
      try {
        await client.connect();
        if (cancelled) return;
        const snapshotRowCount = await loadSnapshotRowCount(client);
        if (cancelled) return;
        setRowCount(snapshotRowCount);
        const nextUnsubscribe = await subscribeLiveRecentChains(client.subscribeDeclaredView, {
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
          void nextUnsubscribe();
          return;
        }
        unsubscribe = nextUnsubscribe;
      } catch (err) {
        if (cancelled) return;
        setStatus("failed");
        setError(errorMessage(err));
      }
    };

    void start();

    return () => {
      cancelled = true;
      if (clientRef.current === client) clientRef.current = null;
      void unsubscribe?.();
      void client.dispose();
    };
  }, [enabled, loadSnapshotRowCount]);

  return { status, error, rowCount, refresh };
}
