import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "@/lib/api";
import type { ConversationMetrics } from "@/types/metrics";

export function useConversationMetrics(conversationId?: string, refreshKey?: number) {
  const [metrics, setMetrics] = useState<ConversationMetrics | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const mounted = useRef(true);

  const fetch_ = useCallback(async () => {
    if (!conversationId) return;
    try {
      setLoading(true);
      setError(null);
      const data = await api.get<ConversationMetrics>(
        `/api/metrics/conversation/${conversationId}`,
      );
      if (mounted.current) setMetrics(data);
    } catch (err) {
      if (mounted.current) setError(err instanceof Error ? err.message : "Failed");
    } finally {
      if (mounted.current) setLoading(false);
    }
  }, [conversationId]);

  useEffect(() => {
    mounted.current = true;
    if (conversationId) {
      fetch_();
    }
    return () => {
      mounted.current = false;
    };
  }, [conversationId, fetch_, refreshKey]);

  return { metrics, loading, error, refresh: fetch_ };
}
