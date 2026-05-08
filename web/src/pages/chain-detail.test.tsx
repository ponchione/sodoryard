import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ChainDetail } from "@/types/chains";

const { apiGet } = vi.hoisted(() => ({
  apiGet: vi.fn(),
}));

vi.mock("@/lib/api", () => ({
  api: {
    get: apiGet,
  },
}));

import { ChainDetailPage } from "./chain-detail";

describe("ChainDetailPage", () => {
  beforeEach(() => {
    apiGet.mockReset();
  });

  it("renders guardrail health warnings from the chain detail response", async () => {
    const detail: ChainDetail = {
      health: "attention",
      warnings: [{ message: "flow: chain completed after coder step 1 without later auditor" }],
      chain: {
        id: "chain-1",
        source_specs: [],
        source_task: "ship guardrails",
        status: "completed",
        summary: "done",
        total_steps: 1,
        total_tokens: 10,
        total_duration_secs: 2,
        resolver_loops: 0,
        started_at: "2026-05-01T12:00:00Z",
        updated_at: "2026-05-01T12:01:00Z",
      },
      steps: [
        {
          id: "step-1",
          chain_id: "chain-1",
          sequence_num: 1,
          role: "coder",
          task: "code",
          status: "completed",
          verdict: "completed",
          receipt_path: "receipts/coder/chain-1-step-001.md",
          tokens_used: 10,
          turns_used: 1,
          duration_secs: 2,
        },
      ],
      receipts: [],
      recent_events: [],
    };
    apiGet.mockResolvedValue(detail);

    render(
      <MemoryRouter initialEntries={["/chains/chain-1"]}>
        <Routes>
          <Route path="/chains/:id" element={<ChainDetailPage />} />
        </Routes>
      </MemoryRouter>,
    );

    await waitFor(() => expect(apiGet).toHaveBeenCalledWith("/api/chains/chain-1"));
    expect(await screen.findByText("Guardrail Warnings")).toBeInTheDocument();
    expect(screen.getByText("- flow: chain completed after coder step 1 without later auditor")).toBeInTheDocument();
    expect(screen.getByText("completed / attention")).toBeInTheDocument();
  });
});
