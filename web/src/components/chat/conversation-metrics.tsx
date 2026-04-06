import { useConversationMetrics } from "@/hooks/use-conversation-metrics";
import { CollapsibleSection } from "@/components/inspector/collapsible-section";

interface ConversationMetricsProps {
  conversationId?: string;
  refreshKey?: number;
}

function formatNum(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
  return String(n);
}

export function ConversationMetricsPanel({ conversationId, refreshKey }: ConversationMetricsProps) {
  const { metrics, loading } = useConversationMetrics(conversationId, refreshKey);

  if (loading) {
    return (
      <div className="px-3 py-2 text-xs text-muted-foreground">Loading metrics…</div>
    );
  }

  if (!metrics) return null;

  const { token_usage: tok, cache_hit_rate_pct: cacheHit, tool_usage: tools, context_quality: ctxQ } = metrics;

  const cacheColor =
    cacheHit > 50 ? "text-green-600 dark:text-green-400"
      : cacheHit > 20 ? "text-yellow-600 dark:text-yellow-400"
      : "text-red-500 dark:text-red-400";

  const hitRateColor =
    ctxQ.avg_hit_rate > 0.7 ? "text-green-600 dark:text-green-400"
      : ctxQ.avg_hit_rate > 0.4 ? "text-yellow-600 dark:text-yellow-400"
      : "text-red-500 dark:text-red-400";

  const reactiveColor =
    ctxQ.reactive_search_count / Math.max(ctxQ.total_turns, 1) < 0.1
      ? "text-green-600 dark:text-green-400"
      : ctxQ.reactive_search_count / Math.max(ctxQ.total_turns, 1) < 0.3
        ? "text-yellow-600 dark:text-yellow-400"
        : "text-red-500 dark:text-red-400";

  return (
    <div className="space-y-1">
      {/* Token Usage */}
      <CollapsibleSection title="Token Usage">
        <div className="grid grid-cols-2 gap-x-4 gap-y-1 text-xs">
          <div className="text-muted-foreground">Input</div>
          <div className="text-right font-mono">{formatNum(tok.tokens_in)}</div>
          <div className="text-muted-foreground">Output</div>
          <div className="text-right font-mono">{formatNum(tok.tokens_out)}</div>
          <div className="text-muted-foreground">Cache hits</div>
          <div className="text-right font-mono">{formatNum(tok.cache_read_tokens)}</div>
          <div className="text-muted-foreground">LLM calls</div>
          <div className="text-right font-mono">{tok.total_calls}</div>
          <div className="text-muted-foreground">Cache hit rate</div>
          <div className={`text-right font-medium ${cacheColor}`}>
            {cacheHit.toFixed(1)}%
          </div>
        </div>
      </CollapsibleSection>

      {/* Tool Usage */}
      {tools.length > 0 && (
        <CollapsibleSection title={`Tools (${tools.length})`}>
          <div className="space-y-1">
            {tools
              .sort((a, b) => b.call_count - a.call_count)
              .map((t) => (
                <div key={t.tool_name} className="flex items-center gap-2 text-xs">
                  <span className="flex-1 font-mono truncate">{t.tool_name}</span>
                  <span className="text-muted-foreground">{t.call_count}×</span>
                  <span className="text-muted-foreground/60">
                    {t.avg_duration_ms.toFixed(0)}ms
                  </span>
                  {t.failure_count > 0 && (
                    <span className="text-red-500 dark:text-red-400">
                      {t.failure_count} fail
                    </span>
                  )}
                </div>
              ))}
          </div>
        </CollapsibleSection>
      )}

      {/* Context Quality */}
      <CollapsibleSection title="Context Quality">
        <div className="grid grid-cols-2 gap-x-4 gap-y-1 text-xs">
          <div className="text-muted-foreground">Turns</div>
          <div className="text-right">{ctxQ.total_turns}</div>
          <div className="text-muted-foreground">Avg hit rate</div>
          <div className={`text-right font-medium ${hitRateColor}`}>
            {(ctxQ.avg_hit_rate * 100).toFixed(0)}%
          </div>
          <div className="text-muted-foreground">Reactive search</div>
          <div className={`text-right font-medium ${reactiveColor}`}>
            {ctxQ.reactive_search_count} / {ctxQ.total_turns}
          </div>
          <div className="text-muted-foreground">Avg budget used</div>
          <div className="text-right">{(ctxQ.avg_budget_used_pct * 100).toFixed(0)}%</div>
        </div>
      </CollapsibleSection>
    </div>
  );
}
