/**
 * REST API types — mirrors the Go types served by the conversation
 * endpoints in internal/server/conversations.go.
 */

export interface ConversationSummary {
  id: string;
  title?: string;
  updated_at: string;
}

export interface MessageView {
  id: number;
  role: "user" | "assistant" | "tool" | "system";
  content?: string;
  tool_use_id?: string;
  tool_name?: string;
  turn_number: number;
  iteration: number;
  sequence: number;
  is_compressed: boolean;
  is_summary: boolean;
  created_at: string;
}

export interface SearchResult {
  id: string;
  title?: string;
  updated_at: string;
  snippet: string;
}
