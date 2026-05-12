import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ChainDetail } from "@/types/chains";

const { apiGet, apiPost } = vi.hoisted(() => ({
  apiGet: vi.fn(),
  apiPost: vi.fn(),
}));

vi.mock("@/lib/api", () => ({
  api: {
    get: apiGet,
    post: apiPost,
  },
}));

import { ChainDetailPage } from "./chain-detail";

function emptyGuardrails(): ChainDetail["guardrails"] {
  return {
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
  };
}

function chainMetrics(overrides: Partial<NonNullable<ChainDetail["metrics"]>> = {}): NonNullable<ChainDetail["metrics"]> {
  return {
    chain_id: "chain-1",
    status: "completed",
    health: "attention",
    launch_mode: "one_step_chain",
    step_max_turns: 4,
    step_max_tokens: 50000,
    has_step_max_turns: true,
    has_step_max_tokens: true,
    total_steps: 1,
    step_rows: 1,
    max_steps: 5,
    step_budget_pct: 20,
    completed_steps: 1,
    running_steps: 0,
    pending_steps: 0,
    failed_steps: 0,
    total_tokens: 10,
    step_token_total: 10,
    step_turn_total: 1,
    token_budget: 100,
    token_budget_pct: 10,
    total_duration_secs: 2,
    step_duration_secs: 2,
    max_duration_secs: 20,
    duration_budget_pct: 10,
    resolver_loops: 0,
    max_resolver_loops: 1,
    resolver_loop_pct: 0,
    event_total: 2,
    output_events: 1,
    step_failed_events: 0,
    changed_file_events: 0,
    step_guardrail_fact_events: 1,
    receipt_warning_events: 1,
    receipt_finding_events: 0,
    finding_lifecycle_fact_events: 0,
    open_finding_count: 1,
    closed_finding_count: 0,
    addressed_finding_count: 0,
    open_finding_ids: ["FIND-correctness-001"],
    closed_finding_ids: [],
    addressed_finding_ids: [],
    reopened_finding_ids: [],
    repeated_resolver_finding_ids: [],
    finding_lifecycle: [],
    source_writer_blocks: 0,
    source_writer_lock_acquires: 0,
    source_writer_lock_releases: 0,
    source_writer_lock_force_releases: 0,
    source_writer_lock_release_failures: 0,
    source_writer_lock_heartbeat_failures: 0,
    source_writer_lock_stale_replacements: 0,
    safety_limit_events: 0,
    reindex_started_events: 0,
    reindex_done_events: 0,
    process_started_events: 1,
    process_exited_events: 1,
    warnings: [{ message: "open audit findings: FIND-correctness-001" }],
    steps: [],
    ...overrides,
  };
}

