import { describe, expect, it } from "vitest";
import { formatModelCapabilitySummary, formatTokenLimit, modelCapabilityLabels } from "./model-capabilities";

describe("model capability formatting", () => {
  it("formats token limits compactly", () => {
    expect(formatTokenLimit(0)).toBe("unknown");
    expect(formatTokenLimit(32_000)).toBe("32k");
    expect(formatTokenLimit(1_500_000)).toBe("1.5m");
  });

  it("renders enabled capability labels in a stable order", () => {
    const labels = modelCapabilityLabels({
      supports_tools: true,
      supports_thinking: false,
      supports_reasoning_effort: true,
      supports_structured_output: true,
      supports_prompt_cache: false,
      supports_images: false,
      supports_tool_choice: true,
      max_output_tokens: 16_000,
    });

    expect(labels).toEqual(["tools", "effort", "structured output", "tool choice"]);
    expect(formatModelCapabilitySummary(undefined)).toBe("no capability metadata");
  });
});
