import type { UseContextReportReturn } from "@/hooks/use-context-report";
import { CollapsibleSection } from "@/components/inspector/collapsible-section";
import { BudgetBar } from "@/components/inspector/budget-bar";
import { buildSignalFlowFallback, normalizeBudgetBreakdown } from "@/lib/context-report";
import type {
  ContextNeeds,
  ContextSignal,
  ContextSignalStreamEntry,
  ExplicitFileResult,
  GraphResult,
  BrainResult,
  RAGResult,
} from "@/types/metrics";

interface ContextInspectorProps {
  ctx: UseContextReportReturn;
  onClose: () => void;
}

export function ContextInspector({ ctx, onClose }: ContextInspectorProps) {
  const { report, loading, error, currentTurn, totalTurns, isFollowingLatest, nextTurn, prevTurn, jumpToLatest } = ctx;
  const budgetCategories = normalizeBudgetBreakdown(report?.budget_breakdown);

  return (
    <div
      data-augmented-ui="tl-clip bl-clip border"
      className="flex w-96 flex-col bg-sidebar overflow-hidden"
      style={{
        "--aug-tl": "15px",
        "--aug-bl": "15px",
        "--aug-border-left": "2px",
        "--aug-border-bg":
          "linear-gradient(180deg, #00e5ff, #00e67640, #00e5ff)",
      } as React.CSSProperties}
    >
      <div className="flex items-center justify-between border-b border-border px-3 py-2">
        <span className="text-xs font-semibold uppercase tracking-widest text-primary text-glow-cyan">
          Context Inspector
        </span>
        <button
          type="button"
          onClick={onClose}
          className="p-0.5 text-muted-foreground hover:bg-muted hover:text-foreground"
          aria-label="Close inspector"
        >
          <XIcon />
        </button>
      </div>

      <div className="border-b border-border px-3 py-1.5 space-y-1.5">
        <div className="flex items-center justify-between">
          <button
            type="button"
            onClick={prevTurn}
            disabled={currentTurn <= 1}
            className="p-0.5 text-muted-foreground hover:bg-muted disabled:opacity-30"
          >
            <ChevronLeftIcon />
          </button>
          <span className="text-xs text-muted-foreground">
            {totalTurns > 0 ? `Turn ${currentTurn} of ${totalTurns}` : "No turns"}
          </span>
          <button
            type="button"
            onClick={nextTurn}
            disabled={currentTurn >= totalTurns}
            className="p-0.5 text-muted-foreground hover:bg-muted disabled:opacity-30"
          >
            <ChevronRightIcon />
          </button>
        </div>
        {!isFollowingLatest && totalTurns > 0 && (
          <button
            type="button"
            onClick={jumpToLatest}
            className="w-full bg-primary/10 px-2 py-1 text-[10px] font-medium uppercase tracking-widest text-primary hover:bg-primary/15"
          >
            Jump to latest
          </button>
        )}
      </div>

      <div className="flex-1 overflow-y-auto px-3 py-2 space-y-1">
        {loading && (
          <p className="py-4 text-center text-xs text-muted-foreground">Loading…</p>
        )}

        {!loading && error && (
          <div className="border border-destructive/40 bg-destructive/10 px-3 py-2 text-xs text-destructive space-y-1">
            <div className="font-medium uppercase tracking-widest">Inspector load failed</div>
            <div>{error}</div>
          </div>
        )}

        {!loading && !report && !error && (
          <p className="py-4 text-center text-xs text-muted-foreground">
            {totalTurns === 0
              ? "No context data yet — send a message to generate the first report"
              : "No stored context report for this turn"}
          </p>
        )}

        {report && (
          <>
            <CollapsibleSection title="Token Budget" sectionColor="#00e5ff" defaultOpen>
              <BudgetBar
                used={report.budget_used ?? 0}
                total={report.budget_total ?? 0}
                categories={budgetCategories}
              />
            </CollapsibleSection>

            <CollapsibleSection title="Quality" sectionColor="#00e676" defaultOpen>
              <QualityMetrics report={report} />
            </CollapsibleSection>

            <CollapsibleSection title="Latency" sectionColor="#ffab00">
              <LatencyDisplay
                analysis={report.analysis_latency_ms}
                retrieval={report.retrieval_latency_ms}
                total={report.total_latency_ms}
              />
            </CollapsibleSection>

            <CollapsibleSection title="Signals" sectionColor="#b388ff" defaultOpen>
              <SignalsList signals={report.signals ?? report.needs?.signals ?? []} />
            </CollapsibleSection>

            <CollapsibleSection title="Signal Flow" sectionColor="#7c4dff" defaultOpen>
              <SignalFlowList
                stream={report.signal_stream ?? buildSignalFlowFallback(report.needs, report.signals)}
              />
            </CollapsibleSection>

            <CollapsibleSection title="Queries" sectionColor="#00e5ff" defaultOpen>
              <QueriesList needs={report.needs} />
            </CollapsibleSection>

            <CollapsibleSection title={`Explicit Files (${report.explicit_files?.length ?? 0})`} sectionColor="#00e5ff">
              <ExplicitFilesList results={report.explicit_files ?? []} />
            </CollapsibleSection>

            <CollapsibleSection title={`Code Chunks (${report.rag_results?.length ?? 0})`} sectionColor="#00e676" defaultOpen>
              <RAGResultsList results={report.rag_results ?? []} />
            </CollapsibleSection>

            <CollapsibleSection title={`Brain (${report.brain_results?.length ?? 0})`} sectionColor="#ffab00">
              <BrainResultsList results={report.brain_results ?? []} />
            </CollapsibleSection>

            <CollapsibleSection title={`Graph (${report.graph_results?.length ?? 0})`} sectionColor="#b388ff">
              <GraphResultsList results={report.graph_results ?? []} />
            </CollapsibleSection>
          </>
        )}
      </div>
    </div>
  );
}

