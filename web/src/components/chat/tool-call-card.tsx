import { useState } from "react";
import type { ToolCallBlock } from "@/hooks/use-conversation";
import { isBrainToolName } from "@/lib/tool-transcript";

interface ToolCallCardProps {
  block: ToolCallBlock;
}

function formatDuration(ns?: number): string {
  if (ns == null) return "";
  const ms = ns / 1_000_000;
  if (ms < 1000) return `${ms.toFixed(0)}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
}

function detailString(details: Record<string, unknown>, key: string): string | undefined {
  const value = details[key];
  return typeof value === "string" && value !== "" ? value : undefined;
}

function detailNumber(details: Record<string, unknown>, key: string): number | undefined {
  const value = details[key];
  return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}

function detailBoolean(details: Record<string, unknown>, key: string): boolean | undefined {
  const value = details[key];
  return typeof value === "boolean" ? value : undefined;
}

function formatBytes(bytes?: number): string | undefined {
  if (bytes == null) return undefined;
  if (Math.abs(bytes) < 1024) return `${bytes} B`;
  const kib = bytes / 1024;
  if (Math.abs(kib) < 1024) return `${kib.toFixed(1)} KiB`;
  return `${(kib / 1024).toFixed(1)} MiB`;
}

interface DetailFact {
  label: string;
  value: string;
  mono?: boolean;
}

function DetailFacts({ facts }: { facts: DetailFact[] }) {
  if (facts.length === 0) return null;
  return (
    <div>
      <div className="mb-1 font-semibold text-muted-foreground uppercase tracking-wider text-[10px]">Details</div>
      <div className="flex flex-wrap gap-1.5">
        {facts.map((fact) => (
          <span
            key={`${fact.label}:${fact.value}`}
            className="inline-flex max-w-full items-center gap-1 rounded-sm border border-border/50 bg-background/40 px-1.5 py-0.5 text-[11px] text-foreground/80"
          >
            <span className="shrink-0 text-muted-foreground">{fact.label}</span>
            <span className={`${fact.mono ? "font-mono" : ""} min-w-0 break-all`}>{fact.value}</span>
          </span>
        ))}
      </div>
    </div>
  );
}

function buildShellFacts(details: Record<string, unknown>): DetailFact[] {
  const facts: DetailFact[] = [];
  const exitCode = detailNumber(details, "exit_code");
  if (exitCode != null) facts.push({ label: "exit", value: String(exitCode), mono: true });

  const timedOut = detailBoolean(details, "timed_out");
  const cancelled = detailBoolean(details, "cancelled");
  if (timedOut) {
    facts.push({ label: "state", value: "timed out" });
  } else if (cancelled) {
    facts.push({ label: "state", value: "cancelled" });
  }

  const outputBytes = detailNumber(details, "output_bytes") ?? detailNumber(details, "returned_size");
  const outputSize = formatBytes(outputBytes);
  if (outputSize) facts.push({ label: "output", value: outputSize, mono: true });

  const stdoutBytes = formatBytes(detailNumber(details, "stdout_bytes"));
  const stderrBytes = formatBytes(detailNumber(details, "stderr_bytes"));
  if (stdoutBytes || stderrBytes) {
    facts.push({ label: "streams", value: `stdout ${stdoutBytes ?? "0 B"} / stderr ${stderrBytes ?? "0 B"}`, mono: true });
  }

  const persistedPath = detailString(details, "persisted_path");
  if (persistedPath) facts.push({ label: "persisted", value: persistedPath, mono: true });
  return facts;
}

function buildFileMutationFacts(details: Record<string, unknown>): DetailFact[] {
  const facts: DetailFact[] = [];
  const path = detailString(details, "path");
  if (path) facts.push({ label: "path", value: path, mono: true });

  const operation = detailString(details, "operation");
  if (operation) facts.push({ label: "op", value: operation });

  const created = detailBoolean(details, "created");
  const changed = detailBoolean(details, "changed");
  if (created) {
    facts.push({ label: "state", value: "created" });
  } else if (changed === false) {
    facts.push({ label: "state", value: "unchanged" });
  } else if (changed === true) {
    facts.push({ label: "state", value: "changed" });
  }

  const before = detailNumber(details, "bytes_before");
  const after = detailNumber(details, "bytes_after");
  if (before != null && after != null) {
    const delta = after - before;
    const deltaText = `${delta >= 0 ? "+" : ""}${formatBytes(delta) ?? `${delta} B`}`;
    facts.push({ label: "bytes", value: `${formatBytes(before) ?? before} -> ${formatBytes(after) ?? after} (${deltaText})`, mono: true });
  }

  const diffLines = detailNumber(details, "diff_line_count");
  if (diffLines != null && diffLines > 0) {
    const suffix = detailBoolean(details, "diff_truncated") ? " truncated" : "";
    facts.push({ label: "diff", value: `${diffLines} lines${suffix}`, mono: true });
  }

  const persistedPath = detailString(details, "persisted_path");
  if (persistedPath) facts.push({ label: "persisted", value: persistedPath, mono: true });
  return facts;
}

function buildDetailFacts(details?: Record<string, unknown>): DetailFact[] {
  if (!details) return [];
  const kind = detailString(details, "kind");
  switch (kind) {
    case "shell":
      return buildShellFacts(details);
    case "file_mutation":
      return buildFileMutationFacts(details);
    default:
      return [];
  }
}

export function ToolCallCard({ block }: ToolCallCardProps) {
  const isBrainTool = isBrainToolName(block.toolName);
  const [open, setOpen] = useState(isBrainTool);
  const detailFacts = buildDetailFacts(block.details);

  const statusColor = block.done
    ? block.success !== false
      ? "text-accent"
      : "text-destructive"
    : "text-[#ffab00]";

  const borderColor = block.done
    ? block.success !== false
      ? "#00e676"
      : "#ff1744"
    : "#ffab00";

  const statusIcon = block.done
    ? block.success !== false
      ? "✓"
      : "✗"
    : "⟳";

  return (
    <div className="my-1.5">
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className="flex items-center gap-1.5 text-xs hover:text-foreground transition-colors"
      >
        <svg
          className={`h-3 w-3 text-muted-foreground transition-transform ${open ? "rotate-90" : ""}`}
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
          strokeWidth={2}
        >
          <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
        </svg>
        <span className={statusColor}>{statusIcon}</span>
        <span className="font-medium text-foreground">{block.toolName}</span>
        {isBrainTool && (
          <span className="rounded bg-primary/10 px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wider text-primary">
            brain
          </span>
        )}
        {block.duration != null && (
          <span className="text-muted-foreground/60">{formatDuration(block.duration)}</span>
        )}
        {!block.done && (
          <span className="ml-1 inline-block h-2 w-2 bg-[#ffab00] pulse-glow" />
        )}
      </button>
      {open && (
        <div
          data-augmented-ui="tl-clip br-clip both"
          className="mt-1.5 ml-4 space-y-2 border-0 bg-muted/30 px-3 py-2 text-xs"
          style={{
            "--aug-tl": "10px",
            "--aug-br": "10px",
            "--aug-border-all": "1px",
            "--aug-border-bg": borderColor,
            "--aug-inlay-all": "3px",
            "--aug-inlay-bg": "#0a0e1480",
          } as React.CSSProperties}
        >
          <DetailFacts facts={detailFacts} />

          {/* Arguments */}
          {block.args && Object.keys(block.args).length > 0 && (
            <div>
              <div className="mb-0.5 font-semibold text-muted-foreground uppercase tracking-wider text-[10px]">Arguments</div>
              <pre className="whitespace-pre-wrap text-foreground/80 max-h-40 overflow-y-auto">
                {JSON.stringify(block.args, null, 2)}
              </pre>
            </div>
          )}

          {/* Streaming output */}
          {block.output && block.output !== block.result && (
            <div>
              <div className="mb-0.5 font-semibold text-muted-foreground uppercase tracking-wider text-[10px]">Output</div>
              <pre className="whitespace-pre-wrap text-foreground/80 max-h-48 overflow-y-auto">
                {block.output}
                {!block.done && (
                  <span className="ml-0.5 inline-block h-3 w-1.5 bg-primary pulse-glow" />
                )}
              </pre>
            </div>
          )}

          {/* Final result (if different from streaming output) */}
          {block.done && block.result && (
            <div>
              <div className="mb-0.5 font-semibold text-muted-foreground uppercase tracking-wider text-[10px]">
                {isBrainTool ? "Brain result" : "Result"}
              </div>
              <pre className="whitespace-pre-wrap text-foreground/80 max-h-48 overflow-y-auto">
                {block.result}
              </pre>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
