package server

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	contextpkg "github.com/ponchione/sirtopham/internal/context"
	appdb "github.com/ponchione/sirtopham/internal/db"
)

// MetricsHandler serves per-conversation metrics and context report endpoints.
type MetricsHandler struct {
	queries *appdb.Queries
	logger  *slog.Logger
}

// NewMetricsHandler creates a handler and registers routes on the server.
func NewMetricsHandler(s *Server, queries *appdb.Queries, logger *slog.Logger) *MetricsHandler {
	h := &MetricsHandler{queries: queries, logger: logger}

	s.HandleFunc("GET /api/metrics/conversation/{id}", h.handleConversationMetrics)
	s.HandleFunc("GET /api/metrics/conversation/{id}/context/{turn}", h.handleContextReport)
	s.HandleFunc("GET /api/metrics/conversation/{id}/context/{turn}/signals", h.handleContextSignalStream)

	return h
}

// ── GET /api/metrics/conversation/:id ────────────────────────────────

type conversationMetricsResponse struct {
	TokenUsage     tokenUsageView     `json:"token_usage"`
	CacheHitRate   float64            `json:"cache_hit_rate_pct"`
	ToolUsage      []toolUsageView    `json:"tool_usage"`
	ContextQuality contextQualityView `json:"context_quality"`
	LastTurn       *lastTurnView      `json:"last_turn,omitempty"`
}

// lastTurnView carries the most recent turn's aggregated token + latency
// usage. Used by the frontend to hydrate the turn-usage badge on conversation
// reload (B3 fix — previously only populated from live `turn_complete` events).
type lastTurnView struct {
	TurnNumber     int64 `json:"turn_number"`
	IterationCount int64 `json:"iteration_count"`
	TokensIn       int64 `json:"tokens_in"`
	TokensOut      int64 `json:"tokens_out"`
	LatencyMs      int64 `json:"latency_ms"`
}

type tokenUsageView struct {
	TokensIn       int64 `json:"tokens_in"`
	TokensOut      int64 `json:"tokens_out"`
	CacheReadTokens int64 `json:"cache_read_tokens"`
	TotalCalls     int64 `json:"total_calls"`
	TotalLatencyMs int64 `json:"total_latency_ms"`
}

type toolUsageView struct {
	ToolName      string  `json:"tool_name"`
	CallCount     int64   `json:"call_count"`
	AvgDurationMs float64 `json:"avg_duration_ms"`
	FailureCount  int64   `json:"failure_count"`
}

type contextQualityView struct {
	TotalTurns           int64   `json:"total_turns"`
	ReactiveSearchCount  int64   `json:"reactive_search_count"`
	AvgHitRate           float64 `json:"avg_hit_rate"`
	AvgBudgetUsedPct     float64 `json:"avg_budget_used_pct"`
}

