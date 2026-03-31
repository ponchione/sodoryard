import type { BudgetCategory } from "@/types/metrics";

interface BudgetBarProps {
  used: number;
  total: number;
  categories: BudgetCategory[];
}

const categoryColors: Record<string, string> = {
  explicit_files: "bg-blue-500",
  brain: "bg-purple-500",
  rag: "bg-green-500",
  structural: "bg-orange-500",
  conventions: "bg-yellow-500",
  git: "bg-cyan-500",
};

function formatTokens(n: number): string {
  if (n >= 1000) return `${(n / 1000).toFixed(1)}k`;
  return String(n);
}

export function BudgetBar({ used, total, categories }: BudgetBarProps) {
  const pct = total > 0 ? (used / total) * 100 : 0;
  const utilColor =
    pct < 70 ? "text-green-600 dark:text-green-400"
      : pct < 90 ? "text-yellow-600 dark:text-yellow-400"
      : "text-red-500 dark:text-red-400";

  // Sort categories by tokens descending.
  const sorted = [...categories].sort((a, b) => b.tokens - a.tokens);

  return (
    <div className="space-y-2">
      {/* Summary */}
      <div className="flex items-baseline justify-between text-xs">
        <span className="text-muted-foreground">
          {formatTokens(used)} / {formatTokens(total)} tokens
        </span>
        <span className={`font-medium ${utilColor}`}>{pct.toFixed(0)}%</span>
      </div>

      {/* Stacked bar */}
      <div className="flex h-2.5 w-full overflow-hidden rounded-full bg-muted">
        {sorted.map((cat) => {
          const catPct = total > 0 ? (cat.tokens / total) * 100 : 0;
          if (catPct < 0.5) return null;
          const color = categoryColors[cat.category] ?? "bg-gray-500";
          return (
            <div
              key={cat.category}
              className={`${color} transition-all`}
              style={{ width: `${catPct}%` }}
              title={`${cat.category}: ${formatTokens(cat.tokens)} (${catPct.toFixed(0)}%)`}
            />
          );
        })}
      </div>

      {/* Category detail list */}
      <div className="space-y-0.5">
        {sorted.map((cat) => {
          const catPct = total > 0 ? (cat.tokens / total) * 100 : 0;
          const dotColor = categoryColors[cat.category] ?? "bg-gray-500";
          return (
            <div key={cat.category} className="flex items-center gap-1.5 text-[10px]">
              <span className={`inline-block h-2 w-2 rounded-full ${dotColor}`} />
              <span className="flex-1 text-muted-foreground">{cat.category}</span>
              <span className="font-mono">{formatTokens(cat.tokens)}</span>
              <span className="w-8 text-right text-muted-foreground/60">{catPct.toFixed(0)}%</span>
            </div>
          );
        })}
      </div>
    </div>
  );
}
