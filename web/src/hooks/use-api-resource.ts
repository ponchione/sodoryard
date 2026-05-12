import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "@/lib/api";

export function useApiResource<T>(
  path: string,
  fallback: T,
  normalize?: (value: T | null | undefined) => T,
) {
  const [data, setData] = useState<T>(fallback);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const mounted = useRef(true);
  const requestSeq = useRef(0);
  const fallbackRef = useRef(fallback);
  const normalizeRef = useRef(normalize);

  fallbackRef.current = fallback;
  normalizeRef.current = normalize;

  const normalizeResponse = useCallback((value: T | null | undefined) => {
    return normalizeRef.current?.(value) ?? value ?? fallbackRef.current;
  }, []);

  const refresh = useCallback(async () => {
    const seq = requestSeq.current + 1;
    requestSeq.current = seq;
    try {
      setLoading(true);
      setError(null);
      const response = await api.get<T>(path);
      if (mounted.current && seq === requestSeq.current) setData(normalizeResponse(response));
    } catch (err) {
      if (mounted.current && seq === requestSeq.current) setError(err instanceof Error ? err.message : "Failed");
    } finally {
      if (mounted.current && seq === requestSeq.current) setLoading(false);
    }
  }, [normalizeResponse, path]);

  useEffect(() => {
    mounted.current = true;
    refresh();
    return () => {
      mounted.current = false;
    };
  }, [refresh]);

  return { data, loading, error, refresh };
}
