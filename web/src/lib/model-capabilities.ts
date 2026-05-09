import type { ModelCapabilities } from "@/types/chains";

export function formatTokenLimit(tokens: number): string {
  if (!Number.isFinite(tokens) || tokens <= 0) return "unknown";
  if (tokens >= 1_000_000) return `${(tokens / 1_000_000).toFixed(tokens % 1_000_000 === 0 ? 0 : 1)}m`;
  if (tokens >= 1_000) return `${(tokens / 1_000).toFixed(tokens % 1_000 === 0 ? 0 : 1)}k`;
  return tokens.toLocaleString();
}

export function modelCapabilityLabels(capabilities?: ModelCapabilities): string[] {
  if (!capabilities) return [];
  const labels: string[] = [];
  if (capabilities.supports_tools) labels.push("tools");
  if (capabilities.supports_thinking) labels.push("thinking");
  if (capabilities.supports_reasoning_effort) labels.push("effort");
  if (capabilities.supports_structured_output) labels.push("structured output");
  if (capabilities.supports_prompt_cache) labels.push("prompt cache");
  if (capabilities.supports_images) labels.push("images");
  if (capabilities.supports_tool_choice) labels.push("tool choice");
  return labels;
}

export function formatModelCapabilitySummary(capabilities?: ModelCapabilities): string {
  const labels = modelCapabilityLabels(capabilities);
  return labels.length > 0 ? labels.join(", ") : "no capability metadata";
}
