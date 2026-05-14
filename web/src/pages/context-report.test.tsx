import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const { apiGetMock } = vi.hoisted(() => ({
  apiGetMock: vi.fn(),
}));

vi.mock("@/lib/api", () => ({
  ApiError: class ApiError extends Error {
    status: number;
    statusText = "Not Found";
    body = "";

    constructor(status = 404) {
      super(`API ${status}`);
      this.status = status;
    }
  },
  api: {
    get: apiGetMock,
  },
}));

import { ContextReportPage } from "./context-report";

describe("ContextReportPage", () => {
  afterEach(() => {
    cleanup();
  });

  beforeEach(() => {
    apiGetMock.mockReset().mockImplementation((url: string) => {
      if (url === "/api/metrics/conversation/conv-1/context/2/signals") {
        return Promise.resolve({ stream: [{ index: 1, kind: "query", value: "auth middleware" }] });
      }
      if (url === "/api/metrics/conversation/conv-1/context/2") {
        return Promise.resolve({
          conversation_id: "conv-1",
          turn_number: 2,
          budget_used: 12,
          budget_total: 24,
          context_hit_rate: 0.75,
          included_count: 2,
          excluded_count: 1,
          needs: { queries: ["auth middleware"], include_git_context: true },
          signals: [{ type: "task", value: "fix auth" }],
          explicit_files: [
            { file_path: "internal/auth/middleware.go", included: true, reason: "explicit" },
          ],
          rag_results: [
            { file_path: "internal/auth/service.go", included: true, score: 0.9 },
          ],
          brain_results: [],
          graph_results: [],
          created_at: "2026-05-14T10:00:00Z",
        });
      }
      return Promise.reject(new Error(`unexpected URL ${url}`));
    });
  });

  it("loads a stored context report as a desktop route", async () => {
    render(
      <MemoryRouter initialEntries={["/context/conv-1/2?chain_id=chain-1"]}>
        <Routes>
          <Route path="/context/:conversationId/:turn" element={<ContextReportPage />} />
        </Routes>
      </MemoryRouter>,
    );

    expect(screen.getByRole("heading", { name: "Context Report" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Chain" })).toHaveAttribute("href", "/chains/chain-1");
    expect(screen.getByRole("link", { name: "Conversation" })).toHaveAttribute("href", "/c/conv-1");

    await waitFor(() => {
      expect(apiGetMock).toHaveBeenCalledWith("/api/metrics/conversation/conv-1/context/2");
    });
    expect(await screen.findByText("Turn 2 of 2")).toBeInTheDocument();
    expect(screen.getByText("Token Budget")).toBeInTheDocument();
    expect(screen.getAllByText("internal/auth/service.go").length).toBeGreaterThan(0);
  });
});