function QualityMetrics({ report }: { report: UseContextReportReturn["report"] }) {
  if (!report) return null;

  const hitRate = report.context_hit_rate;
  const hitColor =
    hitRate == null ? "text-muted-foreground"
      : hitRate > 0.7 ? "text-accent"
      : hitRate > 0.4 ? "text-[#ffab00]"
      : "text-destructive";

  const includedPaths = new Set<string>();
  for (const result of report.rag_results ?? []) {
    if (result.included) includedPaths.add(result.file_path);
  }
  for (const result of report.graph_results ?? []) {
    if (result.included) includedPaths.add(result.file_path);
  }
  for (const result of report.explicit_files ?? []) {
    if (result.included) includedPaths.add(result.file_path);
  }

  const uncoveredReads = (report.agent_read_files ?? []).filter((path) => !includedPaths.has(path));

  return (
    <div className="space-y-2 text-xs">
      <MetricRow
        label="Hit rate"
        value={hitRate != null ? `${(hitRate * 100).toFixed(0)}%` : "—"}
        valueClassName={hitColor}
      />
      <MetricRow
        label="Reactive search"
        value={report.agent_used_search_tool ? "Yes ⚠" : "No"}
        valueClassName={report.agent_used_search_tool ? "text-[#ffab00]" : "text-accent"}
      />
      <MetricRow
        label="Included in context / excluded"
        value={`${report.included_count ?? 0} / ${report.excluded_count ?? 0}`}
      />

      <div className="space-y-1">
        <div className="text-muted-foreground">Agent read files</div>
        {report.agent_read_files && report.agent_read_files.length > 0 ? (
          <CodeList items={report.agent_read_files} />
        ) : (
          <p className="text-[10px] text-muted-foreground">No reactive file reads recorded</p>
        )}
      </div>

      <div className="space-y-1">
        <div className="text-muted-foreground">Reads not already in context</div>
        {uncoveredReads.length > 0 ? (
          <CodeList items={uncoveredReads} danger />
        ) : (
          <p className="text-[10px] text-accent">All reads covered by context assembly</p>
        )}
      </div>
    </div>
  );
}

