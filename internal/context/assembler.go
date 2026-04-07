package context

import (
	stdctx "context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ponchione/sirtopham/internal/config"
	"github.com/ponchione/sirtopham/internal/db"
)

// ContextAssembler orchestrates the full Layer 3 turn-start assembly flow.
type ContextAssembler struct {
	analyzer    TurnAnalyzer
	extractor   QueryExtractor
	momentum    MomentumTracker
	retriever   Retriever
	budgeter    BudgetManager
	serializer  Serializer
	reportStore contextReportStore
	cfg         config.ContextConfig
	now         func() time.Time
}

// NewContextAssembler constructs the Slice 7 capstone orchestrator.
func NewContextAssembler(
	analyzer TurnAnalyzer,
	extractor QueryExtractor,
	momentum MomentumTracker,
	retriever Retriever,
	budgeter BudgetManager,
	serializer Serializer,
	cfg config.ContextConfig,
	database *sql.DB,
) *ContextAssembler {
	return &ContextAssembler{
		analyzer:    analyzer,
		extractor:   extractor,
		momentum:    momentum,
		retriever:   retriever,
		budgeter:    budgeter,
		serializer:  serializer,
		reportStore: NewSQLiteReportStore(database),
		cfg:         cfg,
		now:         time.Now,
	}
}

// Assemble runs the Layer 3 pipeline once at turn start.
func (a *ContextAssembler) Assemble(
	ctx stdctx.Context,
	message string,
	history []db.Message,
	scope AssemblyScope,
	modelContextLimit int,
	historyTokenCount int,
) (*FullContextPackage, bool, error) {
	if ctx == nil {
		ctx = stdctx.Background()
	}
	if err := a.validate(); err != nil {
		return nil, false, err
	}

	startedAt := a.now()
	analysisStart := time.Now()
	needs := a.analyzer.AnalyzeTurn(message, history)
	if needs == nil {
		needs = &ContextNeeds{}
	}
	analysisLatency := time.Since(analysisStart).Milliseconds()

	if a.momentum != nil {
		a.momentum.Apply(history, needs, a.cfg)
	}

	queries := []string{}
	if a.extractor != nil {
		queries = a.extractor.ExtractQueries(message, needs)
	}
	applySemanticQueries(needs, queries)

	retrievalStart := time.Now()
	results, err := a.retriever.Retrieve(ctx, needs, queries, a.cfg)
	if err != nil {
		return nil, false, fmt.Errorf("context assembler: retrieve context: %w", err)
	}
	if results == nil {
		results = &RetrievalResults{}
	}
	retrievalLatency := time.Since(retrievalStart).Milliseconds()

	resolvedHistoryTokens := historyTokenCount
	if resolvedHistoryTokens <= 0 {
		resolvedHistoryTokens = estimateHistoryTokenCount(history)
	}

	budget, err := a.budgeter.Fit(results, modelContextLimit, resolvedHistoryTokens, a.cfg)
	if err != nil {
		return nil, false, fmt.Errorf("context assembler: fit budget: %w", err)
	}
	if budget == nil {
		budget = &BudgetResult{}
	}

	content, err := a.serializer.Serialize(budget, scope.SeenFiles)
	if err != nil {
		return nil, false, fmt.Errorf("context assembler: serialize context: %w", err)
	}

	totalLatency := a.now().Sub(startedAt).Milliseconds()
	report := buildContextAssemblyReport(scope.TurnNumber, needs, results, budget, analysisLatency, retrievalLatency, totalLatency)
	pkg := &FullContextPackage{
		Content:    content,
		TokenCount: approximateTokenCount(content),
		Report:     report,
		Frozen:     true,
	}

	if a.cfg.StoreAssemblyReports {
		if strings.TrimSpace(scope.ConversationID) == "" {
			return nil, false, fmt.Errorf("context assembler: conversation ID is required when report persistence is enabled")
		}
		if scope.TurnNumber <= 0 {
			return nil, false, fmt.Errorf("context assembler: turn number must be positive when report persistence is enabled")
		}
		if a.reportStore == nil {
			return nil, false, fmt.Errorf("context assembler: report store is unavailable")
		}
		if err := a.reportStore.Insert(ctx, scope.ConversationID, report); err != nil {
			return nil, false, fmt.Errorf("context assembler: persist report: %w", err)
		}
	}

	slog.Info(
		"context assembly complete",
		"conversation_id", scope.ConversationID,
		"turn_number", scope.TurnNumber,
		"total_latency_ms", report.TotalLatencyMs,
		"analysis_latency_ms", report.AnalysisLatencyMs,
		"retrieval_latency_ms", report.RetrievalLatencyMs,
		"budget_total", report.BudgetTotal,
		"budget_used", report.BudgetUsed,
		"included", len(report.IncludedChunks),
		"excluded", len(report.ExcludedChunks),
		"compression_needed", budget.CompressionNeeded,
	)

	return pkg, budget.CompressionNeeded, nil
}

