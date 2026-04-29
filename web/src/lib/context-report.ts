import type {
  BudgetCategory,
  ContextNeeds,
  ContextSignal,
  ContextSignalStreamEntry,
} from "@/types/metrics";

export function normalizeBudgetBreakdown(value: unknown): BudgetCategory[] {
  if (!value) return [];
  if (Array.isArray(value)) {
    return value.filter((item): item is BudgetCategory => {
      return typeof item === "object" && item !== null && "category" in item && "tokens" in item;
    });
  }
  if (typeof value === "object") {
    return Object.entries(value as Record<string, unknown>).flatMap(([category, tokens]) => {
      if (typeof tokens !== "number") return [];
      return [{ category, tokens }];
    });
  }
  return [];
}

export function buildSignalFlowFallback(
  needs?: ContextNeeds,
  signals?: ContextSignal[],
): ContextSignalStreamEntry[] {
  if (!needs && (!signals || signals.length === 0)) {
    return [];
  }

  const stream: ContextSignalStreamEntry[] = [];
  const append = (kind: string, type: string | undefined, source: string | undefined, value: string) => {
    stream.push({
      index: stream.length,
      kind,
      type,
      source,
      value,
    });
  };

  for (const signal of signals ?? needs?.signals ?? []) {
    append("signal", signal.type, signal.source, signal.value);
  }
  for (const query of needs?.semantic_queries ?? needs?.queries ?? []) {
    append("semantic_query", undefined, undefined, query);
  }
  for (const path of needs?.explicit_files ?? []) {
    append("explicit_file", undefined, undefined, path);
  }
  for (const symbol of needs?.explicit_symbols ?? []) {
    append("explicit_symbol", undefined, undefined, symbol);
  }
  for (const path of needs?.momentum_files ?? []) {
    append("momentum_file", undefined, undefined, path);
  }
  if (needs?.momentum_module) {
    append("momentum_module", undefined, undefined, needs.momentum_module);
  }
  if (needs?.prefer_brain_context) {
    append("flag", "prefer_brain_context", undefined, "true");
  }
  if (needs?.include_conventions) {
    append("flag", "include_conventions", undefined, "true");
  }
  if (needs?.include_git_context) {
    append(
      "flag",
      "include_git_context",
      undefined,
      needs.git_context_depth ? `depth=${needs.git_context_depth}` : "true",
    );
  }

  return stream;
}
