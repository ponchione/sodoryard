package context

import (
	"sort"
	"strings"

	"github.com/ponchione/sirtopham/internal/config"
)

const (
	defaultSystemPromptReserve = 3000
	defaultToolSchemaReserve   = 3000
	defaultResponseHeadroom    = 16000
	defaultMaxAssembledTokens  = 30000
	defaultTopRAGPriorityCount = 3
)

// PriorityBudgetManager implements the v0.1 context budget fitting rules.
//
// It computes available assembled-context budget, selects content in priority
// order, records exclusions, and signals when conversation history should be
// compressed by the caller.
type PriorityBudgetManager struct {
	brainCfg config.BrainConfig
}

func (m *PriorityBudgetManager) SetBrainConfig(cfg config.BrainConfig) {
	if m == nil {
		return
	}
	m.brainCfg = cfg
}

// Fit selects retrieval results that fit within the assembled-context budget.
func (m PriorityBudgetManager) Fit(results *RetrievalResults, modelContextLimit int, historyTokenCount int, cfg config.ContextConfig) (*BudgetResult, error) {
	if results == nil {
		results = &RetrievalResults{}
	}

	budgetTotal := computeAssembledBudget(modelContextLimit, historyTokenCount, cfg)
	compressionNeeded := shouldCompressHistory(modelContextLimit, historyTokenCount, cfg)
	budget := &BudgetResult{
		BudgetTotal:       budgetTotal,
		BudgetUsed:        0,
		BudgetBreakdown:   map[string]int{"explicit_files": 0, "brain": 0, "rag": 0, "structural": 0, "conventions": 0, "git": 0},
		IncludedChunks:    []string{},
		ExcludedChunks:    []string{},
		ExclusionReasons:  map[string]string{},
		CompressionNeeded: compressionNeeded,
		TokenBudget: TokenBudgetReport{
			ModelContextLimit:          modelContextLimit,
			HistoryTokens:              historyTokenCount,
			ReservedSystemPromptTokens: defaultSystemPromptReserve,
			ReservedToolSchemaTokens:   defaultToolSchemaReserve,
			ReservedOutputTokens:       defaultResponseHeadroom,
		},
	}
	remaining := budgetTotal
	brainBudgetRemaining := brainTokenBudget(m.brainCfg)

	threshold := cfg.RelevanceThreshold
	if threshold == 0 {
		threshold = defaultRelevanceThreshold
	}

	eligibleRAG, belowThreshold := filterEligibleRAG(results.RAGHits, threshold)
	for _, hit := range belowThreshold {
		markExcluded(budget, ragChunkKey(hit), "below_threshold")
	}
	sortRAGHits(eligibleRAG)
	topRAG, lowerRAG := splitPriorityRAG(eligibleRAG)

	for i := range results.FileResults {
		consumeFileResult(&remaining, budget, &results.FileResults[i])
	}
	for i := range results.BrainHits {
		consumeBrainHit(&remaining, &brainBudgetRemaining, budget, &results.BrainHits[i])
	}
	for i := range topRAG {
		consumeRAGHit(&remaining, budget, &topRAG[i])
	}
	for i := range results.GraphHits {
		consumeGraphHit(&remaining, budget, &results.GraphHits[i])
	}
	consumeConventions(&remaining, budget, results.ConventionText, cfg)
	consumeGitContext(&remaining, budget, results.GitContext, cfg)
	for i := range lowerRAG {
		consumeRAGHit(&remaining, budget, &lowerRAG[i])
	}

	budget.BudgetUsed = budget.BudgetTotal - remaining
	budget.TokenBudget.EstimatedContextTokens = budget.BudgetUsed
	budget.TokenBudget.EstimatedRequestTokens = budget.TokenBudget.ReservedSystemPromptTokens + budget.TokenBudget.ReservedToolSchemaTokens + budget.TokenBudget.ReservedOutputTokens + budget.TokenBudget.HistoryTokens + budget.TokenBudget.EstimatedContextTokens
	return budget, nil
}

func computeAssembledBudget(modelContextLimit int, historyTokenCount int, cfg config.ContextConfig) int {
	available := modelContextLimit - defaultSystemPromptReserve - defaultToolSchemaReserve - defaultResponseHeadroom - historyTokenCount
	if available < 0 {
		available = 0
	}
	maxAssembled := cfg.MaxAssembledTokens
	if maxAssembled <= 0 {
		maxAssembled = defaultMaxAssembledTokens
	}
	if available < maxAssembled {
		return available
	}
	return maxAssembled
}

