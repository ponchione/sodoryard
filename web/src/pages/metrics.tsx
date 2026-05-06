import { useMemo } from "react";
import { useApiResource } from "@/hooks/use-api-resource";
import { chainStatusGroup } from "@/lib/chain-status";
import type { ChainSummary, RuntimeStatus } from "@/types/chains";

export function MetricsPage() {
  const { data: chains, loading, error } = useApiResource<ChainSummary[]>("/api/chains?limit=200", []);
  const { data: status } = useApiResource<RuntimeStatus | null>("/api/runtime/status", null);
  const metrics = useMemo(() => {
    const groups = { active: 0, success: 0, failed: 0, other: 0 };
    let tokens = 0;
    let steps = 0;
    for (const chain of chains) {
      groups[chainStatusGroup(chain.status)] += 1;
      tokens += chain.total_tokens;
      steps += chain.total_steps;
    }
    return { groups, tokens, steps };
  }, [chains]);

  return (
    <div className="flex-1 overflow-y-auto px-4 py-6">
      <div className="mx-auto max-w-5xl space-y-5">
        <div className="border-b border-border pb-4">
          <h1 className="text-xl font-bold uppercase tracking-widest text-primary text-glow-cyan">
            Metrics
          </h1>
          <p className="mt-1 text-xs text-muted-foreground">
            Chain-level operating totals from the local Yard store.
          </p>
        </div>

        {status && (
          <section className="grid gap-3 border border-border p-3 text-xs md:grid-cols-4">
            <div>
              <div className="text-[10px] uppercase tracking-widest text-muted-foreground">Provider</div>
              <div className="mt-1 text-sm text-primary">{status.provider}:{status.model}</div>
            </div>
            <div>
              <div className="text-[10px] uppercase tracking-widest text-muted-foreground">Auth</div>
              <div className="mt-1 text-sm text-foreground">{status.auth_status}</div>
            </div>
            <div>
              <div className="text-[10px] uppercase tracking-widest text-muted-foreground">Code Index</div>
              <div className="mt-1 text-sm text-foreground">{status.code_index.status}</div>
            </div>
            <div>
              <div className="text-[10px] uppercase tracking-widest text-muted-foreground">Brain Index</div>
              <div className="mt-1 text-sm text-foreground">{status.brain_index.status}</div>
            </div>
          </section>
        )}

        {loading && <p className="text-xs text-muted-foreground">Loading metrics...</p>}
        {error && <p className="text-xs text-destructive">{error}</p>}

        <section className="grid gap-3 md:grid-cols-4">
          <MetricBlock label="Chains" value={chains.length} />
          <MetricBlock label="Active" value={metrics.groups.active} />
          <MetricBlock label="Steps" value={metrics.steps} />
          <MetricBlock label="Tokens" value={metrics.tokens} />
        </section>

        <section className="space-y-2">
          <h2 className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">Outcomes</h2>
          <div className="grid gap-2 text-xs md:grid-cols-4">
            <Outcome label="Completed" value={metrics.groups.success} tone="text-accent" />
            <Outcome label="Failed/Cancelled" value={metrics.groups.failed} tone="text-destructive" />
            <Outcome label="Active" value={metrics.groups.active} tone="text-warning" />
            <Outcome label="Other" value={metrics.groups.other} tone="text-muted-foreground" />
          </div>
        </section>

        <section className="space-y-2">
          <h2 className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">Recent Chains</h2>
          <div className="border border-border">
            {chains.slice(0, 12).map((chain) => (
              <div key={chain.id} className="grid gap-2 border-b border-border px-3 py-2 text-xs md:grid-cols-[1fr_7rem_5rem_5rem]">
                <span className="truncate font-mono text-primary">{chain.id}</span>
                <span>{chain.status}</span>
                <span className="tabular-nums">{chain.total_steps} steps</span>
                <span className="tabular-nums">{chain.total_tokens} tok</span>
              </div>
            ))}
            {chains.length === 0 && !loading && (
              <p className="p-3 text-xs text-muted-foreground">No chains recorded.</p>
            )}
          </div>
        </section>
      </div>
    </div>
  );
}

function MetricBlock({ label, value }: { label: string; value: number }) {
  return (
    <div className="border border-border bg-muted/40 p-3">
      <div className="text-[10px] uppercase tracking-widest text-muted-foreground">{label}</div>
      <div className="mt-2 text-2xl text-foreground">{value.toLocaleString()}</div>
    </div>
  );
}

function Outcome({ label, value, tone }: { label: string; value: number; tone: string }) {
  return (
    <div className="border border-border px-3 py-2">
      <div className="text-muted-foreground">{label}</div>
      <div className={`mt-1 text-xl ${tone}`}>{value.toLocaleString()}</div>
    </div>
  );
}