function LatencyDisplay({
  analysis,
  retrieval,
  total,
}: {
  analysis?: number;
  retrieval?: number;
  total?: number;
}) {
  const fmt = (ms?: number) => {
    if (ms == null) return "—";
    return ms < 1000 ? `${ms}ms` : `${(ms / 1000).toFixed(1)}s`;
  };
  const color = (ms?: number) => {
    if (ms == null) return "";
    if (ms < 200) return "text-accent";
    if (ms < 500) return "text-[#ffab00]";
    return "text-destructive";
  };

  return (
    <div className="space-y-1 text-xs">
      {[
        ["Analysis", analysis],
        ["Retrieval", retrieval],
        ["Total", total],
      ].map(([label, ms]) => (
        <MetricRow
          key={label as string}
          label={label as string}
          value={fmt(ms as number | undefined)}
          valueClassName={color(ms as number | undefined)}
        />
      ))}
    </div>
  );
}

function SignalsList({ signals }: { signals: ContextSignal[] }) {
  if (signals.length === 0) {
    return <p className="text-xs text-muted-foreground">No signals detected</p>;
  }

  return (
    <div className="space-y-1">
      {signals.map((signal, index) => (
        <div key={`${signal.type}-${signal.value}-${index}`} className="border border-border/50 bg-muted/30 px-2 py-1.5 text-[10px] space-y-1">
          <div className="flex items-center justify-between gap-2">
            <span className="font-medium text-foreground">{signal.type}</span>
            {signal.confidence != null && (
              <span className="text-muted-foreground">conf {(signal.confidence * 100).toFixed(0)}%</span>
            )}
          </div>
          <MetricRow label="Value" value={signal.value || "—"} mono />
          <MetricRow label="Source" value={signal.source || "—"} />
        </div>
      ))}
    </div>
  );
}

function SignalFlowList({ stream }: { stream: ContextSignalStreamEntry[] }) {
  if (stream.length === 0) {
    return <p className="text-xs text-muted-foreground">No ordered signal flow recorded</p>;
  }

  return (
    <div className="space-y-1">
      {stream.map((entry) => (
        <div
          key={`${entry.index}-${entry.kind}-${entry.type ?? ""}-${entry.value ?? ""}`}
          className="border border-border/50 bg-muted/30 px-2 py-1.5 text-[10px] space-y-1"
        >
          <div className="flex items-center justify-between gap-2">
            <span className="font-medium text-foreground">#{entry.index} {entry.kind}</span>
            {entry.type && <span className="text-muted-foreground">{entry.type}</span>}
          </div>
          <MetricRow label="Value" value={entry.value || "—"} mono={entry.kind !== "signal"} />
          {entry.source && <MetricRow label="Source" value={entry.source} />}
        </div>
      ))}
    </div>
  );
}

function QueriesList({ needs }: { needs?: ContextNeeds }) {
  const semanticQueries = needs?.semantic_queries ?? needs?.queries ?? [];
  const explicitFiles = needs?.explicit_files ?? [];
  const explicitSymbols = needs?.explicit_symbols ?? [];
  const momentumFiles = needs?.momentum_files ?? [];

  if (
    semanticQueries.length === 0 &&
    explicitFiles.length === 0 &&
    explicitSymbols.length === 0 &&
    momentumFiles.length === 0 &&
    !needs?.momentum_module
  ) {
    return <p className="text-xs text-muted-foreground">No queries generated</p>;
  }

  return (
    <div className="space-y-1">
      {semanticQueries.map((query, index) => (
        <QueryRow key={`semantic-${index}`} label="semantic" value={query} />
      ))}
      {explicitFiles.map((path, index) => (
        <QueryRow key={`file-${index}`} label="explicit file" value={path} mono />
      ))}
      {explicitSymbols.map((symbol, index) => (
        <QueryRow key={`symbol-${index}`} label="explicit symbol" value={symbol} mono />
      ))}
      {momentumFiles.map((path, index) => (
        <QueryRow key={`momentum-file-${index}`} label="momentum file" value={path} mono />
      ))}
      {needs?.momentum_module && (
        <QueryRow label="momentum module" value={needs.momentum_module} mono />
      )}
      {needs?.include_conventions && <QueryRow label="flag" value="include conventions" />}
      {needs?.include_git_context && (
        <QueryRow label="flag" value={`include git context${needs.git_context_depth ? ` (depth ${needs.git_context_depth})` : ""}`} />
      )}
    </div>
  );
}

