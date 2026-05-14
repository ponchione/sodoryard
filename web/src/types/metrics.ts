/**
 * Types for the metrics and context inspector REST API responses.
 */

// ── GET /api/metrics/conversation/:id ────────────────────────────────

export interface ConversationMetrics {
  token_usage: TokenUsageMetrics;
  cache_hit_rate_pct: number;
  tool_usage: ToolUsageMetrics[];
  context_quality: ContextQualityMetrics;
  /**
   * Latest completed turn's aggregated usage. Present only when the
   * conversation has at least one chat turn with recorded sub_calls. Used by
   * the frontend to hydrate the turn-usage badge on conversation reload.
   */
  last_turn?: LastTurnUsage;
}

export interface LastTurnUsage {
  turn_number: number;
  iteration_count: number;
  tokens_in: number;
  tokens_out: number;
  latency_ms: number;
}

export interface TokenUsageMetrics {
  tokens_in: number;
  tokens_out: number;
  cache_read_tokens: number;
  total_calls: number;
  total_latency_ms: number;
}

export interface ToolUsageMetrics {
  tool_name: string;
  call_count: number;
  avg_duration_ms: number;
  failure_count: number;
}

export interface ContextQualityMetrics {
  total_turns: number;
  reactive_search_count: number;
  avg_hit_rate: number;
  avg_budget_used_pct: number;
}

// ── GET /api/metrics/conversation/:id/context/:turn ──────────────────

export interface ContextReport {
  conversation_id: string;
  turn_number: number;

  analysis_latency_ms?: number;
  retrieval_latency_ms?: number;
  total_latency_ms?: number;

  needs?: ContextNeeds;
  signals?: ContextSignal[];
  rag_results?: RAGResult[];
  brain_results?: BrainResult[];
  graph_results?: GraphResult[];
  explicit_files?: ExplicitFileResult[];
  budget_breakdown?: BudgetCategory[];
  token_budget?: TokenBudgetReport;
  agent_read_files?: string[];

  budget_total?: number;
  budget_used?: number;
  included_count?: number;
  excluded_count?: number;
  agent_used_search_tool?: number;
  context_hit_rate?: number;
  signal_stream?: ContextSignalStreamEntry[];

  created_at: string;
}

export interface ContextSignalStreamResponse {
  conversation_id: string;
  turn_number: number;
  stream: ContextSignalStreamEntry[];
}

export interface ContextSignalStreamEntry {
  index: number;
  kind: string;
  type?: string;
  source?: string;
  value?: string;
}

export interface ContextNeeds {
  queries?: string[];
  semantic_queries?: string[];
  explicit_files?: string[];
  explicit_symbols?: string[];
  prefer_brain_context?: boolean;
  include_conventions?: boolean;
  include_git_context?: boolean;
  git_context_depth?: number;
  momentum_files?: string[];
  momentum_module?: string;
  signals?: ContextSignal[];
  [key: string]: unknown;
}

export interface ContextSignal {
  type: string;
  value: string;
  source?: string;
  confidence?: number;
}

export interface RAGResult {
  chunk_id?: string;
  file_path: string;
  chunk_name?: string;
  name?: string;
  signature?: string;
  description?: string;
  body?: string;
  score?: number;
  similarity_score?: number;
  language?: string;
  chunk_type?: string;
  line_start?: number;
  line_end?: number;
  hit_count?: number;
  from_hop?: boolean;
  matched_by?: string;
  sources?: string[];
  included: boolean;
  reason?: string;
  exclusion_reason?: string;
}

export interface BrainResult {
  vault_path?: string;
  document_path?: string;
  title?: string;
  section_heading?: string;
  snippet?: string;
  score?: number;
  lexical_score?: number;
  semantic_score?: number;
  match_score?: number;
  match_mode?: string;
  match_sources?: string[];
  graph_source_path?: string;
  graph_hop_depth?: number;
  tags?: string[];
  included?: boolean;
  exclusion_reason?: string;
}

export interface GraphResult {
  symbol?: string;
  symbol_name?: string;
  relationship?: string;
  relationship_type?: string;
  depth?: number;
  file_path: string;
  line_start?: number;
  line_end?: number;
  included?: boolean;
  exclusion_reason?: string;
}

export interface ExplicitFileResult {
  file_path: string;
  content?: string;
  token_count?: number;
  truncated?: boolean;
  included?: boolean;
  exclusion_reason?: string;
}

export interface BudgetCategory {
  category: string;
  tokens: number;
  percentage?: number;
}

export interface TokenBudgetReport {
  model_context_limit?: number;
  history_tokens?: number;
  reserved_system_prompt_tokens?: number;
  reserved_tool_schema_tokens?: number;
  reserved_output_tokens?: number;
  estimated_context_tokens?: number;
  estimated_request_tokens?: number;
  actual_input_tokens?: number;
  actual_output_tokens?: number;
  actual_cache_read_tokens?: number;
  actual_cache_creation_tokens?: number;
  actual_latency_ms?: number;
  iteration_count?: number;
  input_delta_tokens?: number;
  output_headroom_delta_tokens?: number;
  output_headroom_exceeded?: boolean;
}

// ── GET /api/providers ───────────────────────────────────────────────

export interface ProviderStatus {
  name: string;
  type: string;
  status: string;
  healthy?: boolean;
  last_error?: string;
  models: ProviderModel[];
  auth?: ProviderAuthStatus;
}

export interface ProviderModel {
  id: string;
  name: string;
  provider?: string;
  context_window: number;
  supports_tools: boolean;
  supports_thinking: boolean;
}

export interface ProviderAuthStatus {
  provider: string;
  mode?: string;
  source?: string;
  store_path?: string;
  source_path?: string;
  active_provider?: string;
  version?: number;
  last_refresh?: string;
  expires_at?: string;
  has_access_token: boolean;
  has_refresh_token: boolean;
  supports_refresh?: boolean;
  detail?: string;
  remediation?: string;
}

// ── GET /api/config ──────────────────────────────────────────────────

export interface AppConfig {
  default_provider: string;
  default_model: string;
  fallback_provider?: string;
  fallback_model?: string;
  agent: {
    max_iterations: number;
    extended_thinking: boolean;
    tool_output_max_tokens: number;
    tool_result_store_root?: string;
    cache_system_prompt: boolean;
    cache_assembled_context: boolean;
    cache_conversation_history: boolean;
  };
  providers: Array<{
    name: string;
    type: string;
    models?: string[];
    status?: string;
    healthy?: boolean;
    last_error?: string;
    auth?: ProviderAuthStatus;
  }>;
}

// ── GET /api/project ─────────────────────────────────────────────────

export interface BrainIndexInfo {
  status: "never_indexed" | "clean" | "stale" | string;
  last_indexed_at?: string;
  stale_since?: string;
  stale_reason?: string;
}

export interface ProjectInfo {
  id: string;
  root_path: string;
  name: string;
  language?: string;
  last_indexed_at?: string;
  last_indexed_commit?: string;
  brain_index?: BrainIndexInfo;
}
