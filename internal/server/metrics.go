package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	contextpkg "github.com/ponchione/sodoryard/internal/context"
	appdb "github.com/ponchione/sodoryard/internal/db"
	"github.com/ponchione/sodoryard/internal/projectmemory"
)

// MetricsHandler serves per-conversation metrics and context report endpoints.
type MetricsHandler struct {
	queries        *appdb.Queries
	contextReports metricsContextReportReader
	turnSubCalls   metricsTurnSubCallReader
	logger         *slog.Logger
}

type metricsContextReportReader interface {
	ReadContextReport(ctx context.Context, conversationID string, turnNumber uint32) (projectmemory.ContextReport, bool, error)
}

type metricsTurnSubCallReader interface {
	ListTurnSubCalls(ctx context.Context, conversationID string, turnNumber uint32) ([]projectmemory.SubCall, error)
}

// NewMetricsHandler creates a handler and registers routes on the server.
func NewMetricsHandler(s *Server, queries *appdb.Queries, logger *slog.Logger, memoryBackends ...any) *MetricsHandler {
	h := &MetricsHandler{queries: queries, logger: logger}
	for _, backend := range memoryBackends {
		if h.contextReports == nil {
			if store, ok := backend.(metricsContextReportReader); ok && store != nil {
				h.contextReports = store
			}
		}
		if h.turnSubCalls == nil {
			if store, ok := backend.(metricsTurnSubCallReader); ok && store != nil {
				h.turnSubCalls = store
			}
		}
	}

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
	TokensIn        int64 `json:"tokens_in"`
	TokensOut       int64 `json:"tokens_out"`
	CacheReadTokens int64 `json:"cache_read_tokens"`
	TotalCalls      int64 `json:"total_calls"`
	TotalLatencyMs  int64 `json:"total_latency_ms"`
}

type toolUsageView struct {
	ToolName      string  `json:"tool_name"`
	CallCount     int64   `json:"call_count"`
	AvgDurationMs float64 `json:"avg_duration_ms"`
	FailureCount  int64   `json:"failure_count"`
}

type contextQualityView struct {
	TotalTurns          int64   `json:"total_turns"`
	ReactiveSearchCount int64   `json:"reactive_search_count"`
	AvgHitRate          float64 `json:"avg_hit_rate"`
	AvgBudgetUsedPct    float64 `json:"avg_budget_used_pct"`
}

