package context

import (
	stdctx "context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	dbpkg "github.com/ponchione/sirtopham/internal/db"
)

type contextReportStore interface {
	Insert(ctx stdctx.Context, conversationID string, report *ContextAssemblyReport) error
	Get(ctx stdctx.Context, conversationID string, turnNumber int) (*ContextAssemblyReport, error)
	UpdateQuality(ctx stdctx.Context, conversationID string, turnNumber int, usedSearchTool bool, readFiles []string, hitRate float64) error
}

// SQLiteReportStore persists context assembly reports in the context_reports table.
type SQLiteReportStore struct {
	queries *dbpkg.Queries
	now     func() time.Time
}

// NewSQLiteReportStore constructs a SQLite-backed report store.
func NewSQLiteReportStore(database *sql.DB) *SQLiteReportStore {
	if database == nil {
		return nil
	}
	return &SQLiteReportStore{
		queries: dbpkg.New(database),
		now:     time.Now,
	}
}

func (s *SQLiteReportStore) Insert(ctx stdctx.Context, conversationID string, report *ContextAssemblyReport) error {
	if ctx == nil {
		ctx = stdctx.Background()
	}
	if s == nil || s.queries == nil {
		return fmt.Errorf("insert context report: store is nil")
	}
	if report == nil {
		return fmt.Errorf("insert context report: report is nil")
	}

	needsJSON, err := marshalJSON(report.Needs)
	if err != nil {
		return fmt.Errorf("insert context report: marshal needs: %w", err)
	}
	signalsJSON, err := marshalJSON(report.Needs.Signals)
	if err != nil {
		return fmt.Errorf("insert context report: marshal signals: %w", err)
	}
	ragJSON, err := marshalJSON(report.RAGResults)
	if err != nil {
		return fmt.Errorf("insert context report: marshal rag results: %w", err)
	}
	brainJSON, err := marshalJSON(report.BrainResults)
	if err != nil {
		return fmt.Errorf("insert context report: marshal brain results: %w", err)
	}
	graphJSON, err := marshalJSON(report.GraphResults)
	if err != nil {
		return fmt.Errorf("insert context report: marshal graph results: %w", err)
	}
	explicitFilesJSON, err := marshalJSON(report.ExplicitFileResults)
	if err != nil {
		return fmt.Errorf("insert context report: marshal explicit file results: %w", err)
	}
	budgetBreakdownJSON, err := marshalJSON(report.BudgetBreakdown)
	if err != nil {
		return fmt.Errorf("insert context report: marshal budget breakdown: %w", err)
	}
	readFilesJSON, err := marshalJSON([]string{})
	if err != nil {
		return fmt.Errorf("insert context report: marshal default read files: %w", err)
	}

	createdAt := s.now().UTC().Format(time.RFC3339)
	return s.queries.InsertContextReport(ctx, dbpkg.InsertContextReportParams{
		ConversationID:      conversationID,
		TurnNumber:          int64(report.TurnNumber),
		AnalysisLatencyMs:   sql.NullInt64{Int64: report.AnalysisLatencyMs, Valid: true},
		RetrievalLatencyMs:  sql.NullInt64{Int64: report.RetrievalLatencyMs, Valid: true},
		TotalLatencyMs:      sql.NullInt64{Int64: report.TotalLatencyMs, Valid: true},
		NeedsJson:           sql.NullString{String: needsJSON, Valid: true},
		SignalsJson:         sql.NullString{String: signalsJSON, Valid: true},
		RagResultsJson:      sql.NullString{String: ragJSON, Valid: true},
		BrainResultsJson:    sql.NullString{String: brainJSON, Valid: true},
		GraphResultsJson:    sql.NullString{String: graphJSON, Valid: true},
		ExplicitFilesJson:   sql.NullString{String: explicitFilesJSON, Valid: true},
		BudgetTotal:         sql.NullInt64{Int64: int64(report.BudgetTotal), Valid: true},
		BudgetUsed:          sql.NullInt64{Int64: int64(report.BudgetUsed), Valid: true},
		BudgetBreakdownJson: sql.NullString{String: budgetBreakdownJSON, Valid: true},
		IncludedCount:       sql.NullInt64{Int64: int64(len(report.IncludedChunks)), Valid: true},
		ExcludedCount:       sql.NullInt64{Int64: int64(len(report.ExcludedChunks)), Valid: true},
		AgentUsedSearchTool: sql.NullInt64{Int64: 0, Valid: true},
		AgentReadFilesJson:  sql.NullString{String: readFilesJSON, Valid: true},
		ContextHitRate:      sql.NullFloat64{Float64: 0, Valid: true},
		CreatedAt:           createdAt,
	})
}

