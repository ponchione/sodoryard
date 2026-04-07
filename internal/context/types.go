package context

import "github.com/ponchione/sirtopham/internal/codeintel"

// Signal records one analyzer extraction decision for observability.
//
// Common Type values in v0.1 are: "file_ref", "file_ref_rejected",
// "symbol_ref", "modification_intent", "creation_intent",
// "git_context", and "continuation".
type Signal struct {
	Type   string `json:"type"`
	Source string `json:"source"`
	Value  string `json:"value"`
}

// ContextNeeds describes the codebase context that should be retrieved for a
// single turn.
type ContextNeeds struct {
	SemanticQueries     []string `json:"semantic_queries,omitempty"`
	ExplicitFiles       []string `json:"explicit_files,omitempty"`
	ExplicitSymbols     []string `json:"explicit_symbols,omitempty"`
	IncludeConventions  bool     `json:"include_conventions,omitempty"`
	IncludeGitContext   bool     `json:"include_git_context,omitempty"`
	GitContextDepth     int      `json:"git_context_depth,omitempty"`
	MomentumFiles       []string `json:"momentum_files,omitempty"`
	MomentumModule      string   `json:"momentum_module,omitempty"`
	PreferBrainContext  bool     `json:"prefer_brain_context,omitempty"`
	Signals             []Signal `json:"signals,omitempty"`
}

// RAGHit represents one semantic-search code result adapted into Layer 3's
// context-assembly format.
type RAGHit struct {
	ChunkID         string              `json:"chunk_id"`
	FilePath        string              `json:"file_path"`
	Name            string              `json:"name"`
	Signature       string              `json:"signature,omitempty"`
	Description     string              `json:"description,omitempty"`
	Body            string              `json:"body,omitempty"`
	SimilarityScore float64             `json:"similarity_score"`
	Language        string              `json:"language,omitempty"`
	ChunkType       codeintel.ChunkType `json:"chunk_type,omitempty"`
	LineStart       int                 `json:"line_start,omitempty"`
	LineEnd         int                 `json:"line_end,omitempty"`
	HitCount        int                 `json:"hit_count,omitempty"`
	FromHop         bool                `json:"from_hop,omitempty"`
	MatchedBy       string              `json:"matched_by,omitempty"`
	Sources         []string            `json:"sources,omitempty"`
	Included        bool                `json:"included,omitempty"`
	ExclusionReason string              `json:"exclusion_reason,omitempty"`
}

