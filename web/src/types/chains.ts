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

export interface RuntimeStatus {
  project_root: string;
  project_name: string;
  provider: string;
  model: string;
  auth_status: string;
  code_index: RuntimeIndexStatus;
  brain_index: RuntimeIndexStatus;
  local_services_status: string;
  active_chains: number;
  warnings: RuntimeWarning[];
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
  total_steps: number;
  total_tokens: number;
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

export interface ChainDetail {
  chain: ChainRecord;
  steps: ChainStep[];
  receipts: ReceiptSummary[];
  recent_events: ChainEvent[];
  health: string;
  warnings: RuntimeWarning[];
  guardrails: ChainGuardrails;
}