func (s *SQLiteReportStore) Get(ctx stdctx.Context, conversationID string, turnNumber int) (*ContextAssemblyReport, error) {
	if ctx == nil {
		ctx = stdctx.Background()
	}
	if s == nil || s.queries == nil {
		return nil, fmt.Errorf("get context report: store is nil")
	}
	row, err := s.queries.GetContextReportByTurn(ctx, dbpkg.GetContextReportByTurnParams{
		ConversationID: conversationID,
		TurnNumber:     int64(turnNumber),
	})
	if err != nil {
		return nil, fmt.Errorf("get context report: %w", err)
	}
	return decodeContextReportRow(row)
}

func (s *SQLiteReportStore) UpdateQuality(ctx stdctx.Context, conversationID string, turnNumber int, usedSearchTool bool, readFiles []string, hitRate float64) error {
	if ctx == nil {
		ctx = stdctx.Background()
	}
	if s == nil || s.queries == nil {
		return fmt.Errorf("update context report quality: store is nil")
	}
	readFiles = normalizeUniquePaths(readFiles)
	readFilesJSON, err := marshalJSON(readFiles)
	if err != nil {
		return fmt.Errorf("update context report quality: marshal read files: %w", err)
	}
	usedSearch := int64(0)
	if usedSearchTool {
		usedSearch = 1
	}
	return s.queries.UpdateContextReportQuality(ctx, dbpkg.UpdateContextReportQualityParams{
		AgentUsedSearchTool: sql.NullInt64{Int64: usedSearch, Valid: true},
		AgentReadFilesJson:  sql.NullString{String: readFilesJSON, Valid: true},
		ContextHitRate:      sql.NullFloat64{Float64: hitRate, Valid: true},
		ConversationID:      conversationID,
		TurnNumber:          int64(turnNumber),
	})
}

