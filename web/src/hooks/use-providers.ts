import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "@/lib/api";
import type { ProviderStatus } from "@/types/metrics";

export function useProviders() {
  const [providers, setProviders] = useState<ProviderStatus[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const mounted = useRef(true);

  const fetch_ = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);
      const data = await api.get<ProviderStatus[]>("/api/providers");
      if (mounted.current) setProviders(data ?? []);
    } catch (err) {
      if (mounted.current) setError(err instanceof Error ? err.message : "Failed");
    } finally {
      if (mounted.current) setLoading(false);
    }
  }, []);

  useEffect(() => {
    mounted.current = true;
    fetch_();
    return () => { mounted.current = false; };
  }, [fetch_]);

  return { providers, loading, error, refresh: fetch_ };
}
