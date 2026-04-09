package context

import (
	"strings"
	"testing"

	"github.com/ponchione/sirtopham/internal/config"
)

func TestPriorityBudgetManagerComputesBudgetTotal(t *testing.T) {
	manager := PriorityBudgetManager{}

	result, err := manager.Fit(&RetrievalResults{}, 200000, 50000, config.ContextConfig{
		MaxAssembledTokens:   30000,
		CompressionThreshold: 0.5,
	})
	if err != nil {
		t.Fatalf("Fit returned error: %v", err)
	}
	if result.BudgetTotal != 30000 {
		t.Fatalf("BudgetTotal = %d, want 30000", result.BudgetTotal)
	}
	if result.BudgetUsed != 0 {
		t.Fatalf("BudgetUsed = %d, want 0", result.BudgetUsed)
	}
	if result.CompressionNeeded {
		t.Fatal("CompressionNeeded = true, want false")
	}
}

func TestPriorityBudgetManagerFillsInPriorityOrderAndTracksBreakdown(t *testing.T) {
	manager := PriorityBudgetManager{}
	file := FileResult{FilePath: "internal/auth/middleware.go", Content: strings.Repeat("f", 40)}
	brain := BrainHit{DocumentPath: "notes/auth.md", Title: "Auth decisions", Snippet: strings.Repeat("b", 40), MatchScore: 0.91, MatchMode: "keyword"}
	ragTop := RAGHit{ChunkID: "rag-top", FilePath: "internal/auth/service.go", Name: "ValidateToken", Description: "Validates auth tokens.", Body: strings.Repeat("r", 32), SimilarityScore: 0.92, HitCount: 3}
	ragLow := RAGHit{ChunkID: "rag-low", FilePath: "internal/auth/helper.go", Name: "Helper", Description: "Helper code.", Body: strings.Repeat("l", 32), SimilarityScore: 0.88, HitCount: 1}
	graph := GraphHit{ChunkID: "graph-1", FilePath: "internal/auth/handler.go", SymbolName: "AuthHandler", RelationshipType: "upstream"}
	conventions := "Tests use table-driven style."
	git := "abc123 fix auth"

	budgetLimit := estimateFileResultTokens(file) + estimateBrainHitTokens(brain) + estimateRAGHitTokens(ragTop)
	result, err := manager.Fit(&RetrievalResults{
		FileResults:    []FileResult{file},
		BrainHits:      []BrainHit{brain},
		RAGHits:        []RAGHit{ragTop, ragLow},
		GraphHits:      []GraphHit{graph},
		ConventionText: conventions,
		GitContext:     git,
	}, 200000, 0, config.ContextConfig{
		MaxAssembledTokens:     budgetLimit,
		ConventionBudgetTokens: 3000,
		GitContextBudgetTokens: 2000,
		RelevanceThreshold:     0.35,
	})
	if err != nil {
		t.Fatalf("Fit returned error: %v", err)
	}

	if len(result.SelectedFileResults) != 1 {
		t.Fatalf("SelectedFileResults = %v, want 1", result.SelectedFileResults)
	}
	if len(result.SelectedBrainHits) != 1 || result.SelectedBrainHits[0].DocumentPath != "notes/auth.md" {
		t.Fatalf("SelectedBrainHits = %v, want notes/auth.md", result.SelectedBrainHits)
	}
	if len(result.SelectedRAGHits) != 1 || result.SelectedRAGHits[0].ChunkID != "rag-top" {
		t.Fatalf("SelectedRAGHits = %v, want rag-top only", result.SelectedRAGHits)
	}
	if len(result.SelectedGraphHits) != 0 {
		t.Fatalf("SelectedGraphHits = %v, want none", result.SelectedGraphHits)
	}
	if result.ConventionText != "" {
		t.Fatalf("ConventionText = %q, want empty", result.ConventionText)
	}
	if result.GitContext != "" {
		t.Fatalf("GitContext = %q, want empty", result.GitContext)
	}
	if result.ExclusionReasons["graph-1"] != "budget_exceeded" {
		t.Fatalf("graph exclusion = %q, want budget_exceeded", result.ExclusionReasons["graph-1"])
	}
	if result.ExclusionReasons["conventions"] != "budget_exceeded" {
		t.Fatalf("conventions exclusion = %q, want budget_exceeded", result.ExclusionReasons["conventions"])
	}
	if result.ExclusionReasons["git"] != "budget_exceeded" {
		t.Fatalf("git exclusion = %q, want budget_exceeded", result.ExclusionReasons["git"])
	}
	if result.ExclusionReasons["rag-low"] != "budget_exceeded" {
		t.Fatalf("rag-low exclusion = %q, want budget_exceeded", result.ExclusionReasons["rag-low"])
	}
	if result.BudgetBreakdown["explicit_files"] != estimateFileResultTokens(file) {
		t.Fatalf("explicit_files breakdown = %d, want %d", result.BudgetBreakdown["explicit_files"], estimateFileResultTokens(file))
	}
	if result.BudgetBreakdown["brain"] != estimateBrainHitTokens(brain) {
		t.Fatalf("brain breakdown = %d, want %d", result.BudgetBreakdown["brain"], estimateBrainHitTokens(brain))
	}
	if result.BudgetBreakdown["rag"] != estimateRAGHitTokens(ragTop) {
		t.Fatalf("rag breakdown = %d, want %d", result.BudgetBreakdown["rag"], estimateRAGHitTokens(ragTop))
	}
	if result.BudgetUsed != budgetLimit {
		t.Fatalf("BudgetUsed = %d, want %d", result.BudgetUsed, budgetLimit)
	}
}