func decodeContextReportRow(row dbpkg.ContextReport) (*ContextAssemblyReport, error) {
	report := &ContextAssemblyReport{
		TurnNumber: int(row.TurnNumber),
	}
	if row.AnalysisLatencyMs.Valid {
		report.AnalysisLatencyMs = row.AnalysisLatencyMs.Int64
	}
	if row.RetrievalLatencyMs.Valid {
		report.RetrievalLatencyMs = row.RetrievalLatencyMs.Int64
	}
	if row.TotalLatencyMs.Valid {
		report.TotalLatencyMs = row.TotalLatencyMs.Int64
	}
	if row.BudgetTotal.Valid {
		report.BudgetTotal = int(row.BudgetTotal.Int64)
	}
	if row.BudgetUsed.Valid {
		report.BudgetUsed = int(row.BudgetUsed.Int64)
	}
	if row.AgentUsedSearchTool.Valid {
		report.AgentUsedSearchTool = row.AgentUsedSearchTool.Int64 != 0
	}
	if row.ContextHitRate.Valid {
		report.ContextHitRate = row.ContextHitRate.Float64
	}
	if row.NeedsJson.Valid {
		if err := unmarshalJSONString(row.NeedsJson.String, &report.Needs); err != nil {
			return nil, fmt.Errorf("decode needs_json: %w", err)
		}
	}
	if row.SignalsJson.Valid {
		if err := unmarshalJSONString(row.SignalsJson.String, &report.Needs.Signals); err != nil {
			return nil, fmt.Errorf("decode signals_json: %w", err)
		}
	}
	if row.RagResultsJson.Valid {
		if err := unmarshalJSONString(row.RagResultsJson.String, &report.RAGResults); err != nil {
			return nil, fmt.Errorf("decode rag_results_json: %w", err)
		}
	}
	if row.BrainResultsJson.Valid {
		if err := unmarshalJSONString(row.BrainResultsJson.String, &report.BrainResults); err != nil {
			return nil, fmt.Errorf("decode brain_results_json: %w", err)
		}
	}
	if row.GraphResultsJson.Valid {
		if err := unmarshalJSONString(row.GraphResultsJson.String, &report.GraphResults); err != nil {
			return nil, fmt.Errorf("decode graph_results_json: %w", err)
		}
	}
	if row.ExplicitFilesJson.Valid {
		if err := unmarshalJSONString(row.ExplicitFilesJson.String, &report.ExplicitFileResults); err != nil {
			return nil, fmt.Errorf("decode explicit_files_json: %w", err)
		}
	}
	if row.BudgetBreakdownJson.Valid {
		if err := unmarshalJSONString(row.BudgetBreakdownJson.String, &report.BudgetBreakdown); err != nil {
			return nil, fmt.Errorf("decode budget_breakdown_json: %w", err)
		}
	}
	if row.AgentReadFilesJson.Valid {
		if err := unmarshalJSONString(row.AgentReadFilesJson.String, &report.AgentReadFiles); err != nil {
			return nil, fmt.Errorf("decode agent_read_files_json: %w", err)
		}
	}
	report.IncludedChunks = collectIncludedChunkKeys(report)
	report.ExcludedChunks, report.ExclusionReasons = collectExcludedChunkKeys(report)
	if report.BudgetBreakdown == nil {
		report.BudgetBreakdown = map[string]int{}
	}
	return report, nil
}

func marshalJSON(value any) (string, error) {
	bytes, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func unmarshalJSONString(raw string, target any) error {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil
	}
	return json.Unmarshal([]byte(raw), target)
}

func collectIncludedChunkKeys(report *ContextAssemblyReport) []string {
	keys := make([]string, 0)
	seen := make(map[string]struct{})
	for _, hit := range report.RAGResults {
		if !hit.Included {
			continue
		}
		key := ragChunkKey(hit)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	for _, hit := range report.GraphResults {
		if !hit.Included {
			continue
		}
		key := graphChunkKey(hit)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	for _, file := range report.ExplicitFileResults {
		if !file.Included {
			continue
		}
		key := strings.TrimSpace(file.FilePath)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func collectExcludedChunkKeys(report *ContextAssemblyReport) ([]string, map[string]string) {
	keys := make([]string, 0)
	reasons := make(map[string]string)
	appendKey := func(key string, reason string) {
		key = strings.TrimSpace(key)
		if key == "" || strings.TrimSpace(reason) == "" {
			return
		}
		if _, exists := reasons[key]; !exists {
			keys = append(keys, key)
		}
		reasons[key] = reason
	}
	for _, hit := range report.RAGResults {
		appendKey(ragChunkKey(hit), hit.ExclusionReason)
	}
	for _, hit := range report.GraphResults {
		appendKey(graphChunkKey(hit), hit.ExclusionReason)
	}
	for _, file := range report.ExplicitFileResults {
		appendKey(file.FilePath, file.ExclusionReason)
	}
	sort.Strings(keys)
	return keys, reasons
}

func normalizeUniquePaths(paths []string) []string {
	if len(paths) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(paths))
	normalized := make([]string, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		normalized = append(normalized, path)
	}
	sort.Strings(normalized)
	return normalized
}
