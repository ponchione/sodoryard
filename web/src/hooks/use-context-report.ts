import { useCallback, useEffect, useState } from "react";
import { api, ApiError } from "@/lib/api";
import type { ContextReport, ContextSignalStreamResponse } from "@/types/metrics";

const latestTurnFetchDeferMs = 250;

export interface UseContextReportReturn {
  report: ContextReport | null;
  loading: boolean;
  error: string | null;
  currentTurn: number;
  totalTurns: number;
  isFollowingLatest: boolean;
  goToTurn: (turn: number) => void;
  nextTurn: () => void;
  prevTurn: () => void;
  jumpToLatest: () => void;
  setHistoryTurns: (totalTurns: number) => void;
  /** Accept a real-time context_debug event from WebSocket. */
  setLiveReport: (report: ContextReport) => void;
}

export function useContextReport(conversationId?: string): UseContextReportReturn {
  const [report, setReport] = useState<ContextReport | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [currentTurn, setCurrentTurn] = useState(0);
  const [totalTurns, setTotalTurns] = useState(0);
  const [isFollowingLatest, setIsFollowingLatest] = useState(true);

  useEffect(() => {
    setReport(null);
    setLoading(false);
    setError(null);
    setCurrentTurn(0);
    setTotalTurns(0);
    setIsFollowingLatest(true);
  }, [conversationId]);

  const fetchReport = useCallback(
    async (turn: number) => {
      if (!conversationId || turn < 1) return;
      try {
        setLoading(true);
        setError(null);
        const [report, signalStream] = await Promise.all([
          api.get<ContextReport>(`/api/metrics/conversation/${conversationId}/context/${turn}`),
          api
            .get<ContextSignalStreamResponse>(
              `/api/metrics/conversation/${conversationId}/context/${turn}/signals`,
            )
            .catch((signalErr) => {
              if (signalErr instanceof ApiError && signalErr.status === 404) {
                return null;
              }
              throw signalErr;
            }),
        ]);

        setReport({
          ...report,
          signal_stream: signalStream?.stream ?? report.signal_stream,
        });
      } catch (err) {
        if (err instanceof ApiError && err.status === 404) {
          setReport(null);
          setError(null);
        } else {
          setError(err instanceof Error ? err.message : "Failed to load context report");
        }
      } finally {
        setLoading(false);
      }
    },
    [conversationId],
  );

  const reportTurn = report?.turn_number ?? 0;

  useEffect(() => {
    if (currentTurn <= 0 || reportTurn === currentTurn) {
      return;
    }

    const shouldDeferLatestFetch = (
      isFollowingLatest
      && currentTurn === totalTurns
      && reportTurn === 0
    );

    if (shouldDeferLatestFetch) {
      const timer = window.setTimeout(() => {
        void fetchReport(currentTurn);
      }, latestTurnFetchDeferMs);
      return () => {
        window.clearTimeout(timer);
      };
    }

    void fetchReport(currentTurn);
  }, [currentTurn, fetchReport, isFollowingLatest, reportTurn, totalTurns]);

  const goToTurn = useCallback(
    (turn: number) => {
      if (turn >= 1 && turn <= totalTurns) {
        setCurrentTurn(turn);
        setIsFollowingLatest(turn === totalTurns);
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

  const jumpToLatest = useCallback(() => {
    if (totalTurns > 0) {
      setCurrentTurn(totalTurns);
      setIsFollowingLatest(true);
    }
  }, [totalTurns]);

  const setHistoryTurns = useCallback((turnCount: number) => {
    if (turnCount <= 0) {
      setReport(null);
      setError(null);
      setCurrentTurn(0);
      setTotalTurns(0);
      setIsFollowingLatest(true);
      return;
    }

    setTotalTurns((prevTotal) => {
      const nextTotal = Math.max(prevTotal, turnCount);
      setCurrentTurn((prevTurn) => {
        if (prevTurn <= 0) return nextTotal;
        return Math.min(prevTurn, nextTotal);
      });
      setIsFollowingLatest((prevFollow) => (prevFollow ? true : currentTurn >= nextTotal));
      return nextTotal;
    });
  }, [currentTurn]);

  const setLiveReport = useCallback((newReport: ContextReport) => {
    setTotalTurns((prev) => Math.max(prev, newReport.turn_number));
    if (isFollowingLatest) {
      setReport(newReport);
      setCurrentTurn(newReport.turn_number);
    }
  }, [isFollowingLatest]);

  return {
    report,
    loading,
    error,
    currentTurn,
    totalTurns,
    isFollowingLatest,
    goToTurn,
    nextTurn,
    prevTurn,
    jumpToLatest,
    setHistoryTurns,
    setLiveReport,
  };
}