func (h *MetricsHandler) handleConversationMetrics(w http.ResponseWriter, r *http.Request) {
	convID := r.PathValue("id")

	ctx := r.Context()

	// Token usage.
	tokenRow, err := h.queries.GetConversationTokenUsage(ctx, sql.NullString{String: convID, Valid: true})
	if err != nil {
		h.logger.Error("get token usage", "error", err, "id", convID)
		writeError(w, http.StatusInternalServerError, "failed to get token usage")
		return
	}

	// Cache hit rate.
	cacheHit, err := h.queries.GetConversationCacheHitRate(ctx, sql.NullString{String: convID, Valid: true})
	if err != nil {
		h.logger.Error("get cache hit rate", "error", err, "id", convID)
		writeError(w, http.StatusInternalServerError, "failed to get cache hit rate")
		return
	}

	// Tool usage.
	toolRows, err := h.queries.GetConversationToolUsage(ctx, convID)
	if err != nil {
		h.logger.Error("get tool usage", "error", err, "id", convID)
		writeError(w, http.StatusInternalServerError, "failed to get tool usage")
		return
	}

	tools := make([]toolUsageView, 0, len(toolRows))
	for _, row := range toolRows {
		tools = append(tools, toolUsageView{
			ToolName:      row.ToolName,
			CallCount:     row.CallCount,
			AvgDurationMs: nullFloat(row.AvgDuration),
			FailureCount:  int64(nullFloat(row.FailureCount)),
		})
	}

	// Context quality.
	ctxRow, err := h.queries.GetConversationContextQuality(ctx, convID)
	if err != nil {
		h.logger.Error("get context quality", "error", err, "id", convID)
		writeError(w, http.StatusInternalServerError, "failed to get context quality")
		return
	}

	// Latest turn usage — best-effort (B3). Absent rows are not an error;
	// conversations with zero completed turns simply omit the field.
	var lastTurn *lastTurnView
	lastTurnRow, err := h.queries.GetConversationLastTurnUsage(ctx, appdb.GetConversationLastTurnUsageParams{
		ConversationID:   sql.NullString{String: convID, Valid: true},
		ConversationID_2: sql.NullString{String: convID, Valid: true},
	})
	if err == nil && lastTurnRow.TurnNumber.Valid {
		lastTurn = &lastTurnView{
			TurnNumber:     lastTurnRow.TurnNumber.Int64,
			IterationCount: lastTurnRow.IterationCount,
			TokensIn:       lastTurnRow.TokensIn,
			TokensOut:      lastTurnRow.TokensOut,
			LatencyMs:      lastTurnRow.LatencyMs,
		}
	} else if err != nil && err != sql.ErrNoRows {
		h.logger.Warn("get last turn usage", "error", err, "id", convID)
	}

	writeJSON(w, http.StatusOK, conversationMetricsResponse{
		TokenUsage: tokenUsageView{
			TokensIn:        tokenRow.TotalIn,
			TokensOut:       tokenRow.TotalOut,
			CacheReadTokens: tokenRow.TotalCacheHits,
			TotalCalls:      tokenRow.TotalCalls,
			TotalLatencyMs:  tokenRow.TotalLatencyMs,
		},
		CacheHitRate: cacheHit,
		ToolUsage:    tools,
		ContextQuality: contextQualityView{
			TotalTurns:          ctxRow.TotalTurns,
			ReactiveSearchCount: int64(nullFloat(ctxRow.ReactiveSearchTurns)),
			AvgHitRate:          nullFloat(ctxRow.AvgHitRate),
			AvgBudgetUsedPct:    nullFloat(ctxRow.AvgBudgetUsed),
		},
		LastTurn: lastTurn,
	})
}

// ── GET /api/metrics/conversation/:id/context/:turn ──────────────────

type contextReportResponse struct {
	ConversationID string `json:"conversation_id"`
	TurnNumber     int64  `json:"turn_number"`

	// Latency.
	AnalysisLatencyMs  *int64 `json:"analysis_latency_ms,omitempty"`
	RetrievalLatencyMs *int64 `json:"retrieval_latency_ms,omitempty"`
	TotalLatencyMs     *int64 `json:"total_latency_ms,omitempty"`

	// JSON blobs — passed through as raw JSON, not double-encoded.
	Needs           json.RawMessage `json:"needs,omitempty"`
	Signals         json.RawMessage `json:"signals,omitempty"`
	RAGResults      json.RawMessage `json:"rag_results,omitempty"`
	BrainResults    json.RawMessage `json:"brain_results,omitempty"`
	GraphResults    json.RawMessage `json:"graph_results,omitempty"`
	ExplicitFiles   json.RawMessage `json:"explicit_files,omitempty"`
	BudgetBreakdown json.RawMessage `json:"budget_breakdown,omitempty"`
	AgentReadFiles  json.RawMessage `json:"agent_read_files,omitempty"`

	// Scalars.
	BudgetTotal     *int64   `json:"budget_total,omitempty"`
	BudgetUsed      *int64   `json:"budget_used,omitempty"`
	IncludedCount   *int64   `json:"included_count,omitempty"`
	ExcludedCount   *int64   `json:"excluded_count,omitempty"`
	AgentUsedSearch *int64   `json:"agent_used_search_tool,omitempty"`
	ContextHitRate  *float64 `json:"context_hit_rate,omitempty"`

	CreatedAt string `json:"created_at"`
}