func shouldCompressHistory(modelContextLimit int, historyTokenCount int, cfg config.ContextConfig) bool {
	if modelContextLimit <= 0 {
		return false
	}
	threshold := cfg.CompressionThreshold
	if threshold == 0 {
		threshold = 0.5
	}
	return float64(historyTokenCount) > float64(modelContextLimit)*threshold
}

func filterEligibleRAG(hits []RAGHit, threshold float64) ([]RAGHit, []RAGHit) {
	eligible := make([]RAGHit, 0, len(hits))
	excluded := make([]RAGHit, 0)
	for _, hit := range hits {
		if hit.SimilarityScore < threshold {
			hit.ExclusionReason = "below_threshold"
			excluded = append(excluded, hit)
			continue
		}
		eligible = append(eligible, hit)
	}
	return eligible, excluded
}

func sortRAGHits(hits []RAGHit) {
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].HitCount != hits[j].HitCount {
			return hits[i].HitCount > hits[j].HitCount
		}
		if hits[i].SimilarityScore != hits[j].SimilarityScore {
			return hits[i].SimilarityScore > hits[j].SimilarityScore
		}
		if hits[i].FromHop != hits[j].FromHop {
			return !hits[i].FromHop && hits[j].FromHop
		}
		if hits[i].FilePath != hits[j].FilePath {
			return hits[i].FilePath < hits[j].FilePath
		}
		return hits[i].ChunkID < hits[j].ChunkID
	})
}

func splitPriorityRAG(hits []RAGHit) ([]RAGHit, []RAGHit) {
	if len(hits) <= defaultTopRAGPriorityCount {
		return hits, nil
	}
	return hits[:defaultTopRAGPriorityCount], hits[defaultTopRAGPriorityCount:]
}

func consumeFileResult(remaining *int, budget *BudgetResult, file *FileResult) {
	tokens := estimateFileResultTokensForBudget(*file)
	if !fits(*remaining, tokens) {
		markExcluded(budget, file.FilePath, "budget_exceeded")
		return
	}
	file.Included = true
	file.ExclusionReason = ""
	budget.SelectedFileResults = append(budget.SelectedFileResults, *file)
	markIncluded(budget, file.FilePath)
	budget.BudgetBreakdown["explicit_files"] += tokens
	*remaining -= tokens
}

func consumeBrainHit(remaining *int, brainRemaining *int, budget *BudgetResult, hit *BrainHit) {
	tokens := estimateBrainHitTokensForBudget(*hit)
	key := hit.DocumentPath
	if brainRemaining != nil && *brainRemaining >= 0 && !fits(*brainRemaining, tokens) {
		markExcluded(budget, key, "budget_exceeded")
		return
	}
	if !fits(*remaining, tokens) {
		markExcluded(budget, key, "budget_exceeded")
		return
	}
	hit.Included = true
	hit.ExclusionReason = ""
	budget.SelectedBrainHits = append(budget.SelectedBrainHits, *hit)
	markIncluded(budget, key)
	budget.BudgetBreakdown["brain"] += tokens
	*remaining -= tokens
	if brainRemaining != nil && *brainRemaining >= 0 {
		*brainRemaining -= tokens
	}
}

func brainTokenBudget(cfg config.BrainConfig) int {
	if cfg.MaxBrainTokens <= 0 {
		return -1
	}
	return cfg.MaxBrainTokens
}

func consumeRAGHit(remaining *int, budget *BudgetResult, hit *RAGHit) {
	tokens := estimateRAGHitTokensForBudget(*hit)
	key := ragChunkKey(*hit)
	if !fits(*remaining, tokens) {
		markExcluded(budget, key, "budget_exceeded")
		return
	}
	hit.Included = true
	hit.ExclusionReason = ""
	if len(hit.Sources) == 0 {
		hit.Sources = []string{"rag"}
	}
	budget.SelectedRAGHits = append(budget.SelectedRAGHits, *hit)
	markIncluded(budget, key)
	budget.BudgetBreakdown["rag"] += tokens
	*remaining -= tokens
}

