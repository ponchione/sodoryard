export interface RuntimeWarning {
  message: string;
}

export interface RuntimeIndexStatus {
  status: string;
  last_indexed_at?: string;
  last_indexed_commit?: string;
  stale_since?: string;
  stale_reason?: string;
}

export interface ModelCapabilities {
  supports_tools: boolean;
  supports_thinking: boolean;
  supports_reasoning_effort: boolean;
  supports_structured_output: boolean;
  supports_prompt_cache: boolean;
  supports_images: boolean;
  supports_tool_choice: boolean;
  max_output_tokens: number;
  known_quirks?: string[];
}

export interface RuntimeStatus {
  project_root: string;
  project_name: string;
  provider: string;
  model: string;
  context_window: number;
  model_capabilities: ModelCapabilities;
  auth_status: string;
  code_index: RuntimeIndexStatus;
  brain_index: RuntimeIndexStatus;
  local_services_status: string;
  active_chains: number;
  warnings: RuntimeWarning[];
}

export interface AgentRoleSummary {
  name: string;
}

export type LaunchMode =
  | "one_step_chain"
  | "manual_roster"
  | "sir_topham_decides"
  | "constrained_orchestration";

export interface LaunchRequest {
  template_id?: string;
  mode: LaunchMode;
  role?: string;
  allowed_roles?: string[];
  roster?: string[];
  source_task?: string;
  source_specs?: string[];
  max_steps?: number;
  max_resolver_loops?: number;
  max_duration?: string;
  token_budget?: number;
  step_max_turns?: number;
  step_max_tokens?: number;
  allow_approval_wait?: boolean;
}

export interface LaunchTemplate {
  id: string;
  mode: LaunchMode;
  label: string;
  description: string;
  input_schema?: unknown;
  default_roles?: string[];
  receipt_schema?: string;
  preflight_checks?: string[];
}

export interface LaunchPreview {
  mode: LaunchMode;
  template: LaunchTemplate;
  role?: string;
  allowed_roles?: string[];
  roster?: string[];
  summary: string;
  compiled_task: string;
  work_packet_markdown: string;
  step_max_turns?: number;
  step_max_tokens?: number;
  allow_approval_wait?: boolean;
  warnings: RuntimeWarning[];
}

export interface LaunchStartResponse {
  chain_id: string;
  status: string;
  preview: LaunchPreview;
}

export interface LaunchDraft {
  id: string;
  request: LaunchRequest;
  updated_at?: string;
}

export interface LaunchDraftRead {
  found: boolean;
  draft?: LaunchDraft;
}

export interface LaunchPreset {
  id: string;
  name: string;
  request: LaunchRequest;
  updated_at?: string;
}

export interface StepSummary {
  id: string;
  sequence_num: number;
  role: string;
  status: string;
  verdict: string;
  receipt_path: string;
  tokens_used: number;
  started_at?: string;
  completed_at?: string;
}

export interface ChainSummary {
  id: string;
  status: string;
  source_task: string;
  source_specs: string[];
  roles: string[];
  total_steps: number;
  total_tokens: number;
  total_duration_secs: number;
  receipt_count: number;
  last_event_type?: string;
  last_event_at?: string;
  started_at: string;
  updated_at: string;
  current_step?: StepSummary;
}

export interface ChainRecord {
  id: string;
  source_specs: string[];
  source_task: string;
  status: string;
  summary: string;
  total_steps: number;
  total_tokens: number;
  total_duration_secs: number;
  resolver_loops: number;
  started_at: string;
  completed_at?: string;
  updated_at: string;
}

export interface ChainStep {
  id: string;
  chain_id: string;
  sequence_num: number;
  role: string;
  task: string;
  status: string;
  verdict: string;
  receipt_path: string;
  tokens_used: number;
  turns_used: number;
  duration_secs: number;
  error_message?: string;
  started_at?: string;
  completed_at?: string;
}

export interface ChainEvent {
  id: number;
  chain_id: string;
  step_id: string;
  event_type: string;
  event_data: string;
  created_at: string;
}

export interface ChainTimelineItem {
  id: string;
  source: "event" | "span" | string;
  kind: string;
  name: string;
  status?: string;
  trace_id?: string;
  span_id?: string;
  parent_span_id?: string;
  conversation_id?: string;
  chain_id?: string;
  step_id?: string;
  turn_number?: number;
  iteration?: number;
  started_at: string;
  ended_at?: string;
  duration_ms?: number;
  attributes?: Record<string, unknown>;
  error?: string;
  event_type?: string;
  event_data?: string;
}

export interface ChainGuardrails {
  open_finding_ids: string[];
  closed_finding_ids: string[];
  addressed_finding_ids: string[];
  reopened_finding_ids: string[];
  repeated_resolver_finding_ids: string[];
  findings: GuardrailFinding[];
  lock_health: GuardrailLockHealth;
  changed_files: ChangedFileManifest[];
  step_facts: StepGuardrailFact[];
}

export interface GuardrailFinding {
  id: string;
  source_role: string;
  status: string;
  severity?: string;
  evidence?: string;
  summary?: string;
  required_fix?: string;
  resolution?: string;
  files_changed: string[];
  validation: string[];
  addressed_count: number;
  closed_count: number;
  reopened_count: number;
  first_seen_step: number;
  last_updated_step: number;
}

export interface GuardrailLockHealth {
  acquired: number;
  released: number;
  blocked: number;
  force_released: number;
  release_failed: number;
  heartbeat_failed: number;
  stale_replaced: number;
  unreleased_writers: number;
}

