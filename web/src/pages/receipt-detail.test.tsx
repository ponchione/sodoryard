import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ChainDetail } from "@/types/chains";

const { apiGet, apiPost, openProjectPath, revealProjectPath, clipboardWriteText } = vi.hoisted(() => ({
  apiGet: vi.fn(),
  apiPost: vi.fn(),
  openProjectPath: vi.fn(),
  revealProjectPath: vi.fn(),
  clipboardWriteText: vi.fn(),
}));

vi.mock("@/lib/api", () => ({
  api: {
    get: apiGet,
    post: apiPost,
  },
}));

vi.mock("@/platform", () => ({
  getYardPlatform: () => ({
    kind: "desktop",
    backendBaseUrl: "",
    openExternal: vi.fn(),
    openProjectPath,
    revealProjectPath,
  }),
}));

import { ReceiptDetailPage } from "./receipt-detail";

function chainDetail(): ChainDetail {
  return {
    health: "ok",
    warnings: [],
    chain: {
      id: "chain-1",
      source_specs: ["docs/specs/24-electron-desktop-app.md"],
      source_task: "review receipt",
      status: "completed",
      summary: "",
      total_steps: 1,
      total_tokens: 42,
      total_duration_secs: 3,
      resolver_loops: 0,
      started_at: "2026-05-11T12:00:00Z",
      updated_at: "2026-05-11T12:05:00Z",
    },
    steps: [
      {
        id: "step-db-1",
        chain_id: "chain-1",
        sequence_num: 1,
        role: "coder",
        task: "code",
        status: "completed",
        verdict: "accepted",
        receipt_path: "receipts/coder/chain-1-step-001.md",
        tokens_used: 42,
        turns_used: 2,
        duration_secs: 3,
      },
    ],
    receipts: [
      {
        label: "orchestrator",
        step: "",
        path: "receipts/orchestrator/chain-1.md",
      },
      {
        label: "step 1 coder",
        step: "1",
        path: "receipts/coder/chain-1-step-001.md",
      },
    ],
    approvals: [],
    recent_events: [
      {
        id: 1,
        chain_id: "chain-1",
        step_id: "step-db-1",
        event_type: "step_completed",
        event_data: "{\"receipt_path\":\"receipts/coder/chain-1-step-001.md\"}",
        created_at: "2026-05-11T12:04:00Z",
      },
    ],
    timeline: [],
    guardrails: {
      open_finding_ids: [],
      closed_finding_ids: [],
      addressed_finding_ids: [],
      reopened_finding_ids: [],
      repeated_resolver_finding_ids: [],
      findings: [],
      lock_health: {
        acquired: 0,
        released: 0,
        blocked: 0,
        force_released: 0,
        release_failed: 0,
        heartbeat_failed: 0,
        stale_replaced: 0,
        unreleased_writers: 0,
      },
      changed_files: [],
      step_facts: [],
    },
  };
}