// BrainHit represents one project-brain retrieval result.
//
// These fields are retained in v0.1 for report/schema continuity, but proactive
// brain retrieval remains out of scope until v0.2.
type BrainHit struct {
	DocumentPath    string   `json:"document_path"`
	Title           string   `json:"title,omitempty"`
	Snippet         string   `json:"snippet,omitempty"`
	MatchScore      float64  `json:"match_score"`
	MatchMode       string   `json:"match_mode,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	Included        bool     `json:"included,omitempty"`
	ExclusionReason string   `json:"exclusion_reason,omitempty"`
}

// GraphHit represents one structural-graph result such as a caller, callee, or
// implementation relationship.
type GraphHit struct {
	ChunkID          string `json:"chunk_id,omitempty"`
	SymbolName       string `json:"symbol_name"`
	FilePath         string `json:"file_path"`
	RelationshipType string `json:"relationship_type"`
	Depth            int    `json:"depth,omitempty"`
	LineStart        int    `json:"line_start,omitempty"`
	LineEnd          int    `json:"line_end,omitempty"`
	Included         bool   `json:"included,omitempty"`
	ExclusionReason  string `json:"exclusion_reason,omitempty"`
}

// FileResult represents one explicit file read requested deterministically from
// the user's turn.
type FileResult struct {
	FilePath        string `json:"file_path"`
	Content         string `json:"content,omitempty"`
	TokenCount      int    `json:"token_count,omitempty"`
	Truncated       bool   `json:"truncated,omitempty"`
	Included        bool   `json:"included,omitempty"`
	ExclusionReason string `json:"exclusion_reason,omitempty"`
}

// RetrievalResults groups the outputs of all retrieval paths before budget
// fitting and serialization.
type RetrievalResults struct {
	RAGHits        []RAGHit     `json:"rag_hits,omitempty"`
	BrainHits      []BrainHit   `json:"brain_hits,omitempty"`
	GraphHits      []GraphHit   `json:"graph_hits,omitempty"`
	FileResults    []FileResult `json:"file_results,omitempty"`
	ConventionText string       `json:"convention_text,omitempty"`
	GitContext     string       `json:"git_context,omitempty"`
}

// BudgetResult contains the content selected for serialization along with the
// accounting metadata produced by budget fitting.
type BudgetResult struct {
	SelectedRAGHits     []RAGHit          `json:"selected_rag_hits,omitempty"`
	SelectedBrainHits   []BrainHit        `json:"selected_brain_hits,omitempty"`
	SelectedGraphHits   []GraphHit        `json:"selected_graph_hits,omitempty"`
	SelectedFileResults []FileResult      `json:"selected_file_results,omitempty"`
	ConventionText      string            `json:"convention_text,omitempty"`
	GitContext          string            `json:"git_context,omitempty"`
	BudgetTotal         int               `json:"budget_total,omitempty"`
	BudgetUsed          int               `json:"budget_used,omitempty"`
	BudgetBreakdown     map[string]int    `json:"budget_breakdown,omitempty"`
	IncludedChunks      []string          `json:"included_chunks,omitempty"`
	ExcludedChunks      []string          `json:"excluded_chunks,omitempty"`
	ExclusionReasons    map[string]string `json:"exclusion_reasons,omitempty"`
	CompressionNeeded   bool              `json:"compression_needed,omitempty"`
}

// ContextAssemblyReport captures one turn's full context-assembly decision
// record.
//
// The report has a two-phase lifecycle: assembly-time fields are populated when
// the context package is built, then post-turn quality fields are filled in by
// the agent loop after the turn completes.
type ContextAssemblyReport struct {
	TurnNumber          int               `json:"turn_number,omitempty"`
	AnalysisLatencyMs   int64             `json:"analysis_latency_ms,omitempty"`
	RetrievalLatencyMs  int64             `json:"retrieval_latency_ms,omitempty"`
	TotalLatencyMs      int64             `json:"total_latency_ms,omitempty"`
	Needs               ContextNeeds      `json:"needs"`
	RAGResults          []RAGHit          `json:"rag_results,omitempty"`
	BrainResults        []BrainHit        `json:"brain_results,omitempty"`
	ExplicitFileResults []FileResult      `json:"explicit_file_results,omitempty"`
	GraphResults        []GraphHit        `json:"graph_results,omitempty"`
	IncludedChunks      []string          `json:"included_chunks,omitempty"`
	ExcludedChunks      []string          `json:"excluded_chunks,omitempty"`
	ExclusionReasons    map[string]string `json:"exclusion_reasons,omitempty"`
	BudgetTotal         int               `json:"budget_total,omitempty"`
	BudgetUsed          int               `json:"budget_used,omitempty"`
	BudgetBreakdown     map[string]int    `json:"budget_breakdown,omitempty"`
	AgentUsedSearchTool bool              `json:"agent_used_search_tool,omitempty"`
	AgentReadFiles      []string          `json:"agent_read_files,omitempty"`
	ContextHitRate      float64           `json:"context_hit_rate,omitempty"`
}

// FullContextPackage is the frozen output consumed by the Layer 5 agent loop.
//
// It intentionally holds only the serialized markdown block, its token count,
// the report, and a frozen marker. Serialization and mutation logic live in
// later Layer 3 components, not in this container type.
type FullContextPackage struct {
	Content    string                 `json:"content"`
	TokenCount int                    `json:"token_count"`
	Report     *ContextAssemblyReport `json:"report,omitempty"`
	Frozen     bool                   `json:"frozen,omitempty"`
}
