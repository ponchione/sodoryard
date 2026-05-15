import type { BudgetCategory } from "@/types/metrics";

interface BudgetBarProps {
  used: number;
  total: number;
  categories: BudgetCategory[];
}

const categoryColors: Record<string, { bg: string; hex: string }> = {
  explicit_files: { bg: "bg-primary", hex: "#00e5ff" },
  brain: { bg: "bg-[#b388ff]", hex: "#b388ff" },
  rag: { bg: "bg-accent", hex: "#00e676" },
  structural: { bg: "bg-[#ffab00]", hex: "#ffab00" },
  conventions: { bg: "bg-[#ffab00]/70", hex: "#ffab00b3" },
  git: { bg: "bg-primary/70", hex: "#00e5ffb3" },
};

function formatTokens(n: number): string {
  if (n >= 1000) return `${(n / 1000).toFixed(1)}k`;
  return String(n);
}

export function BudgetBar({ used, total, categories }: BudgetBarProps) {
  const pct = total > 0 ? (used / total) * 100 : 0;
  const utilColor =
    pct < 70 ? "text-accent"
      : pct < 90 ? "text-[#ffab00]"
      : "text-destructive";

  // Sort categories by tokens descending.
  const sorted = categories.toSorted((a, b) => b.tokens - a.tokens);

  return (
    <div className="space-y-2">
      {/* Summary */}
      <div className="flex items-baseline justify-between text-xs">
        <span className="text-muted-foreground">
          {formatTokens(used)} / {formatTokens(total)} tokens
        </span>
        <span className={`font-medium ${utilColor}`}>{pct.toFixed(0)}%</span>
      </div>

      {/* Stacked bar — gradient glow */}
      <div className="relative flex h-2.5 w-full overflow-hidden bg-muted">
        {sorted.map((cat) => {
          const catPct = total > 0 ? (cat.tokens / total) * 100 : 0;
          if (catPct < 0.5) return null;
          const color = categoryColors[cat.category]?.bg ?? "bg-muted-foreground";
          const hex = categoryColors[cat.category]?.hex ?? "#546e7a";
          return (
            <div
              key={cat.category}
              className={`${color} transition-all`}
              style={{
                width: `${catPct}%`,
                boxShadow: `0 0 8px ${hex}`,
              }}
              title={`${cat.category}: ${formatTokens(cat.tokens)} (${catPct.toFixed(0)}%)`}
            />
          );
        })}
      </div>

      {/* Category detail list */}
      <div className="space-y-0.5">
        {sorted.map((cat) => {
          const catPct = total > 0 ? (cat.tokens / total) * 100 : 0;
          const dotBg = categoryColors[cat.category]?.bg ?? "bg-muted-foreground";
          return (
            <div key={cat.category} className="flex items-center gap-1.5 text-[10px]">
              <span className={`inline-block size-2 ${dotBg}`} />
              <span className="flex-1 text-muted-foreground">{cat.category}</span>
              <span>{formatTokens(cat.tokens)}</span>
              <span className="w-8 text-right text-muted-foreground/60">{catPct.toFixed(0)}%</span>
            </div>
          );
        })}
      </div>
    </div>
  );
}
