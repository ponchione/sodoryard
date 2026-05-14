import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { UseProjectMemoryChainsReturn } from "@/hooks/use-project-memory-chains";
import type { ChainSummary, RuntimeStatus } from "@/types/chains";

const { useApiResourceMock, useProjectMemoryChainsMock, refreshMock } = vi.hoisted(() => ({
  useApiResourceMock: vi.fn(),
  useProjectMemoryChainsMock: vi.fn(),
  refreshMock: vi.fn(),
}));

vi.mock("@/hooks/use-api-resource", () => ({
  useApiResource: useApiResourceMock,
}));

vi.mock("@/hooks/use-project-memory-chains", () => ({
  useProjectMemoryChains: useProjectMemoryChainsMock,
}));

import { ChainsPage } from "./chains";

function runtimeStatus(): RuntimeStatus {
  return {
    project_root: "/tmp/project",
    project_name: "project",
    provider: "codex",
    model: "gpt-5.2",
    context_window: 200000,
    model_capabilities: {
      supports_tools: true,
      supports_thinking: true,
      supports_reasoning_effort: true,
      supports_structured_output: true,
      supports_prompt_cache: true,
      supports_images: false,
      supports_tool_choice: true,
      max_output_tokens: 100000,
    },
    auth_status: "ok",
    code_index: { status: "ready" },
    brain_index: { status: "ready" },
    local_services_status: "ready",
    active_chains: 1,
    warnings: [],
  };
}

function chainSummary(): ChainSummary {
  return {
    id: "chain-1",
    status: "running",
    source_task: "inspect project memory",
    source_specs: [],
    total_steps: 1,
    total_tokens: 42,
    started_at: "2026-01-02T03:04:05Z",
    updated_at: "2026-01-02T03:05:06Z",
    current_step: {
      id: "step-1",
      sequence_num: 1,
      role: "coder",
      status: "running",
      verdict: "",
      receipt_path: "",
      tokens_used: 42,
    },
  };
}

function completedChainSummary(): ChainSummary {
  return {
    ...chainSummary(),
    id: "chain-2",
    status: "completed",
    source_task: "archive chain results",
    total_tokens: 84,
    current_step: undefined,
  };
}

function projectMemoryState(): UseProjectMemoryChainsReturn {
  return {
    status: "connected",
    error: null,
    rowCount: 2,
    eventCount: 1,
    recentEvents: [
      {
        id: "event-1",
        chainId: "chain-1",
        stepId: "step-1",
        sequence: "9",
        eventType: "approval_required",
        createdAt: "2026-01-02T03:05:00Z",
        createdAtUs: 1_767_323_100_000_000n,
        payloadJson: "{\"tool\":\"shell\"}",
      },
    ],
    refresh: vi.fn(),
  };
}

describe("ChainsPage", () => {
  afterEach(() => {
    cleanup();
  });

  beforeEach(() => {
    refreshMock.mockReset();
    useApiResourceMock.mockReset().mockImplementation((path: string, initial: unknown) => {
      if (path === "/api/chains?limit=100") {
        return { data: [chainSummary(), completedChainSummary()], loading: false, error: null, refresh: refreshMock };
      }
      if (path === "/api/runtime/status") {
        return { data: runtimeStatus(), loading: false, error: null, refresh: vi.fn() };
      }
      return { data: initial, loading: false, error: null, refresh: vi.fn() };
    });
    useProjectMemoryChainsMock.mockReset().mockReturnValue(projectMemoryState());
  });

  it("renders Shunter-decoded project memory events alongside REST chain rows", () => {
    render(
      <MemoryRouter>
        <ChainsPage />
      </MemoryRouter>,
    );

    expect(screen.getByText("Recent Project Memory Events")).toBeInTheDocument();
    expect(screen.getByText("project memory: connected / 2 chain rows / 1 event rows")).toBeInTheDocument();
    expect(screen.getByText("approval_required")).toBeInTheDocument();
    expect(screen.getByText("{\"tool\":\"shell\"}")).toBeInTheDocument();
    expect(screen.getAllByRole("link", { name: "chain-1" })[0]).toHaveAttribute("href", "/chains/chain-1");
  });

  it("filters chain rows by status", () => {
    render(
      <MemoryRouter>
        <ChainsPage />
      </MemoryRouter>,
    );

    expect(screen.getAllByText("inspect project memory").length).toBeGreaterThan(0);
    expect(screen.getByText("archive chain results")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "completed" }));

    expect(screen.queryAllByText("inspect project memory")).toHaveLength(0);
    expect(screen.getByText("archive chain results")).toBeInTheDocument();
    expect(screen.getByText((_content, element) => element?.textContent === "1/2")).toBeInTheDocument();
  });
});