describe("ReceiptDetailPage", () => {
  beforeEach(() => {
    apiGet.mockReset();
    apiPost.mockReset();
    openProjectPath.mockReset();
    revealProjectPath.mockReset();
    clipboardWriteText.mockReset().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText: clipboardWriteText },
    });
  });

  afterEach(() => {
    cleanup();
  });

  it("renders a step receipt as markdown with frontmatter and linked events", async () => {
    apiGet.mockImplementation((path: string) => {
      if (path === "/api/chains/chain-1") return Promise.resolve(chainDetail());
      if (path === "/api/chains/chain-1/receipt?step=1") {
        return Promise.resolve({
          chain_id: "chain-1",
          step: "1",
          path: "receipts/coder/chain-1-step-001.md",
          content: [
            "---",
            "role: coder",
            "verdict: accepted",
            "changed_files:",
            "  - web/src/pages/receipt-detail.tsx",
            "---",
            "# Receipt Body",
            "",
            "- Rendered item",
          ].join("\n"),
        });
      }
      return Promise.reject(new Error(`unexpected path ${path}`));
    });
    apiPost.mockImplementation((_path: string, body: { paths: string[] }) => Promise.resolve({
      accepted: body.paths,
      rejected: [],
    }));

    render(
      <MemoryRouter initialEntries={["/receipts/chain-1/1"]}>
        <Routes>
          <Route path="/receipts/:chainId/:step" element={<ReceiptDetailPage />} />
        </Routes>
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(apiGet).toHaveBeenCalledWith("/api/chains/chain-1");
      expect(apiGet).toHaveBeenCalledWith("/api/chains/chain-1/receipt?step=1");
    });
    expect(await screen.findByRole("heading", { name: "Receipt Body" })).toBeInTheDocument();
    expect(screen.getByText("role")).toBeInTheDocument();
    expect(screen.getByText("coder")).toBeInTheDocument();
    expect(screen.getByText("verdict")).toBeInTheDocument();
    expect(screen.getByText("accepted")).toBeInTheDocument();
    expect(screen.getByText("changed files")).toBeInTheDocument();
    expect(screen.getByText("Source Specs")).toBeInTheDocument();
    expect(screen.getByText("docs/specs/24-electron-desktop-app.md")).toBeInTheDocument();
    expect(screen.getAllByText("web/src/pages/receipt-detail.tsx").length).toBeGreaterThan(0);
    expect(screen.getByText("step_completed")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "1 / coder" })).toHaveAttribute("href", "/chains/chain-1#step-step-db-1");
    expect(screen.getByRole("link", { name: /step 1 coder.*receipts\/coder\/chain-1-step-001\.md/ })).toHaveAttribute(
      "href",
      "/receipts/chain-1/1",
    );

    fireEvent.click(screen.getByRole("button", { name: "Copy receipt path" }));
    await waitFor(() => {
      expect(clipboardWriteText).toHaveBeenCalledWith("receipts/coder/chain-1-step-001.md");
    });
    expect(await screen.findByText("Receipt path copied")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Open docs/specs/24-electron-desktop-app.md" }));
    await waitFor(() => {
      expect(apiPost).toHaveBeenCalledWith("/api/project/validate-paths", {
        purpose: "open_editor",
        paths: ["docs/specs/24-electron-desktop-app.md"],
      });
      expect(openProjectPath).toHaveBeenCalledWith("docs/specs/24-electron-desktop-app.md");
    });

    fireEvent.click(screen.getByRole("button", { name: "Reveal docs/specs/24-electron-desktop-app.md" }));
    await waitFor(() => {
      expect(apiPost).toHaveBeenCalledWith("/api/project/validate-paths", {
        purpose: "reveal",
        paths: ["docs/specs/24-electron-desktop-app.md"],
      });
      expect(revealProjectPath).toHaveBeenCalledWith("docs/specs/24-electron-desktop-app.md");
    });

    fireEvent.click(screen.getByRole("button", { name: "Open web/src/pages/receipt-detail.tsx" }));
    await waitFor(() => {
      expect(apiPost).toHaveBeenCalledWith("/api/project/validate-paths", {
        purpose: "open_editor",
        paths: ["web/src/pages/receipt-detail.tsx"],
      });
      expect(openProjectPath).toHaveBeenCalledWith("web/src/pages/receipt-detail.tsx");
    });

    fireEvent.click(screen.getByRole("button", { name: "Reveal web/src/pages/receipt-detail.tsx" }));
    await waitFor(() => {
      expect(apiPost).toHaveBeenCalledWith("/api/project/validate-paths", {
        purpose: "reveal",
        paths: ["web/src/pages/receipt-detail.tsx"],
      });
      expect(revealProjectPath).toHaveBeenCalledWith("web/src/pages/receipt-detail.tsx");
    });
  });

  it("loads the orchestrator receipt when no step route parameter is present", async () => {
    apiGet.mockImplementation((path: string) => {
      if (path === "/api/chains/chain-1") return Promise.resolve(chainDetail());
      if (path === "/api/chains/chain-1/receipt") {
        return Promise.resolve({
          chain_id: "chain-1",
          step: "",
          path: "receipts/orchestrator/chain-1.md",
          content: "orchestrator receipt body",
        });
      }
      return Promise.reject(new Error(`unexpected path ${path}`));
    });

    render(
      <MemoryRouter initialEntries={["/receipts/chain-1"]}>
        <Routes>
          <Route path="/receipts/:chainId" element={<ReceiptDetailPage />} />
        </Routes>
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(apiGet).toHaveBeenCalledWith("/api/chains/chain-1/receipt");
    });
    expect(await screen.findByText("orchestrator receipt body")).toBeInTheDocument();
    expect(screen.getAllByText("orchestrator").length).toBeGreaterThan(0);
  });
});
