import { useCallback, useEffect, useState } from "react";
import { api } from "@/lib/api";
import type { ContextReport } from "@/types/metrics";

export interface UseContextReportReturn {
  report: ContextReport | null;
  loading: boolean;
  error: string | null;
  currentTurn: number;
  totalTurns: number;
  goToTurn: (turn: number) => void;
  nextTurn: () => void;
  prevTurn: () => void;
  /** Accept a real-time context_debug event from WebSocket. */
  setLiveReport: (report: ContextReport) => void;
}

export function useContextReport(conversationId?: string): UseContextReportReturn {
  const [report, setReport] = useState<ContextReport | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [currentTurn, setCurrentTurn] = useState(0);
  const [totalTurns, setTotalTurns] = useState(0);

  const fetchReport = useCallback(
    async (turn: number) => {
      if (!conversationId || turn < 1) return;
      try {
        setLoading(true);
        setError(null);
        const data = await api.get<ContextReport>(
          `/api/metrics/conversation/${conversationId}/context/${turn}`,
        );
        setReport(data);
      } catch (err) {
        if (err instanceof Error && err.message.includes("404")) {
          setReport(null);
          setError(null); // Not an error — just no data for this turn.
        } else {
          setError(err instanceof Error ? err.message : "Failed to load context report");
        }
      } finally {
        setLoading(false);
      }
    },
    [conversationId],
  );

  // Fetch when turn changes.
  useEffect(() => {
    if (currentTurn > 0) {
      fetchReport(currentTurn);
    }
  }, [currentTurn, fetchReport]);

  const goToTurn = useCallback(
    (turn: number) => {
      if (turn >= 1 && turn <= totalTurns) {
        setCurrentTurn(turn);
      }
    },
    [totalTurns],
  );

  const nextTurn = useCallback(() => {
    goToTurn(currentTurn + 1);
  }, [currentTurn, goToTurn]);

  const prevTurn = useCallback(() => {
    goToTurn(currentTurn - 1);
  }, [currentTurn, goToTurn]);

  const setLiveReport = useCallback((newReport: ContextReport) => {
    setReport(newReport);
    setCurrentTurn(newReport.turn_number);
    setTotalTurns((prev) => Math.max(prev, newReport.turn_number));
  }, []);

  return {
    report,
    loading,
    error,
    currentTurn,
    totalTurns,
    goToTurn,
    nextTurn,
    prevTurn,
    setLiveReport,
  };
}