function ExplicitFilesList({ results }: { results: ExplicitFileResult[] }) {
  return <ResultList emptyLabel="No explicit file retrievals for this turn" results={results} render={explicitFileItem} />;
}

function RAGResultsList({ results }: { results: RAGResult[] }) {
  return <ResultList emptyLabel="No code chunks" results={results} render={ragItem} />;
}

function BrainResultsList({ results }: { results: BrainResult[] }) {
  return <ResultList emptyLabel="No brain results for this turn" results={results} render={brainItem} />;
}

function GraphResultsList({ results }: { results: GraphResult[] }) {
  return <ResultList emptyLabel="No graph results for this turn" results={results} render={graphItem} />;
}

interface ContextResult {
  included?: boolean;
}

interface ResultCardData {
  key: string;
  included?: boolean;
  scoreLabel?: string;
  title: string;
  subtitle?: string;
  meta?: string[];
}

function ResultList<T extends ContextResult>({
  emptyLabel,
  results,
  render,
}: {
  emptyLabel: string;
  results: T[];
  render: (result: T, index: number) => ResultCardData;
}) {
  if (results.length === 0) {
    return <p className="text-xs text-muted-foreground">{emptyLabel}</p>;
  }

  const includedCount = results.filter((result) => result.included).length;
  const excludedCount = results.length - includedCount;

  return (
    <div className="space-y-2">
      <ResultSummary includedCount={includedCount} excludedCount={excludedCount} />
      <div className="space-y-1">
        {results.map((result, index) => {
          const item = render(result, index);
          const { key, ...card } = item;
          return <IncludedCard key={key} {...card} />;
        })}
      </div>
    </div>
  );
}

function explicitFileItem(result: ExplicitFileResult, index: number): ResultCardData {
  return {
    key: `${result.file_path}-${index}`,
    included: result.included,
    scoreLabel: result.token_count != null ? `${result.token_count} tok` : result.truncated ? "truncated" : undefined,
    title: result.file_path,
    meta: compactMeta([
      result.truncated ? "truncated" : undefined,
      result.exclusion_reason,
    ]),
  };
}

function ragItem(result: RAGResult, index: number): ResultCardData {
  return {
    key: `${result.file_path}-${result.chunk_id ?? result.chunk_name ?? index}`,
    included: result.included,
    scoreLabel: result.similarity_score != null ? result.similarity_score.toFixed(2) : result.score?.toFixed(2),
    title: result.chunk_name ?? result.name ?? result.file_path,
    subtitle: result.file_path,
    meta: compactMeta([
      result.chunk_type,
      result.signature,
      result.reason,
      result.exclusion_reason,
      result.line_start != null && result.line_end != null
        ? `lines ${result.line_start}-${result.line_end}`
        : undefined,
      result.matched_by,
      result.from_hop ? "structural hop" : undefined,
    ]),
  };
}

function brainItem(result: BrainResult, index: number): ResultCardData {
  return {
    key: `${result.vault_path ?? result.document_path}-${index}`,
    included: result.included,
    scoreLabel: (result.score ?? result.match_score)?.toFixed(2),
    title: result.title ?? result.vault_path ?? result.document_path ?? "untitled brain result",
    subtitle: result.vault_path ?? result.document_path,
    meta: compactMeta([
      result.match_mode,
      result.graph_hop_depth != null ? `hop ${result.graph_hop_depth}` : undefined,
      result.graph_source_path ? `via ${result.graph_source_path}` : undefined,
      result.exclusion_reason,
    ]),
  };
}

