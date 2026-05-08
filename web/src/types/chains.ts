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
}
