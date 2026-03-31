import { useState } from "react";
import type { ToolCallBlock } from "@/hooks/use-conversation";

interface ToolCallCardProps {
  block: ToolCallBlock;
}

function formatDuration(ns?: number): string {
  if (ns == null) return "";
  const ms = ns / 1_000_000;
  if (ms < 1000) return `${ms.toFixed(0)}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
}

export function ToolCallCard({ block }: ToolCallCardProps) {
  const [open, setOpen] = useState(false);

  const statusColor = block.done
    ? block.success !== false
      ? "text-green-600 dark:text-green-400"
      : "text-red-500 dark:text-red-400"
    : "text-yellow-600 dark:text-yellow-400";

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
        <span className="font-mono font-medium text-foreground">{block.toolName}</span>
        {block.duration != null && (
          <span className="text-muted-foreground/60">{formatDuration(block.duration)}</span>
        )}
        {!block.done && (
          <span className="ml-1 inline-block h-2 w-2 animate-pulse rounded-full bg-yellow-500" />
        )}
      </button>
      {open && (
        <div className="mt-1.5 ml-4 space-y-2 rounded-md border border-border/50 bg-muted/30 px-3 py-2 text-xs">
          {/* Arguments */}
          {block.args && Object.keys(block.args).length > 0 && (
            <div>
              <div className="mb-0.5 font-semibold text-muted-foreground">Arguments</div>
              <pre className="whitespace-pre-wrap font-mono text-foreground/80 max-h-40 overflow-y-auto">
                {JSON.stringify(block.args, null, 2)}
              </pre>
            </div>
          )}

          {/* Streaming output */}
          {block.output && (
            <div>
              <div className="mb-0.5 font-semibold text-muted-foreground">Output</div>
              <pre className="whitespace-pre-wrap font-mono text-foreground/80 max-h-48 overflow-y-auto">
                {block.output}
                {!block.done && (
                  <span className="ml-0.5 inline-block h-3 w-0.5 animate-pulse bg-muted-foreground" />
                )}
              </pre>
            </div>
          )}

          {/* Final result (if different from streaming output) */}
          {block.done && block.result && block.result !== block.output && (
            <div>
              <div className="mb-0.5 font-semibold text-muted-foreground">Result</div>
              <pre className="whitespace-pre-wrap font-mono text-foreground/80 max-h-48 overflow-y-auto">
                {block.result}
              </pre>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