function graphItem(result: GraphResult, index: number): ResultCardData {
  return {
    key: `${result.symbol ?? result.symbol_name}-${result.file_path}-${index}`,
    included: result.included,
    scoreLabel: `depth ${result.depth ?? 0}`,
    title: result.symbol ?? result.symbol_name ?? "unknown symbol",
    subtitle: result.file_path,
    meta: compactMeta([
      result.relationship ?? result.relationship_type,
      result.exclusion_reason,
      result.line_start != null && result.line_end != null
        ? `lines ${result.line_start}-${result.line_end}`
        : undefined,
    ]),
  };
}

function compactMeta(values: Array<string | undefined | null | false>): string[] {
  return values.filter(Boolean) as string[];
}

function ResultSummary({
  includedCount,
  excludedCount,
}: {
  includedCount: number;
  excludedCount: number;
}) {
  return (
    <div className="flex justify-between text-[10px] text-muted-foreground">
      <span>Included in context {includedCount}</span>
      <span>Excluded {excludedCount}</span>
    </div>
  );
}

function IncludedCard({
  included,
  scoreLabel,
  title,
  subtitle,
  meta,
}: {
  included?: boolean;
  scoreLabel?: string;
  title: string;
  subtitle?: string;
  meta?: string[];
}) {
  return (
    <div className="border border-border/50 bg-muted/30 px-2 py-1.5 text-[10px] space-y-1">
      <div className="flex items-start gap-2">
        <span className={`shrink-0 px-1 py-0.5 font-medium ${included ? "bg-accent/20 text-accent" : "bg-destructive/20 text-destructive"}`}>
          {included ? "included" : "excluded"}
        </span>
        {scoreLabel && (
          <span className="shrink-0 bg-muted px-1 py-0.5 font-medium text-foreground">{scoreLabel}</span>
        )}
        <div className="min-w-0 flex-1">
          <div className="break-words text-foreground">{title}</div>
          {subtitle && <div className="break-words text-muted-foreground">{subtitle}</div>}
        </div>
      </div>
      {meta && meta.length > 0 && (
        <div className="flex flex-wrap gap-1">
          {meta.map((item, index) => (
            <span key={`${item}-${index}`} className="bg-background/60 px-1 py-0.5 text-muted-foreground">
              {item}
            </span>
          ))}
        </div>
      )}
    </div>
  );
}

function QueryRow({ label, value, mono = false }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="flex gap-2 bg-muted/50 px-2 py-1 text-[10px]">
      <span className="shrink-0 uppercase tracking-widest text-muted-foreground">{label}</span>
      <span className={`break-words ${mono ? "font-mono" : ""}`}>{value}</span>
    </div>
  );
}

function MetricRow({
  label,
  value,
  valueClassName,
  mono = false,
}: {
  label: string;
  value: string;
  valueClassName?: string;
  mono?: boolean;
}) {
  return (
    <div className="flex justify-between gap-3">
      <span className="text-muted-foreground">{label}</span>
      <span className={`${valueClassName ?? ""} text-right ${mono ? "font-mono" : ""}`}>{value}</span>
    </div>
  );
}

function CodeList({ items, danger = false }: { items: string[]; danger?: boolean }) {
  return (
    <div className="space-y-0.5 max-h-24 overflow-y-auto">
      {items.map((item, index) => (
        <div
          key={`${item}-${index}`}
          className={`break-all px-2 py-1 font-mono text-[10px] ${danger ? "bg-destructive/10 text-destructive" : "bg-muted/50 text-muted-foreground"}`}
        >
          {item}
        </div>
      ))}
    </div>
  );
}

function XIcon() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M18 6 6 18" /><path d="m6 6 12 12" />
    </svg>
  );
}

function ChevronLeftIcon() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="m15 18-6-6 6-6" />
    </svg>
  );
}

function ChevronRightIcon() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="m9 18 6-6-6-6" />
    </svg>
  );
}
