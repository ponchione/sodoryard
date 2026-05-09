import { useEffect, useMemo, useState } from "react";
import { Link, useParams, useSearchParams } from "react-router-dom";
import { api } from "@/lib/api";
import { chainStatusClass } from "@/lib/chain-status";
import type { ChainDetail, ChainTimelineItem, ReceiptSummary, ReceiptView } from "@/types/chains";

function anchorID(prefix: string, value: string | number): string {
  return `${prefix}-${String(value).replace(/[^a-zA-Z0-9_-]+/g, "-")}`;
}

function formatDate(value?: string): string {
  if (!value) return "unknown";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}

function formatIDs(values: string[]): string {
  return values.length > 0 ? values.join(", ") : "none";
}

function yesNo(value: boolean): string {
  return value ? "yes" : "no";
}

function timelineMeta(item: ChainTimelineItem): string {
  const parts = [
    item.step_id ? `step=${item.step_id}` : "",
    item.turn_number ? `turn=${item.turn_number}` : "",
    item.iteration ? `iter=${item.iteration}` : "",
    item.duration_ms ? `${item.duration_ms}ms` : "",
  ].filter(Boolean);
  return parts.length > 0 ? parts.join(" / ") : "no linked runtime metadata";
}

function parseTimelineEventData(item: ChainTimelineItem): Record<string, unknown> {
  if (!item.event_data) return {};
  try {
    const parsed = JSON.parse(item.event_data);
    if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
      return parsed as Record<string, unknown>;
    }
  } catch {
    return {};
  }
  return {};
}

