import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "@/lib/api";
import type { ProjectInfo } from "@/types/metrics";

export function useProjectInfo() {
  const [project, setProject] = useState<ProjectInfo | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const mounted = useRef(true);

  const fetch_ = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);
      const data = await api.get<ProjectInfo>("/api/project");
      if (mounted.current) setProject(data);
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

  return { project, loading, error, refresh: fetch_ };
}