// UpdateQuality fills the post-turn quality metrics for a persisted context report.
func (a *ContextAssembler) UpdateQuality(ctx stdctx.Context, conversationID string, turnNumber int, usedSearchTool bool, readFiles []string) error {
	if ctx == nil {
		ctx = stdctx.Background()
	}
	if !a.cfg.StoreAssemblyReports {
		return nil
	}
	if a.reportStore == nil {
		return fmt.Errorf("context assembler: report store is unavailable")
	}
	report, err := a.reportStore.Get(ctx, conversationID, turnNumber)
	if err != nil {
		return fmt.Errorf("context assembler: load report for quality update: %w", err)
	}
	readFiles = normalizeUniquePaths(readFiles)
	hitRate := computeContextHitRate(report, readFiles)
	if err := a.reportStore.UpdateQuality(ctx, conversationID, turnNumber, usedSearchTool, readFiles, hitRate); err != nil {
		return fmt.Errorf("context assembler: persist quality update: %w", err)
	}
	return nil
}

func (a *ContextAssembler) validate() error {
	if a == nil {
		return fmt.Errorf("context assembler: assembler is nil")
	}
	if a.analyzer == nil {
		return fmt.Errorf("context assembler: analyzer is nil")
	}
	if a.retriever == nil {
		return fmt.Errorf("context assembler: retriever is nil")
	}
	if a.budgeter == nil {
		return fmt.Errorf("context assembler: budget manager is nil")
	}
	if a.serializer == nil {
		return fmt.Errorf("context assembler: serializer is nil")
	}
	return nil
}

func buildContextAssemblyReport(
	turnNumber int,
	needs *ContextNeeds,
	results *RetrievalResults,
	budget *BudgetResult,
	analysisLatencyMs int64,
	retrievalLatencyMs int64,
	totalLatencyMs int64,
) *ContextAssemblyReport {
	if needs == nil {
		needs = &ContextNeeds{}
	}
	if results == nil {
		results = &RetrievalResults{}
	}
	if budget == nil {
		budget = &BudgetResult{}
	}

	report := &ContextAssemblyReport{
		TurnNumber:          turnNumber,
		AnalysisLatencyMs:   analysisLatencyMs,
		RetrievalLatencyMs:  retrievalLatencyMs,
		TotalLatencyMs:      totalLatencyMs,
		Needs:               cloneContextNeeds(*needs),
		RAGResults:          annotateRAGResults(results.RAGHits, budget),
		BrainResults:        annotateBrainResults(results.BrainHits, budget),
		ExplicitFileResults: annotateFileResults(results.FileResults, budget),
		GraphResults:        annotateGraphResults(results.GraphHits, budget),
		IncludedChunks:      append([]string(nil), budget.IncludedChunks...),
		ExcludedChunks:      append([]string(nil), budget.ExcludedChunks...),
		ExclusionReasons:    cloneStringMap(budget.ExclusionReasons),
		BudgetTotal:         budget.BudgetTotal,
		BudgetUsed:          budget.BudgetUsed,
		BudgetBreakdown:     cloneIntMap(budget.BudgetBreakdown),
		AgentReadFiles:      []string{},
	}
	if report.BudgetBreakdown == nil {
		report.BudgetBreakdown = map[string]int{}
	}
	if report.ExclusionReasons == nil {
		report.ExclusionReasons = map[string]string{}
	}
	return report
}

func annotateRAGResults(hits []RAGHit, budget *BudgetResult) []RAGHit {
	annotated := append([]RAGHit(nil), hits...)
	included := sliceToSet(budget.IncludedChunks)
	reasons := budget.ExclusionReasons
	for i := range annotated {
		key := ragChunkKey(annotated[i])
		if _, ok := included[key]; ok {
			annotated[i].Included = true
			annotated[i].ExclusionReason = ""
			continue
		}
		if reason, ok := reasons[key]; ok {
			annotated[i].Included = false
			annotated[i].ExclusionReason = reason
		}
	}
	return annotated
}

