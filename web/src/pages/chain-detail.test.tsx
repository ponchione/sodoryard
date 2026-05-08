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
      guardrails: {
        open_finding_ids: ["FIND-correctness-001"],
        closed_finding_ids: [],
        addressed_finding_ids: ["FIND-correctness-001"],
        reopened_finding_ids: ["FIND-correctness-001"],
        repeated_resolver_finding_ids: ["FIND-correctness-001"],
        findings: [
          {
            id: "FIND-correctness-001",
            source_role: "correctness-auditor",
            status: "addressed",
            severity: "high",
            evidence: "internal/example.go:42",
            summary: "nil panic",
            required_fix: "guard nil",
            resolution: "fixed",
            files_changed: ["internal/example.go"],
            validation: ["rtk make test"],
            addressed_count: 2,
            closed_count: 0,
            reopened_count: 1,
            first_seen_step: 1,
            last_updated_step: 3,
          },
        ],
        lock_health: {
          acquired: 1,
          released: 1,
          blocked: 0,
          force_released: 0,
          release_failed: 0,
          heartbeat_failed: 0,
          stale_replaced: 0,
          unreleased_writers: 0,
        },
        changed_files: [
          {
            step_id: "step-1",
            sequence_num: 1,
            role: "coder",
            paths: ["internal/example.go"],
          },
        ],
        step_facts: [
          {
            step_id: "step-1",
            sequence_num: 1,
            role: "coder",
            receipt_path: "receipts/coder/chain-1-step-001.md",
            source_mutating: true,
            receipt_valid: true,
            receipt_schema_valid: true,
            receipt_sections_valid: true,
            changed_file_claim_present: true,
            claimed_changed_files: ["internal/example.go"],
            changed_file_claim_matches_manifest: true,
            changed_file_claim_extra: [],
            changed_file_manifest_unclaimed: [],
            changed_file_manifest_present: true,
            changed_file_count: 1,
            changed_files: ["internal/example.go"],
            source_writer_lock_release_attempted: true,
            source_writer_lock_released: true,
            open_finding_ids: [],
            closed_finding_ids: [],
            addressed_ids: [],
          },
        ],
      },
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
    expect(screen.getByText("Guardrail Details")).toBeInTheDocument();
    expect(screen.getByText("Open: FIND-correctness-001")).toBeInTheDocument();
    expect(screen.getByText("FIND-correctness-001 addressed / high")).toBeInTheDocument();
    expect(screen.getByText("internal/example.go:42")).toBeInTheDocument();
    expect(screen.getByText("Validation: rtk make test")).toBeInTheDocument();
    expect(screen.getByText(/acquired 1 \/ released 1/)).toBeInTheDocument();
    expect(screen.getAllByText(/internal\/example.go/).length).toBeGreaterThan(0);
    expect(screen.getByText("- flow: chain completed after coder step 1 without later auditor")).toBeInTheDocument();
    expect(screen.getByText("completed / attention")).toBeInTheDocument();
  });
});