describe("ChainDetailPage", () => {
  beforeEach(() => {
    apiGet.mockReset();
    apiPost.mockReset();
  });

  afterEach(() => {
    vi.useRealTimers();
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
      approvals: [],
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
      metrics: chainMetrics(),
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
    expect(screen.getByText("Dogfood Metrics")).toBeInTheDocument();
    expect(screen.getByText("10/100")).toBeInTheDocument();
    expect(screen.getByText("2s/20s")).toBeInTheDocument();
    expect(screen.getByText("output=1 guardrail=1")).toBeInTheDocument();
    expect(screen.getByText("open=1 addressed=0")).toBeInTheDocument();
    expect(screen.getByText(/mode=one_step_chain/)).toBeInTheDocument();
    expect(screen.getByText(/step_max_turns=4/)).toBeInTheDocument();
    expect(screen.getByText(/step_max_tokens=50000/)).toBeInTheDocument();
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

  it("replays new persisted events after the last seen event cursor", async () => {
    vi.useFakeTimers();
    const detail: ChainDetail = {
      health: "ok",
      warnings: [],
      chain: {
        id: "chain-2",
        source_specs: [],
        source_task: "watch events",
        status: "running",
        summary: "",
        total_steps: 1,
        total_tokens: 0,
        total_duration_secs: 0,
        resolver_loops: 0,
        started_at: "2026-05-01T12:00:00Z",
        updated_at: "2026-05-01T12:00:00Z",
      },
      steps: [
        {
          id: "step-2",
          chain_id: "chain-2",
          sequence_num: 1,
          role: "coder",
          task: "code",
          status: "running",
          verdict: "",
          receipt_path: "",
          tokens_used: 0,
          turns_used: 0,
          duration_secs: 0,
        },
      ],
      receipts: [],
      approvals: [],
      recent_events: [
        {
          id: 1,
          chain_id: "chain-2",
          step_id: "step-2",
          event_type: "step_started",
          event_data: "{\"role\":\"coder\"}",
          created_at: "2026-05-01T12:00:05Z",
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
          step_id: "step-2",
          event_data: "{\"role\":\"coder\"}",
        },
      ],
      guardrails: emptyGuardrails(),
    };
    apiGet.mockImplementation((url: string) => {
      if (url === "/api/chains/chain-2") return Promise.resolve(detail);
      if (url === "/api/chains/chain-2/events?after_id=1") {
        return Promise.resolve([
          {
            id: 2,
            chain_id: "chain-2",
            step_id: "step-2",
            event_type: "step_completed",
            event_data: "{\"verdict\":\"completed\"}",
            created_at: "2026-05-01T12:00:10Z",
          },
        ]);
      }
      return Promise.resolve([]);
    });

    render(
      <MemoryRouter initialEntries={["/chains/chain-2"]}>
        <Routes>
          <Route path="/chains/:id" element={<ChainDetailPage />} />
        </Routes>
      </MemoryRouter>,
    );

    await act(async () => {
      await Promise.resolve();
    });
    expect(apiGet).toHaveBeenCalledWith("/api/chains/chain-2");
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5_000);
      await Promise.resolve();
    });
    expect(apiGet).toHaveBeenCalledWith("/api/chains/chain-2/events?after_id=1");
    expect(screen.getAllByText("step_completed")).toHaveLength(2);
    expect(screen.getAllByText("{\"verdict\":\"completed\"}")).toHaveLength(2);
  });

  it("renders approval controls and refreshes detail after approving", async () => {
    const pendingDetail: ChainDetail = {
      health: "attention",
      warnings: [],
      chain: {
        id: "chain-approval",
        source_specs: [],
        source_task: "approve risky tool",
        status: "waiting_approval",
        summary: "",
        total_steps: 1,
        total_tokens: 0,
        total_duration_secs: 0,
        resolver_loops: 0,
        started_at: "2026-05-09T12:00:00Z",
        updated_at: "2026-05-09T12:00:00Z",
      },
      steps: [],
      receipts: [],
      approvals: [
        {
          id: "approval-web-1",
          chain_id: "chain-approval",
          step_id: "step-1",
          conversation_id: "conv-1",
          turn_number: 2,
          iteration: 1,
          tool_name: "shell",
          tool_input: { command: "git push --force" },
          reason: "shell command matches approval policy",
          risk_level: "high",
          status: "pending",
          created_at: "2026-05-09T12:01:00Z",
        },
      ],
      recent_events: [],
      timeline: [],
      guardrails: emptyGuardrails(),
    };
    const approvedDetail: ChainDetail = {
      ...pendingDetail,
      approvals: [
        {
          ...pendingDetail.approvals[0],
          status: "approved",
          decision_reason: "reviewed",
          decided_by: "operator",
          decided_at: "2026-05-09T12:02:00Z",
        },
      ],
    };
    apiGet.mockResolvedValueOnce(pendingDetail).mockResolvedValueOnce(approvedDetail);
    apiPost.mockResolvedValue({
      message: "approval approval-web-1 approved",
      approval: approvedDetail.approvals[0],
    });

    render(
      <MemoryRouter initialEntries={["/chains/chain-approval"]}>
        <Routes>
          <Route path="/chains/:id" element={<ChainDetailPage />} />
        </Routes>
      </MemoryRouter>,
    );

    expect(await screen.findByText("Approvals")).toBeInTheDocument();
    expect(screen.getByText("approval-web-1")).toBeInTheDocument();
    expect(screen.getByText(/git push --force/)).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /Approve/ }));

    await waitFor(() => {
      expect(apiPost).toHaveBeenCalledWith("/api/chains/chain-approval/approvals/approval-web-1/approve", {});
    });
    expect(await screen.findByText("approved")).toBeInTheDocument();
    expect(screen.getByText("reviewed")).toBeInTheDocument();
    expect(apiGet).toHaveBeenCalledWith("/api/chains/chain-approval");
  });
});