type contextSignalStreamResponse struct {
	ConversationID string                     `json:"conversation_id"`
	TurnNumber     int64                      `json:"turn_number"`
	Stream         []contextSignalStreamEntry `json:"stream"`
}

type contextSignalStreamEntry struct {
	Index  int    `json:"index"`
	Kind   string `json:"kind"`
	Type   string `json:"type,omitempty"`
	Source string `json:"source,omitempty"`
	Value  string `json:"value,omitempty"`
}

func (h *MetricsHandler) handleContextReport(w http.ResponseWriter, r *http.Request) {
	convID := r.PathValue("id")
	turnStr := r.PathValue("turn")

	turn, err := strconv.ParseInt(turnStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "turn must be a number")
		return
	}

	report, err := h.queries.GetContextReportByTurn(r.Context(), appdb.GetContextReportByTurnParams{
		ConversationID: convID,
		TurnNumber:     turn,
	})
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "context report not found")
			return
		}
		h.logger.Error("get context report", "error", err, "id", convID, "turn", turn)
		writeError(w, http.StatusInternalServerError, "failed to get context report")
		return
	}

	resp := contextReportResponse{
		ConversationID: report.ConversationID,
		TurnNumber:     report.TurnNumber,
		CreatedAt:      report.CreatedAt,
	}

	// Scalars — convert sql.Null* to pointer.
	resp.AnalysisLatencyMs = nullInt64Ptr(report.AnalysisLatencyMs)
	resp.RetrievalLatencyMs = nullInt64Ptr(report.RetrievalLatencyMs)
	resp.TotalLatencyMs = nullInt64Ptr(report.TotalLatencyMs)
	resp.BudgetTotal = nullInt64Ptr(report.BudgetTotal)
	resp.BudgetUsed = nullInt64Ptr(report.BudgetUsed)
	resp.IncludedCount = nullInt64Ptr(report.IncludedCount)
	resp.ExcludedCount = nullInt64Ptr(report.ExcludedCount)
	resp.AgentUsedSearch = nullInt64Ptr(report.AgentUsedSearchTool)
	resp.ContextHitRate = nullFloat64Ptr(report.ContextHitRate)

	// JSON columns — pass through as raw JSON.
	resp.Needs = nullStringToJSON(report.NeedsJson)
	resp.Signals = nullStringToJSON(report.SignalsJson)
	resp.RAGResults = nullStringToJSON(report.RagResultsJson)
	resp.BrainResults = nullStringToJSON(report.BrainResultsJson)
	resp.GraphResults = nullStringToJSON(report.GraphResultsJson)
	resp.ExplicitFiles = nullStringToJSON(report.ExplicitFilesJson)
	resp.BudgetBreakdown = nullStringToJSON(report.BudgetBreakdownJson)
	resp.AgentReadFiles = nullStringToJSON(report.AgentReadFilesJson)

	writeJSON(w, http.StatusOK, resp)
}

func (h *MetricsHandler) handleContextSignalStream(w http.ResponseWriter, r *http.Request) {
	convID := r.PathValue("id")
	turnStr := r.PathValue("turn")

	turn, err := strconv.ParseInt(turnStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "turn must be a number")
		return
	}

	row, err := h.queries.GetContextReportByTurn(r.Context(), appdb.GetContextReportByTurnParams{
		ConversationID: convID,
		TurnNumber:     turn,
	})
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "context report not found")
			return
		}
		h.logger.Error("get context signal stream", "error", err, "id", convID, "turn", turn)
		writeError(w, http.StatusInternalServerError, "failed to get context signal stream")
		return
	}

	needs, err := decodeContextNeeds(row.NeedsJson, row.SignalsJson)
	if err != nil {
		h.logger.Error("decode context signal stream", "error", err, "id", convID, "turn", turn)
		writeError(w, http.StatusInternalServerError, "failed to decode context signal stream")
		return
	}

	writeJSON(w, http.StatusOK, contextSignalStreamResponse{
		ConversationID: row.ConversationID,
		TurnNumber:     row.TurnNumber,
		Stream:         buildContextSignalStream(needs),
	})
}

