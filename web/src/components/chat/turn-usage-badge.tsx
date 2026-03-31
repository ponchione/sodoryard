import type { TurnUsage } from "@/hooks/use-conversation";

interface TurnUsageBadgeProps {
  usage: TurnUsage;
}

function formatDuration(ns?: number): string {
  if (ns == null) return "";
  const ms = ns / 1_000_000;
  if (ms < 1000) return `${ms.toFixed(0)}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
}

function formatTokens(n?: number): string {
  if (n == null) return "";
  if (n >= 1000) return `${(n / 1000).toFixed(1)}k`;
  return String(n);
}

export function TurnUsageBadge({ usage }: TurnUsageBadgeProps) {
  const parts: string[] = [];

  if (usage.inputTokens != null || usage.outputTokens != null) {
    const input = formatTokens(usage.inputTokens);
    const output = formatTokens(usage.outputTokens);
    if (input && output) {
      parts.push(`${input} in / ${output} out`);
    } else if (output) {
      parts.push(`${output} tokens`);
    }
  }

  if (usage.duration != null) {
    parts.push(formatDuration(usage.duration));
  }

  if (usage.iterationCount > 1) {
    parts.push(`${usage.iterationCount} iterations`);
  }

  if (parts.length === 0) return null;

  return (
    <div className="mt-1 flex items-center gap-2 text-[10px] text-muted-foreground/60">
      {parts.map((part, i) => (
        <span key={i}>{part}</span>
      ))}
    </div>
  );
}
