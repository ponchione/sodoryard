import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { UseProjectMemoryChainsReturn } from "@/hooks/use-project-memory-chains";
import type { ConversationSummary } from "@/types/api";
import type { ChainSummary, RuntimeStatus } from "@/types/chains";

const {
  useApiResourceMock,
  useProjectMemoryChainsMock,
  refreshChainsMock,
  refreshRuntimeMock,
  refreshLocalServicesMock,
  apiGetMock,
  apiPostMock,
} = vi.hoisted(() => ({
  useApiResourceMock: vi.fn(),
  useProjectMemoryChainsMock: vi.fn(),
  refreshChainsMock: vi.fn(),
  refreshRuntimeMock: vi.fn(),
  refreshLocalServicesMock: vi.fn(),
  apiGetMock: vi.fn(),
  apiPostMock: vi.fn(),
}));

vi.mock("@/hooks/use-api-resource", () => ({
  useApiResource: useApiResourceMock,
}));

vi.mock("@/hooks/use-project-memory-chains", () => ({
  useProjectMemoryChains: useProjectMemoryChainsMock,
}));

vi.mock("@/lib/api", () => ({
  ApiError: class ApiError extends Error {
    status = 400;
    statusText = "Bad Request";
    body = "";
  },
  api: {
    get: apiGetMock,
    post: apiPostMock,
  },
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

function localServicesStatus() {
  return {
    mode: "auto",
    compose_file: "/tmp/docker-compose.yml",
    project_dir: "/tmp",
    docker_available: true,
    daemon_available: true,
    compose_available: true,
    compose_file_exists: true,
    services: [
      {
        name: "qwen-coder",
        healthy: false,
        reachable: true,
        models_ready: false,
        required: true,
        detail: "models endpoint returned no models",
      },
    ],
    required_services: ["qwen-coder"],
    problems: ["required service qwen-coder unhealthy"],
    remediation: ["inspect stack logs: yard llm logs"],
  };
}

describe("DashboardPage", () => {
  afterEach(() => {
    cleanup();
  });

  beforeEach(() => {
    refreshChainsMock.mockReset();
    refreshRuntimeMock.mockReset();
    refreshLocalServicesMock.mockReset();
    apiGetMock.mockReset().mockResolvedValue({ tail: 80, logs: "line one\nline two" });
    apiPostMock.mockReset().mockResolvedValue({ message: "local services ready", status: localServicesStatus() });
    useApiResourceMock.mockReset().mockImplementation((path: string, fallback: unknown) => {
      if (path === "/api/runtime/status") {
        return { data: runtimeStatus(), loading: false, error: null, refresh: refreshRuntimeMock };
      }
      if (path === "/api/chains?limit=5") {
        return { data: [chainSummary()], loading: false, error: null, refresh: refreshChainsMock };
      }
      if (path === "/api/conversations?limit=5") {
        return { data: [conversationSummary()], loading: false, error: null, refresh: vi.fn() };
      }
      if (path === "/api/runtime/local-services") {
        return { data: localServicesStatus(), loading: false, error: null, refresh: refreshLocalServicesMock };
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
    expect(screen.getByText("required service qwen-coder unhealthy")).toBeInTheDocument();
    expect(screen.getByText("models endpoint returned no models")).toBeInTheDocument();
    expect(screen.getByText("build dashboard")).toBeInTheDocument();
    expect(screen.getAllByRole("link", { name: "Launch" })[0]).toHaveAttribute("href", "/launch");
    expect(screen.getAllByRole("link", { name: "Project" })[0]).toHaveAttribute("href", "/project");
    expect(screen.getAllByRole("link", { name: /chain-1/ })[0]).toHaveAttribute("href", "/chains/chain-1");
    expect(screen.getByText("Runtime follow-up")).toBeInTheDocument();
    expect(screen.getByText("step_completed")).toBeInTheDocument();
  });

  it("runs local service readiness actions", async () => {
    render(
      <MemoryRouter>
        <DashboardPage />
      </MemoryRouter>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Start" }));

    await waitFor(() => {
      expect(apiPostMock).toHaveBeenCalledWith("/api/runtime/local-services/up", {});
    });
    expect(refreshLocalServicesMock).toHaveBeenCalled();
    expect(refreshRuntimeMock).toHaveBeenCalled();
    expect(await screen.findByText("local services ready")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Logs" }));

    await waitFor(() => {
      expect(apiGetMock).toHaveBeenCalledWith("/api/runtime/local-services/logs?tail=80");
    });
    expect(await screen.findByText("line one", { exact: false })).toBeInTheDocument();
  });
});