func decodeContextNeeds(needsJSON sql.NullString, signalsJSON sql.NullString) (contextpkg.ContextNeeds, error) {
	var needs contextpkg.ContextNeeds
	if needsJSON.Valid && strings.TrimSpace(needsJSON.String) != "" {
		if err := json.Unmarshal([]byte(needsJSON.String), &needs); err != nil {
			return contextpkg.ContextNeeds{}, err
		}
	}
	if signalsJSON.Valid && strings.TrimSpace(signalsJSON.String) != "" {
		var signals []contextpkg.Signal
		if err := json.Unmarshal([]byte(signalsJSON.String), &signals); err != nil {
			return contextpkg.ContextNeeds{}, err
		}
		needs.Signals = append([]contextpkg.Signal(nil), signals...)
	}
	return needs, nil
}

func buildContextSignalStream(needs contextpkg.ContextNeeds) []contextSignalStreamEntry {
	stream := make([]contextSignalStreamEntry, 0, len(needs.Signals)+len(needs.SemanticQueries)+len(needs.ExplicitFiles)+len(needs.ExplicitSymbols)+len(needs.MomentumFiles)+4)
	appendEntry := func(kind string, typ string, source string, value string) {
		stream = append(stream, contextSignalStreamEntry{
			Index:  len(stream),
			Kind:   kind,
			Type:   typ,
			Source: source,
			Value:  value,
		})
	}
	for _, signal := range needs.Signals {
		appendEntry("signal", signal.Type, signal.Source, signal.Value)
	}
	for _, query := range needs.SemanticQueries {
		appendEntry("semantic_query", "", "", query)
	}
	for _, path := range needs.ExplicitFiles {
		appendEntry("explicit_file", "", "", path)
	}
	for _, symbol := range needs.ExplicitSymbols {
		appendEntry("explicit_symbol", "", "", symbol)
	}
	for _, path := range needs.MomentumFiles {
		appendEntry("momentum_file", "", "", path)
	}
	if needs.MomentumModule != "" {
		appendEntry("momentum_module", "", "", needs.MomentumModule)
	}
	if needs.PreferBrainContext {
		appendEntry("flag", "prefer_brain_context", "", "true")
	}
	if needs.IncludeConventions {
		appendEntry("flag", "include_conventions", "", "true")
	}
	if needs.IncludeGitContext {
		value := "true"
		if needs.GitContextDepth > 0 {
			value = "depth=" + strconv.Itoa(needs.GitContextDepth)
		}
		appendEntry("flag", "include_git_context", "", value)
	}
	return stream
}

// ── Null helpers ─────────────────────────────────────────────────────

func nullFloat(n sql.NullFloat64) float64 {
	if n.Valid {
		return n.Float64
	}
	return 0
}

func nullInt64Ptr(n sql.NullInt64) *int64 {
	if n.Valid {
		return &n.Int64
	}
	return nil
}

func nullFloat64Ptr(n sql.NullFloat64) *float64 {
	if n.Valid {
		return &n.Float64
	}
	return nil
}

func nullStringToJSON(n sql.NullString) json.RawMessage {
	if !n.Valid || n.String == "" {
		return nil
	}
	// Validate it's actual JSON before passing through.
	if json.Valid([]byte(n.String)) {
		return json.RawMessage(n.String)
	}
	return nil
}