function timelineString(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

function timelineEventID(item: ChainTimelineItem): string {
  return item.source === "event" && item.id.startsWith("event:") ? item.id.slice("event:".length) : "";
}

function timelineReceiptPath(item: ChainTimelineItem): string {
  const eventData = parseTimelineEventData(item);
  return timelineString(eventData.receipt_path) || timelineString(item.attributes?.receipt_path);
}

function findTimelineReceipt(detail: ChainDetail, item: ChainTimelineItem): ReceiptSummary | null {
  const receiptPath = timelineReceiptPath(item);
  return (
    detail.receipts.find((candidate) => candidate.path === receiptPath) ??
    detail.receipts.find((candidate) => item.step_id && candidate.step === item.step_id) ??
    null
  );
}

function formatTimelineValue(value: unknown): string {
  if (typeof value === "string" || typeof value === "number" || typeof value === "boolean") {
    return String(value);
  }
  try {
    return JSON.stringify(value);
  } catch {
    return String(value);
  }
}

function timelineTraceDetails(item: ChainTimelineItem): string {
  if (item.source !== "span") return "";
  const details: Record<string, unknown> = {
    trace_id: item.trace_id,
    span_id: item.span_id,
    parent_span_id: item.parent_span_id,
    conversation_id: item.conversation_id,
    ...item.attributes,
  };
  return Object.entries(details)
    .filter(([, value]) => value !== undefined && value !== null && value !== "")
    .map(([key, value]) => `${key}=${formatTimelineValue(value)}`)
    .join("\n");
}

function isWarningTimelineItem(item: ChainTimelineItem): boolean {
  const text = `${item.event_type ?? ""} ${item.name ?? ""}`.toLowerCase();
  return text.includes("warning") || text.includes("blocked") || text.includes("safety_limit");
}

function timelineStatusLabel(item: ChainTimelineItem): string {
  if (item.status) return item.status;
  if (isWarningTimelineItem(item)) return "warning";
  return item.source;
}

function timelineStatusClass(item: ChainTimelineItem): string {
  const status = timelineStatusLabel(item);
  if (status === "error" || status === "failed" || item.name.includes("failed")) return "text-destructive";
  if (status === "cancelled" || status === "warning") return "text-warning";
  return "text-muted-foreground";
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

  function selectTimelineReceipt(receiptTarget: ReceiptSummary) {
    setSelectedReceipt(receiptTarget);
    const receiptContent = document.getElementById("receipt-content");
    if (typeof receiptContent?.scrollIntoView === "function") {
      receiptContent.scrollIntoView({ block: "start" });
    }
  }

  return (
    <div className="flex-1 overflow-y-auto px-4 py-6">
      <div className="mx-auto max-w-6xl space-y-5">
        <div className="border-b border-border pb-4">
          <Link to="/chains" className="text-xs uppercase tracking-widest text-muted-foreground hover:text-primary">
            Chains
          </Link>
          <h1 className="mt-2 break-all font-mono text-xl font-bold text-primary text-glow-cyan">{id}</h1>
          {detail && (
            <p className={`mt-1 text-xs font-medium ${chainStatusClass(detail.chain.status)}`}>
              {detail.chain.status} / {detail.health || "unknown"}
            </p>
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

            {(detail.warnings ?? []).length > 0 && (
              <section id="guardrail-warnings" className="space-y-2">
                <h2 className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
                  Guardrail Warnings
                </h2>
                <ul className="space-y-1 border border-border p-3 text-xs text-warning">
                  {(detail.warnings ?? []).map((warning, index) => (
                    <li key={`${warning.message}-${index}`}>- {warning.message}</li>
                  ))}
                </ul>
              </section>
            )}

            <section id="guardrail-details" className="space-y-2">
              <h2 className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
                Guardrail Details
              </h2>
              <div className="grid gap-3 border border-border p-3 text-xs lg:grid-cols-3">
                <div className="space-y-1">
                  <div className="text-[10px] uppercase tracking-widest text-muted-foreground">Finding IDs</div>
                  <p className="text-foreground">Open: {formatIDs(detail.guardrails.open_finding_ids)}</p>
                  <p className="text-muted-foreground">Addressed: {formatIDs(detail.guardrails.addressed_finding_ids)}</p>
                  <p className="text-muted-foreground">Closed: {formatIDs(detail.guardrails.closed_finding_ids)}</p>
                  <p className="text-warning">Reopened: {formatIDs(detail.guardrails.reopened_finding_ids)}</p>
                  <p className="text-warning">
                    Repeated resolver: {formatIDs(detail.guardrails.repeated_resolver_finding_ids)}
                  </p>
                  {detail.guardrails.findings.length > 0 && (
                    <div className="space-y-1 border-t border-border/70 pt-2">
                      {detail.guardrails.findings.slice(0, 3).map((finding) => (
                        <div key={`${finding.id}:${finding.source_role}`} className="space-y-1">
                          <p className="font-mono text-foreground">
                            {finding.id} {finding.status}
                            {finding.severity ? ` / ${finding.severity}` : ""}
                          </p>
                          <p className="text-muted-foreground">
                            {finding.source_role || "unknown"} addressed={finding.addressed_count} closed=
                            {finding.closed_count} reopened={finding.reopened_count}
                          </p>
                          {finding.evidence && <p className="font-mono text-muted-foreground">{finding.evidence}</p>}
                          {finding.summary && <p className="text-muted-foreground">{finding.summary}</p>}
                          {finding.resolution && <p className="text-muted-foreground">Resolution: {finding.resolution}</p>}
                          {finding.files_changed.length > 0 && (
                            <p className="font-mono text-muted-foreground">
                              Files: {finding.files_changed.join(", ")}
                            </p>
                          )}
                          {finding.validation.length > 0 && (
                            <p className="font-mono text-muted-foreground">
                              Validation: {finding.validation.join("; ")}
                            </p>
                          )}
                        </div>
                      ))}
                    </div>
                  )}
                </div>
                <div className="space-y-1">
                  <div className="text-[10px] uppercase tracking-widest text-muted-foreground">Source Writer Lock</div>
                  <p className="text-foreground">
                    acquired {detail.guardrails.lock_health.acquired} / released{" "}
                    {detail.guardrails.lock_health.released}
                  </p>
                  <p className="text-muted-foreground">
                    unreleased {detail.guardrails.lock_health.unreleased_writers} / blocked{" "}
                    {detail.guardrails.lock_health.blocked}
                  </p>
                  <p className="text-muted-foreground">
                    forced {detail.guardrails.lock_health.force_released} / stale{" "}
                    {detail.guardrails.lock_health.stale_replaced}
                  </p>
                  <p className="text-warning">
                    release failed {detail.guardrails.lock_health.release_failed} / heartbeat failed{" "}
                    {detail.guardrails.lock_health.heartbeat_failed}
                  </p>
                </div>
                <div className="space-y-2">
                  <div className="text-[10px] uppercase tracking-widest text-muted-foreground">Post-Step Facts</div>
                  {detail.guardrails.step_facts.length === 0 && (
                    <p className="text-muted-foreground">No guardrail fact events recorded.</p>
                  )}
                  {detail.guardrails.step_facts.slice(-3).map((fact) => (
                    <div key={`${fact.step_id}:${fact.sequence_num}`} className="space-y-1 border-t border-border/70 pt-2">
                      <p className="text-foreground">
                        Step {fact.sequence_num} {fact.role}: verdict {fact.parsed_verdict || "none"}, receipt{" "}
                        {yesNo(fact.receipt_valid)}, manifest {yesNo(fact.changed_file_manifest_present)}, lock released{" "}
                        {yesNo(fact.source_writer_lock_released)}
                      </p>
                      <p className="font-mono text-muted-foreground">
                        receipt_present={yesNo(fact.receipt_present)} synthetic=
                        {yesNo(fact.synthetic_receipt_written)} schema={yesNo(fact.receipt_schema_valid)} sections=
                        {yesNo(fact.receipt_sections_valid)}
                      </p>
                      <p className="font-mono text-muted-foreground">
                        changed={fact.changed_file_count} findings={fact.finding_count} open=
                        {formatIDs(fact.open_finding_ids)} closed={formatIDs(fact.closed_finding_ids)} addressed=
                        {formatIDs(fact.addressed_ids)}
                      </p>
                      {fact.claimed_validation_commands.length > 0 && (
                        <p className="font-mono text-muted-foreground">
                          validation={formatIDs(fact.claimed_validation_commands)}
                        </p>
                      )}
                      <p className="font-mono text-muted-foreground">
                        claim={yesNo(fact.changed_file_claim_present)} matches=
                        {yesNo(fact.changed_file_claim_matches_manifest)} claimed=
                        {formatIDs(fact.claimed_changed_files)}
                      </p>
                      {(fact.changed_file_claim_extra.length > 0 ||
                        fact.changed_file_manifest_unclaimed.length > 0) && (
                        <p className="font-mono text-warning">
                          extra={formatIDs(fact.changed_file_claim_extra)} unclaimed=
                          {formatIDs(fact.changed_file_manifest_unclaimed)}
                        </p>
                      )}
                      {fact.source_mutating && (
                        <p className="font-mono text-muted-foreground">
                          code_index_dirty={yesNo(fact.code_index_dirty)} code_index_mark_supported=
                          {yesNo(fact.code_index_dirty_mark_supported)} code_index_mark_attempted=
                          {yesNo(fact.code_index_dirty_mark_attempted)} code_index_marked=
                          {yesNo(fact.code_index_dirty_marked)} brain_index_dirty={yesNo(fact.brain_index_dirty)}
                          {fact.code_index_dirty_reason ? ` code_reason=${fact.code_index_dirty_reason}` : ""}
                          {fact.code_index_dirty_mark_error ? ` mark_error=${fact.code_index_dirty_mark_error}` : ""}
                          {fact.brain_index_dirty_reason ? ` brain_reason=${fact.brain_index_dirty_reason}` : ""}
                        </p>
                      )}
                      {fact.receipt_error && <p className="text-warning">{fact.receipt_error}</p>}
                      {fact.suspicious_verdict_finding_reason && (
                        <p className="text-warning">{fact.suspicious_verdict_finding_reason}</p>
                      )}
                      {fact.run_error && <p className="text-warning">{fact.run_error}</p>}
                    </div>
                  ))}
                </div>
              </div>
              {detail.guardrails.changed_files.length > 0 && (
                <div className="border border-border p-3 text-xs">
                  <div className="text-[10px] uppercase tracking-widest text-muted-foreground">Changed Files</div>
                  <div className="mt-2 space-y-1">
                    {detail.guardrails.changed_files.map((manifest) => (
                      <p key={`${manifest.step_id}:${manifest.sequence_num}`} className="font-mono text-muted-foreground">
                        step {manifest.sequence_num} {manifest.role}:{" "}
                        {manifest.paths.length > 0 ? manifest.paths.join(", ") : "none"}
                        {manifest.error ? ` (${manifest.error})` : ""}
                      </p>
                    ))}
                  </div>
                </div>
              )}
            </section>

            <section id="chain-source" className="space-y-2">
              <h2 className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">Source</h2>
              <p className="border border-border bg-muted/40 p-3 text-xs text-foreground">{source}</p>
            </section>

            <section id="steps" className="space-y-2">
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
                      <tr
                        id={step.id ? anchorID("step", step.id) : undefined}
                        key={step.id || `${step.sequence_num}-${step.role}`}
                        className="border-b border-border/70"
                      >
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

            <section className="space-y-2">
              <h2 className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">Timeline</h2>
              <div className="overflow-hidden border border-border">
                <table className="w-full text-left text-xs">
                  <thead className="border-b border-border bg-muted text-[10px] uppercase tracking-widest text-muted-foreground">
                    <tr>
                      <th className="px-3 py-2 font-medium">Time</th>
                      <th className="px-3 py-2 font-medium">Kind</th>
                      <th className="px-3 py-2 font-medium">Name</th>
                      <th className="px-3 py-2 font-medium">Status</th>
                      <th className="px-3 py-2 font-medium">Details</th>
                    </tr>
                  </thead>
                  <tbody>
                    {(detail.timeline ?? []).length === 0 && (
                      <tr>
                        <td colSpan={5} className="px-3 py-3 text-muted-foreground">
                          No timeline entries recorded.
                        </td>
                      </tr>
                    )}
                    {(detail.timeline ?? []).map((item) => {
                      const eventID = timelineEventID(item);
                      const hasStepLink = Boolean(item.step_id && detail.steps.some((step) => step.id === item.step_id));
                      const receiptTarget = findTimelineReceipt(detail, item);
                      const traceDetails = timelineTraceDetails(item);

                      return (
                        <tr
                          id={anchorID("timeline", item.id)}
                          key={item.id}
                          className="border-b border-border/70 align-top"
                        >
                          <td className="px-3 py-2 text-muted-foreground">{formatDate(item.started_at)}</td>
                          <td className="px-3 py-2 font-mono text-muted-foreground">{item.kind || item.source}</td>
                          <td className="px-3 py-2 text-foreground">{item.name}</td>
                          <td className={`px-3 py-2 ${timelineStatusClass(item)}`}>{timelineStatusLabel(item)}</td>
                          <td className="space-y-2 px-3 py-2">
                            <p className="font-mono text-muted-foreground">{timelineMeta(item)}</p>
                            <div className="flex flex-wrap gap-2 text-[10px] uppercase tracking-widest">
                              {hasStepLink && (
                                <a href={`#${anchorID("step", item.step_id ?? "")}`} className="text-primary hover:underline">
                                  Step
                                </a>
                              )}
                              {eventID && (
                                <a href={`#${anchorID("event", eventID)}`} className="text-primary hover:underline">
                                  Event
                                </a>
                              )}
                              {receiptTarget && (
                                <button
                                  type="button"
                                  onClick={() => selectTimelineReceipt(receiptTarget)}
                                  className="text-primary hover:underline"
                                >
                                  Receipt
                                </button>
                              )}
                              {item.kind === "context" && (
                                <a href="#chain-source" className="text-primary hover:underline">
                                  Source
                                </a>
                              )}
                              {(isWarningTimelineItem(item) || item.event_type?.includes("finding")) && (
                                <a href="#guardrail-details" className="text-primary hover:underline">
                                  Guardrails
                                </a>
                              )}
                            </div>
                            {item.error && <p className="text-destructive">{item.error}</p>}
                            {item.event_data && (
                              <p className="truncate font-mono text-muted-foreground">{item.event_data}</p>
                            )}
                            {traceDetails && (
                              <details>
                                <summary className="cursor-pointer text-[10px] uppercase tracking-widest text-muted-foreground">
                                  Trace details
                                </summary>
                                <pre className="mt-2 max-h-40 overflow-auto whitespace-pre-wrap border border-border/70 bg-background p-2 font-mono text-[10px] leading-relaxed text-muted-foreground">
                                  {traceDetails}
                                </pre>
                              </details>
                            )}
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            </section>

            <section id="receipts" className="grid gap-4 lg:grid-cols-[18rem_1fr]">
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
                <pre
                  id="receipt-content"
                  className="max-h-[32rem] overflow-auto whitespace-pre-wrap border border-border bg-background p-3 text-xs leading-relaxed text-foreground"
                >
                  {receipt?.content || "No receipt selected."}
                </pre>
              </div>
            </section>

            <section id="recent-events" className="space-y-2">
              <h2 className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">Recent Events</h2>
              <div className="space-y-1 border border-border p-3">
                {detail.recent_events.length === 0 && (
                  <p className="text-xs text-muted-foreground">No events recorded.</p>
                )}
                {detail.recent_events.map((event) => (
                  <div
                    id={anchorID("event", event.id)}
                    key={event.id}
                    className="grid gap-2 text-xs md:grid-cols-[10rem_12rem_1fr]"
                  >
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
