/**
 * Types for the metrics and context inspector REST API responses.
 */

// ── GET /api/metrics/conversation/:id ────────────────────────────────

export interface ConversationMetrics {
  token_usage: TokenUsageMetrics;
  cache_hit_rate_pct: number;
  tool_usage: ToolUsageMetrics[];
  context_quality: ContextQualityMetrics;
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
  explicit_files?: unknown;
  budget_breakdown?: BudgetCategory[];
  agent_read_files?: string[];

  budget_total?: number;
  budget_used?: number;
  included_count?: number;
  excluded_count?: number;
  agent_used_search_tool?: number;
  context_hit_rate?: number;

  created_at: string;
}

export interface ContextNeeds {
  queries?: string[];
  [key: string]: unknown;
}

export interface ContextSignal {
  type: string;
  value: string;
  source?: string;
  confidence?: number;
}

export interface RAGResult {
  file_path: string;
  chunk_name?: string;
  score: number;
  included: boolean;
  reason?: string;
}

export interface BrainResult {
  vault_path: string;
  title?: string;
  score: number;
  match_mode?: string;
}

export interface GraphResult {
  symbol: string;
  relationship: string;
  depth: number;
  file_path: string;
}

export interface BudgetCategory {
  category: string;
  tokens: number;
  percentage?: number;
}

// ── GET /api/providers ───────────────────────────────────────────────

export interface ProviderStatus {
  name: string;
  type: string;
  status: string;
  models: ProviderModel[];
}

export interface ProviderModel {
  id: string;
  name: string;
  context_window: number;
  supports_tools: boolean;
  supports_thinking: boolean;
}

// ── GET /api/config ──────────────────────────────────────────────────

export interface AppConfig {
  default_provider: string;
  default_model: string;
  fallback_provider: string;
  fallback_model: string;
  agent: {
    max_iterations: number;
    extended_thinking: boolean;
  };
  providers: Array<{
    name: string;
    type: string;
    models?: string[];
  }>;
}

// ── GET /api/project ─────────────────────────────────────────────────

export interface ProjectInfo {
  root_path: string;
  name: string;
  language?: string;
}
