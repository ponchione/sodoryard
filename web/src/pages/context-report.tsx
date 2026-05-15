import { useEffect } from "react";
import { Link, useNavigate, useParams, useSearchParams } from "react-router-dom";
import { ContextInspector } from "@/components/inspector/context-inspector";
import { useContextReport } from "@/hooks/use-context-report";

function parseTurn(value?: string): number {
  const parsed = Number.parseInt(value ?? "", 10);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : 0;
}

export function ContextReportPage() {
  const { conversationId = "", turn } = useParams();
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const turnNumber = parseTurn(turn);
  const chainID = searchParams.get("chain_id") ?? "";
  const ctx = useContextReport(conversationId, false, Boolean(conversationId && turnNumber > 0));
  const { currentTurn, goToTurn, setHistoryTurns, totalTurns } = ctx;

  useEffect(() => {
    if (turnNumber > 0) {
      setHistoryTurns(turnNumber);
    }
  }, [setHistoryTurns, turnNumber]);

  useEffect(() => {
    if (turnNumber > 0 && totalTurns >= turnNumber && currentTurn !== turnNumber) {
      goToTurn(turnNumber);
    }
  }, [currentTurn, goToTurn, totalTurns, turnNumber]);

  const closeInspector = () => {
    if (chainID) {
      navigate(`/chains/${encodeURIComponent(chainID)}`);
      return;
    }
    navigate(`/c/${encodeURIComponent(conversationId)}`);
  };

  return (
    <div className="flex-1 overflow-y-auto px-4 py-6">
      <div className="w-full space-y-5">
        <header className="border-b border-border pb-4">
          <div className="flex flex-wrap items-center gap-3 text-xs uppercase tracking-widest">
            {chainID && (
              <Link to={`/chains/${encodeURIComponent(chainID)}`} className="text-muted-foreground hover:text-primary">
                Chain
              </Link>
            )}
            <Link to={`/c/${encodeURIComponent(conversationId)}`} className="text-muted-foreground hover:text-primary">
              Conversation
            </Link>
          </div>
          <h1 className="mt-2 text-xl font-semibold uppercase tracking-widest text-primary text-glow-cyan">
            Context Report
          </h1>
          <p className="mt-1 break-all font-mono text-xs text-muted-foreground">
            {conversationId || "unknown"} / turn {turnNumber || "unknown"}
          </p>
        </header>

        {turnNumber > 0 ? (
          <ContextInspector
            ctx={ctx}
            onClose={closeInspector}
            className="min-h-[36rem] !w-full"
          />
        ) : (
          <p className="text-xs text-destructive">Invalid context report turn.</p>
        )}
      </div>
    </div>
  );
}
