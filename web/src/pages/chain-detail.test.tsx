import { fireEvent, render, screen, waitFor } from "@testing-library/react";
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
      receipts: [
        {
          label: "orchestrator",
          step: "",
          path: "receipts/orchestrator/chain-1.md",
        },
        {
          label: "coder",
          step: "step-1",
          path: "receipts/coder/chain-1-step-001.md",
        },
      ],
      recent_events: [
        {
          id: 1,
          chain_id: "chain-1",
          step_id: "step-1",
          event_type: "step_started",
          event_data: "{\"role\":\"coder\",\"receipt_path\":\"receipts/coder/chain-1-step-001.md\"}",
          created_at: "2026-05-01T12:00:05Z",
        },
        {
          id: 2,
          chain_id: "chain-1",
          step_id: "step-1",
          event_type: "receipt_validation_warning",
          event_data: "{\"receipt_path\":\"receipts/coder/chain-1-step-001.md\",\"warning\":\"missing findings\"}",
          created_at: "2026-05-01T12:00:07Z",
        },
      ],
      timeline: [
        {
          id: "event:1",
          source: "event",
          kind: "event",
          name: "step_started",
          event_type: "step_started",
          started_at: "2026-05-01T12:00:05Z",
          step_id: "step-1",
          event_data: "{\"role\":\"coder\",\"receipt_path\":\"receipts/coder/chain-1-step-001.md\"}",
        },
        {
          id: "span:span-1",
          source: "span",
          kind: "provider",
          name: "provider.stream",
          status: "error",
          trace_id: "trace-1",
          span_id: "span-1",
          parent_span_id: "span-parent",
          conversation_id: "conv-1",
          started_at: "2026-05-01T12:00:06Z",
          duration_ms: 1250,
          step_id: "step-1",
          turn_number: 1,
          iteration: 1,
          attributes: {
            provider: "codex",
            model: "gpt-5.5",
          },
          error: "provider failed",
        },
        {
          id: "event:2",
          source: "event",
          kind: "event",
          name: "receipt_validation_warning",
          event_type: "receipt_validation_warning",
          started_at: "2026-05-01T12:00:07Z",
          step_id: "step-1",
          event_data: "{\"receipt_path\":\"receipts/coder/chain-1-step-001.md\",\"warning\":\"missing findings\"}",
        },
      ],
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
            exit_code: 0,
            duration_secs: 2,
            receipt_present: true,
            synthetic_receipt_written: false,
            receipt_valid: true,
            receipt_schema_valid: true,
            receipt_step_valid: true,
            receipt_sections_valid: true,
            parsed_verdict: "completed",
            tokens_used: 10,
            turns_used: 1,
            receipt_duration_seconds: 2,
            claimed_validation_commands: ["rtk make test"],
            changed_file_claim_present: true,
            claimed_changed_files: ["internal/example.go"],
            changed_file_claim_matches_manifest: true,
            changed_file_claim_extra: [],
            changed_file_manifest_unclaimed: [],
            changed_file_manifest_present: true,
            changed_file_count: 1,
            changed_files: ["internal/example.go"],
            code_index_state_supported: true,
            code_index_state_found: true,
            code_index_dirty_mark_supported: true,
            code_index_dirty_mark_attempted: true,
            code_index_dirty_marked: true,
            code_index_dirty: true,
            code_index_dirty_reason: "source_write",
            brain_index_state_supported: true,
            brain_index_state_found: true,
            brain_index_dirty: true,
            brain_index_dirty_reason: "complete_step_with_receipt",
            source_writer_lock_release_attempted: true,
            source_writer_lock_released: true,
            finding_count: 1,
            open_finding_count: 1,
            closed_finding_count: 0,
            addressed_finding_count: 0,
            finding_ids: ["FIND-correctness-001"],
            open_finding_ids: [],
            closed_finding_ids: [],
            addressed_ids: [],
            suspicious_verdict_finding_combination: false,
          },
        ],
      },
    };
    apiGet.mockImplementation((url: string) => {
      if (url.includes("/receipt?step=step-1")) {
        return Promise.resolve({
          chain_id: "chain-1",
          step: "step-1",
          path: "receipts/coder/chain-1-step-001.md",
          content: "coder receipt body",
        });
      }
      if (url.includes("/receipt")) {
        return Promise.resolve({
          chain_id: "chain-1",
          step: "",
          path: "receipts/orchestrator/chain-1.md",
          content: "orchestrator receipt body",
        });
      }
      return Promise.resolve(detail);
    });

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
    expect(screen.getByText(/code_index_mark_supported=yes/)).toBeInTheDocument();
    expect(screen.getByText(/code_index_mark_attempted=yes/)).toBeInTheDocument();
    expect(screen.getByText(/code_index_marked=yes/)).toBeInTheDocument();
    expect(screen.getByText(/verdict completed/)).toBeInTheDocument();
    expect(screen.getByText(/validation=rtk make test/)).toBeInTheDocument();
    expect(screen.getByText("- flow: chain completed after coder step 1 without later auditor")).toBeInTheDocument();
    expect(screen.getByText("completed / attention")).toBeInTheDocument();
    expect(screen.getByText("Timeline")).toBeInTheDocument();
    expect(screen.getByText("provider.stream")).toBeInTheDocument();
    expect(screen.getByText(/turn=1 \/ iter=1 \/ 1250ms/)).toBeInTheDocument();
    expect(screen.getByText("provider failed")).toBeInTheDocument();
    expect(screen.getAllByRole("link", { name: "Step" })[0]).toHaveAttribute("href", "#step-step-1");
    expect(screen.getAllByRole("link", { name: "Event" })[0]).toHaveAttribute("href", "#event-1");
    expect(screen.getByRole("link", { name: "Guardrails" })).toHaveAttribute("href", "#guardrail-details");
    expect(screen.getAllByText("receipt_validation_warning").length).toBeGreaterThan(0);
    expect(screen.getByText("warning")).toBeInTheDocument();
    expect(screen.getByText("Trace details")).toBeInTheDocument();
    expect(screen.getByText(/trace_id=trace-1/)).toBeInTheDocument();
    expect(screen.getByText(/provider=codex/)).toBeInTheDocument();

    fireEvent.click(screen.getAllByRole("button", { name: "Receipt" })[0]);
    await waitFor(() => {
      expect(apiGet).toHaveBeenCalledWith("/api/chains/chain-1/receipt?step=step-1");
    });
    expect(await screen.findByText("coder receipt body")).toBeInTheDocument();
  });
});
