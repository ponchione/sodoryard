import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { LaunchPreview, LaunchTemplate, RuntimeStatus } from "@/types/chains";

const { useApiResourceMock, apiPostMock, navigateMock } = vi.hoisted(() => ({
  useApiResourceMock: vi.fn(),
  apiPostMock: vi.fn(),
  navigateMock: vi.fn(),
}));

vi.mock("react-router-dom", async () => {
  const actual = await vi.importActual<typeof import("react-router-dom")>("react-router-dom");
  return {
    ...actual,
    useNavigate: () => navigateMock,
  };
});

vi.mock("@/hooks/use-api-resource", () => ({
  useApiResource: useApiResourceMock,
}));

vi.mock("@/lib/api", () => ({
  ApiError: class ApiError extends Error {
    status = 400;
    statusText = "Bad Request";
    body = "";
  },
  api: {
    post: apiPostMock,
  },
}));

import { LaunchPage } from "./launch";

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
    active_chains: 0,
    warnings: [],
  };
}

function templates(): LaunchTemplate[] {
  return [
    {
      id: "one_step",
      mode: "one_step_chain",
      label: "One Step",
      description: "Run one selected role.",
      receipt_schema: "yard.receipt.v1",
      preflight_checks: ["task_or_specs", "role"],
    },
    {
      id: "manual_roster",
      mode: "manual_roster",
      label: "Manual Roster",
      description: "Run a fixed roster.",
      receipt_schema: "yard.receipt.v1",
      preflight_checks: ["task_or_specs", "roster_roles"],
    },
  ];
}

function preview(): LaunchPreview {
  const [template] = templates();
  return {
    mode: "one_step_chain",
    template,
    role: "coder",
    source_specs: [],
    summary: "one-step coder launch",
    compiled_task: "Launch task: Ship launch workbench preview",
    work_packet_markdown: "Launch task: Ship launch workbench preview",
    run_sheet_markdown: "Run sheet\n\n1. coder\n   Produces: coder receipt",
    warnings: [{ message: "no source specs selected" }],
  };
}

describe("LaunchPage", () => {
  afterEach(() => {
    cleanup();
  });

  beforeEach(() => {
    navigateMock.mockReset();
    apiPostMock.mockReset().mockImplementation((path: string) => {
      if (path === "/api/launch/start") {
        return Promise.resolve({ chain_id: "chain-started", status: "running", preview: preview() });
      }
      if (path === "/api/project/validate-paths") {
        return Promise.resolve({ accepted: ["docs/specs/24-electron-desktop-app.md"], rejected: [] });
      }
      return Promise.resolve(preview());
    });
    useApiResourceMock.mockReset().mockImplementation((path: string, fallback: unknown) => {
      if (path === "/api/runtime/status") {
        return { data: runtimeStatus(), loading: false, error: null, refresh: vi.fn() };
      }
      if (path === "/api/roles") {
        return {
          data: [{ name: "coder" }, { name: "planner" }, { name: "correctness-auditor" }],
          loading: false,
          error: null,
          refresh: vi.fn(),
        };
      }
      if (path === "/api/chains/templates") {
        return { data: templates(), loading: false, error: null, refresh: vi.fn() };
      }
      if (path === "/api/launch/draft") {
        return { data: { found: false }, loading: false, error: null, refresh: vi.fn() };
      }
      if (path === "/api/launch/presets") {
        return { data: [], loading: false, error: null, refresh: vi.fn() };
      }
      if (path === "/api/project/tree?depth=4") {
        return {
          data: {
            name: ".",
            type: "dir",
            children: [
              {
                name: "docs",
                type: "dir",
                children: [
                  {
                    name: "specs",
                    type: "dir",
                    children: [{ name: "24-electron-desktop-app.md", type: "file" }],
                  },
                ],
              },
            ],
          },
          loading: false,
          error: null,
          refresh: vi.fn(),
        };
      }
      return { data: fallback, loading: false, error: null, refresh: vi.fn() };
    });
  });

  it("builds a one-step launch preview from the workbench form", async () => {
    render(
      <MemoryRouter>
        <LaunchPage />
      </MemoryRouter>,
    );

    fireEvent.click(screen.getByRole("button", { name: "coder" }));
    fireEvent.change(screen.getByLabelText("Task"), {
      target: { value: "Ship launch workbench preview" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Preview" }));

    await waitFor(() => {
      expect(apiPostMock).toHaveBeenCalledWith("/api/launch/preview", expect.objectContaining({
        template_id: "one_step",
        mode: "one_step_chain",
        role: "coder",
        steps: [{ role: "coder" }],
        source_task: "Ship launch workbench preview",
      }));
    });

    expect(await screen.findByText("one-step coder launch")).toBeInTheDocument();
    expect(screen.getByText(/Run sheet/)).toBeInTheDocument();
    expect(screen.getByText("no source specs selected")).toBeInTheDocument();
  });

  it("attaches validated project files to source specs before previewing", async () => {
    render(
      <MemoryRouter>
        <LaunchPage />
      </MemoryRouter>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Work" }));

    await waitFor(() => {
      expect(apiPostMock).toHaveBeenCalledWith("/api/project/validate-paths", {
        purpose: "launch_attachment",
        paths: ["docs/specs/24-electron-desktop-app.md"],
      });
    });
    expect(screen.getByLabelText("Project Sources")).toHaveValue("docs/specs/24-electron-desktop-app.md");

    fireEvent.click(screen.getByRole("button", { name: "coder" }));
    fireEvent.click(screen.getByRole("button", { name: "Preview" }));

    await waitFor(() => {
      expect(apiPostMock).toHaveBeenCalledWith("/api/launch/preview", expect.objectContaining({
        source_specs: ["docs/specs/24-electron-desktop-app.md"],
      }));
    });
  });

  it("loads source spec query attachments from the project browser", async () => {
    render(
      <MemoryRouter initialEntries={["/?source_spec=docs%2Fspec.md&source_spec=README.md"]}>
        <LaunchPage />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByLabelText("Project Sources")).toHaveValue("docs/spec.md\nREADME.md");
    });
  });

  it("starts a launch and navigates to the started chain", async () => {
    render(
      <MemoryRouter>
        <LaunchPage />
      </MemoryRouter>,
    );

    fireEvent.click(screen.getByRole("button", { name: "coder" }));
    fireEvent.change(screen.getByLabelText("Task"), {
      target: { value: "Start launch workbench chain" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Start" }));

    await waitFor(() => {
      expect(apiPostMock).toHaveBeenCalledWith("/api/launch/start", expect.objectContaining({
        template_id: "one_step",
        mode: "one_step_chain",
        role: "coder",
        steps: [{ role: "coder" }],
        source_task: "Start launch workbench chain",
      }));
    });
    expect(navigateMock).toHaveBeenCalledWith("/chains/chain-started");
  });
});