func (h *MetricsHandler) handleConversationMetrics(w http.ResponseWriter, r *http.Request) {
	if !h.requireQueries(w) {
		return
	}
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
	Needs           json.RawMessage               `json:"needs,omitempty"`
	Signals         json.RawMessage               `json:"signals,omitempty"`
	RAGResults      json.RawMessage               `json:"rag_results,omitempty"`
	BrainResults    json.RawMessage               `json:"brain_results,omitempty"`
	GraphResults    json.RawMessage               `json:"graph_results,omitempty"`
	ExplicitFiles   json.RawMessage               `json:"explicit_files,omitempty"`
	BudgetBreakdown json.RawMessage               `json:"budget_breakdown,omitempty"`
	AgentReadFiles  json.RawMessage               `json:"agent_read_files,omitempty"`
	TokenBudget     *contextpkg.TokenBudgetReport `json:"token_budget,omitempty"`

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

type metricsContextReport struct {
	ConversationID      string
	TurnNumber          int64
	AnalysisLatencyMs   *int64
	RetrievalLatencyMs  *int64
	TotalLatencyMs      *int64
	NeedsJSON           string
	SignalsJSON         string
	RAGResultsJSON      string
	BrainResultsJSON    string
	GraphResultsJSON    string
	ExplicitFilesJSON   string
	BudgetTotal         *int64
	BudgetUsed          *int64
	BudgetBreakdownJSON string
	TokenBudgetJSON     string
	IncludedCount       *int64
	ExcludedCount       *int64
	AgentUsedSearch     *int64
	AgentReadFilesJSON  string
	ContextHitRate      *float64
	CreatedAt           string
}

func (h *MetricsHandler) handleContextReport(w http.ResponseWriter, r *http.Request) {
	report, ok := h.contextReportForRequest(w, r, "get context report", "failed to get context report")
	if !ok {
		return
	}

	resp := contextReportResponse{
		ConversationID: report.ConversationID,
		TurnNumber:     report.TurnNumber,
		CreatedAt:      report.CreatedAt,
	}

	// Scalars — convert sql.Null* to pointer.
	resp.AnalysisLatencyMs = report.AnalysisLatencyMs
	resp.RetrievalLatencyMs = report.RetrievalLatencyMs
	resp.TotalLatencyMs = report.TotalLatencyMs
	resp.BudgetTotal = report.BudgetTotal
	resp.BudgetUsed = report.BudgetUsed
	resp.IncludedCount = report.IncludedCount
	resp.ExcludedCount = report.ExcludedCount
	resp.AgentUsedSearch = report.AgentUsedSearch
	resp.ContextHitRate = report.ContextHitRate
	if tokenBudget, usageErr := h.buildTokenBudgetReport(r, report); usageErr == nil {
		resp.TokenBudget = tokenBudget
	} else {
		h.logger.Warn("get turn token budget", "error", usageErr, "id", report.ConversationID, "turn", report.TurnNumber)
	}

	// JSON columns — pass through as raw JSON.
	resp.Needs = stringToJSON(report.NeedsJSON)
	resp.Signals = stringToJSON(report.SignalsJSON)
	resp.RAGResults = stringToJSON(report.RAGResultsJSON)
	resp.BrainResults = stringToJSON(report.BrainResultsJSON)
	resp.GraphResults = stringToJSON(report.GraphResultsJSON)
	resp.ExplicitFiles = stringToJSON(report.ExplicitFilesJSON)
	resp.BudgetBreakdown = stringToJSON(report.BudgetBreakdownJSON)
	resp.AgentReadFiles = stringToJSON(report.AgentReadFilesJSON)

	writeJSON(w, http.StatusOK, resp)
}

func (h *MetricsHandler) handleContextSignalStream(w http.ResponseWriter, r *http.Request) {
	row, ok := h.contextReportForRequest(w, r, "get context signal stream", "failed to get context signal stream")
	if !ok {
		return
	}

	needs, err := decodeContextNeeds(row.NeedsJSON, row.SignalsJSON)
	if err != nil {
		h.logger.Error("decode context signal stream", "error", err, "id", row.ConversationID, "turn", row.TurnNumber)
		writeError(w, http.StatusInternalServerError, "failed to decode context signal stream")
		return
	}

	writeJSON(w, http.StatusOK, contextSignalStreamResponse{
		ConversationID: row.ConversationID,
		TurnNumber:     row.TurnNumber,
		Stream:         buildContextSignalStream(needs),
	})
}

func (h *MetricsHandler) contextReportForRequest(w http.ResponseWriter, r *http.Request, logMessage, clientError string) (metricsContextReport, bool) {
	convID := r.PathValue("id")
	turn, err := strconv.ParseInt(r.PathValue("turn"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "turn must be a number")
		return metricsContextReport{}, false
	}
	if turn <= 0 || turn > maxMetricsTurnNumber {
		writeError(w, http.StatusBadRequest, "turn must be a positive 32-bit number")
		return metricsContextReport{}, false
	}

	if h != nil && h.queries != nil {
		report, err := h.queries.GetContextReportByTurn(r.Context(), appdb.GetContextReportByTurnParams{
			ConversationID: convID,
			TurnNumber:     turn,
		})
		if err != nil {
			if err == sql.ErrNoRows {
				writeError(w, http.StatusNotFound, "context report not found")
				return metricsContextReport{}, false
			}
			h.logger.Error(logMessage, "error", err, "id", convID, "turn", turn)
			writeError(w, http.StatusInternalServerError, clientError)
			return metricsContextReport{}, false
		}
		return metricsContextReportFromSQLite(report), true
	}

	if h != nil && h.contextReports != nil {
		report, found, err := h.contextReports.ReadContextReport(r.Context(), convID, uint32(turn))
		if err != nil {
			h.logger.Error(logMessage, "error", err, "id", convID, "turn", turn)
			writeError(w, http.StatusInternalServerError, clientError)
			return metricsContextReport{}, false
		}
		if !found {
			writeError(w, http.StatusNotFound, "context report not found")
			return metricsContextReport{}, false
		}
		out, err := metricsContextReportFromProjectMemory(report)
		if err != nil {
			h.logger.Error("decode project memory context report", "error", err, "id", convID, "turn", turn)
			writeError(w, http.StatusInternalServerError, "failed to decode context report")
			return metricsContextReport{}, false
		}
		return out, true
	}

	writeError(w, http.StatusServiceUnavailable, "metrics are unavailable for this memory backend")
	return metricsContextReport{}, false
}

func (h *MetricsHandler) requireQueries(w http.ResponseWriter) bool {
	if h != nil && h.queries != nil {
		return true
	}
	writeError(w, http.StatusServiceUnavailable, "metrics are unavailable for this memory backend")
	return false
}

func (h *MetricsHandler) buildTokenBudgetReport(r *http.Request, report metricsContextReport) (*contextpkg.TokenBudgetReport, error) {
	budget := &contextpkg.TokenBudgetReport{}
	if strings.TrimSpace(report.TokenBudgetJSON) != "" {
		if err := json.Unmarshal([]byte(report.TokenBudgetJSON), budget); err != nil {
			return nil, fmt.Errorf("decode token budget: %w", err)
		}
	} else {
		budget.ReservedSystemPromptTokens = 3000
		budget.ReservedToolSchemaTokens = 3000
		budget.ReservedOutputTokens = 16000
		if report.BudgetUsed != nil {
			budget.EstimatedContextTokens = int(*report.BudgetUsed)
		}
		if report.BudgetTotal != nil {
			budget.EstimatedRequestTokens = int(int64PtrValue(report.BudgetUsed)) + budget.ReservedSystemPromptTokens + budget.ReservedToolSchemaTokens + budget.ReservedOutputTokens
		}
	}
	usage := turnTokenUsage{IterationCount: 1}
	if h != nil && h.queries != nil {
		row, err := h.queries.GetTurnTokenUsage(r.Context(), appdb.GetTurnTokenUsageParams{
			ConversationID: sql.NullString{String: report.ConversationID, Valid: true},
			TurnNumber:     sql.NullInt64{Int64: report.TurnNumber, Valid: true},
		})
		if err != nil {
			return budget, err
		}
		usage = turnTokenUsage{
			TokensIn:            row.TokensIn,
			TokensOut:           row.TokensOut,
			CacheReadTokens:     row.CacheReadTokens,
			CacheCreationTokens: row.CacheCreationTokens,
			LatencyMs:           row.LatencyMs,
			IterationCount:      row.IterationCount,
		}
	} else if h != nil && h.turnSubCalls != nil {
		subCalls, err := h.turnSubCalls.ListTurnSubCalls(r.Context(), report.ConversationID, uint32(report.TurnNumber))
		if err != nil {
			return budget, err
		}
		usage = aggregateProjectMemoryTurnUsage(subCalls)
	}
	budget.ActualInputTokens = usage.TokensIn
	budget.ActualOutputTokens = usage.TokensOut
	budget.ActualCacheReadTokens = usage.CacheReadTokens
	budget.ActualCacheCreationTokens = usage.CacheCreationTokens
	budget.ActualLatencyMs = usage.LatencyMs
	budget.IterationCount = usage.IterationCount
	budget.InputDeltaTokens = usage.TokensIn - int64(budget.EstimatedRequestTokens)
	budget.OutputHeadroomDeltaTokens = int64(budget.ReservedOutputTokens) - usage.TokensOut
	budget.OutputHeadroomExceeded = usage.TokensOut > int64(budget.ReservedOutputTokens)
	return budget, nil
}

func decodeContextNeeds(needsJSON string, signalsJSON string) (contextpkg.ContextNeeds, error) {
	var needs contextpkg.ContextNeeds
	if strings.TrimSpace(needsJSON) != "" {
		if err := json.Unmarshal([]byte(needsJSON), &needs); err != nil {
			return contextpkg.ContextNeeds{}, err
		}
	}
	if strings.TrimSpace(signalsJSON) != "" {
		var signals []contextpkg.Signal
		if err := json.Unmarshal([]byte(signalsJSON), &signals); err != nil {
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

func stringToJSON(raw string) json.RawMessage {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	// Validate it's actual JSON before passing through.
	if json.Valid([]byte(raw)) {
		return json.RawMessage(raw)
	}
	return nil
}

func metricsContextReportFromSQLite(row appdb.ContextReport) metricsContextReport {
	return metricsContextReport{
		ConversationID:      row.ConversationID,
		TurnNumber:          row.TurnNumber,
		AnalysisLatencyMs:   nullInt64Ptr(row.AnalysisLatencyMs),
		RetrievalLatencyMs:  nullInt64Ptr(row.RetrievalLatencyMs),
		TotalLatencyMs:      nullInt64Ptr(row.TotalLatencyMs),
		NeedsJSON:           nullStringValue(row.NeedsJson),
		SignalsJSON:         nullStringValue(row.SignalsJson),
		RAGResultsJSON:      nullStringValue(row.RagResultsJson),
		BrainResultsJSON:    nullStringValue(row.BrainResultsJson),
		GraphResultsJSON:    nullStringValue(row.GraphResultsJson),
		ExplicitFilesJSON:   nullStringValue(row.ExplicitFilesJson),
		BudgetTotal:         nullInt64Ptr(row.BudgetTotal),
		BudgetUsed:          nullInt64Ptr(row.BudgetUsed),
		BudgetBreakdownJSON: nullStringValue(row.BudgetBreakdownJson),
		TokenBudgetJSON:     nullStringValue(row.TokenBudgetJson),
		IncludedCount:       nullInt64Ptr(row.IncludedCount),
		ExcludedCount:       nullInt64Ptr(row.ExcludedCount),
		AgentUsedSearch:     nullInt64Ptr(row.AgentUsedSearchTool),
		AgentReadFilesJSON:  nullStringValue(row.AgentReadFilesJson),
		ContextHitRate:      nullFloat64Ptr(row.ContextHitRate),
		CreatedAt:           row.CreatedAt,
	}
}

type metricsContextReportQuality struct {
	AgentUsedSearchTool *bool    `json:"agent_used_search_tool"`
	AgentReadFiles      []string `json:"agent_read_files"`
	ContextHitRate      *float64 `json:"context_hit_rate"`
}

func metricsContextReportFromProjectMemory(row projectmemory.ContextReport) (metricsContextReport, error) {
	var report contextpkg.ContextAssemblyReport
	if strings.TrimSpace(row.ReportJSON) != "" {
		if err := json.Unmarshal([]byte(row.ReportJSON), &report); err != nil {
			return metricsContextReport{}, fmt.Errorf("decode report_json: %w", err)
		}
	}
	var quality metricsContextReportQuality
	if strings.TrimSpace(row.QualityJSON) != "" {
		if err := json.Unmarshal([]byte(row.QualityJSON), &quality); err != nil {
			return metricsContextReport{}, fmt.Errorf("decode quality_json: %w", err)
		}
	}
	if report.TurnNumber == 0 {
		report.TurnNumber = int(row.TurnNumber)
	}
	usedSearch := report.AgentUsedSearchTool
	if quality.AgentUsedSearchTool != nil {
		usedSearch = *quality.AgentUsedSearchTool
	}
	readFiles := report.AgentReadFiles
	if quality.AgentReadFiles != nil {
		readFiles = append([]string(nil), quality.AgentReadFiles...)
	}
	hitRate := report.ContextHitRate
	if quality.ContextHitRate != nil {
		hitRate = *quality.ContextHitRate
	}

	needsJSON, err := marshalMetricsJSON(report.Needs)
	if err != nil {
		return metricsContextReport{}, fmt.Errorf("marshal needs: %w", err)
	}
	signalsJSON, err := marshalMetricsJSON(report.Needs.Signals)
	if err != nil {
		return metricsContextReport{}, fmt.Errorf("marshal signals: %w", err)
	}
	ragJSON, err := marshalMetricsJSON(report.RAGResults)
	if err != nil {
		return metricsContextReport{}, fmt.Errorf("marshal rag results: %w", err)
	}
	brainJSON, err := marshalMetricsJSON(report.BrainResults)
	if err != nil {
		return metricsContextReport{}, fmt.Errorf("marshal brain results: %w", err)
	}
	graphJSON, err := marshalMetricsJSON(report.GraphResults)
	if err != nil {
		return metricsContextReport{}, fmt.Errorf("marshal graph results: %w", err)
	}
	explicitFilesJSON, err := marshalMetricsJSON(report.ExplicitFileResults)
	if err != nil {
		return metricsContextReport{}, fmt.Errorf("marshal explicit file results: %w", err)
	}
	budgetBreakdownJSON, err := marshalMetricsJSON(report.BudgetBreakdown)
	if err != nil {
		return metricsContextReport{}, fmt.Errorf("marshal budget breakdown: %w", err)
	}
	tokenBudgetJSON, err := marshalMetricsJSON(report.TokenBudget)
	if err != nil {
		return metricsContextReport{}, fmt.Errorf("marshal token budget: %w", err)
	}
	readFilesJSON, err := marshalMetricsJSON(readFiles)
	if err != nil {
		return metricsContextReport{}, fmt.Errorf("marshal agent read files: %w", err)
	}

	return metricsContextReport{
		ConversationID:      row.ConversationID,
		TurnNumber:          int64(row.TurnNumber),
		AnalysisLatencyMs:   int64Ptr(report.AnalysisLatencyMs),
		RetrievalLatencyMs:  int64Ptr(report.RetrievalLatencyMs),
		TotalLatencyMs:      int64Ptr(report.TotalLatencyMs),
		NeedsJSON:           needsJSON,
		SignalsJSON:         signalsJSON,
		RAGResultsJSON:      ragJSON,
		BrainResultsJSON:    brainJSON,
		GraphResultsJSON:    graphJSON,
		ExplicitFilesJSON:   explicitFilesJSON,
		BudgetTotal:         int64Ptr(int64(report.BudgetTotal)),
		BudgetUsed:          int64Ptr(int64(report.BudgetUsed)),
		BudgetBreakdownJSON: budgetBreakdownJSON,
		TokenBudgetJSON:     tokenBudgetJSON,
		IncludedCount:       int64Ptr(int64(len(report.IncludedChunks))),
		ExcludedCount:       int64Ptr(int64(len(report.ExcludedChunks))),
		AgentUsedSearch:     int64Ptr(boolAsInt64(usedSearch)),
		AgentReadFilesJSON:  readFilesJSON,
		ContextHitRate:      float64Ptr(hitRate),
		CreatedAt:           unixMicroString(row.CreatedAtUS),
	}, nil
}

type turnTokenUsage struct {
	TokensIn            int64
	TokensOut           int64
	CacheReadTokens     int64
	CacheCreationTokens int64
	LatencyMs           int64
	IterationCount      int64
}

func aggregateProjectMemoryTurnUsage(subCalls []projectmemory.SubCall) turnTokenUsage {
	usage := turnTokenUsage{IterationCount: 1}
	for _, subCall := range subCalls {
		if subCall.Purpose != "chat" {
			continue
		}
		usage.TokensIn += uint64ToInt64(subCall.TokensIn)
		usage.TokensOut += uint64ToInt64(subCall.TokensOut)
		usage.CacheReadTokens += uint64ToInt64(subCall.CacheReadTokens)
		usage.CacheCreationTokens += uint64ToInt64(subCall.CacheCreationTokens)
		usage.LatencyMs += uint64ToInt64(subCall.LatencyMs)
		if int64(subCall.Iteration) > usage.IterationCount {
			usage.IterationCount = int64(subCall.Iteration)
		}
	}
	return usage
}

func marshalMetricsJSON(value any) (string, error) {
	bytes, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func nullStringValue(n sql.NullString) string {
	if n.Valid {
		return n.String
	}
	return ""
}

func int64Ptr(value int64) *int64 {
	return &value
}

func float64Ptr(value float64) *float64 {
	return &value
}

func int64PtrValue(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func boolAsInt64(value bool) int64 {
	if value {
		return 1
	}
	return 0
}

func uint64ToInt64(value uint64) int64 {
	if value > maxMetricsInt64 {
		return int64(maxMetricsInt64)
	}
	return int64(value)
}

func unixMicroString(value uint64) string {
	if value > maxMetricsInt64 {
		value = maxMetricsInt64
	}
	return time.UnixMicro(int64(value)).UTC().Format(time.RFC3339)
}

const (
	maxMetricsTurnNumber = int64(^uint32(0))
	maxMetricsInt64      = uint64(1<<63 - 1)
)