func annotateBrainResults(hits []BrainHit, budget *BudgetResult) []BrainHit {
	annotated := append([]BrainHit(nil), hits...)
	selected := make(map[string]struct{}, len(budget.SelectedBrainHits))
	for _, hit := range budget.SelectedBrainHits {
		selected[hit.DocumentPath] = struct{}{}
	}
	for i := range annotated {
		if _, ok := selected[annotated[i].DocumentPath]; ok {
			annotated[i].Included = true
			annotated[i].ExclusionReason = ""
			continue
		}
		if reason, ok := budget.ExclusionReasons[annotated[i].DocumentPath]; ok {
			annotated[i].ExclusionReason = reason
		}
	}
	return annotated
}

func annotateFileResults(files []FileResult, budget *BudgetResult) []FileResult {
	annotated := append([]FileResult(nil), files...)
	included := sliceToSet(budget.IncludedChunks)
	reasons := budget.ExclusionReasons
	for i := range annotated {
		key := annotated[i].FilePath
		if _, ok := included[key]; ok {
			annotated[i].Included = true
			annotated[i].ExclusionReason = ""
			continue
		}
		if reason, ok := reasons[key]; ok {
			annotated[i].ExclusionReason = reason
		}
	}
	return annotated
}

func annotateGraphResults(hits []GraphHit, budget *BudgetResult) []GraphHit {
	annotated := append([]GraphHit(nil), hits...)
	included := sliceToSet(budget.IncludedChunks)
	reasons := budget.ExclusionReasons
	for i := range annotated {
		key := graphChunkKey(annotated[i])
		if _, ok := included[key]; ok {
			annotated[i].Included = true
			annotated[i].ExclusionReason = ""
			continue
		}
		if reason, ok := reasons[key]; ok {
			annotated[i].ExclusionReason = reason
		}
	}
	return annotated
}

func computeContextHitRate(report *ContextAssemblyReport, readFiles []string) float64 {
	readFiles = normalizeUniquePaths(readFiles)
	if len(readFiles) == 0 {
		return 1.0
	}
	includedFiles := includedFilePathSet(report)
	if len(includedFiles) == 0 {
		return 0
	}
	hits := 0
	for _, path := range readFiles {
		if _, ok := includedFiles[path]; ok {
			hits++
		}
	}
	return float64(hits) / float64(len(readFiles))
}

func includedFilePathSet(report *ContextAssemblyReport) map[string]struct{} {
	included := make(map[string]struct{})
	if report == nil {
		return included
	}
	for _, hit := range report.RAGResults {
		if hit.Included && strings.TrimSpace(hit.FilePath) != "" {
			included[hit.FilePath] = struct{}{}
		}
	}
	for _, file := range report.ExplicitFileResults {
		if file.Included && strings.TrimSpace(file.FilePath) != "" {
			included[file.FilePath] = struct{}{}
		}
	}
	for _, hit := range report.GraphResults {
		if hit.Included && strings.TrimSpace(hit.FilePath) != "" {
			included[hit.FilePath] = struct{}{}
		}
	}
	return included
}

func estimateHistoryTokenCount(history []db.Message) int {
	total := 0
	for _, msg := range history {
		if msg.Content.Valid {
			total += approximateTokenCount(msg.Content.String)
		}
		if msg.ToolUseID.Valid {
			total += approximateTokenCount(msg.ToolUseID.String)
		}
		if msg.ToolName.Valid {
			total += approximateTokenCount(msg.ToolName.String)
		}
	}
	return total
}

func cloneContextNeeds(needs ContextNeeds) ContextNeeds {
	return ContextNeeds{
		SemanticQueries:    append([]string(nil), needs.SemanticQueries...),
		ExplicitFiles:      append([]string(nil), needs.ExplicitFiles...),
		ExplicitSymbols:    append([]string(nil), needs.ExplicitSymbols...),
		IncludeConventions: needs.IncludeConventions,
		IncludeGitContext:  needs.IncludeGitContext,
		GitContextDepth:    needs.GitContextDepth,
		MomentumFiles:      append([]string(nil), needs.MomentumFiles...),
		MomentumModule:     needs.MomentumModule,
		PreferBrainContext: needs.PreferBrainContext,
		Signals:            append([]Signal(nil), needs.Signals...),
	}
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func cloneIntMap(input map[string]int) map[string]int {
	if len(input) == 0 {
		return map[string]int{}
	}
	out := make(map[string]int, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func sliceToSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		set[value] = struct{}{}
	}
	return set
}

func applySemanticQueries(needs *ContextNeeds, queries []string) {
	if needs == nil {
		return
	}
	needs.SemanticQueries = nil
	for _, query := range queries {
		appendUniqueQuery(&needs.SemanticQueries, query)
	}
}
