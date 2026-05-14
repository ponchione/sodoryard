import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { UseProjectMemoryChainsReturn } from "@/hooks/use-project-memory-chains";
import type { ConversationSummary } from "@/types/api";
import type { ChainSummary, RuntimeStatus } from "@/types/chains";

const { useApiResourceMock, useProjectMemoryChainsMock, refreshChainsMock } = vi.hoisted(() => ({
  useApiResourceMock: vi.fn(),
  useProjectMemoryChainsMock: vi.fn(),
  refreshChainsMock: vi.fn(),
}));

vi.mock("@/hooks/use-api-resource", () => ({
  useApiResource: useApiResourceMock,
}));

vi.mock("@/hooks/use-project-memory-chains", () => ({
  useProjectMemoryChains: useProjectMemoryChainsMock,
}));

import { DashboardPage } from "./dashboard";

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
    code_index: { status: "ready", last_indexed_at: "2026-01-02T03:00:00Z" },
    brain_index: { status: "stale", stale_reason: "new brain docs" },
    local_services_status: "ready",
    active_chains: 2,
    warnings: [{ message: "code index is stale" }],
  };
}

function chainSummary(): ChainSummary {
  return {
    id: "chain-1",
    status: "running",
    source_task: "build dashboard",
    source_specs: [],
    total_steps: 2,
    total_tokens: 500,
    started_at: "2026-01-02T03:04:05Z",
    updated_at: "2026-01-02T03:05:06Z",
    current_step: {
      id: "step-1",
      sequence_num: 1,
      role: "coder",
      status: "running",
      verdict: "",
      receipt_path: "",
      tokens_used: 500,
    },
  };
}

function conversationSummary(): ConversationSummary {
  return {
    id: "conv-1",
    title: "Runtime follow-up",
    updated_at: "2026-01-02T04:05:06Z",
  };
}

function projectMemoryState(): UseProjectMemoryChainsReturn {
  return {
    status: "connected",
    error: null,
    rowCount: 3,
    eventCount: 7,
    recentEvents: [
      {
        id: "event-1",
        chainId: "chain-1",
        stepId: "step-1",
        sequence: "4",
        eventType: "step_completed",
        createdAt: "2026-01-02T03:06:00Z",
        createdAtUs: 1_767_323_160_000_000n,
        payloadJson: "{}",
      },
    ],
    refresh: vi.fn(),
  };
}

describe("DashboardPage", () => {
  beforeEach(() => {
    refreshChainsMock.mockReset();
    useApiResourceMock.mockReset().mockImplementation((path: string, fallback: unknown) => {
      if (path === "/api/runtime/status") {
        return { data: runtimeStatus(), loading: false, error: null, refresh: vi.fn() };
      }
      if (path === "/api/chains?limit=5") {
        return { data: [chainSummary()], loading: false, error: null, refresh: refreshChainsMock };
      }
      if (path === "/api/conversations?limit=5") {
        return { data: [conversationSummary()], loading: false, error: null, refresh: vi.fn() };
      }
      return { data: fallback, loading: false, error: null, refresh: vi.fn() };
    });
    useProjectMemoryChainsMock.mockReset().mockReturnValue(projectMemoryState());
  });

  it("renders runtime, chain, conversation, and project-memory summaries", () => {
    render(
      <MemoryRouter>
        <DashboardPage />
      </MemoryRouter>,
    );

    expect(screen.getByRole("heading", { name: "Dashboard" })).toBeInTheDocument();
    expect(screen.getByText("/tmp/project", { exact: false })).toBeInTheDocument();
    expect(screen.getByText("2")).toBeInTheDocument();
    expect(screen.getAllByText(/ready/).length).toBeGreaterThan(0);
    expect(screen.getByText("stale: new brain docs")).toBeInTheDocument();
    expect(screen.getByText("connected / 3 chains / 7 events")).toBeInTheDocument();
    expect(screen.getByText("code index is stale")).toBeInTheDocument();
    expect(screen.getByText("build dashboard")).toBeInTheDocument();
    expect(screen.getAllByRole("link", { name: /chain-1/ })[0]).toHaveAttribute("href", "/chains/chain-1");
    expect(screen.getByText("Runtime follow-up")).toBeInTheDocument();
    expect(screen.getByText("step_completed")).toBeInTheDocument();
  });
});
