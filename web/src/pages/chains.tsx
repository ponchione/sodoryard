import { Link } from "react-router-dom";
import { useMemo, useState } from "react";
import { useApiResource } from "@/hooks/use-api-resource";
import { useProjectMemoryChains } from "@/hooks/use-project-memory-chains";
import { chainStatusClass } from "@/lib/chain-status";
import { formatModelCapabilitySummary, formatTokenLimit } from "@/lib/model-capabilities";
import type { ChainSummary, RuntimeStatus } from "@/types/chains";

function formatDate(value?: string): string {
  if (!value) return "unknown";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}

function taskLabel(chain: ChainSummary): string {
  return chain.source_task || chain.source_specs.join(", ") || "No task recorded";
}

function formatProjectMemoryStatus(status: string, rowCount: number | null, eventCount: number | null): string {
  if (status === "connected") {
    const parts = ["connected"];
    if (rowCount !== null) parts.push(`${rowCount} chain rows`);
    if (eventCount !== null) parts.push(`${eventCount} event rows`);
    return parts.join(" / ");
  }
  if (status === "idle") return "idle";
  return status;
}

function eventPayloadPreview(payloadJson: string): string {
  const text = payloadJson.trim();
  if (!text || text === "{}") return "no payload";
  if (text.length <= 140) return text;
  return `${text.slice(0, 137)}...`;
}

export function ChainsPage() {
  const [query, setQuery] = useState("");
  const { data: chains, loading, error, refresh } = useApiResource<ChainSummary[]>("/api/chains?limit=100", []);
  const { data: status } = useApiResource<RuntimeStatus | null>("/api/runtime/status", null);
  const projectMemory = useProjectMemoryChains({ onChanged: refresh });
  const normalizedQuery = query.trim().toLowerCase();
  const visibleChains = useMemo(() => {
    if (!normalizedQuery) return chains;
    return chains.filter((chain) => {
      const haystack = [
        chain.id,
        chain.status,
        chain.source_task,
        ...chain.source_specs,
        chain.current_step?.role ?? "",
        chain.current_step?.status ?? "",
        chain.current_step?.verdict ?? "",
      ].join(" ").toLowerCase();
      return haystack.includes(normalizedQuery);
    });
  }, [chains, normalizedQuery]);

  return (
    <div className="flex-1 overflow-y-auto px-4 py-6">
      <div className="mx-auto max-w-5xl space-y-5">
        <div className="flex flex-col gap-3 border-b border-border pb-4 md:flex-row md:items-end md:justify-between">
          <div>
            <h1 className="text-xl font-bold uppercase tracking-widest text-primary text-glow-cyan">
              Chains
            </h1>
            <p className="mt-1 text-xs text-muted-foreground">
              {status
                ? `${status.provider}:${status.model} / ${formatTokenLimit(status.context_window)} context / auth ${status.auth_status}`
                : "Runtime status loading"}
            </p>
            {status && (
              <p className="mt-1 text-xs text-muted-foreground">
                capabilities: {formatModelCapabilitySummary(status.model_capabilities)}
              </p>
            )}
            <p className="mt-1 text-xs text-muted-foreground">
              project memory: {formatProjectMemoryStatus(
                projectMemory.status,
                projectMemory.rowCount,
                projectMemory.eventCount,
              )}
            </p>
            {projectMemory.error && (
              <p className="mt-1 text-xs text-warning">project memory: {projectMemory.error}</p>
            )}
          </div>
          <div className="flex gap-2">
            <input
              type="search"
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder="Filter chains"
              className="w-64 border border-border bg-background px-3 py-2 text-xs text-foreground outline-none focus:border-primary"
            />
            <button
              type="button"
              onClick={refresh}
              className="border border-border px-3 py-2 text-xs font-medium uppercase tracking-widest text-muted-foreground hover:border-primary hover:text-primary"
            >
              Refresh
            </button>
          </div>
        </div>

        {status && status.warnings.length > 0 && (
          <section className="border border-warning/50 bg-warning/5 p-3">
            <h2 className="text-[10px] font-semibold uppercase tracking-widest text-warning">Readiness</h2>
            <div className="mt-2 space-y-1 text-xs text-warning">
              {status.warnings.map((warning) => (
                <p key={warning.message}>{warning.message}</p>
              ))}
            </div>
          </section>
        )}

        {projectMemory.status === "connected" && (
          <section className="border border-border">
            <div className="flex items-center justify-between border-b border-border bg-muted px-3 py-2">
              <h2 className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
                Recent Project Memory Events
              </h2>
              <span className="text-[10px] uppercase tracking-widest text-muted-foreground">
                Shunter SDK
              </span>
            </div>
            {projectMemory.recentEvents.length === 0 ? (
              <p className="px-3 py-3 text-xs text-muted-foreground">No recent project memory events.</p>
            ) : (
              <div className="divide-y divide-border/70">
                {projectMemory.recentEvents.map((event) => (
                  <div
                    key={event.id}
                    className="grid gap-2 px-3 py-2 text-xs md:grid-cols-[minmax(0,1fr)_auto]"
                  >
                    <div className="min-w-0">
                      <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                        <span className="font-medium text-foreground">{event.eventType}</span>
                        <Link to={`/chains/${event.chainId}`} className="font-mono text-primary hover:underline">
                          {event.chainId}
                        </Link>
                        {event.stepId && (
                          <span className="font-mono text-muted-foreground">{event.stepId}</span>
                        )}
                      </div>
                      <div className="mt-1 truncate font-mono text-[11px] text-muted-foreground">
                        {eventPayloadPreview(event.payloadJson)}
                      </div>
                    </div>
                    <div className="text-muted-foreground md:text-right">
                      <div>{formatDate(event.createdAt)}</div>
                      <div className="mt-1 font-mono text-[11px]">seq {event.sequence}</div>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </section>
        )}

        {loading && <p className="text-xs text-muted-foreground">Loading chains...</p>}
        {error && <p className="text-xs text-destructive">{error}</p>}
        {!loading && !error && visibleChains.length === 0 && (
          <p className="text-xs text-muted-foreground">No chains match.</p>
        )}

        <div className="overflow-hidden border border-border">
          <table className="w-full text-left text-xs">
            <thead className="border-b border-border bg-muted text-[10px] uppercase tracking-widest text-muted-foreground">
              <tr>
                <th className="px-3 py-2 font-medium">Chain</th>
                <th className="px-3 py-2 font-medium">Status</th>
                <th className="px-3 py-2 font-medium">Current Step</th>
                <th className="px-3 py-2 font-medium">Tokens</th>
                <th className="px-3 py-2 font-medium">Updated</th>
              </tr>
            </thead>
            <tbody>
              {visibleChains.map((chain) => (
                <tr key={chain.id} className="border-b border-border/70 hover:bg-muted/50">
                  <td className="max-w-md px-3 py-2">
                    <Link to={`/chains/${chain.id}`} className="font-mono text-primary hover:underline">
                      {chain.id}
                    </Link>
                    <div className="mt-1 truncate text-muted-foreground">{taskLabel(chain)}</div>
                  </td>
                  <td className={`px-3 py-2 font-medium ${chainStatusClass(chain.status)}`}>{chain.status}</td>
                  <td className="px-3 py-2 text-muted-foreground">
                    {chain.current_step
                      ? `${chain.current_step.sequence_num} ${chain.current_step.role} ${chain.current_step.status}`
                      : "none"}
                  </td>
                  <td className="px-3 py-2 tabular-nums">{chain.total_tokens}</td>
                  <td className="px-3 py-2 text-muted-foreground">{formatDate(chain.updated_at)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}
