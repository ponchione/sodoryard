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

function formatIDs(values: string[]): string {
  return values.length > 0 ? values.join(", ") : "none";
}

function yesNo(value: boolean): string {
  return value ? "yes" : "no";
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
              <section className="space-y-2">
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

            <section className="space-y-2">
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
