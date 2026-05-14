import { useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { ExternalLink, FolderOpen } from "lucide-react";
import { MarkdownContent } from "@/components/chat/markdown-content";
import { api } from "@/lib/api";
import { chainStatusClass } from "@/lib/chain-status";
import { parseReceiptDocument, receiptProjectPaths, receiptRouteForSummary } from "@/lib/receipts";
import { getYardPlatform } from "@/platform";
import type { ChainDetail, ChainEvent, ChainStep, ReceiptSummary, ReceiptView } from "@/types/chains";

interface ValidatePathsResponse {
  accepted: string[];
  rejected: Array<{ path: string; reason: string }>;
}

function formatDate(value?: string): string {
  if (!value) return "unknown";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}

function receiptAPIPath(chainID: string, step?: string): string {
  const base = `/api/chains/${encodeURIComponent(chainID)}/receipt`;
  const trimmedStep = step?.trim() ?? "";
  return trimmedStep ? `${base}?step=${encodeURIComponent(trimmedStep)}` : base;
}

function selectedStep(detail: ChainDetail | null, receipt: ReceiptView | null): ChainStep | null {
  if (!detail || !receipt) return null;
  return (
    detail.steps.find((step) => String(step.sequence_num) === receipt.step) ??
    detail.steps.find((step) => step.receipt_path === receipt.path) ??
    null
  );
}

function linkedEvents(detail: ChainDetail | null, receipt: ReceiptView | null, step: ChainStep | null): ChainEvent[] {
  if (!detail || !receipt) return [];
  return detail.recent_events.filter((event) => (
    event.event_data.includes(receipt.path) ||
    Boolean(step?.id && event.step_id === step.id)
  ));
}

function fieldLabel(key: string): string {
  return key.replace(/_/g, " ");
}

function receiptLabel(receipt: ReceiptSummary): string {
  return receipt.label || (receipt.step ? `step ${receipt.step}` : "orchestrator");
}

export function ReceiptDetailPage() {
  const { chainId = "", step } = useParams();
  const platform = useMemo(() => getYardPlatform(), []);
  const [detail, setDetail] = useState<ChainDetail | null>(null);
  const [receipt, setReceipt] = useState<ReceiptView | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [fileActionPath, setFileActionPath] = useState<string | null>(null);
  const [fileActionError, setFileActionError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    async function load() {
      try {
        setLoading(true);
        setError(null);
        const [loadedDetail, loadedReceipt] = await Promise.all([
          api.get<ChainDetail>(`/api/chains/${encodeURIComponent(chainId)}`),
          api.get<ReceiptView>(receiptAPIPath(chainId, step)),
        ]);
        if (cancelled) return;
        setDetail(loadedDetail);
        setReceipt(loadedReceipt);
      } catch (err) {
        if (!cancelled) setError(err instanceof Error ? err.message : "Failed to load receipt");
      } finally {
        if (!cancelled) setLoading(false);
      }
    }
    load();
    return () => {
      cancelled = true;
    };
  }, [chainId, step]);

  const parsed = useMemo(() => parseReceiptDocument(receipt?.content ?? ""), [receipt]);
  const changedFiles = useMemo(() => receiptProjectPaths(parsed), [parsed]);
  const hasFileActions = Boolean(platform.openProjectPath || platform.revealProjectPath);
  const currentStep = selectedStep(detail, receipt);
  const events = linkedEvents(detail, receipt, currentStep);
  const frontmatterEntries = Object.entries(parsed.frontmatter);

  async function validateProjectPath(path: string, purpose: "open_editor" | "reveal"): Promise<string> {
    const result = await api.post<ValidatePathsResponse>("/api/project/validate-paths", {
      purpose,
      paths: [path],
    });
    if (result.accepted.length > 0) return result.accepted[0];
    const rejection = result.rejected[0];
    throw new Error(rejection ? `${rejection.path}: ${rejection.reason}` : "path was not accepted");
  }

  async function openProjectPath(path: string) {
    if (!platform.openProjectPath) return;
    setFileActionPath(`open:${path}`);
    setFileActionError(null);
    try {
      const acceptedPath = await validateProjectPath(path, "open_editor");
      await platform.openProjectPath(acceptedPath);
    } catch (err) {
      setFileActionError(err instanceof Error ? err.message : "Failed to open file");
    } finally {
      setFileActionPath(null);
    }
  }

  async function revealProjectPath(path: string) {
    if (!platform.revealProjectPath) return;
    setFileActionPath(`reveal:${path}`);
    setFileActionError(null);
    try {
      const acceptedPath = await validateProjectPath(path, "reveal");
      await platform.revealProjectPath(acceptedPath);
    } catch (err) {
      setFileActionError(err instanceof Error ? err.message : "Failed to reveal file");
    } finally {
      setFileActionPath(null);
    }
  }

  return (
    <div className="flex-1 overflow-y-auto px-4 py-6">
      <div className="mx-auto max-w-6xl space-y-5">
        <div className="border-b border-border pb-4">
          <div className="flex flex-wrap items-center gap-3 text-xs uppercase tracking-widest">
            <Link to={`/chains/${encodeURIComponent(chainId)}`} className="text-muted-foreground hover:text-primary">
              Chain
            </Link>
            <Link to={`/chains/${encodeURIComponent(chainId)}#receipts`} className="text-muted-foreground hover:text-primary">
              Receipts
            </Link>
          </div>
          <h1 className="mt-2 text-xl font-bold uppercase tracking-widest text-primary text-glow-cyan">
            Receipt
          </h1>
          <p className="mt-1 break-all font-mono text-xs text-muted-foreground">{receipt?.path ?? "loading"}</p>
          {detail && (
            <p className={`mt-1 text-xs font-medium ${chainStatusClass(detail.chain.status)}`}>
              {detail.chain.status} / {detail.health || "unknown"}
            </p>
          )}
        </div>

        {loading && <p className="text-xs text-muted-foreground">Loading receipt...</p>}
        {error && <p className="text-xs text-destructive">{error}</p>}

        {detail && receipt && (
          <div className="grid gap-5 lg:grid-cols-[18rem_1fr]">
            <aside className="space-y-4">
              <section className="border border-border p-3 text-xs">
                <h2 className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
                  Summary
                </h2>
                <dl className="mt-3 space-y-2">
                  <div>
                    <dt className="text-[10px] uppercase tracking-widest text-muted-foreground">Chain</dt>
                    <dd className="mt-0.5 break-all font-mono text-foreground">{detail.chain.id}</dd>
                  </div>
                  <div>
                    <dt className="text-[10px] uppercase tracking-widest text-muted-foreground">Step</dt>
                    <dd className="mt-0.5 text-foreground">
                      {receipt.step || "orchestrator"}
                      {currentStep?.role ? ` / ${currentStep.role}` : ""}
                    </dd>
                  </div>
                  <div>
                    <dt className="text-[10px] uppercase tracking-widest text-muted-foreground">Status</dt>
                    <dd className="mt-0.5 text-foreground">
                      {currentStep?.status || detail.chain.status}
                      {currentStep?.verdict ? ` / ${currentStep.verdict}` : ""}
                    </dd>
                  </div>
                  <div>
                    <dt className="text-[10px] uppercase tracking-widest text-muted-foreground">Updated</dt>
                    <dd className="mt-0.5 text-foreground">{formatDate(detail.chain.updated_at)}</dd>
                  </div>
                </dl>
              </section>

              {frontmatterEntries.length > 0 && (
                <section className="border border-border p-3 text-xs">
                  <h2 className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
                    Frontmatter
                  </h2>
                  <dl className="mt-3 space-y-2">
                    {frontmatterEntries.map(([key, value]) => (
                      <div key={key}>
                        <dt className="text-[10px] uppercase tracking-widest text-muted-foreground">{fieldLabel(key)}</dt>
                        <dd className="mt-0.5 break-words font-mono text-foreground">{value}</dd>
                      </div>
                    ))}
                  </dl>
                </section>
              )}

              {changedFiles.length > 0 && (
                <section className="border border-border p-3 text-xs">
                  <h2 className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
                    Changed Files
                  </h2>
                  {fileActionError && <p className="mt-2 text-warning">{fileActionError}</p>}
                  <div className="mt-3 grid gap-2">
                    {changedFiles.map((path) => (
                      <div key={path} className="grid gap-2 border-t border-border/70 pt-2">
                        <span className="break-all font-mono text-foreground">{path}</span>
                        {hasFileActions && (
                          <div className="flex flex-wrap gap-2">
                            {platform.openProjectPath && (
                              <button
                                type="button"
                                onClick={() => void openProjectPath(path)}
                                disabled={fileActionPath !== null}
                                aria-label={`Open ${path}`}
                                className="inline-flex items-center gap-1 border border-border px-2 py-1 text-[10px] font-medium uppercase tracking-widest text-muted-foreground hover:border-primary hover:text-primary disabled:cursor-not-allowed disabled:opacity-60"
                              >
                                <ExternalLink size={12} aria-hidden="true" />
                                {fileActionPath === `open:${path}` ? "Opening" : "Open"}
                              </button>
                            )}
                            {platform.revealProjectPath && (
                              <button
                                type="button"
                                onClick={() => void revealProjectPath(path)}
                                disabled={fileActionPath !== null}
                                aria-label={`Reveal ${path}`}
                                className="inline-flex items-center gap-1 border border-border px-2 py-1 text-[10px] font-medium uppercase tracking-widest text-muted-foreground hover:border-primary hover:text-primary disabled:cursor-not-allowed disabled:opacity-60"
                              >
                                <FolderOpen size={12} aria-hidden="true" />
                                {fileActionPath === `reveal:${path}` ? "Revealing" : "Reveal"}
                              </button>
                            )}
                          </div>
                        )}
                      </div>
                    ))}
                  </div>
                </section>
              )}

              <section className="border border-border text-xs">
                <h2 className="border-b border-border bg-muted px-3 py-2 text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
                  Other Receipts
                </h2>
                {detail.receipts.length === 0 ? (
                  <p className="p-3 text-muted-foreground">No receipt list available.</p>
                ) : (
                  <div className="divide-y divide-border/70">
                    {detail.receipts.map((candidate) => (
                      <Link
                        key={`${candidate.step}:${candidate.path}`}
                        to={receiptRouteForSummary(detail.chain.id, candidate)}
                        className={`block px-3 py-2 hover:bg-muted ${
                          candidate.path === receipt.path ? "bg-muted text-primary" : "text-muted-foreground"
                        }`}
                      >
                        <span className="block font-medium">{receiptLabel(candidate)}</span>
                        <span className="block truncate font-mono text-[10px]">{candidate.path}</span>
                      </Link>
                    ))}
                  </div>
                )}
              </section>
            </aside>

            <main className="min-w-0 space-y-4">
              <section className="border border-border p-3 text-xs">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <h2 className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
                    Linked Events
                  </h2>
                  <span className="font-mono text-[10px] text-muted-foreground">{events.length}</span>
                </div>
                {events.length === 0 ? (
                  <p className="mt-2 text-muted-foreground">No linked events found in the current chain snapshot.</p>
                ) : (
                  <div className="mt-2 space-y-2">
                    {events.slice(0, 8).map((event) => (
                      <div key={event.id} className="grid gap-1 border-t border-border/70 pt-2 md:grid-cols-[10rem_12rem_1fr]">
                        <span className="text-muted-foreground">{formatDate(event.created_at)}</span>
                        <span className="font-medium text-primary">{event.event_type}</span>
                        <span className="truncate font-mono text-muted-foreground">{event.event_data}</span>
                      </div>
                    ))}
                  </div>
                )}
              </section>

              <section className="min-w-0 border border-border p-4">
                <h2 className="mb-3 text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
                  Receipt Markdown
                </h2>
                <div className="text-sm leading-relaxed text-foreground">
                  <MarkdownContent content={parsed.body || receipt.content} />
                </div>
              </section>
            </main>
          </div>
        )}
      </div>
    </div>
  );
}