func consumeGraphHit(remaining *int, budget *BudgetResult, hit *GraphHit) {
	tokens := estimateGraphHitTokensForBudget(*hit)
	key := graphChunkKey(*hit)
	if !fits(*remaining, tokens) {
		markExcluded(budget, key, "budget_exceeded")
		return
	}
	hit.Included = true
	hit.ExclusionReason = ""
	budget.SelectedGraphHits = append(budget.SelectedGraphHits, *hit)
	markIncluded(budget, key)
	budget.BudgetBreakdown["structural"] += tokens
	*remaining -= tokens
}

func consumeConventions(remaining *int, budget *BudgetResult, text string, cfg config.ContextConfig) {
	capTokens := cfg.ConventionBudgetTokens
	if capTokens <= 0 {
		capTokens = 3000
	}
	selected, tokens := trimTextToBudget(text, min(*remaining, capTokens))
	if selected == "" {
		if strings.TrimSpace(text) != "" {
			markExcluded(budget, "conventions", "budget_exceeded")
		}
		return
	}
	budget.ConventionText = selected
	markIncluded(budget, "conventions")
	budget.BudgetBreakdown["conventions"] += tokens
	*remaining -= tokens
}

func consumeGitContext(remaining *int, budget *BudgetResult, text string, cfg config.ContextConfig) {
	capTokens := cfg.GitContextBudgetTokens
	if capTokens <= 0 {
		capTokens = 2000
	}
	selected, tokens := trimTextToBudget(text, min(*remaining, capTokens))
	if selected == "" {
		if strings.TrimSpace(text) != "" {
			markExcluded(budget, "git", "budget_exceeded")
		}
		return
	}
	budget.GitContext = selected
	markIncluded(budget, "git")
	budget.BudgetBreakdown["git"] += tokens
	*remaining -= tokens
}

func trimTextToBudget(text string, tokenBudget int) (string, int) {
	text = strings.TrimSpace(text)
	if text == "" || tokenBudget <= 0 {
		return "", 0
	}
	if count := approximateTokenCount(text); count <= tokenBudget {
		return text, count
	}
	lines := strings.Split(text, "\n")
	selected := make([]string, 0, len(lines))
	used := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lineTokens := approximateTokenCount(line)
		if used+lineTokens > tokenBudget {
			break
		}
		selected = append(selected, line)
		used += lineTokens
	}
	if len(selected) == 0 {
		return "", 0
	}
	return strings.Join(selected, "\n"), used
}

func estimateFileResultTokensForBudget(file FileResult) int {
	return approximateTokenCount(file.FilePath + "\n" + file.Content)
}

func estimateRAGHitTokensForBudget(hit RAGHit) int {
	return approximateTokenCount(hit.FilePath + "\n" + hit.Description + "\n" + hit.Body)
}

func estimateBrainHitTokensForBudget(hit BrainHit) int {
	return approximateTokenCount(hit.DocumentPath + "\n" + hit.Title + "\n" + hit.Snippet)
}

func estimateGraphHitTokensForBudget(hit GraphHit) int {
	return approximateTokenCount(hit.FilePath + "\n" + hit.SymbolName + "\n" + hit.RelationshipType)
}

// approximateTokenCount estimates the token count for a text string using the common
// heuristic of ~4 characters per token. The +3 provides ceiling-division rounding.
// This is intentionally rough — exact tokenization depends on the model's tokenizer
// and is not needed for budget allocation decisions.
func approximateTokenCount(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	return (len(text) + 3) / 4
}

func ragChunkKey(hit RAGHit) string {
	if hit.ChunkID != "" {
		return hit.ChunkID
	}
	return hit.FilePath + "#" + hit.Name
}

func graphChunkKey(hit GraphHit) string {
	if hit.ChunkID != "" {
		return hit.ChunkID
	}
	return hit.FilePath + "#" + hit.SymbolName
}

func fits(remaining int, tokens int) bool {
	return tokens > 0 && remaining >= tokens
}

func markIncluded(budget *BudgetResult, key string) {
	for _, existing := range budget.IncludedChunks {
		if existing == key {
			return
		}
	}
	budget.IncludedChunks = append(budget.IncludedChunks, key)
}

func markExcluded(budget *BudgetResult, key string, reason string) {
	for _, existing := range budget.ExcludedChunks {
		if existing == key {
			budget.ExclusionReasons[key] = reason
			return
		}
	}
	budget.ExcludedChunks = append(budget.ExcludedChunks, key)
	budget.ExclusionReasons[key] = reason
}
