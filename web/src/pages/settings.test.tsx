import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { AppConfig, ProviderStatus } from "@/types/metrics";

const { useProvidersMock, useProjectInfoMock, apiGetMock, apiPutMock } = vi.hoisted(() => ({
  useProvidersMock: vi.fn(),
  useProjectInfoMock: vi.fn(),
  apiGetMock: vi.fn(),
  apiPutMock: vi.fn(),
}));

vi.mock("@/hooks/use-providers", () => ({
  useProviders: useProvidersMock,
}));

vi.mock("@/hooks/use-project-info", () => ({
  useProjectInfo: useProjectInfoMock,
}));

vi.mock("@/lib/api", () => ({
  ApiError: class ApiError extends Error {
    status = 400;
    statusText = "Bad Request";
    body = "";
  },
  api: {
    get: apiGetMock,
    put: apiPutMock,
  },
}));

import { SettingsPage } from "./settings";

function appConfig(overrides: Partial<AppConfig> = {}): AppConfig {
  return {
    default_provider: "codex",
    default_model: "gpt-5.5",
    fallback_provider: "anthropic",
    fallback_model: "claude-sonnet-4-5",
    agent: {
      max_iterations: 8,
      extended_thinking: true,
      tool_output_max_tokens: 12000,
      tool_result_store_root: "/tmp/tool-results",
      cache_system_prompt: true,
      cache_assembled_context: false,
      cache_conversation_history: true,
    },
    providers: [
      { name: "codex", type: "codex", models: ["gpt-5.5"] },
      { name: "openai", type: "openai", models: ["gpt-5.4"] },
      { name: "anthropic", type: "anthropic", models: ["claude-sonnet-4-5"] },
    ],
    ...overrides,
  };
}

function providerStatuses(): ProviderStatus[] {
  return [
    {
      name: "codex",
      type: "codex",
      status: "available",
      healthy: true,
      models: [
        {
          id: "gpt-5.5",
          name: "GPT-5.5",
          provider: "codex",
          context_window: 200000,
          supports_tools: true,
          supports_thinking: true,
        },
      ],
      auth: {
        provider: "codex",
        mode: "chatgpt",
        source: "sirtopham_store",
        has_access_token: true,
        has_refresh_token: true,
        expires_at: "2099-01-02T03:04:05Z",
      },
    },
    {
      name: "openai",
      type: "openai",
      status: "available",
      healthy: true,
      models: [
        {
          id: "gpt-5.4",
          name: "GPT-5.4",
          provider: "openai",
          context_window: 128000,
          supports_tools: true,
          supports_thinking: false,
        },
      ],
      auth: {
        provider: "openai",
        mode: "api_key",
        source: "env:OPENAI_API_KEY",
        has_access_token: true,
        has_refresh_token: false,
      },
    },
  ];
}

describe("SettingsPage", () => {
  afterEach(() => {
    cleanup();
  });

  beforeEach(() => {
    apiGetMock.mockReset().mockResolvedValue(appConfig());
    apiPutMock.mockReset().mockResolvedValue(appConfig({
      default_provider: "openai",
      default_model: "gpt-5.4",
    }));
    useProvidersMock.mockReset().mockReturnValue({
      providers: providerStatuses(),
      loading: false,
      error: null,
      refresh: vi.fn(),
    });
    useProjectInfoMock.mockReset().mockReturnValue({
      project: {
        id: "/tmp/project",
        name: "sodoryard",
        root_path: "/tmp/project",
        language: "go",
        last_indexed_at: "2026-01-02T03:04:05Z",
        last_indexed_commit: "abc123",
        brain_index: { status: "clean", last_indexed_at: "2026-01-02T03:05:06Z" },
      },
      loading: false,
      error: null,
      refresh: vi.fn(),
    });
  });

  it("renders project, routing, provider, model, and auth settings", async () => {
    render(<SettingsPage />);

    expect(await screen.findByRole("heading", { name: "Settings" })).toBeInTheDocument();
    expect(await screen.findByText("sodoryard")).toBeInTheDocument();
    expect(screen.getAllByText("codex:gpt-5.5").length).toBeGreaterThan(0);
    expect(screen.getByText("anthropic:claude-sonnet-4-5")).toBeInTheDocument();
    expect(screen.getByLabelText("Default Provider")).toHaveValue("codex");
    expect(screen.getByLabelText("Default Model")).toHaveValue("gpt-5.5");
    expect(screen.getAllByText("access token ready").length).toBeGreaterThan(0);
    expect(screen.getByText("mode chatgpt")).toBeInTheDocument();
    expect(screen.getByText("200k ctx")).toBeInTheDocument();
    expect(screen.getAllByLabelText("tools").length).toBeGreaterThan(0);
    expect(screen.getByLabelText("thinking")).toBeInTheDocument();
  });

  it("saves default provider and model through the validated config endpoint", async () => {
    render(<SettingsPage />);

    await screen.findByLabelText("Default Provider");
    fireEvent.change(screen.getByLabelText("Default Provider"), { target: { value: "openai" } });
    fireEvent.change(screen.getByLabelText("Default Model"), { target: { value: "gpt-5.4" } });
    fireEvent.click(screen.getByRole("button", { name: "Save Default" }));

    await waitFor(() => {
      expect(apiPutMock).toHaveBeenCalledWith("/api/config", {
        default_provider: "openai",
        default_model: "gpt-5.4",
      });
    });
    expect(await screen.findByText("Default route saved")).toBeInTheDocument();
  });

  it("surfaces backend validation errors when a runtime override is rejected", async () => {
    apiPutMock.mockRejectedValue(new Error("runtime default override is locked to codex/gpt-5.5"));

    render(<SettingsPage />);

    await screen.findByLabelText("Default Provider");
    fireEvent.change(screen.getByLabelText("Default Provider"), { target: { value: "openai" } });
    fireEvent.click(screen.getByRole("button", { name: "Save Default" }));

    expect(await screen.findByText("runtime default override is locked to codex/gpt-5.5")).toBeInTheDocument();
  });
});
