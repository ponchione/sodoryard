import { useEffect, useMemo, useState } from "react";
import { Link, useParams, useSearchParams } from "react-router-dom";
import { api } from "@/lib/api";
import { chainStatusClass } from "@/lib/chain-status";
import type { ChainDetail, ReceiptSummary, ReceiptView } from "@/types/chains";

function formatDate(value?: string): string {
  if (!value) return "unknown";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}

export function ChainDetailPage() {
  const { id = "" } = useParams();
  const [searchParams] = useSearchParams();
  const requestedReceipt = searchParams.get("receipt") ?? "";
  const [detail, setDetail] = useState<ChainDetail | null>(null);
  const [receipt, setReceipt] = useState<ReceiptView | null>(null);
  const [selectedReceipt, setSelectedReceipt] = useState<ReceiptSummary | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    async function load() {
      try {
        setLoading(true);
        setError(null);
        setDetail(null);
        setReceipt(null);
        setSelectedReceipt(null);
        const chain = await api.get<ChainDetail>(`/api/chains/${encodeURIComponent(id)}`);
        if (cancelled) return;
        setDetail(chain);
        const initialReceipt =
          chain.receipts.find((candidate) => candidate.path === requestedReceipt) ??
          chain.receipts[0] ??
          null;
        setSelectedReceipt(initialReceipt);
      } catch (err) {
        if (!cancelled) setError(err instanceof Error ? err.message : "Failed to load chain");
      } finally {
        if (!cancelled) setLoading(false);
      }
    }
    load();
    return () => {
      cancelled = true;
    };
  }, [id, requestedReceipt]);

  useEffect(() => {
    let cancelled = false;
    async function loadReceipt() {
      if (!selectedReceipt) {
        setReceipt(null);
        return;
      }
      try {
        const suffix = selectedReceipt.step ? `?step=${encodeURIComponent(selectedReceipt.step)}` : "";
        const loaded = await api.get<ReceiptView>(`/api/chains/${encodeURIComponent(id)}/receipt${suffix}`);
        if (!cancelled) setReceipt(loaded);
      } catch {
        if (!cancelled) setReceipt(null);
      }
    }
    loadReceipt();
    return () => {
      cancelled = true;
    };
  }, [id, selectedReceipt]);

  const source = useMemo(() => {
    if (!detail) return "";
    return detail.chain.source_task || detail.chain.source_specs.join(", ") || "No task recorded";
  }, [detail]);

  return (
    <div className="flex-1 overflow-y-auto px-4 py-6">
      <div className="mx-auto max-w-6xl space-y-5">
        <div className="border-b border-border pb-4">
          <Link to="/chains" className="text-xs uppercase tracking-widest text-muted-foreground hover:text-primary">
            Chains
          </Link>
          <h1 className="mt-2 break-all font-mono text-xl font-bold text-primary text-glow-cyan">{id}</h1>
          {detail && (
            <p className={`mt-1 text-xs font-medium ${chainStatusClass(detail.chain.status)}`}>{detail.chain.status}</p>
          )}
        </div>

        {loading && <p className="text-xs text-muted-foreground">Loading chain...</p>}
        {error && <p className="text-xs text-destructive">{error}</p>}

        {detail && (
          <>
            <section className="grid gap-3 border border-border p-3 text-xs md:grid-cols-4">
              <div>
                <div className="text-[10px] uppercase tracking-widest text-muted-foreground">Steps</div>
                <div className="mt-1 text-lg text-foreground">{detail.chain.total_steps}</div>
              </div>
              <div>
                <div className="text-[10px] uppercase tracking-widest text-muted-foreground">Tokens</div>
                <div className="mt-1 text-lg text-foreground">{detail.chain.total_tokens}</div>
              </div>
              <div>
                <div className="text-[10px] uppercase tracking-widest text-muted-foreground">Duration</div>
                <div className="mt-1 text-lg text-foreground">{detail.chain.total_duration_secs}s</div>
              </div>
              <div>
                <div className="text-[10px] uppercase tracking-widest text-muted-foreground">Updated</div>
                <div className="mt-1 text-sm text-foreground">{formatDate(detail.chain.updated_at)}</div>
              </div>
            </section>

            <section className="space-y-2">
              <h2 className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">Source</h2>
              <p className="border border-border bg-muted/40 p-3 text-xs text-foreground">{source}</p>
            </section>

            <section className="space-y-2">
              <h2 className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">Steps</h2>
              <div className="overflow-hidden border border-border">
                <table className="w-full text-left text-xs">
                  <thead className="border-b border-border bg-muted text-[10px] uppercase tracking-widest text-muted-foreground">
                    <tr>
                      <th className="px-3 py-2 font-medium">#</th>
                      <th className="px-3 py-2 font-medium">Role</th>
                      <th className="px-3 py-2 font-medium">Status</th>
                      <th className="px-3 py-2 font-medium">Verdict</th>
                      <th className="px-3 py-2 font-medium">Receipt</th>
                    </tr>
                  </thead>
                  <tbody>
                    {detail.steps.map((step) => (
                      <tr key={step.id || `${step.sequence_num}-${step.role}`} className="border-b border-border/70">
                        <td className="px-3 py-2 tabular-nums">{step.sequence_num}</td>
                        <td className="px-3 py-2">{step.role}</td>
                        <td className={`px-3 py-2 ${chainStatusClass(step.status)}`}>{step.status}</td>
                        <td className="px-3 py-2 text-muted-foreground">{step.verdict || "none"}</td>
                        <td className="px-3 py-2 font-mono text-muted-foreground">{step.receipt_path || "none"}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </section>

            <section className="grid gap-4 lg:grid-cols-[18rem_1fr]">
              <div className="space-y-2">
                <h2 className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">Receipts</h2>
                <div className="border border-border">
                  {detail.receipts.length === 0 && (
                    <p className="p-3 text-xs text-muted-foreground">No receipts recorded.</p>
                  )}
                  {detail.receipts.map((candidate) => (
                    <button
                      key={`${candidate.step}:${candidate.path}`}
                      type="button"
                      onClick={() => setSelectedReceipt(candidate)}
                      className={`block w-full border-b border-border px-3 py-2 text-left text-xs hover:bg-muted ${
                        selectedReceipt?.path === candidate.path ? "bg-muted text-primary" : "text-muted-foreground"
                      }`}
                    >
                      <span className="block font-medium">{candidate.label || "receipt"}</span>
                      <span className="block truncate font-mono text-[10px]">{candidate.path}</span>
                    </button>
                  ))}
                </div>
              </div>
              <div className="min-w-0 space-y-2">
                <h2 className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
                  Receipt Content
                </h2>
                <pre className="max-h-[32rem] overflow-auto whitespace-pre-wrap border border-border bg-background p-3 text-xs leading-relaxed text-foreground">
                  {receipt?.content || "No receipt selected."}
                </pre>
              </div>
            </section>

            <section className="space-y-2">
              <h2 className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">Recent Events</h2>
              <div className="space-y-1 border border-border p-3">
                {detail.recent_events.length === 0 && (
                  <p className="text-xs text-muted-foreground">No events recorded.</p>
                )}
                {detail.recent_events.map((event) => (
                  <div key={event.id} className="grid gap-2 text-xs md:grid-cols-[10rem_12rem_1fr]">
                    <span className="text-muted-foreground">{formatDate(event.created_at)}</span>
                    <span className="font-medium text-primary">{event.event_type}</span>
                    <span className="truncate font-mono text-muted-foreground">{event.event_data}</span>
                  </div>
                ))}
              </div>
            </section>
          </>
        )}
      </div>
    </div>
  );
}