export interface ChangedFileManifest {
  step_id: string;
  sequence_num: number;
  role: string;
  paths: string[];
  error?: string;
}

export interface StepGuardrailFact {
  step_id: string;
  sequence_num: number;
  role: string;
  receipt_path: string;
  source_mutating: boolean;
  exit_code: number;
  duration_secs: number;
  receipt_present: boolean;
  synthetic_receipt_written: boolean;
  receipt_valid: boolean;
  receipt_schema_valid: boolean;
  receipt_step_valid: boolean;
  receipt_sections_valid: boolean;
  receipt_error?: string;
  parsed_verdict?: string;
  tokens_used: number;
  turns_used: number;
  receipt_duration_seconds: number;
  claimed_validation_commands: string[];
  changed_file_claim_present: boolean;
  claimed_changed_files: string[];
  changed_file_claim_matches_manifest: boolean;
  changed_file_claim_extra: string[];
  changed_file_manifest_unclaimed: string[];
  changed_file_manifest_present: boolean;
  changed_file_manifest_error?: string;
  changed_file_count: number;
  changed_files: string[];
  code_index_state_supported: boolean;
  code_index_state_found: boolean;
  code_index_dirty_mark_supported: boolean;
  code_index_dirty_mark_attempted: boolean;
  code_index_dirty_marked: boolean;
  code_index_dirty_mark_error?: string;
  code_index_dirty: boolean;
  code_index_dirty_reason?: string;
  code_index_state_error?: string;
  brain_index_state_supported: boolean;
  brain_index_state_found: boolean;
  brain_index_dirty: boolean;
  brain_index_dirty_reason?: string;
  brain_index_state_error?: string;
  source_writer_lock_release_attempted: boolean;
  source_writer_lock_released: boolean;
  source_writer_lock_release_error?: string;
  finding_count: number;
  open_finding_count: number;
  closed_finding_count: number;
  addressed_finding_count: number;
  finding_ids: string[];
  open_finding_ids: string[];
  closed_finding_ids: string[];
  addressed_ids: string[];
  suspicious_verdict_finding_combination: boolean;
  suspicious_verdict_finding_reason?: string;
  run_error?: string;
}

export interface ReceiptSummary {
  label: string;
  step: string;
  path: string;
}

export interface ReceiptView {
  chain_id: string;
  step: string;
  path: string;
  content: string;
}

export interface ChainApproval {
  id: string;
  chain_id: string;
  step_id?: string;
  conversation_id?: string;
  turn_number?: number;
  iteration?: number;
  tool_name: string;
  tool_input?: unknown;
  reason?: string;
  risk_level?: string;
  status: string;
  created_at?: string;
  decided_at?: string;
  decision_reason?: string;
  decided_by?: string;
}

export interface ApprovalDecisionResult {
  approval: ChainApproval;
  message: string;
}

export interface ChainControlResult {
  chain_id: string;
  previous_status?: string;
  target_status?: string;
  status?: string;
  event_type?: string;
  message: string;
  already?: boolean;
  signaled_pids?: number[];
  warnings?: RuntimeWarning[];
}

export interface ChainMetricsReport {
  chain_id: string;
  status: string;
  health: string;
  launch_mode?: string;
  step_max_turns: number;
  step_max_tokens: number;
  has_step_max_turns: boolean;
  has_step_max_tokens: boolean;
  total_steps: number;
  step_rows: number;
  max_steps: number;
  step_budget_pct: number;
  completed_steps: number;
  running_steps: number;
  pending_steps: number;
  failed_steps: number;
  total_tokens: number;
  step_token_total: number;
  step_turn_total: number;
  token_budget: number;
  token_budget_pct: number;
  total_duration_secs: number;
  step_duration_secs: number;
  max_duration_secs: number;
  duration_budget_pct: number;
  resolver_loops: number;
  max_resolver_loops: number;
  resolver_loop_pct: number;
  event_total: number;
  output_events: number;
  step_failed_events: number;
  changed_file_events: number;
  step_guardrail_fact_events: number;
  receipt_warning_events: number;
  receipt_finding_events: number;
  finding_lifecycle_fact_events: number;
  open_finding_count: number;
  closed_finding_count: number;
  addressed_finding_count: number;
  open_finding_ids: string[];
  closed_finding_ids: string[];
  addressed_finding_ids: string[];
  reopened_finding_ids: string[];
  repeated_resolver_finding_ids: string[];
  finding_lifecycle: GuardrailFinding[];
  source_writer_blocks: number;
  source_writer_lock_acquires: number;
  source_writer_lock_releases: number;
  source_writer_lock_force_releases: number;
  source_writer_lock_release_failures: number;
  source_writer_lock_heartbeat_failures: number;
  source_writer_lock_stale_replacements: number;
  safety_limit_events: number;
  reindex_started_events: number;
  reindex_done_events: number;
  process_started_events: number;
  process_exited_events: number;
  warnings: RuntimeWarning[];
  steps: ChainStepMetric[];
}

export interface ChainStepMetric {
  sequence_num: number;
  role: string;
  status: string;
  verdict: string;
  receipt_path: string;
  tokens_used: number;
  turns_used: number;
  duration_secs: number;
  exit_code?: number;
  error_message?: string;
}

export interface ChainDetail {
  chain: ChainRecord;
  steps: ChainStep[];
  receipts: ReceiptSummary[];
  approvals: ChainApproval[];
  recent_events: ChainEvent[];
  timeline?: ChainTimelineItem[];
  health: string;
  warnings: RuntimeWarning[];
  guardrails: ChainGuardrails;
  metrics?: ChainMetricsReport;
}