func TestPriorityBudgetManagerMarksBelowThresholdAndCompressionNeeded(t *testing.T) {
	manager := PriorityBudgetManager{}
	belowThreshold := RAGHit{ChunkID: "rag-low-score", FilePath: "internal/auth/noise.go", Name: "Noise", Description: "Noise", Body: strings.Repeat("n", 20), SimilarityScore: 0.2}

	result, err := manager.Fit(&RetrievalResults{
		RAGHits: []RAGHit{belowThreshold},
	}, 100000, 60000, config.ContextConfig{
		MaxAssembledTokens:   30000,
		RelevanceThreshold:   0.35,
		CompressionThreshold: 0.5,
	})
	if err != nil {
		t.Fatalf("Fit returned error: %v", err)
	}
	if result.ExclusionReasons["rag-low-score"] != "below_threshold" {
		t.Fatalf("exclusion reason = %q, want below_threshold", result.ExclusionReasons["rag-low-score"])
	}
	if !result.CompressionNeeded {
		t.Fatal("CompressionNeeded = false, want true")
	}
}

func TestPriorityBudgetManagerHonorsMaxBrainTokens(t *testing.T) {
	manager := PriorityBudgetManager{}
	manager.SetBrainConfig(config.BrainConfig{MaxBrainTokens: 20})
	first := BrainHit{DocumentPath: "notes/first.md", Title: "First", Snippet: strings.Repeat("a", 40), MatchScore: 0.95, MatchMode: "keyword"}
	second := BrainHit{DocumentPath: "notes/second.md", Title: "Second", Snippet: strings.Repeat("b", 40), MatchScore: 0.90, MatchMode: "keyword"}

	result, err := manager.Fit(&RetrievalResults{BrainHits: []BrainHit{first, second}}, 200000, 0, config.ContextConfig{
		MaxAssembledTokens: 30000,
	})
	if err != nil {
		t.Fatalf("Fit returned error: %v", err)
	}
	if len(result.SelectedBrainHits) != 1 || result.SelectedBrainHits[0].DocumentPath != "notes/first.md" {
		t.Fatalf("SelectedBrainHits = %v, want only first hit within brain budget", result.SelectedBrainHits)
	}
	if result.ExclusionReasons["notes/second.md"] != "budget_exceeded" {
		t.Fatalf("notes/second.md exclusion = %q, want budget_exceeded", result.ExclusionReasons["notes/second.md"])
	}
	if result.BudgetBreakdown["brain"] != estimateBrainHitTokens(first) {
		t.Fatalf("brain breakdown = %d, want %d", result.BudgetBreakdown["brain"], estimateBrainHitTokens(first))
	}
}

func estimateFileResultTokens(file FileResult) int {
	return approxTokens(file.FilePath + "\n" + file.Content)
}

func estimateRAGHitTokens(hit RAGHit) int {
	return approxTokens(hit.FilePath + "\n" + hit.Description + "\n" + hit.Body)
}

func estimateBrainHitTokens(hit BrainHit) int {
	return approxTokens(hit.DocumentPath + "\n" + hit.Title + "\n" + hit.Snippet)
}

func approxTokens(text string) int {
	if text == "" {
		return 0
	}
	return (len(text) + 3) / 4
}
